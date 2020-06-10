package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"html/template"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"

	"github.com/gorilla/websocket"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/jackc/pgx/v4"

	// "github.com/go-chi/render"
	"github.com/antonlindstrom/pgstore"
	"github.com/go-playground/validator/v10"
	"github.com/gorilla/schema"

	"github.com/gorilla/sessions"

	// "github.com/jackc/pgx/v4/pgxpool"
	"github.com/jackc/pgx/v4/stdlib"
	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"
)

const addr = ":8000"

var clients = make(map[string]*websocket.Conn, 0)
var decoder = schema.NewDecoder()

type User struct {
	conn      *websocket.Conn
	id        string
	chatrooms []string
}

const (
	id      = "id"
	message = "message"
)

type UserMessage struct {
	Message     string
	MessageType string
	Id          string
	ChatroomId  string
}
type Message struct {
	Message     string
	MessageType string
	Id          string
}

type Chatroom struct {
	id    string
	users []*websocket.Conn
}

type UserSignup struct {
	Email           string `form:"email" validate:"required,email,max=50"`
	Username        string `form:"username" validate:"required,min=3,max=30"`
	Password        string `form:"password" validate:"required,eqfield=ConfirmPassword,min=8,max=50"`
	ConfirmPassword string `form:"confirmPassword" validate:"required,min=8,max=50"`
}

type UserLogin struct {
	Email    string `form:"email" validate:"required,email,max=50"`
	Password string `form:"password" validate:"required,min=8,max=50"`
}

func NewChatroom() *Chatroom {
	room := new(Chatroom)
	room.id = string("test1")
	room.users = make([]*websocket.Conn, 0)
	return room
}

var chatrooms = make(map[string]*Chatroom, 0)

// var db *pgxpool.Pool
var db *sql.DB
var validate *validator.Validate
var tmpl *template.Template

var store *pgstore.PGStore

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file:\n", err)
	}

	tmpl, err = template.New("templates").ParseGlob("templates/*.html")
	if err != nil {
		log.Println(err)
		return
	}

	validate = validator.New()
	// dbpool, err = pgxpool.Connect(context.Background(), os.Getenv("DATABASE_URL"))
	db, err = sql.Open("pgx", os.Getenv("DATABASE_URL"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to connect to database: %v\n", err)
		os.Exit(1)
	}
	store, err = pgstore.NewPGStoreFromPool(db, []byte(os.Getenv("SESSION_SECRET")))
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}
	defer db.Close()

	_, err = db.Exec(
		`CREATE TABLE IF NOT EXISTS Users (
			id serial PRIMARY KEY,
			email TEXT NOT NULL,
			username TEXT NOT NULL,
			password TEXT NOT NULL
		)`,
	)

	if err != nil {
		fmt.Fprintf(os.Stderr, "Problem creating database table: %v\n", err)
	}

	router := chi.NewRouter()
	router.Use(middleware.Logger)
	router.Post("/signup", signup)
	router.Post("/login", login)
	router.With(validateUserSession).Get("/chat", chat)
	router.Handle("/", http.FileServer(http.Dir("./frontend")))

	FileServer(router, "/", http.Dir("./frontend"))
	http.ListenAndServe(addr, router)
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

