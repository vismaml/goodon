package goodon

import (
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/e-conomic/ctxtrace"
	"github.com/e-conomic/ctxvml"
	"github.com/e-conomic/zapvml"
	grpc_zap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	api_health "google.golang.org/grpc/health/grpc_health_v1"
)

type GoodonConfig struct {
	GRPCPort           string  `required:"true" split_words:"true"`
	HostIP             string  `required:"true" split_words:"true"`
	TraceRate          float64 `required:"true" split_words:"true"`
	GRPCMaxEdgeMsgSize int     `required:"true" split_words:"true"`
}

// StartWithDefaults starts the gRPC server with default settings including logging, and health checks.
func StartWithDefaults(cfg GoodonConfig, serviceName string, server *grpc.Server, healthServ api_health.HealthServer) error {
	// Setup logging
	zap.ReplaceGlobals(zapvml.Log)
	grpc_zap.ReplaceGrpcLogger(zapvml.Log)

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
	err = zapvml.Log.Sync()
	if err != nil {
		zap.L().Error("failed to sync zap logger", zap.Error(err))
	}
	return res
}

// grpcOptions returns the default goodon gRPC server options
func grpcOptions(maxMsgSize int) []grpc.ServerOption {
	return []grpc.ServerOption{
		grpc.MaxRecvMsgSize(maxMsgSize),
		grpc.MaxSendMsgSize(maxMsgSize),
		grpc.ChainUnaryInterceptor(
			grpc_ctxtags.UnaryServerInterceptor(
				grpc_ctxtags.WithFieldExtractor(grpc_ctxtags.CodeGenRequestFieldExtractor),
			),
			ctxvml.UnaryServerInterceptor(),
			ctxtrace.UnaryServerInterceptor(),
			grpc_zap.UnaryServerInterceptor(zapvml.Log, grpc_zap.WithLevels(zapvml.CodeToLevel)),
		),
		grpc.ChainStreamInterceptor(
			grpc_ctxtags.StreamServerInterceptor(
				grpc_ctxtags.WithFieldExtractor(grpc_ctxtags.CodeGenRequestFieldExtractor),
			),
			ctxvml.StreamServerInterceptor(),
			ctxtrace.StreamServerInterceptor(),
			grpc_zap.StreamServerInterceptor(zapvml.Log, grpc_zap.WithLevels(zapvml.CodeToLevel)),
		),
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
	}
}

// NewGRPCServer creates a new gRPC server with the default gRPC options.
// Takes a maxMsgSize parameter to set the maximum message size.
func NewGRPCServer(maxMsgSize int) *grpc.Server {
	opts := grpcOptions(maxMsgSize)
	server := grpc.NewServer(opts...)
	return server
}
