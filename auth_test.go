package app

import (
	"chat/applog"
	"chat/database"
	"chat/validate"
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
	"github.com/davecgh/go-spew/spew"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/gocql/gocql"
	"github.com/jackc/pgx"
	"github.com/jackc/pgx/stdlib"
	"github.com/joho/godotenv"
	"github.com/scylladb/gocqlx/v2"
	"github.com/sony/sonyflake"
	"golang.org/x/crypto/bcrypt"
)

var snowflake *sonyflake.Sonyflake
var scyllaSession gocqlx.Session

func init() {
	err := godotenv.Load("../.env")
	if err != nil {
		applog.Sugar.Error("Error loading .env file: ", err)
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
	// router.With(UserSession).Get("/chat", chat)
	// router.Handle("/", http.FileServer(http.Dir("./frontend")))
	router.With(UserSession).Get("/ws", chatroom.OpenWsConnection)
	router.With(UserSession).Post("/create-room", chatroom.Create)
	router.With(UserSession).Post("/join-room", chatroom.Join)
	router.With(UserSession).Post("/create-invite", chatroom.CreateInvite)
	return router
}

func addUser() {
	type tempUser struct {
		email    string `form:"email" validate:"required,email,max=50"`
		username string `form:"username" validate:"required,min=3,max=30"`
		password string `form:"password" validate:"required,eqfield=ConfirmPassword,min=8,max=50"`
	}

	user := tempUser{email: "test@gmail.com", username: "art", password: "secretpassy"}
	err := validate.Validate.Struct(user)
	if err != nil {
		applog.Sugar.Error("err validating user data: ", err)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(user.password), bcrypt.MinCost)
	if err != nil {
		applog.Sugar.Error("err generating from password: ", err)
		return
	}
	_, err = database.PgDB.Exec(
		`INSERT INTO Users (email, username, password) VALUES ($1, $2, $3)`,
		user.email,
		user.username,
		string(hash),
	)
	if err != nil {
		applog.Sugar.Error("error inserting new user: ", err)
	}
}

func TestSignup(t *testing.T) {
	func(t *testing.T) {
		t.Cleanup(func() {
			_, err := database.PgDB.Exec(
				`DELETE FROM users;`,
			)
			if err != nil {
				applog.Sugar.Error("error deleting all users: ", err)
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

func TestLogin(t *testing.T) {
	func(t *testing.T) {
		addUser()
		t.Cleanup(func() {
			_, err := database.PgDB.Exec(
				`DELETE FROM users;`,
			)
			if err != nil {
				applog.Sugar.Error("error deleting all users: ", err)
			}
		})
	}(t)

	form := url.Values{}
	form.Set("email", "test@gmail.com")
	form.Set("password", "secretpassy")

	encodedForm := strings.NewReader(form.Encode())
	req, err := http.NewRequest(http.MethodPost, "/login", encodedForm)
	if err != nil {
		t.Fatal("error creating new request: ", err)
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	res := httptest.NewRecorder()
	router := newRouter()
	router.ServeHTTP(res, req)

	status := res.Code
	if status != http.StatusSeeOther {
		t.Errorf("login endpoint returned wrong status code: got %v want %v\n", status, http.StatusSeeOther)
	}
}

func TestLogout(t *testing.T) {
	func(t *testing.T) {
		t.Cleanup(func() {
			_, err := database.PgDB.Exec(
				`DELETE FROM users;`,
			)
			if err != nil {
				applog.Sugar.Error("error deleting all users: ", err)
			}
		})
	}(t)
	// server := httptest.NewServer(newRouter())
	server := httptest.NewUnstartedServer(newRouter())
	server.Config = &http.Server{
		Addr: "http://localhost:8000",
	}
	server.Start()
	defer server.Close()

	client := server.Client()

	form := url.Values{}
	form.Set("email", "test@gmail.com")
	form.Set("username", "art")
	form.Set("password", "secretpassy")
	form.Set("confirmPassword", "secretpassy")
	res, err := client.PostForm("/signup", form)
	fmt.Println(err)
	fmt.Println(res)

	form = url.Values{}
	form.Set("email", "test@gmail.com")
	form.Set("password", "secretpassy")
	res, _ = client.PostForm("/login", form)
	applog.Sugar.Error(res)

	// req, _ := http.NewRequest(http.MethodPost, "/logout", strings.NewReader(""))
	// res, _ := client.Do(req)
	// res, _ := client.Post("/logout", "text/plain", strings.NewReader(""))
	form = url.Values{}
	res, _ = client.PostForm("/logout", form)
	// status := res.
	spew.Dump(res)
	// fmt.Println(status)
	// if status != http.StatusSeeOther {
	// 	t.Errorf("logout endpoint returned wrong status code: got %v want %v\n", status, http.StatusSeeOther)
	// }

	// encodedForm := strings.NewReader(form.Encode())
	// req, err := http.NewRequest(http.MethodPost, "/login", encodedForm)
	// if err != nil {
	// 	t.Fatal("error creating new request: ", err)
	// }
	// req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	// addUser()
	// session, err := database.PgStore.Get(req, "session-name")
	// if err != nil {
	// 	applog.Sugar.Error("error creating new unsaved session: ", err)
	// 	return
	// }
	// session.Options = &sessions.Options{
	// 	Path:     "/",
	// 	MaxAge:   60 * 60 * 24 * 7,
	// 	HttpOnly: true,
	// }
	// username := "art"
	// session.Values["username"] = username

	// res := httptest.NewRecorder()
	// router := newRouter()
	// router.ServeHTTP(res, req)

	// status := res.Code
	// if status != http.StatusSeeOther {
	// 	t.Errorf("login endpoint returned wrong status code: got %v want %v\n", status, http.StatusSeeOther)
	// }
}
