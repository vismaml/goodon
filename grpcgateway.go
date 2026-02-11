package goodon

import (
	"bytes"
	"context"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"
)

type RegisterFunc func(ctx context.Context, mux *runtime.ServeMux, endpoint string, opts []grpc.DialOption) error

// requestLogger is a middleware that logs the incoming request body.
func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			log.Printf("failed to read request body: %v", err)
			http.Error(w, "can't read body", http.StatusBadRequest)
			return
		}
		r.Body.Close()

		// Restore the body so it can be read again.
		r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		// Log the body.
		log.Printf("incoming request body: %s", bodyBytes)

		// Call the next handler.
		next.ServeHTTP(w, r)
	})
}

// customHeaderMatcher forwards the "x-request-id" and "vml-username" headers to the gRPC metadata.
func customHeaderMatcher(key string) (string, bool) {
	lowerKey := strings.ToLower(key)
	if lowerKey == "x-request-id" || lowerKey == "vml-username" {
		return lowerKey, true
	}
	return runtime.DefaultHeaderMatcher(key)
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
				EmitUnpopulated: false, // this option omits fields with zero values
			},
			UnmarshalOptions: protojson.UnmarshalOptions{
				DiscardUnknown: false, // this option ignores unknown fields in the incoming JSON
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
		Handler:      requestLogger(mux),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  30 * time.Second,
	}
	return server.ListenAndServe()
}
