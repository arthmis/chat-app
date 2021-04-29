package app

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx"
	"github.com/jackc/pgx/stdlib"
	"github.com/joho/godotenv"
	"nhooyr.io/websocket"
)

var application *App
var conn *sql.DB

func TestMain(m *testing.M) {
	InitLogger()
	err := godotenv.Load("../.env")
	if err != nil {
		Sugar.Fatalw("Error loading .env file: ", err)
	}

	pgHost, ok := os.LookupEnv("PGTEST_HOST")
	if !ok {
		Sugar.Fatal("Could not find POSTGRES_HOST env")
	}
	pgDb, ok := os.LookupEnv("PGTEST_DB")
	if !ok {
		Sugar.Fatal("Could not find POSTGRES_DB env")
	}
	pgUser, ok := os.LookupEnv("PGTEST_USER")
	if !ok {
		Sugar.Fatal("Could not find POSTGRES_USER env")
	}
	pgPassword, ok := os.LookupEnv("PGTEST_PASSWORD")
	if !ok {
		Sugar.Fatal("Could not find POSTGRES_PASSWORD env")
	}
	pgPortStr, ok := os.LookupEnv("PGTEST_PORT")
	if !ok {
		Sugar.Fatal("Could not find POSTGRES_PORT env")
	}
	pgPort, err := strconv.ParseInt(pgPortStr, 10, 16)
	if err != nil {
		Sugar.Fatalf("Could not convert POSTGRES_PORT to a number. %v", pgPortStr)
	}
	pgConfig := PgConfig{
		Host:     pgHost,
		Db:       pgDb,
		User:     pgUser,
		Password: pgPassword,
		Port:     uint16(pgPort),
	}

	scyllaHost, ok := os.LookupEnv("SCYLLA_HOST")
	if !ok {
		Sugar.Fatal("Could not find SCYLLA_HOST env")
	}
	scyllaKeyspace, ok := os.LookupEnv("KEYSPACE")
	if !ok {
		Sugar.Fatal("Could not find KEYSPACE env")
	}
	scyConfig := ScyllaConfig{
		Host:     scyllaHost,
		Keyspace: scyllaKeyspace,
	}

	// connection to another "root" database lets me delete testdb
	conn = stdlib.OpenDB(pgx.ConnConfig{
		Host:     pgHost,
		Port:     uint16(pgPort),
		Database: "root",
		User:     pgUser,
		Password: pgPassword,
	})

	// _, err = conn.Exec("CREATE DATABASE root")
	// if err != nil {
	// 	Sugar.Warnf("err creating root database: %v", err)
	// }

	// create test database
	_, err = conn.Exec("CREATE DATABASE testdb")
	if err != nil {
		Sugar.Warnf("err creating testdb: %v", err)
	}

	application = NewApp(pgConfig, scyConfig, "../templates/*.html")

	code := m.Run()

	err = application.Pg.Close()
	if err != nil {
		Sugar.Errorf("Error closing application connection to db: %v", err)
	}

	// dropping database to start from clean slate next time
	// maybe dropping the tables is enough, keeping this here
	// for now
	// _, err = conn.Exec("DROP DATABASE IF EXISTS testdb")
	// if err != nil {
	// 	Sugar.Fatalf("err deleting testdb: %v", err)
	// }

	os.Exit(code)
}

func authenticatedSetup() (*httptest.Server, *http.Client, *websocket.Conn, error) {
	// figure out how to set the server listen address to port 8000
	server := httptest.NewUnstartedServer(application.Routes())
	server.Config.Addr = "http://localhost:8000"
	server.Start()

	var err error

	client := server.Client()
	client.Jar, err = cookiejar.New(nil)

	// signup user
	form := url.Values{}
	form.Set("username", "artemis")
	form.Set("email", "kup@gmail.com")
	form.Set("password", "secretpassy")
	form.Set("confirmPassword", "secretpassy")
	_, err = client.PostForm(server.URL+"/api/user/signup", form)
	if err != nil {
		return nil, nil, nil, err
	}

	// login user
	form = url.Values{}
	form.Set("email", "kup@gmail.com")
	form.Set("password", "secretpassy")
	_, err = client.PostForm(server.URL+"/api/user/login", form)
	if err != nil {
		Sugar.Error("failed to login")
		return nil, nil, nil, err
	}

	// make websocket connection
	options := websocket.DialOptions{
		HTTPClient: client,
	}
	var conn *websocket.Conn
	conn, _, err = websocket.Dial(context.Background(), server.URL+"/api/ws", &options)
	if err != nil {
		Sugar.Error("failed to get websocket connection")
		return nil, nil, nil, err
	}

	return server, client, conn, nil
}

func authenticatedTakeDown() {
	_, err := application.Pg.Exec("DROP TABLE IF EXISTS http_sessions")
	if err != nil {
		Sugar.Errorf("error dropping table http_sessions: %v", err)
	}
	_, err = application.Pg.Exec("DROP TABLE IF EXISTS invites")
	if err != nil {
		Sugar.Errorf("error dropping table invites: %v", err)
	}
	_, err = application.Pg.Exec("DROP TABLE IF EXISTS rooms")
	if err != nil {
		Sugar.Errorf("error dropping table rooms: %v", err)
	}
	_, err = application.Pg.Exec("DROP TABLE IF EXISTS users")
	if err != nil {
		Sugar.Errorf("error dropping table users: %v", err)
	}

	err = application.ScyllaDb.ExecStmt("DROP TABLE messages")
	if err != nil {
		Sugar.Errorf("error dropping table messages: %v", err)
	}
	err = application.ScyllaDb.ExecStmt("DROP TABLE users")
	if err != nil {
		Sugar.Errorf("error dropping table users: %v", err)
	}
}

func TestCreateRoom(t *testing.T) {
	server, client, conn, err := authenticatedSetup()
	if err != nil {
		t.Errorf("Setting up server and database was a failure: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	defer server.Close()

	form := url.Values{}
	form.Set("chatroom_name", "test chatroom")
	res, err := client.PostForm(server.URL+"/api/room/create", form)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if res.StatusCode != http.StatusCreated {
		t.Fatalf("handled returned wrong status code: got %v want %v", res.StatusCode, http.StatusInternalServerError)

	}

	err = res.Body.Close()
	if err != nil {
		t.Errorf("error closing body: %v", err)
	}
	authenticatedTakeDown()
}

func TestCreateRoomUnauthenticated(t *testing.T) {
	form := url.Values{}
	form.Set("chatroom_name", "test chatroom")

	encodedForm := strings.NewReader(form.Encode())
	req, err := http.NewRequest(http.MethodPost, "api/room/create", encodedForm)
	if err != nil {
		t.Fatal("error creating new request: ", err)
	}
	// this seems to be necessary in order for the body to be read
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(application.Create)

	handler.ServeHTTP(rr, req)

	status := rr.Code
	if status != http.StatusInternalServerError {
		t.Errorf("handled returned wrong status code: got %v want %v", status, http.StatusInternalServerError)
	}
}
