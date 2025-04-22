package goodon

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/e-conomic/ctxtrace"
	"github.com/e-conomic/ctxvml"
	"github.com/e-conomic/zapvml"
	grpc_zap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
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
	"go.uber.org/zap"
	"google.golang.org/grpc"
	api_health "google.golang.org/grpc/health/grpc_health_v1"
)

var (
	Tracer oteltrace.Tracer
	Meter  otelmetric.Meter
)

type GoodonConfig struct {
	GRPCPort  string  `required:"true" split_words:"true"`
	HostIp    string  `required:"true" split_words:"true"`
	TraceRate float64 `required:"true" split_words:"true"`
}

const (
	alloyGrpcPort         = "4317"
	otelCollectorGrpcPort = "4317"
)

func StartWithDefaults(cfg GoodonConfig, serviceName string, healthServ api_health.HealthServer) error {
	// Setup logging
	zap.ReplaceGlobals(zapvml.Log)
	grpc_zap.ReplaceGrpcLogger(zapvml.Log)

	// Setup Goodon
	shutdownTelemetry, err := StartTelemetryWithDefaults(serviceName, cfg.HostIp, cfg.TraceRate)
	defer shutdownTelemetry()
	if err != nil {
		return err
	}

	// Setup gRPC server
	opts := grpcOptions()
	server := grpc.NewServer(opts...)
	grpc_prometheus.EnableHandlingTimeHistogram()
	grpc_prometheus.Register(server)

	// Register: Health
	api_health.RegisterHealthServer(server, healthServ)

	// Graceful stop
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		zap.L().Info("graceful shutdown")
		server.GracefulStop()
	}()

	// init
	lis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		return err
	}
	res := server.Serve(lis)
	_ = zapvml.Log.Sync()
	return res
}

// StartTelemetryWihDefaults initializes OpenTelemetry with default settings
func StartTelemetryWithDefaults(serviceName string, backendIp string, traceFreq float64) (func(), error) {
	shutdownTracer, err := initTracer(serviceName, backendIp, traceFreq)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize tracer: %w", err)
	}

	shutdownMeter, err := initMeterProvider(serviceName, backendIp)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize meter provider: %w", err)
	}

	return func() {
		shutdownTracer()
		shutdownMeter()
	}, nil
}

// initTracer sets up the OpenTelemetry tracer provider with OTLP exporter
func initTracer(serviceName string, backendIp string, traceFreq float64) (func() error, error) {
	ctx := context.Background()

	// Create OTLP trace exporter
	otlpExporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(backendIp+":"+otelCollectorGrpcPort),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP trace exporter %w", err)
	}

	resources, err := newResources(ctx, serviceName)
	if err != nil {
		return nil, fmt.Errorf("failed to create rtrace esources: %w", err)
	}

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

// initMeterProvider sets up the OpenTelemetry meter provider with OTLP exporter
func initMeterProvider(serviceName string, backendIp string) (func() error, error) {
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

// grpcOptions returns the default goodon gRPC server options
func grpcOptions() []grpc.ServerOption {
	return []grpc.ServerOption{
		grpc.MaxRecvMsgSize(15 << 20),
		grpc.ChainUnaryInterceptor(
			grpc_prometheus.UnaryServerInterceptor,
			grpc_ctxtags.UnaryServerInterceptor(
				grpc_ctxtags.WithFieldExtractor(grpc_ctxtags.CodeGenRequestFieldExtractor),
			),
			ctxvml.UnaryServerInterceptor(),
			ctxtrace.UnaryServerInterceptor(),
			grpc_zap.UnaryServerInterceptor(zapvml.Log, grpc_zap.WithLevels(zapvml.CodeToLevel)),
		),
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
	}
}

// grpcWithAdditionalOptions returns the default goodon gRPC server options with additional options
func grpcWithAdditionalOptions(additionalOptions func() []grpc.ServerOption) []grpc.ServerOption {
	options := []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(
			grpc_ctxtags.UnaryServerInterceptor(
				grpc_ctxtags.WithFieldExtractor(grpc_ctxtags.CodeGenRequestFieldExtractor),
			),
			ctxvml.UnaryServerInterceptor(),
			ctxtrace.UnaryServerInterceptor(),
			grpc_zap.UnaryServerInterceptor(zapvml.Log, grpc_zap.WithLevels(zapvml.CodeToLevel)),
		),
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
	}

	if additionalOptions != nil {
		options = append(options, additionalOptions()...)
	}

	return options
}

func CreateGRPCServer() *grpc.Server {
	opts := grpcOptions()
	server := grpc.NewServer(opts...)
	grpc_prometheus.EnableHandlingTimeHistogram()
	grpc_prometheus.Register(server)
	return server
}
