package app

import (
	"database/sql"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"time"

	"github.com/antonlindstrom/pgstore"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/go-playground/validator"
	"github.com/gocql/gocql"
	"github.com/jackc/pgx"
	"github.com/jackc/pgx/stdlib"
	"github.com/scylladb/gocqlx/v2"
	"github.com/sony/sonyflake"
)

var Validate *validator.Validate = validator.New()

type App struct {
	Pg               *sql.DB
	PgStore          *pgstore.PGStore
	ScyllaDb         gocqlx.Session
	Snowflake        *sonyflake.Sonyflake
	Chatrooms        map[string]*Chatroom
	Clients          map[string]*User
	ChatroomChannels map[string]chan MessageWithCtx
	Tmpl             *template.Template
	Invitations      *Invitations
}

type PgConfig struct {
	Host     string
	Db       string
	User     string
	Password string
	Port     uint16
}

type ScyllaConfig struct {
	Host     string
	Keyspace string
}

func NewApp(pg PgConfig, scy ScyllaConfig, templates string) *App {
	app := new(App)

	var err error
	// app.Tmpl, err = template.New("templates").ParseGlob("templates/*.html")
	app.Tmpl, err = template.New("templates").ParseGlob(templates)
	if err != nil {
		Sugar.Fatalw("Error instantiating templates: ", err)
	}

	app.Pg = stdlib.OpenDB(pgx.ConnConfig{
		Host:     pg.Host,
		Port:     pg.Port,
		Database: pg.Db,
		User:     pg.User,
		Password: pg.Password,
	})

	app.Invitations = &Invitations{
		pg: app.Pg,
	}

	app.PgStore, err = pgstore.NewPGStoreFromPool(app.Pg, []byte(os.Getenv("SESSION_SECRET")))
	if err != nil {
		Sugar.Fatal("Error creating session store using postgres:", err)
	}

	_, err = app.Pg.Exec(
		`CREATE TABLE IF NOT EXISTS Users (
			id serial PRIMARY KEY,
			email TEXT NOT NULL,
			username TEXT NOT NULL,
			password TEXT NOT NULL
		)`,
	)

	if err != nil {
		Sugar.Fatalw("Problem creating Users table: ", err)
	}

	_, err = app.Pg.Exec(
		`CREATE TABLE IF NOT EXISTS Invites (
			id serial PRIMARY KEY,
			invite TEXT NOT NULL,
			chatroom TEXT NOT NULL,
			expires TIMESTAMPTZ NOT NULL
		)`,
	)

	if err != nil {
		Sugar.Fatalw("Problem creating Invites table: ", err)
	}

	_, err = app.Pg.Exec(
		`CREATE TABLE IF NOT EXISTS Rooms (
			id serial PRIMARY KEY,
			name TEXT NOT NULL
		)`,
	)

	if err != nil {
		Sugar.Fatalw("Problem creating Rooms table: ", err)
	}

	Sugar.Info("Postgres database has been initialized.")

	go RemoveExpiredInvites(app.Pg, time.Minute*10)

	// TODO: I think I will have to figure out whether I need to remove this
	// most likely I will. This should be unnecessary, the keyspace should be
	// created in a different way
	// creating temporary cassandra cluster in order to create keyspace
	tempCluster := gocql.NewCluster(scy.Host)
	tempCluster.ProtoVersion = 4
	cqlSession, err := tempCluster.CreateSession()
	if err != nil {
		Sugar.Fatalw("Failed to create cluster session: ", err)
	}

	createKeyspace := cqlSession.Query(
		fmt.Sprintf(
			`CREATE KEYSPACE IF NOT EXISTS %s
				WITH replication = {
					'class' : 'SimpleStrategy',
					'replication_factor' : 1
				}`,
			os.Getenv("KEYSPACE"),
		), nil)
	err = createKeyspace.Exec()
	if err != nil {
		Sugar.Fatalw("Failed to create keyspace: ", err)
	}

	// creating scylla cluster
	cluster := gocql.NewCluster(scy.Host)
	cluster.Keyspace = scy.Keyspace
	app.ScyllaDb, err = gocqlx.WrapSession(cluster.CreateSession())
	if err != nil {
		Sugar.Fatalw("Failed to wrap new cluster session: ", err)
	}

	err = app.ScyllaDb.ExecStmt(
		`CREATE TABLE IF NOT EXISTS messages(
			chatroom_name TEXT,
			user_id TEXT,
			content TEXT,
			message_id bigint,
			PRIMARY KEY (chatroom_name, message_id)
		) WITH CLUSTERING ORDER BY (message_id DESC)`,
	)
	if err != nil {
		Sugar.Fatalw("Create messages store error:", err)
	}
	err = app.ScyllaDb.ExecStmt(
		`CREATE TABLE IF NOT EXISTS users(
			user TEXT,
			current_chatroom TEXT STATIC,
			chatroom TEXT,
			PRIMARY KEY (user, chatroom)
		) WITH CLUSTERING ORDER BY (chatroom ASC)`,
	)
	if err != nil {
		Sugar.Fatalw("Create messages store error:", err)
	}
	Sugar.Infow("CassandraDB has been initialized.")

	// this will generate unique ids for each message on this
	// particular server instance
	app.Snowflake = sonyflake.NewSonyflake(
		sonyflake.Settings{
			StartTime: time.Unix(0, 0),
		},
	)

	rows, err := app.Pg.Query(
		`SELECT name FROM Rooms`,
	)
	if err != nil {
		Sugar.Fatalw("couldn't get room rows", err)
	}

	// initialize chatrooms
	var name string
	app.Chatrooms = make(map[string]*Chatroom)
	app.ChatroomChannels = make(map[string]chan MessageWithCtx)
	for rows.Next() {
		err = rows.Scan(&name)
		if err != nil {
			Sugar.Fatalw("couldn't scan row: ", err)
		}

		room := NewChatroom()
		room.Id = name
		room.ScyllaSession = &app.ScyllaDb
		room.Snowflake = app.Snowflake
		room.Clients = []*ChatroomClient{}
		room.Messages = make([]IncomingMessage, 20)
		room.Channel = make(chan MessageWithCtx)

		app.Chatrooms[room.Id] = room
		app.ChatroomChannels[room.Id] = room.Channel

		go room.Run()
	}

	app.Clients = make(map[string]*User)
	Sugar.Infow("Chatrooms initialized.")
	return app
}

