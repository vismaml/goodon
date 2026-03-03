package goodon

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"
)

type RegisterFunc func(ctx context.Context, mux *runtime.ServeMux, endpoint string, opts []grpc.DialOption) error

// customHeaderMatcher forwards the "x-request-id" and "vml-username" headers to the gRPC metadata.
func customHeaderMatcher(key string) (string, bool) {
	lowerKey := strings.ToLower(key)
	if lowerKey == "x-request-id" || lowerKey == "vml-username" {
		return lowerKey, true
	}
	return runtime.DefaultHeaderMatcher(key)
}

// allowCORS allows Cross-Origin Resource Sharing.
func allowCORS(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		h.ServeHTTP(w, r)
	})
}

// StartHTTPGateway starts a gateway that proxies requests to translate HTTP/JSON requests into gRPC calls.
// To start the gateway, call this function within a new goroutine and provide the gRPC functions to register.
//
// Example:
//
//	go func() {
//		if err := goodon.StartHTTPGateway(cfg.GRPCPort, cfg.HTTPPort,
//			pb.RegisterHandlerFromEndpoint,
//		); err != nil && err != http.ErrServerClosed {
//			log.Fatalf("failed to start HTTP gateway: %v", err)
//		}
//	}()
func StartHTTPGateway(grpcPort, httpPort string, registerFuncs ...RegisterFunc) error {
	ctx := context.Background()

	mux := runtime.NewServeMux(
		runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{
			MarshalOptions: protojson.MarshalOptions{
				UseProtoNames:   false,
				EmitUnpopulated: false, // this option omits fields with zero values
			},
			UnmarshalOptions: protojson.UnmarshalOptions{
				DiscardUnknown: true, // this option ignores unknown fields in the incoming JSON
			},
		}),
		runtime.WithIncomingHeaderMatcher(customHeaderMatcher),
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
		Handler:      allowCORS(mux),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  30 * time.Second,
	}
	return server.ListenAndServe()
}

// StartGRPCGatewayWithWeb starts a gateway that proxies both HTTP/JSON and gRPC-Web requests to gRPC calls.
// It requires the gRPC server instance to wrap it for gRPC-Web support.
// To start the gateway, call this function within a new goroutine.
//
// Example:
//
//	go func() {
//		if err := goodon.StartGRPCGatewayWithWeb(grpcServer, cfg.GRPCPort, cfg.HTTPPort,
//			pb.RegisterHandlerFromEndpoint,
//		); err != nil && err != http.ErrServerClosed {
//			log.Fatalf("failed to start HTTP gateway with gRPC-Web: %v", err)
//		}
//	}()
func StartGRPCGatewayWithWeb(grpcServer *grpc.Server, grpcPort, httpPort string, registerFuncs ...RegisterFunc) error {
	ctx := context.Background()

	// gRPC-Gateway mux for HTTP/JSON to gRPC proxying
	gatewayMux := runtime.NewServeMux(
		runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{
			MarshalOptions: protojson.MarshalOptions{
				UseProtoNames:   false,
				EmitUnpopulated: false,
			},
			UnmarshalOptions: protojson.UnmarshalOptions{
				DiscardUnknown: true,
			},
		}),
		runtime.WithIncomingHeaderMatcher(customHeaderMatcher),
	)

	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	grpcEndpoint := "localhost:" + grpcPort

	for _, register := range registerFuncs {
		if err := register(ctx, gatewayMux, grpcEndpoint, opts); err != nil {
			return err
		}
	}

	// gRPC-Web proxy
	grpcWebProxy := NewGRPCWebProxy(grpcServer)

	// Combined handler that routes gRPC-Web requests to the gRPC-Web proxy,
	// and all other requests to the gRPC-Gateway mux.
	combinedHandler := grpcWebProxy.Handler(gatewayMux)

	server := &http.Server{
		Addr:         ":" + httpPort,
		Handler:      allowCORS(combinedHandler),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	return server.ListenAndServe()
}
