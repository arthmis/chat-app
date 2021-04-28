package main

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi"
	"github.com/joho/godotenv"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpgrpc"
	"go.opentelemetry.io/otel/metric/global"

	controller "go.opentelemetry.io/otel/sdk/metric/controller/basic"
	processor "go.opentelemetry.io/otel/sdk/metric/processor/basic"
	"go.opentelemetry.io/otel/sdk/metric/selector/simple"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc"

	"chat/app"
)

const addr = ":8000"

var tp *sdktrace.TracerProvider

func main() {
	app.InitLogger()
	err := godotenv.Load()
	if err != nil {
		app.Sugar.Fatalw("Error loading .env file: ", err)
	}

	pgHost, ok := os.LookupEnv("POSTGRES_HOST")
	if !ok {
		app.Sugar.Fatal("Could not find POSTGRES_HOST env")
	}
	pgDb, ok := os.LookupEnv("POSTGRES_DB")
	if !ok {
		app.Sugar.Fatal("Could not find POSTGRES_DB env")
	}
	pgUser, ok := os.LookupEnv("POSTGRES_USER")
	if !ok {
		app.Sugar.Fatal("Could not find POSTGRES_USER env")
	}
	pgPassword, ok := os.LookupEnv("POSTGRES_PASSWORD")
	if !ok {
		app.Sugar.Fatal("Could not find POSTGRES_PASSWORD env")
	}
	pgPortStr, ok := os.LookupEnv("POSTGRES_PORT")
	if !ok {
		app.Sugar.Fatal("Could not find POSTGRES_PORT env")
	}
	pgPort, err := strconv.ParseInt(pgPortStr, 10, 16)
	if err != nil {
		app.Sugar.Fatalf("Could not convert POSTGRES_PORT to a number. %v", pgPortStr)
	}
	pgConfig := app.PgConfig{
		Host:     pgHost,
		Db:       pgDb,
		User:     pgUser,
		Password: pgPassword,
		Port:     uint16(pgPort),
	}

	scyllaHost, ok := os.LookupEnv("SCYLLA_HOST")
	if !ok {
		app.Sugar.Fatal("Could not find SCYLLA_HOST env")
	}
	scyllaKeyspace, ok := os.LookupEnv("KEYSPACE")
	if !ok {
		app.Sugar.Fatal("Could not find KEYSPACE env")
	}
	scyConfig := app.ScyllaConfig{
		Host:     scyllaHost,
		Keyspace: scyllaKeyspace,
	}

	application := app.NewApp(pgConfig, scyConfig, "templates/*.html")
	tracerCleanup := initTracer()
	defer tracerCleanup()

	defer application.Pg.Close()

	app.Sugar.Infow("Setting up router.")

	routes := application.Routes()
	FileServer(routes, "/", http.Dir("./frontend"))
	err = http.ListenAndServe(addr, routes)

	if err != nil {
		app.Sugar.Fatal("error starting server: ", err)
	}
	app.Sugar.Info("Starting server.")
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
		app.Sugar.Warn("failed to initialize honeycomb export pipeline: ", err)
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

	app.Sugar.Info("Tracing initialized.")
	return func() {
		ctx := context.Background()
		err := provider.Shutdown(ctx)
		if err != nil {
			app.Sugar.Fatal(err)
		}
		err = pusher.Stop(ctx)
		if err != nil {
			app.Sugar.Fatal(err)
		}
	}
}
