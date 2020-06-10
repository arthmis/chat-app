package main

import (
	"context"
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
	"github.com/go-playground/validator/v10"
	"github.com/gorilla/schema"
	"github.com/jackc/pgx/v4/pgxpool"
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

var dbpool *pgxpool.Pool
var validate *validator.Validate
var tmpl *template.Template

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file:\n", err)
	}

	// fmt.Println(os.Getenv("DATABASE_URL"))
	tmpl, err = template.New("templates").ParseGlob("templates/*.html")
	if err != nil {
		log.Println(err)
		return
	}
	validate = validator.New()
	dbpool, err = pgxpool.Connect(context.Background(), os.Getenv("DATABASE_URL"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to connect to database: %v\n", err)
		os.Exit(1)
	}
	defer dbpool.Close()

	_, err = dbpool.Exec(
		context.Background(),
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
	router.Get("/chat", chat)

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

	// // _, _ = checkUserExists(ctx, dbpool, form.Email, form.Username)
	emailExists, usernameExists := checkUserExists(req.Context(), dbpool, form.Email, form.Username)

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
	// // TODO COMPLETE THIS
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

	_, err = dbpool.Exec(
		req.Context(),
		`INSERT INTO Users (email, username, password) VALUES ($1, $2, $3)`,
		form.Email,
		form.Username,
		string(hash),
	)
	if err != nil {
		log.Println("error inserting values: \n", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	tmpl.ExecuteTemplate(w, "login.html", nil)
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

	row := dbpool.QueryRow(
		req.Context(),
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

	// password did match
	// TODO add sessions handling; refresh session
	w.WriteHeader(http.StatusAccepted)
	tmpl.ExecuteTemplate(w, "rooms.html", nil)
	// ctx.HTML(http.StatusSeeOther, "room.tmpl", gin.H{})
}

func chat(w http.ResponseWriter, req *http.Request) {
	// check session and if session doesn't exist then redirect to login
	http.Redirect(w, req, "login", http.StatusSeeOther)
}

func checkUserExists(ctx context.Context, dbpool *pgxpool.Pool, email string, username string) (emailExists, usernameExists bool) {
	row := dbpool.QueryRow(
		ctx,
		`SELECT email FROM Users WHERE email=$1`,
		email,
	)

	err := row.Scan(&email)
	emailExists = true
	if err == pgx.ErrNoRows {
		emailExists = false
	}
	row = dbpool.QueryRow(
		ctx,
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
