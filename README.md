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
import (
    "crypto/tls"

    tracing "github.com/soulteary/tracing-kit"
)

func main() {
    tp, err := tracing.InitTracerWithConfig(tracing.Config{
        ServiceName:    "my-service",
        ServiceVersion: "v1.0.0",
        OTLPEndpoint:   "collector.internal:4318",
        TLSConfig:      &tls.Config{MinVersion: tls.VersionTLS12},
        SampleRatio:    0.1,
        ExportTimeout:  10 * time.Second,
    })
    if err != nil {
        log.Fatal(err)
    }
    defer func() {
        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()
        tracing.Shutdown(ctx)
    }()

    if tracing.IsEnabled() {
        log.Println("Tracing is enabled")
    }
}
```

With an empty `OTLPEndpoint`, tracing is disabled and a **no-op provider** is
returned — so `defer tp.Shutdown(ctx)` is always safe.

`InitTracer(name, version, endpoint)` is the older three-argument form. It keeps
its previous behaviour — **plaintext export, and every span sampled** — so
existing callers are unaffected. Prefer `InitTracerWithConfig` for anything that
leaves a trusted local network.

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
| `InitTracerWithConfig(cfg)` | Initialize from a `Config` — TLS, sampling, export timeout |
| `InitTracer(serviceName, version, endpoint)` | Three-argument form; plaintext export, samples everything |
| `Shutdown(ctx)` | Gracefully shut down the tracer provider |
| `GetTracer()` | The global tracer (no-op when not initialized) |
| `IsEnabled()` | Whether tracing is enabled |
| `ShutdownTracerProvider(tp)` | Shut down a specific provider |
| `ForceFlushTracerProvider(tp)` | Flush a specific provider's pending spans |

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

```go
type Config struct {
    ServiceName    string      // required
    ServiceVersion string
    OTLPEndpoint   string      // empty disables tracing
    Insecure       bool        // plaintext HTTP export
    TLSConfig      *tls.Config // used when Insecure is false; nil = system defaults
    SampleRatio    float64     // 0 means DefaultSampleRatio (0.1)
    SampleNone     bool        // explicit "sample nothing"
    ExportTimeout  time.Duration
}
```

| Field | Default | Notes |
|-------|---------|-------|
| `ServiceName` | — | required |
| `ServiceVersion` | empty | reported as the service version attribute |
| `OTLPEndpoint` | empty | empty disables tracing and returns a no-op provider |
| `Insecure` | `false` | see the warning below |
| `TLSConfig` | `nil` | system defaults when `Insecure` is false |
| `SampleRatio` | `DefaultSampleRatio` (0.1) | **zero means the default**, not "sample nothing" |
| `SampleNone` | `false` | the explicit way to sample nothing |
| `ExportTimeout` | SDK default | bound on an export attempt |

### Transport security

**`Insecure: true` sends trace data over plaintext HTTP.** Traces carry request
paths, user identifiers, SQL and error detail, so exporting them in the clear
across any network wider than loopback discloses all of it. Use it only for a
collector reached over loopback or a trusted local network:

```go
// Development
cfg := tracing.Config{ServiceName: "svc", OTLPEndpoint: "localhost:4318", Insecure: true}

// Production
cfg = tracing.Config{
    ServiceName:  "svc",
    OTLPEndpoint: "collector.internal:4318",
    TLSConfig:    &tls.Config{MinVersion: tls.VersionTLS12},
}
```

### Sampling

The sampler is **`ParentBased`**, so a trace that arrived sampled stays sampled
across your service — a ratio applies to traces this service starts, not to spans
it continues.

`SampleRatio`'s zero value means `DefaultSampleRatio` (10%), so leaving the field
unset cannot silently change behaviour. To record nothing, say so:

```go
cfg := tracing.Config{ServiceName: "svc", OTLPEndpoint: endpoint, SampleNone: true}
```

### Reading the endpoint from the environment

There is no built-in environment-variable handling; read them yourself:

```go
cfg := tracing.Config{
    ServiceName:    os.Getenv("OTEL_SERVICE_NAME"),
    ServiceVersion: buildVersion,
    OTLPEndpoint:   os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
    SampleRatio:    0.1,
}
```

## Test Coverage

Measured on this tree with `go test ./... -cover`: **99.1% of statements**.

```bash
go test ./... -coverprofile=coverage.out -covermode=atomic
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
    // Setup test tracer with in-memory exporter.
    // SetupTestTracer takes a tracing.TestingT (Helper + Cleanup), which
    // *testing.T satisfies, so a custom harness can pass its own.
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

