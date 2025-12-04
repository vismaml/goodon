package goodon

import (
	"context"
	"net/http"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"
)

type RegisterFunc func(ctx context.Context, mux *runtime.ServeMux, endpoint string, opts []grpc.DialOption) error

// StartHTTPGateway starts a gateway that proxies requests to translate HTTP/JSON requests into gRPC calls.
// To start the gateway, call this function within a new goroutine and provide the gRPC functions to register.
//
// Example:
//
//	go func() {
//		if err := goodon.StartHTTPGateway(cfg.GRPCPort, cfg.HTTPPort,
//			annotatorpb.RegisterDocumentAnnotatorHandlerFromEndpoint,
//		); err != nil && err != http.ErrServerClosed {
//			zap.L().Error("HTTP gateway failed in <SERVICE NAME>", zap.Error(err))
//		}
//	}()
func StartHTTPGateway(grpcPort, httpPort string, registerFuncs ...RegisterFunc) error {
	ctx := context.Background()

	mux := runtime.NewServeMux(
		runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{
			MarshalOptions: protojson.MarshalOptions{
				EmitUnpopulated: false, // this option omits fields with zero values
			},
			UnmarshalOptions: protojson.UnmarshalOptions{
				DiscardUnknown: true, // this option ignores unknown fields in the incoming JSON
			},
		}),
	)
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	grpcEndpoint := "localhost:" + grpcPort

	for _, register := range registerFuncs {
		if err := register(ctx, mux, grpcEndpoint, opts); err != nil {
			return err
		}
	}

	server := &http.Server{
		Addr:         ":" + httpPort,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  30 * time.Second,
	}
	return server.ListenAndServe()
}
