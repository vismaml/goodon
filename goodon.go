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
	alloyGrpcPort         = "4317"
	otelCollectorGrpcPort = "4317"
)

// StartTelemetryWihDefaults initializes OpenTelemetry with default settings
func StartTelemetryWithDefaults(serviceName string, backendIp string, traceFreq float64) (func(), error) {
	shutdownTracer, err := InitTracer(serviceName, backendIp, traceFreq)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize tracer: %w", err)
	}

	shutdownMeter, err := InitMeterProvider(serviceName, backendIp)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize meter provider: %w", err)
	}

	return func() {
		shutdownTracer()
		shutdownMeter()
	}, nil
}

// InitTracer sets up the OpenTelemetry tracer provider with OTLP exporter
func InitTracer(serviceName string, backendIp string, traceFreq float64) (func() error, error) {
	ctx := context.Background()

	// Create OTLP trace exporter
	otlpExporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(backendIp+":"+otelCollectorGrpcPort),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP trace exporter %w", err)
	}

	resources := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(serviceName),
	)

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(otlpExporter),
		sdktrace.WithResource(resources),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(traceFreq)),
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

// InitMeterProvider sets up the OpenTelemetry meter provider with OTLP exporter
func InitMeterProvider(serviceName string, backendIp string) (func() error, error) {
	ctx := context.Background()

	// Create OTLP exporter
	otlpExporter, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint(backendIp+":"+otelCollectorGrpcPort),
		otlpmetricgrpc.WithInsecure(),
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

	Meter = otel.Meter(serviceName)

	if err != nil {
		return nil, fmt.Errorf("failed to init default metrics: %w", err)
	}

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
