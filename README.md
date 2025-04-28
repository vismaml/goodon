# goodon

Goodon is a Go utility library that simplifies setting up gRPC services with integrated OpenTelemetry observability.

## Features

- Simple API for configuring OpenTelemetry components
- Pre-configured OTLP exporters for sending telemetry data via gRPC
- Unified setup for both traces and metrics
- Proper OTel shutdown handling for clean resource management
- Automatic tracing for gRPC requests
- Graceful gRPC server shutdown handling

## Installation

```go
go get github.com/vismaml/goodon
```
