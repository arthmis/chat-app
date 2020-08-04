package main

import (
	"html/template"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx"
	"github.com/jackc/pgx/stdlib"
	"github.com/sony/sonyflake"

	"github.com/antonlindstrom/pgstore"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"

	"github.com/gocql/gocql"
	"github.com/joho/godotenv"
	"github.com/scylladb/gocqlx/v2"

	"chat/auth"
	"chat/chatroom"
	// "chat/validate"
)

const addr = ":8000"

func init() {
	err := godotenv.Load()
	if err != nil {
		log.Fatalln("Error loading .env file: ", err)
	}

	auth.Tmpl, err = template.New("templates").ParseGlob("templates/*.html")
	if err != nil {
		log.Fatalln("Error instantiating templates: ", err)
	}

	dbPort, err := strconv.ParseUint(os.Getenv("DB_PORT"), 10, 16)
	if err != nil {
		log.Fatalln("Failed to convert db port from environment variable to int: ", err)
	}
	auth.Db = stdlib.OpenDB(pgx.ConnConfig{
		Host:     os.Getenv("DB_HOST"),
		Port:     uint16(dbPort),
		Database: os.Getenv("DATABASE"),
		User:     os.Getenv("DB_USER"),
		Password: os.Getenv("DB_PASSWORD"),
	})

	auth.Store, err = pgstore.NewPGStoreFromPool(auth.Db, []byte(os.Getenv("SESSION_SECRET")))
	if err != nil {
		log.Fatalln("Error creating session store using postgres: ", err)
	}

	_, err = auth.Db.Exec(
		`CREATE TABLE IF NOT EXISTS Users (
			id serial PRIMARY KEY,
			email TEXT NOT NULL,
			username TEXT NOT NULL,
			password TEXT NOT NULL
		)`,
	)

	if err != nil {
		log.Fatalln("Problem creating Users table: ", err)
	}

	_, err = auth.Db.Exec(
		`CREATE TABLE IF NOT EXISTS Invites (
			id serial PRIMARY KEY,
			invite TEXT NOT NULL,
			chatroom TEXT NOT NULL,
			expires TIMESTAMPTZ NOT NULL
		)`,
	)

	if err != nil {
		log.Fatalln("Problem creating Invites table: ", err)
	}

	go chatroom.RemoveExpiredInvites(auth.Db, time.Minute*10)

	// creating scylla cluster
	cluster := gocql.NewCluster("127.0.0.1")
	cluster.Keyspace = os.Getenv("KEYSPACE")
	chatroom.ScyllaSession, err = gocqlx.WrapSession(cluster.CreateSession())
	if err != nil {
		log.Fatalln("Failed to wrap new cluster session: ", err)
	}

	err = chatroom.ScyllaSession.ExecStmt(
		`CREATE TABLE IF NOT EXISTS messages(
			chatroom_name TEXT,
			user_id TEXT,
			content TEXT,
			message_id bigint,
			PRIMARY KEY (chatroom_name, message_id)
		) WITH CLUSTERING ORDER BY (message_id DESC)`,
	)
	if err != nil {
		log.Fatalln("Create messages store error:", err)
	}

	// this will generate unique ids for each message on this
	// particular server instance
	chatroom.Snowflake = sonyflake.NewSonyflake(
		sonyflake.Settings{
			StartTime: time.Unix(0, 0),
		},
	)
}

func main() {
	defer auth.Db.Close()

	router := chi.NewRouter()
	router.Use(middleware.Logger)
	// add validation middleware
	router.Post("/signup", auth.Signup)
	// add validation middleware
	router.Post("/login", auth.Login)
	router.With(auth.UserSession).Post("/logout", auth.Logout)
	router.With(auth.UserSession).Get("/chat", chat)
	router.Handle("/", http.FileServer(http.Dir("./frontend")))
	router.With(auth.UserSession).Get("/ws", chatroom.OpenWsConnection)
	// add validation middleware
	router.With(auth.UserSession).Post("/create-room", chatroom.Create)
	// add validation middleware
	router.With(auth.UserSession).Post("/join-room", chatroom.Join)
	// add validation middleware
	router.With(auth.UserSession).Post("/create-invite", chatroom.CreateInvite)

	FileServer(router, "/", http.Dir("./frontend"))
	err := http.ListenAndServe(addr, router)
	if err != nil {
		log.Fatalln("error starting server: ", err)
	}
}

func FileServer(r chi.Router, path string, root http.FileSystem) {
	if strings.ContainsAny(path, "{}*") {
		panic("FileServer does not permit any URL parameters.")
	}

	if path != "/" && path[len(path)-1] != '/' {
		r.Get(path, http.RedirectHandler(path+"/", 301).ServeHTTP)
		path += "/"
	}
	path += "*"

	r.Get(path, func(w http.ResponseWriter, r *http.Request) {
		rctx := chi.RouteContext(r.Context())
		pathPrefix := strings.TrimSuffix(rctx.RoutePattern(), "/*")
		fs := http.StripPrefix(pathPrefix, http.FileServer(root))
		fs.ServeHTTP(w, r)
	})
}

func chat(w http.ResponseWriter, req *http.Request) {
	http.ServeFile(w, req, "./frontend/chat")
}