## Upgrade Notes (v1.5.1)

Nothing to do. One import path changed inside `tracing.go`; no API, no
behaviour and no dependency moved.

- **The semantic conventions are `semconv/v1.43.0` (were `v1.21.0`).** That
  package is 22 spec releases newer and is the current one bundled in the
  `go.opentelemetry.io/otel` v1.46.0 this module already requires — so `go.mod`
  and `go.sum` are untouched. Nothing is downloaded that was not there before.
- **The exported resource is byte-for-byte what it was.** The two helpers this
  module calls, `ServiceName` and `ServiceVersion`, have the same signature in
  both versions and return the same keys, `service.name` and `service.version`.
  `InitTracer` never set a schema URL and still does not: the resource is built
  with `resource.WithAttributes` and `resource.WithFromEnv`, so `SchemaURL()` is
  `""` before and after. Your collector sees no difference.
- **Requirements said OpenTelemetry Go SDK v1.39.0+**; `go.mod` requires
  `go.opentelemetry.io/otel` v1.46.0, and `semconv/v1.43.0` first shipped in
  otel v1.45.0, so v1.39.0 could not have compiled this import anyway. The line
  now matches `go.mod`.

## Upgrade Notes (v1.5.0)

`InitTracer` keeps its signature **and its previous behaviour**, so existing
callers are unaffected. Everything here is additive, except one nil that became a
value and three test hooks that are no longer exported.

- **TLS and sampling are configurable.** They were hard-coded —
  `otlptracehttp.WithInsecure()` and `AlwaysSample()` — with comments telling the
  reader to change them in production and no way to do so. **Every deployment
  exported traces in cleartext and recorded 100% of them.**
  `InitTracerWithConfig` takes a `Config` with `Insecure`, `TLSConfig`,
  `SampleRatio`, `SampleNone` and `ExportTimeout`. If you read the old README and
  believed you had configured TLS or a ratio, you had not — move to
  `InitTracerWithConfig`.
- **`InitTracer` with no endpoint returns a no-op provider, not `(nil, nil)`.**
  The idiomatic `tp, err := InitTracer(...)` followed by
  `defer tp.Shutdown(ctx)` was a nil dereference whenever tracing was switched
  off. If you added a nil check for that, it is now unnecessary.
- **The package-level tracer state is synchronised.** `InitTracer` racing with
  `GetTracer` or `IsEnabled` was a data race, never caught because the tests run
  sequentially.
- **`SetResourceNewFunc`, `SetOtlptraceNewFunc` and `ResetHooks` are gone from the
  public API.** They lived in `test_helpers.go`, a normal source file importing
  `testing` — which linked the testing package into every production binary
  depending on this library, registered its `-test.*` flags into
  `flag.CommandLine`, and let anything in the process swap the trace exporter at
  runtime. The file is now `export_test.go`, compiled only during tests.
- **`SetupTestTracer` takes a `tracing.TestingT`** (`Helper` + `Cleanup`) rather
  than `*testing.T`. `*testing.T` satisfies it, so existing calls compile
  unchanged.
- **Requirements said Go 1.26**; `go.mod` requires `1.27.0`.

## Requirements

- **Go 1.27+** (`go.mod` declares `go 1.27.0`)
- OpenTelemetry Go SDK v1.46.0+ (`go.mod` requires `go.opentelemetry.io/otel` v1.46.0)

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.
