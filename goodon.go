package goodon

import (
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/e-conomic/ctxtrace"
	"github.com/e-conomic/zapvml"
	grpc_zap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

type GoodonConfig struct {
	HostIP string `required:"true" split_words:"true"`
}

// StartWithDefaults starts the gRPC server with default settings including logging, and health checks.
func StartWithDefaults(server *grpc.Server) error {
	// Setup logging
	grpc_zap.ReplaceGrpcLogger(zapvml.Log)

	// Graceful stop
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		zap.L().Info("graceful shutdown")
		server.GracefulStop()
	}()

	// init
	lis, err := net.Listen("tcp", ":"+"50051")
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

// grpcOptions returns the default goodon gRPC server options with additional options
func grpcOptions(additionalOptions func() []grpc.ServerOption) []grpc.ServerOption {
	options := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(15 << 20),
		grpc.MaxSendMsgSize(15 << 20),
		grpc.ChainUnaryInterceptor(
			grpc_ctxtags.UnaryServerInterceptor(
				grpc_ctxtags.WithFieldExtractor(grpc_ctxtags.CodeGenRequestFieldExtractor),
			),
			ctxtrace.UnaryServerInterceptor(),
			grpc_zap.UnaryServerInterceptor(zapvml.Log, grpc_zap.WithLevels(zapvml.CodeToLevel)),
		),
		grpc.ChainStreamInterceptor(
			grpc_ctxtags.StreamServerInterceptor(
				grpc_ctxtags.WithFieldExtractor(grpc_ctxtags.CodeGenRequestFieldExtractor),
			),
			ctxtrace.StreamServerInterceptor(),
			grpc_zap.StreamServerInterceptor(zapvml.Log, grpc_zap.WithLevels(zapvml.CodeToLevel)),
		),
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
	}

	if additionalOptions != nil {
		options = append(options, additionalOptions()...)
	}

	return options
}

// NewGRPCServer creates a new gRPC server with the default gRPC options.
// The additionalOptions parameter is a function that returns additional server options
// and can be nil if no additional options are needed.
func NewGRPCServer(additionalOptions func() []grpc.ServerOption) *grpc.Server {
	opts := grpcOptions(additionalOptions)
	server := grpc.NewServer(opts...)
	return server
}
