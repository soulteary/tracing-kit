# tracing-kit

[![Go Reference](https://pkg.go.dev/badge/github.com/soulteary/tracing-kit.svg)](https://pkg.go.dev/github.com/soulteary/tracing-kit)
[![Go Report Card](.github/goreportcard.svg)](.github/goreportcard-report.md)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![codecov](https://codecov.io/gh/soulteary/tracing-kit/graph/badge.svg)](https://codecov.io/gh/soulteary/tracing-kit)

[中文文档](README_CN.md)

A lightweight Go library for OpenTelemetry distributed tracing. This toolkit provides simple and easy-to-use APIs for trace context propagation, span management, and tracer initialization with OTLP export support.

## Features

- **Tracer Initialization** - Easy setup of OpenTelemetry tracer with OTLP HTTP exporter
- **Span Management** - Simple APIs for creating, configuring, and managing spans
- **Context Propagation** - Extract and inject trace context for distributed tracing
- **Attribute Support** - Set span attributes with type-safe methods
- **Error Recording** - Record errors with automatic status setting
- **Test Helpers** - Utilities for testing tracing code with in-memory exporters

## Installation

```bash
go get github.com/soulteary/tracing-kit
```

## Quick Start

### Initialize Tracer

```go
import tracing "github.com/soulteary/tracing-kit"

func main() {
    // Initialize tracer with OTLP endpoint
    tp, err := tracing.InitTracer("my-service", "v1.0.0", "localhost:4318")
    if err != nil {
        log.Fatal(err)
    }
    defer func() {
        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()
        tracing.Shutdown(ctx)
    }()

    // Check if tracing is enabled
    if tracing.IsEnabled() {
        log.Println("Tracing is enabled")
    }
}
```

### Create and Manage Spans

```go
import (
    tracing "github.com/soulteary/tracing-kit"
    "go.opentelemetry.io/otel/codes"
    "go.opentelemetry.io/otel/trace"
)

func processRequest(ctx context.Context) error {
    // Start a new span
    ctx, span := tracing.StartSpan(ctx, "process.request")
    defer span.End()

    // Set string attributes
    tracing.SetSpanAttributes(span, map[string]string{
        "request.id":   "12345",
        "request.type": "api",
    })

    // Set mixed type attributes
    tracing.SetSpanAttributesFromMap(span, map[string]interface{}{
        "user.id":       42,
        "request.size":  int64(1024),
        "response.time": 0.125,
        "cached":        true,
    })

    // Simulate some work
    if err := doWork(ctx); err != nil {
        // Record error on span
        tracing.RecordError(span, err)
        return err
    }

    // Set success status
    tracing.SetSpanStatus(span, codes.Ok, "request processed successfully")
    return nil
}

func doWork(ctx context.Context) error {
    // Create a child span
    ctx, span := tracing.StartSpan(ctx, "do.work", 
        trace.WithSpanKind(trace.SpanKindInternal))
    defer span.End()

    // Get span from context
    currentSpan := tracing.GetSpanFromContext(ctx)
    currentSpan.AddEvent("work started")

    // ... do actual work ...

    return nil
}
```

### Trace Context Propagation

```go
import tracing "github.com/soulteary/tracing-kit"

// Extract trace context from incoming request headers
func handleIncomingRequest(headers map[string]string) {
    ctx := tracing.ExtractTraceContext(context.Background(), headers)
    
    // Continue with extracted context
    ctx, span := tracing.StartSpan(ctx, "handle.request")
    defer span.End()
    
    // Process request...
}

// Inject trace context into outgoing request headers
func makeOutgoingRequest(ctx context.Context) {
    headers := make(map[string]string)
    tracing.InjectTraceContext(ctx, headers)
    
    // Use headers for outgoing HTTP request
    // req.Header.Set("traceparent", headers["traceparent"])
}

// Round-trip example for distributed tracing
func propagateTrace(ctx context.Context) {
    // Service A: Inject context
    headers := make(map[string]string)
    tracing.InjectTraceContext(ctx, headers)
    
    // ... send request to Service B ...
    
    // Service B: Extract context and continue trace
    ctx = tracing.ExtractTraceContext(context.Background(), headers)
    ctx, span := tracing.StartSpan(ctx, "service.b.operation")
    defer span.End()
}
```

### HTTP Middleware Example

```go
import (
    tracing "github.com/soulteary/tracing-kit"
    "net/http"
)

func TracingMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Extract trace context from headers
        headers := make(map[string]string)
        for k, v := range r.Header {
            if len(v) > 0 {
                headers[k] = v[0]
            }
        }
        ctx := tracing.ExtractTraceContext(r.Context(), headers)
        
        // Start span for this request
        ctx, span := tracing.StartSpan(ctx, r.Method+" "+r.URL.Path)
        defer span.End()
        
        // Set request attributes
        tracing.SetSpanAttributes(span, map[string]string{
            "http.method": r.Method,
            "http.url":    r.URL.String(),
            "http.host":   r.Host,
        })
        
        // Continue with traced context
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

## API Reference

### Tracer Initialization

| Function | Description |
|----------|-------------|
| `InitTracer(serviceName, version, endpoint)` | Initialize tracer with OTLP HTTP exporter |
| `Shutdown(ctx)` | Gracefully shutdown the tracer provider |
| `GetTracer()` | Get the global tracer (returns noop if not initialized) |
| `IsEnabled()` | Check if tracing is enabled |

### Span Operations

| Function | Description |
|----------|-------------|
| `StartSpan(ctx, name, opts...)` | Start a new span |
| `SetSpanAttributes(span, attrs)` | Set string attributes on a span |
| `SetSpanAttributesFromMap(span, attrs)` | Set mixed-type attributes on a span |
| `RecordError(span, err)` | Record an error and set error status |
| `SetSpanStatus(span, code, description)` | Set the span status |
| `GetSpanFromContext(ctx)` | Retrieve span from context |

### Context Propagation

| Function | Description |
|----------|-------------|
| `ExtractTraceContext(ctx, headers)` | Extract trace context from headers |
| `InjectTraceContext(ctx, headers)` | Inject trace context into headers |

## Configuration

The tracer can be configured through environment variables or programmatically:

| Environment Variable | Description | Default |
|---------------------|-------------|---------|
| `OTEL_SERVICE_NAME` | Service name (overridden by parameter) | - |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP endpoint (use parameter instead) | - |

Example with environment detection:

```go
tp, err := tracing.InitTracer(
    "my-service",
    "v1.0.0",
    os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
)
```

## Test Coverage

This project maintains 100% test coverage:

| File | Coverage |
|------|----------|
| tracing.go | 100% |
| span.go | 100% |
| propagation.go | 100% |
| test_helpers.go | 100% |
| **Total** | **100%** |

Run tests with coverage:

```bash
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

## Testing Support

The library provides test helpers for unit testing:

```go
import (
    tracing "github.com/soulteary/tracing-kit"
    "testing"
)

func TestMyTracedFunction(t *testing.T) {
    // Setup test tracer with in-memory exporter
    tp, exporter := tracing.SetupTestTracer(t)
    defer func() {
        tracing.ShutdownTracerProvider(tp)
        tracing.TeardownTestTracer()
    }()

    // Run your traced code
    ctx := context.Background()
    ctx, span := tracing.StartSpan(ctx, "test.operation")
    span.End()

    // Flush and verify spans
    tracing.ForceFlushTracerProvider(tp)
    spans := exporter.GetSpans()
    
    if len(spans) == 0 {
        t.Fatal("Expected at least one span")
    }
}
```

## Requirements

- Go 1.26 or later
- OpenTelemetry Go SDK v1.39.0+

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.
