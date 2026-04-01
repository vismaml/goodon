package goodon

import (
	"context"
	"net/http"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// healthHandler wraps an http.Handler with /healthz and /readyz endpoints.
// /healthz is a liveness probe that always returns 200.
// /readyz is a readiness probe that checks the upstream gRPC server is reachable
// and serving via the gRPC Health Checking Protocol.
func healthHandler(next http.Handler, grpcEndpoint string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		case "/readyz":
			if err := checkGRPCHealth(grpcEndpoint); err != nil {
				http.Error(w, err.Error(), http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// checkGRPCHealth dials the upstream gRPC server and calls the Health service.
func checkGRPCHealth(endpoint string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, err := grpc.NewClient(endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := grpc_health_v1.NewHealthClient(conn)
	_, err = client.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
	return err
}
