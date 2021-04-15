// package main

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/antonlindstrom/pgstore"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/gocql/gocql"
	"github.com/jackc/pgx"
	"github.com/jackc/pgx/stdlib"
	"github.com/joho/godotenv"
	"github.com/opentracing/opentracing-go"
	"github.com/scylladb/gocqlx/v2"
	"github.com/sony/sonyflake"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/stdout"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"chat/applog"
	"chat/auth"
	"chat/chatroom"
	"chat/database"
	// "chat/validate"
)

const addr = ":8000"

func init() {
	applog.InitLogger()

	// app.AppState = app.NewApp()

	err := godotenv.Load()
	if err != nil {
		applog.Sugar.Fatalw("Error loading .env file: ", err)
	}

	auth.Tmpl, err = template.New("templates").ParseGlob("templates/*.html")
	if err != nil {
		applog.Sugar.Fatalw("Error instantiating templates: ", err)
	}

	dbPort, err := strconv.ParseUint(os.Getenv("POSTGRES_PORT"), 10, 16)
	if err != nil {
		applog.Sugar.Fatalw("Failed to convert db port from environment variable to int: ", err)
	}
	database.PgDB = stdlib.OpenDB(pgx.ConnConfig{
		Host:     os.Getenv("POSTGRES_HOST"),
		Port:     uint16(dbPort),
		Database: os.Getenv("POSTGRES_DB"),
		User:     os.Getenv("POSTGRES_USER"),
		Password: os.Getenv("POSTGRES_PASSWORD"),
	})

	database.PgStore, err = pgstore.NewPGStoreFromPool(database.PgDB, []byte(os.Getenv("SESSION_SECRET")))
	if err != nil {
		applog.Sugar.Fatal("Error creating session store using postgres:", err)
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
		applog.Sugar.Fatalw("Problem creating Users table: ", err)
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
		applog.Sugar.Fatalw("Problem creating Invites table: ", err)
	}

	_, err = database.PgDB.Exec(
		`CREATE TABLE IF NOT EXISTS Rooms (
			id serial PRIMARY KEY,
			name TEXT NOT NULL
		)`,
	)

	if err != nil {
		applog.Sugar.Fatalw("Problem creating Rooms table: ", err)
	}

	applog.Sugar.Info("Postgres database has been initialized.")

	go chatroom.RemoveExpiredInvites(database.PgDB, time.Minute*10)

	// creating temporary cassandra cluster in order to create keyspace
	tempCluster := gocql.NewCluster(os.Getenv("SCYLLA_HOST"))
	tempCluster.ProtoVersion = 4
	cqlSession, err := tempCluster.CreateSession()
	if err != nil {
		applog.Sugar.Fatalw("Failed to create cluster session: ", err)
	}

	createKeyspace := cqlSession.Query(
		fmt.Sprintf(
			`CREATE KEYSPACE IF NOT EXISTS %s
				WITH replication = {
					'class' : 'SimpleStrategy',
					'replication_factor' : 1
				}`,
			os.Getenv("KEYSPACE"),
		), nil)
	err = createKeyspace.Exec()
	if err != nil {
		applog.Sugar.Fatalw("Failed to create keyspace: ", err)
	}

	// creating scylla cluster
	cluster := gocql.NewCluster(os.Getenv("SCYLLA_HOST"))
	cluster.Keyspace = os.Getenv("KEYSPACE")
	chatroom.ScyllaSession, err = gocqlx.WrapSession(cluster.CreateSession())
	if err != nil {
		applog.Sugar.Fatalw("Failed to wrap new cluster session: ", err)
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
		applog.Sugar.Fatalw("Create messages store error:", err)
	}
	err = chatroom.ScyllaSession.ExecStmt(
		`CREATE TABLE IF NOT EXISTS users(
			user TEXT,
			current_chatroom TEXT STATIC,
			chatroom TEXT,
			PRIMARY KEY (user, chatroom)
		) WITH CLUSTERING ORDER BY (chatroom ASC)`,
	)
	if err != nil {
		applog.Sugar.Fatalw("Create messages store error:", err)
	}
	applog.Sugar.Infow("CassandraDB has been initialized.")

	// this will generate unique ids for each message on this
	// particular server instance
	chatroom.Snowflake = sonyflake.NewSonyflake(
		sonyflake.Settings{
			StartTime: time.Unix(0, 0),
		},
	)

	rows, err := database.PgDB.Query(
		`SELECT name FROM Rooms`,
	)
	if err != nil {
		applog.Sugar.Fatalw("couldn't get room rows", err)
	}

	// initialize chatrooms
	for {
		if rows.Next() {
			var name string
			err = rows.Scan(&name)
			if err != nil {
				applog.Sugar.Fatalw("couldn't scan row: ", err)
			}

			room := chatroom.NewChatroom()
			room.Id = name
			room.ScyllaSession = &chatroom.ScyllaSession
			room.Snowflake = chatroom.Snowflake
			// room.Clients = make([]*chatroom.User, 0)
			room.Clients = []*chatroom.ChatroomClient{}
			room.Messages = make([]chatroom.UserMessage, 20)
			room.Channel = make(chan chatroom.UserMessage)

			chatroom.Chatrooms[room.Id] = room
			chatroom.ChatroomChannels[room.Id] = room.Channel

			go room.Run()
		} else {
			break
		}
	}
	applog.Sugar.Infow("Chatrooms initialized.")
}