func signup(w http.ResponseWriter, req *http.Request) {
	var form UserSignup
	req.ParseMultipartForm(50000)
	err := decoder.Decode(&form, req.PostForm)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	fmt.Printf("%+v\n", form)

	err = validate.Struct(form)
	if err != nil {
		log.Println(err)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(form.Password), bcrypt.MinCost)
	if err != nil {
		log.Println(err)
	}
	fmt.Println(string(hash))

	emailExists, usernameExists := checkUserExists(req.Context(), db, form.Email, form.Username)

	userExists := struct {
		username       string
		email          string
		usernameExists string
		emailExists    string
	}{
		form.Username,
		form.Email,
		"Username already exists",
		"Email already exists",
	}

	fmt.Println(emailExists, usernameExists)
	// TODO COMPLETE THIS AND ACTUALLY IMPLEMENT THE TEMPLATES
	if usernameExists && emailExists {
		w.WriteHeader(http.StatusOK)
		tmpl.ExecuteTemplate(w, "signup.html", userExists)

		return
	} else if usernameExists {
		userExists.email = ""
		w.WriteHeader(http.StatusOK)
		tmpl.ExecuteTemplate(w, "signup.html", userExists)

		return
	} else if emailExists {
		userExists.username = ""
		w.WriteHeader(http.StatusOK)
		tmpl.ExecuteTemplate(w, "signup.html", userExists)

		return
	}

	conn, err := stdlib.AcquireConn(db)
	if err == nil {
		conn.Exec(
			req.Context(),
			`INSERT INTO Users (email, username, password) VALUES ($1, $2, $3)`,
			form.Email,
			form.Username,
			string(hash),
		)
		stdlib.ReleaseConn(db, conn)
	} else {
		log.Println("error inserting values: \n", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
	tmpl.ExecuteTemplate(w, "login.html", nil)
}

func validateUserSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		session, err := store.Get(req, "session-name")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.Println(err)
			return
		}
		if session.IsNew {
			http.Redirect(w, req, "login", http.StatusSeeOther)
			return
		}
		fmt.Printf("%+v\n", session)
		next.ServeHTTP(w, req)
	})
}

func login(w http.ResponseWriter, req *http.Request) {
	var form UserLogin
	err := req.ParseMultipartForm(50000)
	if err != nil {
		log.Println(err)
	}
	err = decoder.Decode(&form, req.PostForm)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	row := db.QueryRow(
		`SELECT password FROM Users WHERE email=$1`,
		form.Email,
	)
	var password string
	err = row.Scan(&password)
	if err != nil {
		// email is not available
		w.WriteHeader(http.StatusOK)
		tmpl.ExecuteTemplate(w, "login.html", nil)
		return
	}

	err = bcrypt.CompareHashAndPassword([]byte(password), []byte(form.Password))
	// password did not match
	if err != nil {
		w.WriteHeader(http.StatusOK)
		tmpl.ExecuteTemplate(w, "login.html", nil)
		return
	}

	session, err := store.New(req, "session-name")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println(err)
		return
	}
	session.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   60 * 60 * 24 * 7,
		HttpOnly: true,
	}
	err = session.Save(req, w)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	http.Redirect(w, req, "/chat", http.StatusSeeOther)
}

func chat(w http.ResponseWriter, req *http.Request) {
	http.ServeFile(w, req, "./frontend/chat")
}

func checkUserExists(ctx context.Context, dbpool *sql.DB, email string, username string) (emailExists, usernameExists bool) {
	row := dbpool.QueryRow(
		`SELECT email FROM Users WHERE email=$1`,
		email,
	)

	err := row.Scan(&email)
	emailExists = true
	if err == pgx.ErrNoRows {
		emailExists = false
	}
	row = dbpool.QueryRow(
		`SELECT username FROM Users WHERE username=$1`,
		username,
	)

	err = row.Scan(&username)
	usernameExists = true
	if err == pgx.ErrNoRows {
		usernameExists = false
	}

	return emailExists, usernameExists
}

// TODO think about tracking users and the rooms they are a part of
// track rooms and users that are authorized to use it
func createRoom(writer http.ResponseWriter, req *http.Request) {
	// err := req.ParseForm()
	err := req.ParseMultipartForm(2 << 14)
	if err != nil {
		writer.WriteHeader(http.StatusNotFound)
		log.Println("error parsing form")
	}

	room := Chatroom{}
	keyBytes := make([]byte, 16)
	if _, err := rand.Read(keyBytes); err != nil {
		log.Println("error generating random bytes")
	}

	key := base64.StdEncoding.EncodeToString(keyBytes)
	_, ok := chatrooms[key]

	for ok == true {
		keyBytes = make([]byte, 16)
		if _, err := rand.Read(keyBytes); err != nil {
			log.Println("error generating random bytes")
		}

		key = base64.StdEncoding.EncodeToString(keyBytes)
		fmt.Println(key)
	}
	// chatrooms = append(chatrooms, &room)
	room.id = key
	// fmt.Println(key)
	// fmt.Printf("%+v", chatrooms)
	// fmt.Println("room: ", room)
	chatrooms[key] = &room
	// fmt.Printf("chatrooms: %+v\n", chatrooms)

	resString, err := json.Marshal(fmt.Sprint(key))
	if err != nil {
		log.Println("err: ", err)
	}

	writer.WriteHeader(http.StatusAccepted)
	writer.Write([]byte(resString))
}

