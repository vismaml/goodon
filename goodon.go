package goodon

import (
	"context"
	"log"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func InitTracer() func() {
	ctx := context.Background()
	// Create stdout exporter to see traces in the console
	stdoutExporter, err := stdouttrace.New(
		stdouttrace.WithPrettyPrint(),
		stdouttrace.WithWriter(os.Stdout),
	)
	if err != nil {
		log.Fatalf("Failed to create stdout exporter: %v", err)
	}

	// Create OTLP exporter
	otlpExporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint("alloy:4317"), // FIX THIS
		otlptracegrpc.WithInsecure(),             // FIX THIS
	)
	if err != nil {
		log.Fatalf("Failed to create OTLP trace exporter: %v", err)
	}

	// Use both exporters
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(stdoutExporter),
		sdktrace.WithBatcher(otlpExporter),
		sdktrace.WithSampler(sdktrace.AlwaysSample()), //decide on sampling
	)

	otel.SetTracerProvider(provider)

	return func() {
		ctx := context.Background()
		if err := provider.Shutdown(ctx); err != nil {
			log.Printf("Error shutting down tracer provider: %v", err)
		}
	}
}

func init() {
	prop := newPropagator()
	otel.SetTextMapPropagator(prop)
}

func newPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}
