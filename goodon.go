package goodon

import (
	"context"
	"fmt"
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

const (
	alloyPort         = "4317"
	otelCollectorPort = "4317"
)

// StartTelemetryWihDefaults initializes OpenTelemetry with default settings
func StartTelemetryWithDefaults(serviceName string) (func(), error) {
	shutdownTracer, err := InitTracer(serviceName)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize tracer: %w", err)
	}

	shutdownMeter, err := InitMeterProvider(serviceName)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize meter provider: %w", err)
	}

	return func() {
		shutdownTracer()
		shutdownMeter()
	}, nil
}

// InitTracer sets up the OpenTelemetry tracer provider with OTLP exporter
func InitTracer(serviceName string) (func() error, error) {
	ctx := context.Background()

	// Create OTLP trace exporter
	otlpExporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint("alloy:"+alloyPort), // FIX THIS
		otlptracegrpc.WithInsecure(),                   // FIX THIS
	)
	if err != nil {
		return nil, fmt.Errorf("Failed to create OTLP trace exporter %w", err)
	}

	resources := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(serviceName),
	)

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(otlpExporter),
		sdktrace.WithResource(resources),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(0.7)),
	)

	otel.SetTracerProvider(provider)
	Tracer = otel.Tracer(serviceName)

	otel.SetTextMapPropagator(newPropagator())

	return func() error {
		ctx := context.Background()
		if err := provider.Shutdown(ctx); err != nil {
			return fmt.Errorf("Error shutting down tracer provider: %w", err)
		}
		return nil
	}, nil
}

// InitMeterProvider sets up the OpenTelemetry meter provider with OTLP exporter
func InitMeterProvider(serviceName string) (func() error, error) {
	ctx := context.Background()

	// Create OTLP exporter
	otlpExporter, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint("otel-collector:"+otelCollectorPort), //"alloy:4317"),
		otlpmetricgrpc.WithInsecure(),                                    // FIX THIS
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP metric exporter: %w", err)
	}

	// Create metric reader with periodic export to the collector
	reader := metric.NewPeriodicReader(otlpExporter, metric.WithInterval(60*time.Second))

	resources := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(serviceName),
	)

	// Create a new MeterProvider with the OTLP exporter
	meterProvider := metric.NewMeterProvider(
		metric.WithReader(reader),
		metric.WithResource(resources),
	)

	// Set the global MeterProvider
	otel.SetMeterProvider(meterProvider)
	//meter = meterProvider.Meter("coffee-server")

	Meter = otel.Meter(serviceName)

	err = initDefaultMetrics()
	if err != nil {
		return nil, fmt.Errorf("failed to init default metrics: %w", err)
	}

	// Return a function to shut down the meter provider
	return func() error {
		shutdownCtx := context.Background()
		if err := meterProvider.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("Error shutting down meter provider: %w", err)
		}
		return nil
	}, nil
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
func initDefaultMetrics() error {
	var err error
	HttpRequestCounter, err = Meter.Int64Counter(
		"http_server_duration_count",
		otelmetric.WithDescription("Number of HTTP requests"),
		otelmetric.WithUnit("{request}"),
	)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request counter: %w", err)

	}
	return nil
}
