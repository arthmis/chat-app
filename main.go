package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sony/sonyflake"

	"github.com/antonlindstrom/pgstore"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"

	// "github.com/jackc/pgx/v4/pgxpool"
	"github.com/davecgh/go-spew/spew"
	"github.com/gocql/gocql"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/scylladb/gocqlx/v2"

	"chat/auth"
	"chat/chatroom"
	// "chat/validate"
)

const addr = ":8000"

var clients = make(map[string]*chatroom.User)

var chatrooms = make(map[string]*chatroom.Chatroom)
var chatroomChannels = make(map[string]chan chatroom.UserMessage)

// var db *pgxpool.Pool
var snowflake *sonyflake.Sonyflake
var scyllaSession gocqlx.Session

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file:\n", err)
	}
	spew.Dump(chatroomChannels)

	auth.Tmpl, err = template.New("templates").ParseGlob("templates/*.html")
	if err != nil {
		log.Println(err)
		return
	}

	// dbpool, err = pgxpool.Connect(context.Background(), os.Getenv("DATABASE_URL"))
	auth.Db, err = sql.Open("pgx", os.Getenv("DATABASE_URL"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to connect to database: %v\n", err)
		os.Exit(1)
	}
	auth.Store, err = pgstore.NewPGStoreFromPool(auth.Db, []byte(os.Getenv("SESSION_SECRET")))
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}
	defer auth.Db.Close()

	_, err = auth.Db.Exec(
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

	_, err = auth.Db.Exec(
		`CREATE TABLE IF NOT EXISTS Invites (
			id serial PRIMARY KEY,
			invite TEXT NOT NULL,
			chatroom TEXT NOT NULL,
			expires TIMESTAMPTZ NOT NULL
		)`,
	)

	if err != nil {
		fmt.Fprintf(os.Stderr, "Problem creating database table: %v\n", err)
	}

	go removeExpiredInvites(auth.Db, time.Minute*10)

	// creating scylla cluster
	cluster := gocql.NewCluster("127.0.0.1")
	cluster.Keyspace = "chatserver"
	scyllaSession, err = gocqlx.WrapSession(cluster.CreateSession())
	if err != nil {
		log.Fatal("Failed to wrap new cluster session: ", err)
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
		log.Fatal("Create chatrooms error: ", err)
	}

	// this will generate unique ids for each message on this
	// particular server instance
	snowflake = sonyflake.NewSonyflake(
		sonyflake.Settings{
			StartTime: time.Unix(0, 0),
		},
	)
	if err != nil {
		log.Fatal("Create messages store error:", err)
	}

	router := chi.NewRouter()
	router.Use(middleware.Logger)
	router.Post("/signup", auth.Signup)
	router.Post("/login", auth.Login)
	router.With(auth.UserSession).Post("/logout", auth.Logout)
	router.With(auth.UserSession).Get("/chat", chat)
	router.Handle("/", http.FileServer(http.Dir("./frontend")))
	router.With(auth.UserSession).Get("/ws", openWsConnection)
	// add validation middleware
	router.With(auth.UserSession).Post("/create-room", createRoom)
	// add validation middleware
	router.With(auth.UserSession).Post("/join-room", joinRoom)
	// add validation middleware
	router.With(auth.UserSession).Post("/create-invite", createInvite)

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

// TODO add channel that would allow breaking out of this function
// might be necessary
func removeExpiredInvites(db *sql.DB, interval time.Duration) {
	ticker := time.NewTicker(interval)

	defer func() {
		ticker.Stop()
	}()

	for {
		select {

		case <-ticker.C:
			_, err := db.Exec("DELETE FROM Invites WHERE expires < now()")
			if err != nil {
				log.Printf("Unable to delete invites: %v", err)
			}
		}
	}
}

// TODO check to see if the user is actually a part of the chatroom
// before they are allowed to create an invite
func createInvite(w http.ResponseWriter, req *http.Request) {
	err := req.ParseMultipartForm(50000)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	roomName := req.FormValue("chatroom_name")
	// timeLimit := req.FormValue("invite_timelimit")
	inviteTimeLimit := time.Time{}
	forever := 0.0
	switch timeLimit := req.FormValue("invite_timelimit"); timeLimit {
	case "1 day":
		inviteTimeLimit = time.Now().Add(time.Hour * 24)
	case "1 week":
		inviteTimeLimit = time.Now().Add(time.Hour * 24 * 7)
	case "Forever":
		forever = math.Inf(1)
	default:
		w.WriteHeader(http.StatusBadRequest)
		_, err := w.Write([]byte("Expiry value is not one of the possible choices"))
		if err != nil {
			log.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}
	log.Println(roomName)
	// log.Println(timeLimit)

	inviteCodeUUID, err := uuid.NewRandom()
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	inviteCode := inviteCodeUUID.String()
	inviteCode = strings.ReplaceAll(inviteCode, "-", "")
	fmt.Println(inviteCode)
	if forever == math.Inf(1) {
		_, err := auth.Db.Exec(
			`INSERT INTO Invites (invite, chatroom, expires) VALUES ($1, $2, $3)`,
			inviteCode,
			roomName,
			"infinity",
		)
		if err != nil {
			log.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	} else {
		_, err := auth.Db.Exec(
			`INSERT INTO Invites (invite, chatroom, expires) VALUES ($1, $2, $3)`,
			inviteCode,
			roomName,
			inviteTimeLimit,
		)
		if err != nil {
			log.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
	encodedInviteCode, err := json.Marshal(inviteCode)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	w.Write(encodedInviteCode)

}

// add validation for this endpoint
func joinRoom(writer http.ResponseWriter, req *http.Request) {
	err := req.ParseMultipartForm(50000)
	if err != nil {
		log.Println(err)
		writer.WriteHeader(http.StatusInternalServerError)
	}

	inviteCode := req.FormValue("invite_code")
	// fmt.Println(inviteCode)

	row := auth.Db.QueryRow(
		`SELECT chatroom, expires FROM Invites WHERE invite=$1`,
		inviteCode,
	)

	var chatroomName string
	// TODO: use this to figure out whether invite is past its expiration
	// before allowing user to use it
	var inviteExpiration string
	err = row.Scan(&chatroomName, &inviteExpiration)
	if err != nil {
		log.Println(err)
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}
	// fmt.Printf("\n\n chatroom: %+v \nexpire: %+v \n\n", chatroomName, inviteExpiration)

	session, err := auth.Store.Get(req, "session-name")
	if err != nil {
		log.Println(err)
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	user := session.Values["username"].(string)

	chatrooms[chatroomName].Users = append(chatrooms[chatroomName].Users, clients[user])
	// todo add chatroom to user also

	// writer.WriteHeader(http.StatusInternalServerError)
	name, err := json.Marshal(chatroomName)
	if err != nil {
		log.Println(err)
	}

	writer.WriteHeader(http.StatusAccepted)
	_, err = writer.Write(name)
	if err != nil {
		log.Println(err)
	}
}

func chat(w http.ResponseWriter, req *http.Request) {
	http.ServeFile(w, req, "./frontend/chat")
}

// TODO think about tracking users and the rooms they are a part of
// track rooms and users that are authorized to use it
func createRoom(writer http.ResponseWriter, req *http.Request) {
	err := req.ParseMultipartForm(50000)
	if err != nil {
		writer.WriteHeader(http.StatusNotFound)
		log.Println("error parsing form")
		return
	}

	session, err := auth.Store.Get(req, "session-name")
	if err != nil {
		log.Println(err)
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	room := chatroom.NewChatroom()
	room.Id = req.FormValue("chatroom_name")
	room.Snowflake = snowflake
	room.ScyllaSession = &scyllaSession
	clients[session.Values["username"].(string)].Chatrooms = append(clients[session.Values["username"].(string)].Chatrooms, room.Id)
	room.Users = append(room.Users, clients[session.Values["username"].(string)])
	chatroomChannels[room.Id] = room.Channel
	chatrooms[room.Id] = room

	go room.Run()

	chatroomNameEncoded, err := json.Marshal(room.Id)
	if err != nil {
		log.Println(err)
	}

	writer.WriteHeader(http.StatusAccepted)
	_, err = writer.Write(chatroomNameEncoded)
	if err != nil {
		log.Println(err)
	}
}

func connectToRoom(writer http.ResponseWriter, req *http.Request) {
	err := req.ParseMultipartForm(10000000)
	if err != nil {
		log.Println("error parsing form", err)
		writer.WriteHeader(http.StatusNotFound)
		return
	}

	// user := req.FormValue("user")
	chatroomName := req.FormValue("room-id")

	// chatroomUsers := chatrooms[chatroomName].users
	// _, ok := clients[user]
	// if ok {
	// chatroomUsers = append(chatroomUsers, clients[user])
	// chatrooms[chatroomName].users = append(chatroomUsers, clients[user])
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

func openWsConnection(writer http.ResponseWriter, req *http.Request) {
	upgrader := websocket.Upgrader{}
	conn, err := upgrader.Upgrade(writer, req, nil)
	if err != nil {
		log.Fatalln("upgrade: ", err)
	}

	session, err := auth.Store.Get(req, "session-name")
	if err != nil {
		log.Println(err)
	}
	clientName := session.Values["username"].(string)

	if err != nil {
		log.Println("could not parse Message struct")
	}
	user := chatroom.User{Conn: conn, Id: clientName, Chatrooms: make([]string, 0)}
	clients[clientName] = &user

	defer conn.Close()

	for {
		messageType, message, err := conn.ReadMessage()

		if err != nil {
			log.Println("connection closed: ", err)
			break
		}
		userMessage := chatroom.UserMessage{}
		err = json.Unmarshal([]byte(message), &userMessage)
		// TODO: Figure out why user that is sent is "art" and not appropriate user
		userMessage.User = clientName

		if err != nil {
			log.Println("error json parsing user message: ", err)
			break
		}

		fmt.Println("message type: ", messageType)
		fmt.Println("client name: ", clientName)
		fmt.Println()
		// fmt.Printf("%+v\n", userMessage)
		// fmt.Printf("%+v\n", chatroomChannels[userMessage.ChatroomName])
		chatroomChannels[userMessage.ChatroomName] <- userMessage
		if err != nil {
			log.Println("could not parse Message struct")
			break
		}
	}
	delete(clients, clientName)
}