func (app App) Routes() *chi.Mux {
	router := chi.NewRouter()
	router.Use(middleware.Logger)
	router.Route("/api", func(router chi.Router) {
		router.With(app.UserSession).Get("/chat", chat)
		router.With(app.UserSession).Get("/ws", app.OpenWsConnection)
		router.Route("/room", func(router chi.Router) {
			// add validation middleware for create
			router.With(app.UserSession).Post("/create", app.Create)
			// add validation middleware for join
			router.With(app.UserSession).Post("/join/*", app.Join)
			// add validation middleware for invite
			router.With(app.UserSession).Post("/invite", app.CreateInvite)
			// add validation middleware for messages
			router.With(app.UserSession).Post("/messages", app.GetRoomMessages)
			// router.With(auth.UserSession).Post("/delete", chatroom.GetCurrentRoomMessages)
		})
		router.Route("/user", func(router chi.Router) {
			router.With(app.UserSession).Post("/chatrooms", app.GetUserInfo)
			// add validation middleware for signup
			router.Post("/signup", app.Signup)
			// add validation middleware for login
			router.Post("/login", app.Login)
			router.With(app.UserSession).Post("/logout", app.Logout)
			// router.With(auth.UserSession).Get("/", user.GetUser)
		})
	})
	return router
}

func chat(w http.ResponseWriter, req *http.Request) {
	http.ServeFile(w, req, "./frontend/chat")
}
