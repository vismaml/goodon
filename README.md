# goodon

Goodon is a Go utility library for building production-ready gRPC services. It provides opinionated defaults for server setup, HTTP/JSON and gRPC-Web gateways, health checks, and OpenTelemetry observability.

## Installation

```go
go get github.com/vismaml/goodon
```

## Features

### gRPC Server

Create a gRPC server with sensible defaults (15 MB message limits, context tracing interceptors, OpenTelemetry instrumentation) and graceful shutdown handling.

```go
server := goodon.NewGRPCServer()

// Or with additional options:
server := goodon.NewGRPCServer(grpc.MaxRecvMsgSize(30 << 20))

// Start with graceful shutdown on SIGINT/SIGTERM:
if err := goodon.StartWithDefaults(server); err != nil {
    log.Fatal(err)
}
```

### HTTP/JSON Gateway (gRPC-Gateway)

Expose your gRPC services as a REST/JSON API via [grpc-gateway](https://github.com/grpc-ecosystem/grpc-gateway). Includes custom header forwarding (`x-request-id`, `vml-username`), rate-limit header passthrough, and proper HTTP 413 mapping for oversized messages.

```go
go func() {
    if err := goodon.StartHTTPGateway(cfg.GRPCPort, cfg.HTTPPort,
        pb.RegisterHandlerFromEndpoint,
    ); err != nil && err != http.ErrServerClosed {
        log.Fatalf("failed to start HTTP gateway: %v", err)
    }
}()
```

### gRPC-Web Gateway

Serve both HTTP/JSON and gRPC-Web clients from a single port. Useful for browser-based gRPC-Web clients alongside traditional REST consumers.

```go
go func() {
    if err := goodon.StartGRPCGatewayWithWeb(grpcServer, cfg.GRPCPort, cfg.HTTPPort,
        pb.RegisterHandlerFromEndpoint,
    ); err != nil && err != http.ErrServerClosed {
        log.Fatalf("failed to start HTTP gateway with gRPC-Web: %v", err)
    }
}()
```

The `GRPCWebProxy` can also be used standalone with any HTTP handler:

```go
proxy := goodon.NewGRPCWebProxy(grpcServer)
http.ListenAndServe(":8080", proxy.Handler(myHTTPHandler))
```

### Health Checks

Both gateway functions automatically expose health check endpoints for Kubernetes probes:

- `/healthz` — liveness probe, returns `200` if the gateway process is running.
- `/readyz` — readiness probe, checks the upstream gRPC server via the [gRPC Health Checking Protocol](https://github.com/grpc/grpc/blob/master/doc/health-checking.md). Returns `503` if the upstream is unreachable.

Your gRPC server must register the health service for `/readyz` to work:

```go
import "google.golang.org/grpc/health"
import "google.golang.org/grpc/health/grpc_health_v1"

healthServer := health.NewServer()
grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)
```

Kubernetes probe configuration:

```yaml
livenessProbe:
  httpGet:
    path: /healthz
    port: 8080
readinessProbe:
  httpGet:
    path: /readyz
    port: 8080
```

### OpenTelemetry

Initialize traces and metrics with OTLP gRPC exporters in one call. Sets up global tracer and meter providers with proper shutdown handling.

```go
shutdown, err := goodon.StartTelemetryWithDefaults("my-service", cfg.HostIP)
if err != nil {
    log.Fatal(err)
}
defer shutdown()

// Use the global tracer and meter:
ctx, span := goodon.Tracer.Start(ctx, "operation")
defer span.End()
```
