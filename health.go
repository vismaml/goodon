package goodon

import (
	"context"
	"log"
	"net/http"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// withHealthCheck wraps an http.Handler with /healthz and /readyz endpoints.
// /healthz is a liveness probe that always returns 200.
// /readyz is a readiness probe that checks the upstream gRPC server is reachable
// and serving via the gRPC Health Checking Protocol.
func withHealthCheck(next http.Handler, grpcEndpoint string) http.Handler {
	conn, err := grpc.NewClient(grpcEndpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("failed to create health check client: %v", err)
	}
	healthClient := grpc_health_v1.NewHealthClient(conn)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			resp, err := checkGRPCHealth(r.Context(), healthClient)
			if err != nil {
				http.Error(w, err.Error(), http.StatusServiceUnavailable)
				return
			}
			if resp.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
				http.Error(w, resp.GetStatus().String(), http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(resp.GetStatus().String()))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// checkGRPCHealth calls the gRPC Health Checking Protocol on the upstream server.
func checkGRPCHealth(ctx context.Context, client grpc_health_v1.HealthClient) (*grpc_health_v1.HealthCheckResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return client.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
}
