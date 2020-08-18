package auth

import (
	"chat/chatroom"
	"chat/database"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/antonlindstrom/pgstore"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/gocql/gocql"
	"github.com/jackc/pgx"
	"github.com/jackc/pgx/stdlib"
	"github.com/joho/godotenv"
	"github.com/scylladb/gocqlx/v2"
	"github.com/sony/sonyflake"
)

var snowflake *sonyflake.Sonyflake
var scyllaSession gocqlx.Session

func init() {
	err := godotenv.Load("../.env")
	if err != nil {
		log.Println("Error loading .env file: ", err)
	}

	Tmpl, err = template.New("templates").ParseGlob("../templates/*.html")
	if err != nil {
		log.Fatalln("Error instantiating templates: ", err)
	}

	dbPort, err := strconv.ParseUint(os.Getenv("PGTEST_PORT"), 10, 16)
	if err != nil {
		log.Fatalln("Failed to convert db port from environment variable to int: ", err)
	}
	database.PgDB = stdlib.OpenDB(pgx.ConnConfig{
		Host:     os.Getenv("PGTEST_HOST"),
		Port:     uint16(dbPort),
		Database: os.Getenv("PGTEST_DB"),
		User:     os.Getenv("PGTEST_USER"),
		Password: os.Getenv("PGTEST_PASSWORD"),
	})

	database.PgStore, err = pgstore.NewPGStoreFromPool(database.PgDB, []byte(os.Getenv("SESSION_SECRET")))
	if err != nil {
		log.Fatalln("Error creating session store using postgres: ", err)
	}

	_, err = database.PgDB.Exec(
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

	_, err = database.PgDB.Exec(
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

	// creating temporary cassandra cluster in order to create keyspace
	tempCluster := gocql.NewCluster("localhost")
	keyspace := os.Getenv("KEYSPACE")
	cqlSession, err := tempCluster.CreateSession()
	if err != nil {
		log.Fatalln("Failed to create cluster session: ", err)
	}

	createKeyspace := cqlSession.Query(
		fmt.Sprintf(
			`CREATE KEYSPACE IF NOT EXISTS %s
				WITH replication = {
					'class' : 'SimpleStrategy',
					'replication_factor' : 3
				}`,
			os.Getenv("KEYSPACE"),
		), nil)
	err = createKeyspace.Exec()
	if err != nil {
		log.Fatalln("Failed to create keyspace: ", err)
	}

	cluster := gocql.NewCluster("localhost")
	cluster.Keyspace = keyspace
	scyllaSession, err = gocqlx.WrapSession(cluster.CreateSession())
	if err != nil {
		log.Fatalln("Failed to wrap new cluster session: ", err)
	}

	err = scyllaSession.ExecStmt(
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
	snowflake = sonyflake.NewSonyflake(
		sonyflake.Settings{
			StartTime: time.Unix(0, 0),
		},
	)
}

func newRouter() *chi.Mux {
	router := chi.NewRouter()
	router.Use(middleware.Logger)
	router.Post("/signup", Signup)
	router.Post("/login", Login)
	router.With(UserSession).Post("/logout", Logout)
	router.With(UserSession).Post("/logout", Logout)
	// router.With(UserSession).Get("/chat", chat)
	router.Handle("/", http.FileServer(http.Dir("./frontend")))
	router.With(UserSession).Get("/ws", chatroom.OpenWsConnection)
	router.With(UserSession).Post("/create-room", chatroom.Create)
	router.With(UserSession).Post("/join-room", chatroom.Join)
	router.With(UserSession).Post("/create-invite", chatroom.CreateInvite)
	return router
}

func TestSignup(t *testing.T) {
	func(t *testing.T) {
		t.Cleanup(func() {
			_, err := database.PgDB.Exec(
				`DELETE FROM users;`,
			)
			if err != nil {
				log.Println("error deleting all users: ", err)
			}
		})
	}(t)

	form := url.Values{}
	form.Set("email", "test@gmail.com")
	form.Set("username", "art")
	form.Set("password", "secretpassy")
	form.Set("confirmPassword", "secretpassy")

	encodedForm := strings.NewReader(form.Encode())
	req, err := http.NewRequest(http.MethodPost, "/signup", encodedForm)
	if err != nil {
		t.Fatal("error creating new request: ", err)
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	res := httptest.NewRecorder()
	router := newRouter()
	router.ServeHTTP(res, req)

	status := res.Code
	if status != http.StatusCreated {
		t.Errorf("signup endpoint returned wrong status code: got %v want %v\n", status, http.StatusOK)
	}
}
