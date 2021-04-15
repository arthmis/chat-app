package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi"
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
	apikey, _ := os.LookupEnv("HONEYCOMB_API_KEY")
	dataset, _ := os.LookupEnv("HONEYCOMB_DATASET")

	ctx := context.Background()
	exporter, err := otlp.NewExporter(
		ctx,
		otlpgrpc.NewDriver(
			otlpgrpc.WithTLSCredentials(credentials.NewClientTLSFromCert(nil, "")),
			otlpgrpc.WithEndpoint("api.honeycomb.io:443"),
			otlpgrpc.WithHeaders(map[string]string{
				"x-honeycomb-team":    apikey,
				"x-honeycomb-dataset": dataset,
			}),
		),
	)
	if err != nil {
		// applog.Sugar.Fatal("failed to initialize http export pipeline: %v", err)
		log.Fatalf("failed to initialize http export pipeline: %v", err)
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
	log.Printf("%v", provider)
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

	router := chi.NewRouter()
	router.Use(doTracing)
	// router.With(doTracing).Get("/do-thing", doThing)
	router.Get("/do-thing", doThing)
	// otlphttp.
	// router.Get("/do-thing", doThing)
	// router.Get("/do-thing", otelhttp.NewHandler(doThing, "test"))
	_ = http.ListenAndServe(":8000", router)
}

func doThing(w http.ResponseWriter, req *http.Request) {
	// applog.Sugar.Info("hello")
	// span := trace.SpanFromContext(req.Context())
	// tracer := otel.Tracer("doThing")
	// var span trace.Span
	// _, span = tracer.Start(req.Context(), "doThing")
	// defer span.End()
	// span.AddEvent("Hello, Finished doing thing!")
	// span.SetAttributes(kv.Any("id", id), kv.Any("price", price))
	// applog.Sugar.Info("goodbye")

	w.WriteHeader(200)
}

func doTracing(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		tracer := otel.Tracer("")
		tracerprovider := otel.GetTracerProvider()
		// spew.Dump(tracerprovider)
		log.Printf("%v", tracerprovider)

		ctx, span := tracer.Start(req.Context(), "doTracing")
		defer span.End()

		span.AddEvent("Doing Tracing!", trace.WithAttributes((attribute.Int("bogons", 100))))

		next.ServeHTTP(w, req.WithContext(ctx))
		// ww := middleware.NewWrapResponseWriter(w, req.ProtoMajor)
		// next.ServeHTTP(ww, req.WithContext(ctx))
	})

}
