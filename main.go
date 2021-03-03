package main

import (
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"

	"chat/auth"
	"chat/chatroom"
	"chat/database"
	// "chat/validate"
)

const addr = ":8000"

func init() {
	// err := godotenv.Load()
	// if err != nil {
	// 	log.Fatalln("Error loading .env file: ", err)
	// }

	// auth.Tmpl, err = template.New("templates").ParseGlob("templates/*.html")
	// if err != nil {
	// 	log.Fatalln("Error instantiating templates: ", err)
	// }

	// dbPort, err := strconv.ParseUint(os.Getenv("POSTGRES_PORT"), 10, 16)
	// if err != nil {
	// 	log.Fatalln("Failed to convert db port from environment variable to int: ", err)
	// }
	// database.PgDB = stdlib.OpenDB(pgx.ConnConfig{
	// 	Host:     os.Getenv("POSTGRES_HOST"),
	// 	Port:     uint16(dbPort),
	// 	Database: os.Getenv("POSTGRES_DB"),
	// 	User:     os.Getenv("POSTGRES_USER"),
	// 	Password: os.Getenv("POSTGRES_PASSWORD"),
	// })

	// // spew.Dump(
	// // 	os.Getenv("POSTGRES_HOST"),
	// // 	uint16(dbPort),
	// // 	os.Getenv("POSTGRES_DB"),
	// // 	os.Getenv("POSTGRES_USER"),
	// // 	os.Getenv("POSTGRES_PASSWORD"),
	// // )
	// database.PgStore, err = pgstore.NewPGStoreFromPool(database.PgDB, []byte(os.Getenv("SESSION_SECRET")))
	// if err != nil {
	// 	log.Fatalln("Error creating session store using postgres:", err)
	// }

	// _, err = database.PgDB.Exec(
	// 	`CREATE TABLE IF NOT EXISTS Users (
	// 		id serial PRIMARY KEY,
	// 		email TEXT NOT NULL,
	// 		username TEXT NOT NULL,
	// 		password TEXT NOT NULL
	// 	)`,
	// )

	// if err != nil {
	// 	log.Fatalln("Problem creating Users table: ", err)
	// }

	// _, err = database.PgDB.Exec(
	// 	`CREATE TABLE IF NOT EXISTS Invites (
	// 		id serial PRIMARY KEY,
	// 		invite TEXT NOT NULL,
	// 		chatroom TEXT NOT NULL,
	// 		expires TIMESTAMPTZ NOT NULL
	// 	)`,
	// )

	// if err != nil {
	// 	log.Fatalln("Problem creating Invites table: ", err)
	// }

	// go chatroom.RemoveExpiredInvites(database.PgDB, time.Minute*10)

	// // creating temporary cassandra cluster in order to create keyspace
	// tempCluster := gocql.NewCluster(os.Getenv("SCYLLA_HOST"))
	// cqlSession, err := tempCluster.CreateSession()
	// if err != nil {
	// 	log.Fatalln("Failed to create cluster session: ", err)
	// }

	// createKeyspace := cqlSession.Query(
	// 	fmt.Sprintf(
	// 		`CREATE KEYSPACE IF NOT EXISTS %s
	// 			WITH replication = {
	// 				'class' : 'SimpleStrategy',
	// 				'replication_factor' : 1
	// 			}`,
	// 		os.Getenv("KEYSPACE"),
	// 	), nil)
	// err = createKeyspace.Exec()
	// if err != nil {
	// 	log.Fatalln("Failed to create keyspace: ", err)
	// }

	// // creating scylla cluster
	// cluster := gocql.NewCluster(os.Getenv("SCYLLA_HOST"))
	// cluster.Keyspace = os.Getenv("KEYSPACE")
	// chatroom.ScyllaSession, err = gocqlx.WrapSession(cluster.CreateSession())
	// if err != nil {
	// 	log.Fatalln("Failed to wrap new cluster session: ", err)
	// }

	// err = chatroom.ScyllaSession.ExecStmt(
	// 	`CREATE TABLE IF NOT EXISTS messages(
	// 		chatroom_name TEXT,
	// 		user_id TEXT,
	// 		content TEXT,
	// 		message_id bigint,
	// 		PRIMARY KEY (chatroom_name, message_id)
	// 	) WITH CLUSTERING ORDER BY (message_id DESC)`,
	// )
	// if err != nil {
	// 	log.Fatalln("Create messages store error:", err)
	// }
	// err = chatroom.ScyllaSession.ExecStmt(
	// 	`CREATE TABLE IF NOT EXISTS users(
	// 		user TEXT,
	// 		current_chatroom TEXT STATIC,
	// 		chatroom TEXT,
	// 		PRIMARY KEY (user, chatroom)
	// 	) WITH CLUSTERING ORDER BY (chatroom ASC)`,
	// )
	// if err != nil {
	// 	log.Fatalln("Create messages store error:", err)
	// }

	// // this will generate unique ids for each message on this
	// // particular server instance
	// chatroom.Snowflake = sonyflake.NewSonyflake(
	// 	sonyflake.Settings{
	// 		StartTime: time.Unix(0, 0),
	// 	},
	// )
}

func main() {
	defer database.PgDB.Close()

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
	// TODO turn create-room into /room/create and the other ones too
	router.With(auth.UserSession).Post("/create-room", chatroom.Create)
	// add validation middleware
	router.With(auth.UserSession).Post("/join-room", chatroom.Join)
	// add validation middleware
	router.With(auth.UserSession).Post("/create-invite", chatroom.CreateInvite)
	router.With(auth.UserSession).Post("/user/chatrooms", chatroom.GetUserChatrooms)

	// router.ServeHTTP()

	FileServer(router, "/", http.Dir("./frontend"))
	// fileServer := http.FileServer(http.Dir("./frontend"))
	// http.Handle("/", fileServer)
	// http.Handle("/", http.StripPrefix("/", fileServer))
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
