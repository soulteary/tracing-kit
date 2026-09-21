package otlp

import (
	"context"
	"crypto/tls"
	"time"

	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

// Config configures tracer initialisation.
//
// The original InitTracer hard-coded otlptracehttp.WithInsecure() and
// AlwaysSample(), with comments telling the reader to use TLS and a ratio
// sampler in production -- while providing no way to do either. Every
// deployment exported trace data (request paths, user identifiers, SQL,
// error detail) in cleartext and sampled 100% of it.
type Config struct {
	// ServiceName identifies the service in traces. Required.
	//
	// It names the tracer, so it is what appears as the instrumentation
	// scope. It is also the service.name resource attribute -- unless
	// OTEL_SERVICE_NAME is set, which by OpenTelemetry's documented
	// precedence replaces it in the resource the collector sees.
	ServiceName string

	// ServiceVersion is reported as the service version attribute.
	ServiceVersion string

	// Endpoint is the collector's host:port. When empty, tracing is disabled
	// and [InitTracerWithConfig] returns a provider that records nothing.
	Endpoint string

	// Insecure sends trace data over plaintext HTTP.
	//
	// Only appropriate for a collector reached over loopback or a trusted
	// local network. Traces carry request paths, user identifiers and error
	// detail, so exporting them in the clear across any wider network
	// discloses all of it.
	Insecure bool

	// TLSConfig is used when Insecure is false. Nil means the system defaults.
	TLSConfig *tls.Config

	// SampleRatio is the fraction of traces to record, in [0, 1].
	//
	// Zero means "use the default" (DefaultSampleRatio), not "sample nothing":
	// a zero value must not silently change sampling behaviour. Use
	// SampleNone to disable sampling. Values at or above 1 record everything.
	SampleRatio float64

	// SampleNone disables sampling entirely.
	SampleNone bool

	// ExportTimeout bounds a single export attempt. Zero uses the exporter
	// default.
	ExportTimeout time.Duration
}

// DefaultSampleRatio is the fraction of traces recorded when SampleRatio is
// left unset. Recording every trace on a busy service is a self-inflicted
// load problem for both the service and the collector.
const DefaultSampleRatio = 0.1

// Sampler returns the sampler this configuration asks for.
//
// SampleNone maps to NeverSample rather than a zero ratio. Wrapping a
// zero-ratio sampler in ParentBased still RECORDS any span whose incoming
// parent carries the sampled bit, so a service that had explicitly opted out
// of sampling went on recording and exporting spans for every distributed
// request that reached it with a sampled parent.
//
// It is exported so that a provider assembled by hand -- around a different
// exporter, say -- can sample the way [InitTracerWithConfig] does.
func (c Config) Sampler() sdktrace.Sampler {
	if c.SampleNone {
		return sdktrace.NeverSample()
	}
	return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(c.sampleRatio()))
}

// Resource builds the resource [InitTracerWithConfig] attaches to the
// provider: the configured service name and version, plus whatever
// OTEL_RESOURCE_ATTRIBUTES and the other standard environment variables add.
//
// It reads the environment, so it can fail on a malformed value; that is the
// only error InitTracerWithConfig reports before touching any global state.
func (c Config) Resource(ctx context.Context) (*resource.Resource, error) {
	return resourceNewFunc(ctx,
		resource.WithAttributes(
			semconv.ServiceName(c.ServiceName),
			semconv.ServiceVersion(c.ServiceVersion),
		),
		resource.WithFromEnv(), // Automatically detect resource attributes from environment
	)
}

// sampleRatio returns the effective ratio.
func (c Config) sampleRatio() float64 {
	if c.SampleNone {
		return 0
	}
	if c.SampleRatio <= 0 {
		return DefaultSampleRatio
	}
	if c.SampleRatio > 1 {
		return 1
	}
	return c.SampleRatio
}
