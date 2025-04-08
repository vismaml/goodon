package goodon

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	otelmetric "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	oteltrace "go.opentelemetry.io/otel/trace"
)

var (
	HttpRequestCounter otelmetric.Int64Counter
	Tracer             oteltrace.Tracer
	Meter              otelmetric.Meter
)

// InitTracer sets up the OpenTelemetry tracer provider with OTLP exporter
func InitTracer(serviceName string) func() {
	ctx := context.Background()

	// Create OTLP exporter
	otlpExporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint("alloy:4317"), // FIX THIS
		otlptracegrpc.WithInsecure(),             // FIX THIS
	)
	if err != nil {
		log.Fatalf("Failed to create OTLP trace exporter: %v", err)
	}

	resources := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(serviceName),
	)

	// Use both exporters
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(otlpExporter),
		sdktrace.WithResource(resources),
		sdktrace.WithSampler(sdktrace.AlwaysSample()), //FIX THIS decide on sampling
	)

	otel.SetTracerProvider(provider)
	Tracer = otel.Tracer(serviceName)

	return func() {
		ctx := context.Background()
		if err := provider.Shutdown(ctx); err != nil {
			log.Printf("Error shutting down tracer provider: %v", err)
		}
	}
}

// InitMeterProvider sets up the OpenTelemetry meter provider with OTLP exporter
func InitMeterProvider(serviceName string) (func(), error) {
	ctx := context.Background()

	// Create OTLP exporter
	otlpExporter, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint("otel-collector:4317"), //"alloy:4317"),
		otlpmetricgrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP metric exporter: %w", err)
	}

	// Create metric reader with periodic export to the collector
	reader := metric.NewPeriodicReader(otlpExporter, metric.WithInterval(5*time.Second))

	// resources := resource.NewWithAttributes(
	// 	semconv.SchemaURL,
	// 	semconv.ServiceName("coffee-server"),
	// 	semconv.ServiceVersion("1.0.0"),
	// )

	// Create a new MeterProvider with the OTLP exporter
	meterProvider := metric.NewMeterProvider(
		metric.WithReader(reader),
		//metric.WithResource(resources),
	)

	// Set the global MeterProvider
	otel.SetMeterProvider(meterProvider)
	//meter = meterProvider.Meter("coffee-server")

	Meter = otel.Meter(serviceName)

	initDefaultMetrics()

	// Return a function to shut down the meter provider
	return func() {
		shutdownCtx := context.Background()
		if err := meterProvider.Shutdown(shutdownCtx); err != nil {
			fmt.Errorf("Error shutting down meter provider: %v", err)
		}
	}, nil
}

func init() {
	otel.SetTextMapPropagator(newPropagator())
}

// newPropagator creates a new composite text map propagator
func newPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

// initDefaultMetrics initializes default metrics (e.g. HTTP request counter)
// and registers them with the global meter provider.
func initDefaultMetrics() {
	var err error
	HttpRequestCounter, err = Meter.Int64Counter(
		"http_server_duration_count",
		otelmetric.WithDescription("Number of HTTP requests"),
		otelmetric.WithUnit("{request}"),
	)
	if err != nil {
		fmt.Errorf("failed to create HTTP request counter: %w", err)
	}
}
