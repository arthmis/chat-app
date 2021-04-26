package main

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
	"github.com/scylladb/gocqlx/v2"
	"github.com/sony/sonyflake"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpgrpc"
	"go.opentelemetry.io/otel/metric/global"

	controller "go.opentelemetry.io/otel/sdk/metric/controller/basic"
	processor "go.opentelemetry.io/otel/sdk/metric/processor/basic"
	"go.opentelemetry.io/otel/sdk/metric/selector/simple"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc"

	"chat/applog"
	"chat/auth"
	"chat/chatroom"
	"chat/database"
	// "chat/validate"
)

const addr = ":8000"

func init() {
	applog.InitLogger()

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
	var name string
	for rows.Next() {
		err = rows.Scan(&name)
		if err != nil {
			applog.Sugar.Fatalw("couldn't scan row: ", err)
		}

		room := chatroom.NewChatroom()
		room.Id = name
		room.ScyllaSession = &chatroom.ScyllaSession
		room.Snowflake = chatroom.Snowflake
		room.Clients = []*chatroom.ChatroomClient{}
		room.Messages = make([]chatroom.IncomingMessage, 20)
		room.Channel = make(chan chatroom.MessageWithCtx)

		chatroom.Chatrooms[room.Id] = room
		chatroom.ChatroomChannels[room.Id] = room.Channel

		go room.Run()
	}

	applog.Sugar.Infow("Chatrooms initialized.")
}

var tp *sdktrace.TracerProvider

func main() {
	tracerCleanup := initTracer()
	defer tracerCleanup()

	defer database.PgDB.Close()

	applog.Sugar.Infow("Setting up router.")
	router := chi.NewRouter()
	router.Use(middleware.Logger)
	router.Route("/api", func(router chi.Router) {
		router.With(auth.UserSession).Get("/chat", chat)
		router.With(auth.LogRequest).With(auth.UserSession).Get("/ws", chatroom.OpenWsConnection)
		router.Route("/room", func(router chi.Router) {
			// add validation middleware for create
			router.With(auth.UserSession).Post("/create", chatroom.Create)
			// add validation middleware for join
			router.With(auth.UserSession).Post("/join", chatroom.Join)
			// add validation middleware for invite
			router.With(auth.UserSession).Post("/invite", chatroom.CreateInvite)
			// add validation middleware for messages
			router.With(auth.UserSession).Post("/messages", chatroom.GetRoomMessages)
			// router.With(auth.UserSession).Post("/delete", chatroom.GetCurrentRoomMessages)
		})
		router.Route("/user", func(router chi.Router) {
			router.With(auth.UserSession).Post("/chatrooms", chatroom.GetUserInfo)
			// add validation middleware for signup
			router.Post("/signup", auth.Signup)
			// add validation middleware for login
			router.Post("/login", auth.Login)
			router.With(auth.UserSession).Post("/logout", auth.Logout)
			// router.With(auth.UserSession).Post("/", user.GetUser)
		})
	})

	FileServer(router, "/", http.Dir("./frontend"))
	err := http.ListenAndServe(addr, router)

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

func addTracing(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		tracer := otel.Tracer("")

		ctx, span := tracer.Start(req.Context(), "")
		defer span.End()

		next.ServeHTTP(w, req.WithContext(ctx))
	})

}

func initTracer() func() {
	ctx := context.Background()
	exporter, err := otlp.NewExporter(
		ctx,
		otlpgrpc.NewDriver(
			otlpgrpc.WithInsecure(),
			otlpgrpc.WithEndpoint("localhost:4317"),
			otlpgrpc.WithDialOption(grpc.WithBlock()),
		),
	)
	if err != nil {
		applog.Sugar.Warn("failed to initialize honeycomb export pipeline: ", err)
		return func() {}
	}

	pusher := controller.New(
		processor.New(
			simple.NewWithExactDistribution(),
			exporter,
		),
		controller.WithExporter(exporter),
		controller.WithCollectPeriod(5*time.Second),
	)

	err = pusher.Start(ctx)

	bsp := sdktrace.NewBatchSpanProcessor(exporter)
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(bsp),
		// sdktrace.WithBatcher(exporter),
	)
	otel.SetTracerProvider(provider)
	global.SetMeterProvider(pusher.MeterProvider())
	// propagator := propagation.NewCompositeTextMapPropagator(propagation.Baggage{}, propagation.TraceContext{})
	// otel.SetTextMapPropagator(propagator)

	applog.Sugar.Info("Tracing initialized.")
	return func() {
		ctx := context.Background()
		err := provider.Shutdown(ctx)
		if err != nil {
			applog.Sugar.Fatal(err)
		}
		err = pusher.Stop(ctx)
		if err != nil {
			applog.Sugar.Fatal(err)
		}
	}
}