var tp *sdktrace.TracerProvider

func main() {
	defer database.PgDB.Close()

	exporter, err := stdout.NewExporter(stdout.WithPrettyPrint())
	if err != nil {
		applog.Sugar.Fatal("failed to nitialize stdout export pipeline: %v", err)
	}

	bsp := sdktrace.NewBatchSpanProcessor(exporter)
	tp = sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()), sdktrace.WithSpanProcessor(bsp))

	otel.SetTracerProvider(tp)
	propagator := propagation.NewCompositeTextMapPropagator(propagation.Baggage{}, propagation.TraceContext{})
	otel.SetTextMapPropagator(propagator)

	tracer := tp.Tracer("chat-application")

	ctx := context.Background()
	defer func() { _ = tp.Shutdown(ctx) }()

	var span trace.Span
	ctx, span = tracer.Start(ctx, "operation")
	defer span.End()
	span.AddEvent("Nice operation!", trace.WithAttributes((attribute.Int("bogons", 100))))
	applog.Sugar.Infow("Setting up router.")
	router := chi.NewRouter()
	router.Use(middleware.Logger)
	// router.Use(httptracer.Tracer(tracer, nil))
	// add validation middleware
	// router.Get("/", func(w http.ResponseWriter, req *http.Request) {
	// 	w.WriteHeader(200)
	// })
	router.Route("/api", func(router chi.Router) {
		router.With(doTracing).With(auth.UserSession).Get("/chat", chat)
		router.With(auth.LogRequest).With(auth.UserSession).Get("/ws", chatroom.OpenWsConnection)
		router.Route("/room", func(router chi.Router) {
			router.With(auth.UserSession).Post("/create", chatroom.Create)
			router.With(auth.UserSession).Post("/join", chatroom.Join)
			router.With(auth.UserSession).Post("/invite", chatroom.CreateInvite)
			router.With(auth.UserSession).Post("/messages", chatroom.GetRoomMessages)
			// router.With(auth.UserSession).Post("/delete", chatroom.GetCurrentRoomMessages)
		})
		router.Route("/user", func(router chi.Router) {
			router.With(auth.LogRequest).With(auth.UserSession).Post("/chatrooms", chatroom.GetUserInfo)
			router.Post("/signup", auth.Signup)
			router.Post("/login", auth.Login)
			router.With(auth.UserSession).Post("/logout", auth.Logout)
			// router.With(auth.UserSession).Post("/", user.GetUser)
		})
	})

	// router.ServeHTTP()

	FileServer(router, "/", http.Dir("./frontend"))
	// fileServer := http.FileServer(http.Dir("./frontend"))
	// http.Handle("/", fileServer)
	// http.Handle("/", http.StripPrefix("/", fileServer))
	err = http.ListenAndServe(addr, router)

	if err != nil {
		applog.Sugar.Fatal("error starting server: ", err)
	}
	applog.Sugar.Info("Starting server.")
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

func doTracing(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		tracer := tp.Tracer("chat-application")

		// ctx := context.Background()
		ctx := req.Context()
		// var span trace.Span
		span, traceCtx := opentracing.StartSpanFromContextWithTracer(ctx, tracer, "operating", nil)
		defer func() { _ = tp.Shutdown(ctx) }()

		ctx, span = tracer.Start(ctx, "operation")
		defer span.Finish()
		span.("Nice operation!", trace.WithAttributes((attribute.Int("bogons", 100))))
		next.ServeHTTP(w, req)
	})

}
