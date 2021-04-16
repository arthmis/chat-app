package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi"
	"github.com/joho/godotenv"
	"google.golang.org/grpc/credentials"

	// "github.com/honeycombio/opentelemetry-exporter-go/honeycomb"
	// otelhttp "go.opentelemetry.io/contrib/instrumentation/net/http"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpgrpc"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func initTracer() func() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file: ", err)
	}

	apikey, _ := os.LookupEnv("HONEYCOMB_API_KEY")
	dataset, _ := os.LookupEnv("HONEYCOMB_DATASET")

	ctx := context.Background()
	// exporter, err := otlp.NewExporter(
	exporter := otlp.NewUnstartedExporter(
		// ctx,
		otlpgrpc.NewDriver(
			otlpgrpc.WithTLSCredentials(credentials.NewClientTLSFromCert(nil, "")),
			otlpgrpc.WithEndpoint("api.honeycomb.io:443"),
			otlpgrpc.WithHeaders(map[string]string{
				"x-honeycomb-team":    apikey,
				"x-honeycomb-dataset": dataset,
			}),
		),
	)
	// exporter, err := otlp.NewExporter(
	// 	ctx,
	// 	otlphttp.NewDriver(
	// 		// otlpgrpc.WithTLSCredentials(credentials.NewClientTLSFromCert(nil, "")),
	// 		otlphttp.WithEndpoint("api.honeycomb.io:443"),
	// 		otlphttp.WithHeaders(map[string]string{
	// 			"x-honeycomb-team":    apikey,
	// 			"x-honeycomb-dataset": dataset,
	// 		}),
	// 	),
	// )
	// otlphttp.NewDriver(otlphttp.)
	err = exporter.Start(ctx)
	if err != nil {
		// applog.Sugar.Fatal("failed to initialize http export pipeline: %v", err)
		log.Fatalf("failed to initialize honeycomb export pipeline: %v", err)
	}
	// exporter, err := stdout.NewExporter(stdout.WithPrettyPrint())
	// if err != nil {
	// 	applog.Sugar.Fatal("failed to nitialize stdout export pipeline: %v", err)
	// }

	// bsp := sdktrace.NewBatchSpanProcessor(exporter)
	// provider := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()), sdktrace.WithSpanProcessor(bsp))
	// otel.SetTracerProvider(provider)

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithBatcher(exporter),
	)
	// log.Printf("%v", provider)
	otel.SetTracerProvider(provider)

	return func() {
		ctx := context.Background()
		err := provider.Shutdown(ctx)
		if err != nil {
			log.Fatal(err)
		}
	}
}
func main() {
	cleanup := initTracer()
	defer cleanup()

	// ctx := context.Background()
	// tracer := otel.Tracer("")
	// tracerprovider := otel.GetTracerProvider()
	// // spew.Dump(tracerprovider)
	// log.Printf("%v", tracerprovider)

	// ctx, span := tracer.Start(ctx, "doTracing")
	// defer span.End()

	// span.AddEvent("Doing Tracing!", trace.WithAttributes((attribute.Int("bogons", 100))))
	// span.SetAttributes(attribute.Bool("isTrue", true))
	// apikey, isThere := os.LookupEnv("HONEYCOMB_API_KEY")
	// log.Print(isThere)
	// dataset, _ := os.LookupEnv("HONEYCOMB_DATASET")
	// beeline.Init(beeline.Config{
	// 	// Get this via https://ui.honeycomb.io/account after signing up for Honeycomb
	// 	WriteKey: apikey,
	// 	// The name of your app is a good choice to start with
	// 	Dataset: dataset,
	// })
	// defer beeline.Close()
	router := chi.NewRouter()
	// router.Use(doTracing)
	// // router.With(doTracing).Get("/do-thing", doThing)
	// router.Get("/do-thing", doThing)
	router.Get("/do-thing", doThing)
	// // router.Get("/do-thing", otelhttp.NewHandler(doThing, "test"))
	// _ = http.ListenAndServe(":8000", hnynethttp.WrapHandler(router))
	_ = http.ListenAndServe(":8000", router)
}

func doThing(w http.ResponseWriter, req *http.Request) {
	// applog.Sugar.Info("hello")
	tracer := otel.Tracer("")
	_, span := tracer.Start(req.Context(), "doTracing")
	// span := trace.SpanFromContext(req.Context())
	// tracer := otel.Tracer("doThing")
	// var span trace.Span
	// _, span = tracer.Start(req.Context(), "doThing")
	defer span.End()
	span.AddEvent("Hello, Finished doing thing!")
	// span.SetAttributes(kv.Any("id", id), kv.Any("price", price))
	// applog.Sugar.Info("goodbye")

	w.WriteHeader(200)
}

func doTracing(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		tracer := otel.Tracer("")
		// tracerprovider := otel.GetTracerProvider()
		// spew.Dump(tracerprovider)
		// log.Printf("%v", tracerprovider)

		ctx, span := tracer.Start(req.Context(), "doTracing")
		defer span.End()

		span.AddEvent("Doing Tracing!", trace.WithAttributes((attribute.Int("bogons", 100))))
		span.SetAttributes(attribute.Bool("isTrue", true))

		next.ServeHTTP(w, req.WithContext(ctx))
		// ww := middleware.NewWrapResponseWriter(w, req.ProtoMajor)
		// next.ServeHTTP(ww, req.WithContext(ctx))
	})

}
