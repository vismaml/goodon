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
	Tracer oteltrace.Tracer
	Meter  otelmetric.Meter
)

const (
	alloyGRPCPort         = "4317"
	otelCollectorGRPCPort = "4317"
)

// StartTelemetryWithDefaults initializes OpenTelemetry with default settings for traces and metrics.
func StartTelemetryWithDefaults(serviceName string, collectorIP string) (func(), error) {
	shutdownTracer, err := initTracer(serviceName, collectorIP, 0.05)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize tracer: %w", err)
	}

	shutdownMeter, err := initMeterProvider(serviceName, collectorIP)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize meter provider: %w", err)
	}

	return func() {
		shutdownTracer()
		shutdownMeter()
	}, nil
}

// initTracer sets up the OpenTelemetry tracer provider with OTLP exporter
func initTracer(serviceName string, collectorIP string, traceFreqency float64) (func() error, error) {
	ctx := context.Background()

	// Create OTLP trace exporter
	otlpExporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(collectorIP+":"+otelCollectorGRPCPort),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP trace exporter %w", err)
	}

	resources, err := newResources(ctx, serviceName)
	if err != nil {
		return nil, fmt.Errorf("failed to create trace resources: %w", err)
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(otlpExporter),
		sdktrace.WithResource(resources),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(traceFreqency)),
	)

	otel.SetTracerProvider(provider)
	Tracer = otel.Tracer(serviceName)

	otel.SetTextMapPropagator(newPropagator())

	return func() error {
		ctx := context.Background()
		if err := provider.Shutdown(ctx); err != nil {
			return fmt.Errorf("error shutting down tracer provider: %w", err)
		}
		return nil
	}, nil
}

// initMeterProvider sets up the OpenTelemetry meter provider with OTLP exporter
func initMeterProvider(serviceName string, collectorIP string) (func() error, error) {
	ctx := context.Background()

	// Create OTLP exporter
	otlpExporter, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint(collectorIP+":"+otelCollectorGRPCPort),
		otlpmetricgrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP metric exporter: %w", err)
	}

	// Create metric reader with periodic export to the collector
	reader := metric.NewPeriodicReader(otlpExporter, metric.WithInterval(60*time.Second))

	resources, err := newResources(ctx, serviceName)
	if err != nil {
		return nil, fmt.Errorf("failed to create metric resources: %w", err)
	}

	// Create a new MeterProvider with the OTLP exporter
	meterProvider := metric.NewMeterProvider(
		metric.WithReader(reader),
		metric.WithResource(resources),
	)

	// Set the global MeterProvider
	otel.SetMeterProvider(meterProvider)

	Meter = otel.Meter(serviceName)

	// Return a function to shut down the meter provider
	return func() error {
		shutdownCtx := context.Background()
		if err := meterProvider.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("error shutting down meter provider: %w", err)
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

// newResources creates a new resource with host and container information
func newResources(ctx context.Context, serviceName string) (*resource.Resource, error) {
	return resource.New(
		ctx,
		resource.WithHost(),      // Host info
		resource.WithContainer(), // Container info
		resource.WithAttributes( // Manual attributes using semconv
			semconv.ServiceName(serviceName),
		),
	)
}
