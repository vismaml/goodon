package goodon

import (
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	"github.com/vismaml/ctxtrace"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
)

type GoodonConfig struct {
	HostIP string `required:"true" split_words:"true"`
}

// StartWithDefaults starts the gRPC server with default settings.
func StartWithDefaults(server *grpc.Server) error {
	// Graceful stop
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		log.Println("graceful shutdown")
		server.GracefulStop()
	}()

	lis, err := net.Listen("tcp", ":"+"50051")
	if err != nil {
		return err
	}
	res := server.Serve(lis)
	return res
}

// grpcOptions returns the default goodon gRPC server options.
// If the additionalOptions function is provided (non-nil), its returned options
// will be appended to the defaults.
func grpcOptions(additionalOptions ...grpc.ServerOption) []grpc.ServerOption {
	options := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(15 << 20),
		grpc.MaxSendMsgSize(15 << 20),
		grpc.ChainUnaryInterceptor(
			grpc_ctxtags.UnaryServerInterceptor(
				grpc_ctxtags.WithFieldExtractor(grpc_ctxtags.CodeGenRequestFieldExtractor),
			),
			ctxtrace.UnaryServerInterceptor(),
		),
		grpc.ChainStreamInterceptor(
			grpc_ctxtags.StreamServerInterceptor(
				grpc_ctxtags.WithFieldExtractor(grpc_ctxtags.CodeGenRequestFieldExtractor),
			),
			ctxtrace.StreamServerInterceptor(),
		),
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
	}

	if additionalOptions != nil {
		options = append(options, additionalOptions...)
	}

	return options
}

// NewGRPCServer creates a new gRPC server with the default gRPC options.
// The additionalOptions parameter is a function that returns additional server options
// and can be nil if no additional options are needed.
func NewGRPCServer(additionalOptions ...grpc.ServerOption) *grpc.Server {
	opts := grpcOptions(additionalOptions...)
	server := grpc.NewServer(opts...)
	return server
}
