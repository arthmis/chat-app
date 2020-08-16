package auth

import (
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

	dbPort, err := strconv.ParseUint(os.Getenv("POSTGRES_PORT"), 10, 16)
	if err != nil {
		log.Fatalln("Failed to convert db port from environment variable to int: ", err)
	}
	Db = stdlib.OpenDB(pgx.ConnConfig{
		Host:     os.Getenv("POSTGRES_HOST"),
		Port:     uint16(dbPort),
		Database: os.Getenv("POSTGRES_DB"),
		User:     os.Getenv("POSTGRES_USER"),
		Password: os.Getenv("POSTGRES_PASSWORD"),
	})

	Store, err = pgstore.NewPGStoreFromPool(Db, []byte(os.Getenv("SESSION_SECRET")))
	if err != nil {
		log.Fatalln("Error creating session store using postgres: ", err)
	}

	_, err = Db.Exec(
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

	_, err = Db.Exec(
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

	// creating scylla cluster
	// cluster := gocql.NewCluster("127.0.0.1:9042")
	cluster := gocql.NewCluster("localhost")
	cluster.Keyspace = os.Getenv("KEYSPACE")
	cqlSession, err := cluster.CreateSession()
	if err != nil {
		log.Fatalln("Failed to create cluster session: ", err)
	}

	scyllaSession, err = gocqlx.WrapSession(cqlSession, err)
	if err != nil {
		log.Fatalln("Failed to wrap new cluster session: ", err)
	}

	createKeyspace := scyllaSession.Query(
		fmt.Sprintf(
			`CREATE KEYSPACE %s
				WITH replication = {
					'class' : 'SimpleStrategy',
					'replication_factor' : 3
				}`,
			cluster.Keyspace,
		), nil)
	err = createKeyspace.Exec()
	if err != nil {
		log.Fatalln("Failed to create keyspace: ", err)
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
	// router.With(UserSession).Get("/chat", chat)
	return router
}
func TestSignup(t *testing.T) {
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
		t.Errorf("handle returned wrong status code: got %v want %v\n", status, http.StatusOK)
	}
}
