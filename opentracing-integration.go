package main

import (
	"chat/applog"
	"context"
	"net/http"
	"os"

	"github.com/go-chi/chi"
	"google.golang.org/grpc/credentials"

	// "github.com/honeycombio/opentelemetry-exporter-go/honeycomb"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpgrpc"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

var tp *sdktrace.TracerProvider

func main() {
	ctx := context.Background()
	// exporter, err := stdout.NewExporter(stdout.WithPrettyPrint())
	// if err != nil {
	// 	applog.Sugar.Fatal("failed to nitialize stdout export pipeline: %v", err)
	// }
	apikey, _ := os.LookupEnv("HONEYCOMB_API_KEY")
	dataset, _ := os.LookupEnv("HONEYCOMB_DATASET")
	// hny, err := honeycomb.NewExporter(honeycomb.Config{APIKey: apikey}, honeycomb.TargetingDataset(dataset), honeycomb.WithServiceName("rume-test"))
	// if err != nil {
	// 	applog.Sugar.Fatal(err)
	// }

	// httpConfig := otlphttp.WithEndpoint("localhost:9001")
	// metricsDriver := otlphttp.NewDriver()
	// tracesDriver := otlphttp.NewDriver(httpConfig)
	// // tracesDriver := otlphttp.NewDriver()
	// config := otlp.SplitConfig{ForMetrics: metricsDriver, ForTraces: tracesDriver}
	// driver := otlp.NewSplitDriver(config)
	// exporter, err := otlp.NewExporter(ctx, driver)
	// if err != nil {
	// 	applog.Sugar.Fatal("failed to nitialize http export pipeline: %v", err)
	// }

	// bsp := sdktrace.NewBatchSpanProcessor(hny)
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
		applog.Sugar.Fatal("failed to initialize http export pipeline: %v", err)
	}

	bsp := sdktrace.NewBatchSpanProcessor(exporter)
	tp = sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()), sdktrace.WithSpanProcessor(bsp))
	// provider := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()), trace.WithBatcher(exporter))

	otel.SetTracerProvider(tp)
	propagator := propagation.NewCompositeTextMapPropagator(propagation.Baggage{}, propagation.TraceContext{})
	otel.SetTextMapPropagator(propagator)

	// tracer := tp.Tracer("chat-application")

	defer func() { _ = tp.Shutdown(ctx) }()
	// defer hny.Shutdown(ctx)
	// var span trace.Span
	// ctx, span = tracer.Start(ctx, "operation")
	// defer span.End()

	router := chi.NewRouter()
	router.With(doTracing).Get("/do-thing", doThing)
	// otlphttp.
	// router.Get("/do-thing", doThing)
	err = http.ListenAndServe(":8000", router)
}

func doThing(w http.ResponseWriter, req *http.Request) {
	applog.Sugar.Info("hello")
	tracer := otel.Tracer("doThing")
	var span trace.Span
	_, span = tracer.Start(req.Context(), "doThing")
	span.AddEvent("Hello, Finished doing thing!")
	defer span.End()
	applog.Sugar.Info("goodbye")

	w.WriteHeader(200)
}

func doTracing(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		tracer := otel.Tracer("doThing")

		// ctx := context.Background()
		ctx := req.Context()
		var span trace.Span
		// span, traceCtx := opentracing.StartSpanFromContextWithTracer(ctx, tracer, "operating", nil)
		defer func() { _ = tp.Shutdown(ctx) }()

		ctx, span = tracer.Start(ctx, "operation")
		defer span.End()
		span.AddEvent("Nice operation!", trace.WithAttributes((attribute.Int("bogons", 100))))
		next.ServeHTTP(w, req.WithContext(ctx))
		// ww := middleware.NewWrapResponseWriter(w, req.ProtoMajor)
		// next.ServeHTTP(ww, req.WithContext(ctx))
	})

}