func connectToRoom(writer http.ResponseWriter, req *http.Request) {
	err := req.ParseMultipartForm(10000000)
	if err != nil {
		log.Println("error parsing form", err)
		writer.WriteHeader(http.StatusNotFound)
		return
	}

	user := req.FormValue("user")
	chatroomName := req.FormValue("room-id")

	chatroomUsers := chatrooms[chatroomName].users
	// _, ok := clients[user]
	// if ok {
	// chatroomUsers = append(chatroomUsers, clients[user])
	chatrooms[chatroomName].users = append(chatroomUsers, clients[user])
	// }

	// fmt.Println(chatroomName)

	writer.WriteHeader(http.StatusAccepted)
	chatroomId, err := json.Marshal(fmt.Sprint(chatroomName))
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		log.Println("error parsing chatroomName")
	}
	writer.Write([]byte(chatroomId))
}

func openWSConnection(writer http.ResponseWriter, req *http.Request) {
	upgrader := websocket.Upgrader{}
	conn, err := upgrader.Upgrade(writer, req, nil)
	if err != nil {
		log.Fatalln("upgrade: ", err)
	}

	// generate random name for client
	keyBytes := make([]byte, 16)
	if _, err := rand.Read(keyBytes); err != nil {
		log.Println("error generating random bytes")
	}

	key := base64.StdEncoding.EncodeToString(keyBytes)

	tempMessage := Message{"here is your user id", id, key}
	// fmt.Printf("%+v\n", tempMessage)
	message, err := json.Marshal(tempMessage)
	if err != nil {
		log.Println("could not parse Message struct")
	}
	// fmt.Println("message: ", message)
	conn.WriteMessage(websocket.TextMessage, message)

	clients[key] = conn
	// clients = append(clients, conn)
	defer conn.Close()

	for {
		// messageType, message, err := conn.ReadMessage()
		_, message, err := conn.ReadMessage()

		if err != nil {
			log.Println("connection closed: ", err)
			break
		} else {
			userMessage := UserMessage{}
			err = json.Unmarshal([]byte(message), &userMessage)
			// fmt.Printf("%+v\n", userMessage)

			if err != nil {
				log.Println("error json parsing user message: ", err)
			}
			// fmt.Println("message type: ", messageType)
			// fmt.Println(string(message))
			// fmt.Println("chatroomid: ", userMessage.ChatroomId)
			// fmt.Printf("%+v\n", userMessage)
			tempMessage := UserMessage{userMessage.Message, "message", userMessage.Id, userMessage.ChatroomId}
			// fmt.Printf("%+v\n", tempMessage)
			broadcastedMessage, err := json.Marshal(tempMessage)
			if err != nil {
				log.Println("could not parse Message struct")
			}
			// fmt.Println(message)
			// fmt.Printf("%+v\n", chatrooms[userMessage.ChatroomId].users)
			// for i := range chatrooms[userMessage.ChatroomId].users {
			// 	fmt.Printf("%+v\n", chatrooms[userMessage.ChatroomId].users)
			// 	chatrooms[userMessage.ChatroomId].users[i].WriteMessage(websocket.TextMessage, broadcastedMessage)
			// }
			for _, user := range chatrooms[userMessage.ChatroomId].users {
				// fmt.Printf("%#+v\n", user)
				user.WriteMessage(websocket.TextMessage, broadcastedMessage)
			}
			// conn.WriteMessage(websocket.TextMessage, broadcastedMessage)
		}
	}
	delete(clients, key)
}