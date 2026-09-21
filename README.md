# tracing-kit

[![Go Reference](https://pkg.go.dev/badge/github.com/soulteary/tracing-kit/v2.svg)](https://pkg.go.dev/github.com/soulteary/tracing-kit/v2)
[![Go Report Card](.github/goreportcard.svg)](.github/goreportcard-report.md)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![codecov](https://codecov.io/gh/soulteary/tracing-kit/graph/badge.svg)](https://codecov.io/gh/soulteary/tracing-kit)

[中文文档](README_CN.md)

A lightweight Go library for OpenTelemetry distributed tracing: span helpers,
trace-context propagation over plain string maps, and one-call tracer setup
with OTLP export.

## Layout

| Package | Depends on | Use it for |
|---------|------------|------------|
| `github.com/soulteary/tracing-kit/v2` | the OpenTelemetry **API** only | starting spans, propagating context, installing a provider |
| `.../v2/otlp` | + the OpenTelemetry SDK, the OTLP/HTTP exporter, gRPC, protobuf | initialising tracing at application startup |
| `.../v2/tracingtest` | + the SDK's `tracetest` exporter | testing traced code |

The root package does not import the SDK and does not import an exporter. That
is the split OpenTelemetry itself asks for: instrumentation depends on the API,
and the application chooses the SDK and the exporter. A library that only
starts spans links neither.

The numbers, for a program that imports only the root package — measured
against v1.5.1, where the exporter was in the root package:

| | v1.5.1 | v2.0.0 |
|---|---:|---:|
| binary | 18,095,419 B | 7,319,801 B (**−59.5%**) |
| linked packages outside the standard library | 191 | 42 |
| modules contributing to the build | 22 | 8 |
| `// indirect` requirements in your `go.mod` | 21 | 7 |
| modules in your `go.sum` | 28 | 13 |

Importing `.../v2/otlp` costs exactly what v1.5.1 cost. You pay for the
exporter when you use it.

## Installation

```bash
go get github.com/soulteary/tracing-kit/v2
```

## Quick Start

### Initialize Tracer

Once, at application startup:

```go
import (
    "crypto/tls"
    "time"

    tracing "github.com/soulteary/tracing-kit/v2"
    "github.com/soulteary/tracing-kit/v2/otlp"
)

func main() {
    tp, err := otlp.InitTracerWithConfig(otlp.Config{
        ServiceName:    "my-service",
        ServiceVersion: "v1.0.0",
        Endpoint:       "collector.internal:4318",
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
        tp.Shutdown(ctx)
    }()

    if tracing.IsEnabled() {
        log.Println("Tracing is enabled")
    }
}
```

With an empty `Endpoint`, tracing is disabled and the returned provider
**records nothing** rather than being nil — so `defer tp.Shutdown(ctx)` is
always safe.

`otlp.InitTracer(name, version, endpoint)` is the older three-argument form. It
keeps its previous behaviour — **plaintext export, and every span sampled** —
so existing callers are unaffected. Prefer `InitTracerWithConfig` for anything
that leaves a trusted local network.

### Bring your own exporter

`tracing.Install` takes an interface — `Tracer` plus `Shutdown` — so any
provider installs the same way, and nothing forces the `otlp` subpackage on
you:

```go
exp, _ := stdouttrace.New()
tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exp))

tracing.Install(tp, "my-service")
tracing.SetDefaultPropagator()
defer tracing.Uninstall()
```

`SetDefaultPropagator` is a separate call on purpose. OpenTelemetry's global
propagator carries nothing until something sets it, and a service that exports
no spans of its own still has to pass an incoming trace context to the next
hop — so propagation is worth switching on even where tracing is off.

### Create and Manage Spans

Everything below needs the root package only, so it is what a library writes:

```go
import (
    tracing "github.com/soulteary/tracing-kit/v2"
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

Until something is installed, `GetTracer` hands back a no-op tracer, so code
written this way runs unchanged in a process that never configures tracing.

### Trace Context Propagation

```go
import tracing "github.com/soulteary/tracing-kit/v2"

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
```

### HTTP Middleware Example

```go
import (
    tracing "github.com/soulteary/tracing-kit/v2"
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

### Root package — `github.com/soulteary/tracing-kit/v2`

Span operations:

| Function | Description |
|----------|-------------|
| `StartSpan(ctx, name, opts...)` | Start a new span |
| `SetSpanAttributes(span, attrs)` | Set string attributes on a span |
| `SetSpanAttributesFromMap(span, attrs)` | Set mixed-type attributes on a span |
| `RecordError(span, err)` | Record an error and set error status; a nil error is ignored |
| `SetSpanStatus(span, code, description)` | Set the span status |
| `GetSpanFromContext(ctx)` | Retrieve span from context |

Context propagation:

| Function | Description |
|----------|-------------|
| `SetDefaultPropagator()` | Install the W3C trace context + baggage propagator |
| `ExtractTraceContext(ctx, headers)` | Extract trace context from headers |
| `InjectTraceContext(ctx, headers)` | Inject trace context into headers |

The installed provider:

| Function | Description |
|----------|-------------|
| `Install(p, serviceName)` | Install a `Provider` process-wide; retires the one it replaces |
| `InstallDisabled(p, serviceName)` | The same, but `IsEnabled` reports false |
| `Uninstall()` | Drop the installed provider without shutting it down |
| `Shutdown(ctx)` | Gracefully shut down the installed provider |
| `GetTracer()` | The installed tracer (no-op when nothing is installed) |
| `IsEnabled()` | Whether a provider expected to export is installed |

`Provider` is `trace.TracerProvider` plus `Shutdown(ctx) error`.
`*sdktrace.TracerProvider` satisfies it.

### `.../v2/otlp`

| Function | Description |
|----------|-------------|
| `InitTracerWithConfig(cfg)` | Build the exporter and provider from a `Config`, and install them |
| `InitTracer(serviceName, version, endpoint)` | Three-argument form; plaintext export, samples everything |
| `NewExporter(ctx, cfg)` | Just the OTLP/HTTP exporter, for a provider you assemble |
| `Config.Sampler()` | The sampler a `Config` asks for |
| `Config.Resource(ctx)` | The resource a `Config` asks for |

### `.../v2/tracingtest`

| Function | Description |
|----------|-------------|
| `Setup(t)` | Install an in-memory tracer; registers its own cleanup |
| `Teardown()` | Drop it again |
| `Shutdown(tp)` | Shut a provider down, ignoring the error |
| `ForceFlush(tp)` | Flush a provider's pending spans, ignoring the error |

## Configuration

```go
type Config struct {
    ServiceName    string      // required
    ServiceVersion string
    Endpoint       string      // empty disables tracing
    Insecure       bool        // plaintext HTTP export
    TLSConfig      *tls.Config // used when Insecure is false; nil = system defaults
    SampleRatio    float64     // 0 means DefaultSampleRatio (0.1)
    SampleNone     bool        // explicit "sample nothing"
    ExportTimeout  time.Duration
}
```

| Field | Default | Notes |
|-------|---------|-------|
| `ServiceName` | — | required; `OTEL_SERVICE_NAME` overrides it in the exported resource |
| `ServiceVersion` | empty | reported as the service version attribute |
| `Endpoint` | empty | empty disables tracing and returns a provider that records nothing |
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
cfg := otlp.Config{ServiceName: "svc", Endpoint: "localhost:4318", Insecure: true}

// Production
cfg = otlp.Config{
    ServiceName: "svc",
    Endpoint:    "collector.internal:4318",
    TLSConfig:   &tls.Config{MinVersion: tls.VersionTLS12},
}
```

### Sampling

The sampler is **`ParentBased`**, so a trace that arrived sampled stays sampled
across your service — a ratio applies to traces this service starts, not to spans
it continues.

`SampleRatio`'s zero value means `DefaultSampleRatio` (10%), so leaving the field
unset cannot silently change behaviour. To record nothing, say so:

```go
cfg := otlp.Config{ServiceName: "svc", Endpoint: endpoint, SampleNone: true}
```

### Reading the endpoint from the environment

There is no built-in environment-variable handling; read them yourself:

```go
cfg := otlp.Config{
    ServiceName:    os.Getenv("OTEL_SERVICE_NAME"),
    ServiceVersion: buildVersion,
    Endpoint:       os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
    SampleRatio:    0.1,
}
```

`OTEL_SERVICE_NAME` and `OTEL_RESOURCE_ATTRIBUTES` are read by the SDK's own
resource detection, which runs after `ServiceName` is applied and therefore
wins in the resource your collector sees. `ServiceName` still names the tracer,
which is what appears as the instrumentation scope.

## Test Coverage

Measured on this tree with `go test -race ./... -covermode=atomic`: **100% of
statements**, in all three packages.

```bash
go test ./... -coverprofile=coverage.out -covermode=atomic
go tool cover -func=coverage.out
```

## Testing Support

```go
import (
    "testing"

    tracing "github.com/soulteary/tracing-kit/v2"
    "github.com/soulteary/tracing-kit/v2/tracingtest"
)

func TestMyTracedFunction(t *testing.T) {
    // Setup a tracer with an in-memory exporter. Setup registers its own
    // cleanup, so nothing has to be deferred.
    //
    // It takes a tracingtest.TestingT (Helper + Cleanup), which *testing.T
    // satisfies, so a custom harness can pass its own.
    tp, exporter := tracingtest.Setup(t)

    // Run your traced code
    ctx, span := tracing.StartSpan(context.Background(), "test.operation")
    span.End()

    // Flush and verify spans
    tracingtest.ForceFlush(tp)
    spans := exporter.GetSpans()

    if len(spans) == 0 {
        t.Fatal("Expected at least one span")
    }
}
```

## Upgrade Notes (v2.0.0)

The import path changes for everyone, and the exporter and the test helpers
move out of the root package. See [CHANGELOG.md](CHANGELOG.md) for the full
detail, including what it buys.

```go
// before
import tracing "github.com/soulteary/tracing-kit"

tp, err := tracing.InitTracerWithConfig(tracing.Config{
    ServiceName:  "svc",
    OTLPEndpoint: endpoint,
})

// after
import (
    tracing "github.com/soulteary/tracing-kit/v2"
    "github.com/soulteary/tracing-kit/v2/otlp"
)

tp, err := otlp.InitTracerWithConfig(otlp.Config{
    ServiceName: "svc",
    Endpoint:    endpoint,
})
```

| v1.5.1 | v2.0.0 |
|---|---|
| `tracing.InitTracer` | `otlp.InitTracer` |
| `tracing.InitTracerWithConfig` | `otlp.InitTracerWithConfig` |
| `tracing.Config` | `otlp.Config` |
| `tracing.Config.OTLPEndpoint` | `otlp.Config.Endpoint` |
| `tracing.DefaultSampleRatio` | `otlp.DefaultSampleRatio` |
| `tracing.SetupTestTracer` | `tracingtest.Setup` |
| `tracing.TeardownTestTracer` | `tracingtest.Teardown` |
| `tracing.ShutdownTracerProvider` | `tracingtest.Shutdown` |
| `tracing.ForceFlushTracerProvider` | `tracingtest.ForceFlush` |
| `tracing.TestingT` | `tracingtest.TestingT` |

Everything else in the root package — `StartSpan`, the attribute and status
helpers, `ExtractTraceContext`, `InjectTraceContext`, `GetTracer`, `IsEnabled`
and `Shutdown` — keeps its name, signature and behaviour.

There are no deprecated shims. A shim for the moved initialiser would have to
import the OTLP exporter, which relinks gRPC and protobuf into every consumer
and gives back the entire benefit of moving it.

## Requirements

- **Go 1.27+** (`go.mod` declares `go 1.27.0`)
- OpenTelemetry Go SDK v1.46.0+ (`go.mod` requires `go.opentelemetry.io/otel` v1.46.0)

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.
