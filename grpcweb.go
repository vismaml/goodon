package goodon

import (
	"net/http"
	"strings"

	"github.com/improbable-eng/grpc-web/go/grpcweb"
	"google.golang.org/grpc"
)

// GRPCWebProxy wraps a gRPC server to handle gRPC-Web requests.
// It can be used standalone with any HTTP server or mux.
type GRPCWebProxy struct {
	wrapped *grpcweb.WrappedGrpcServer
}

// NewGRPCWebProxy creates a new gRPC-Web proxy wrapping the given gRPC server.
// The proxy allows all origins and all request headers by default.
func NewGRPCWebProxy(grpcServer *grpc.Server) *GRPCWebProxy {
	wrapped := grpcweb.WrapServer(grpcServer,
		grpcweb.WithOriginFunc(func(origin string) bool { return true }),
		grpcweb.WithAllowedRequestHeaders([]string{"*"}),
	)
	return &GRPCWebProxy{wrapped: wrapped}
}

// IsGRPCWebRequest returns true if the request is a gRPC-Web request
// (either by content-type or by the grpc-web library's own detection).
func (p *GRPCWebProxy) IsGRPCWebRequest(r *http.Request) bool {
	ct := r.Header.Get("Content-Type")
	return strings.HasPrefix(ct, "application/grpc-web") || p.wrapped.IsGrpcWebRequest(r)
}

// ServeHTTP implements http.Handler and forwards gRPC-Web requests to the wrapped gRPC server.
func (p *GRPCWebProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.wrapped.ServeHTTP(w, r)
}

// Handler returns an http.Handler that routes gRPC-Web requests to the proxy
// and falls back to the provided fallback handler for everything else.
//
// This makes it easy to compose with any existing HTTP handler/mux:
//
//	httpMux := http.NewServeMux()
//	httpMux.HandleFunc("/api/", apiHandler)
//
//	proxy := goodon.NewGRPCWebProxy(grpcServer)
//	http.ListenAndServe(":8080", proxy.Handler(httpMux))
func (p *GRPCWebProxy) Handler(fallback http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p.IsGRPCWebRequest(r) {
			p.wrapped.ServeHTTP(w, r)
			return
		}
		fallback.ServeHTTP(w, r)
	})
}
