package app

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/joho/godotenv"
)

var application *App

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

	application = NewApp(pgConfig, scyConfig, "../templates/*.html")
	code := m.Run()
	os.Exit(code)
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
