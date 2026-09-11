package tracing

import (
	"crypto/tls"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Config configures tracer initialisation.
//
// The previous InitTracer hard-coded otlptracehttp.WithInsecure() and
// AlwaysSample(), with comments telling the reader to use TLS and a ratio
// sampler in production -- while providing no way to do either. Every
// deployment exported trace data (request paths, user identifiers, SQL,
// error detail) in cleartext and sampled 100% of it.
type Config struct {
	// ServiceName identifies the service in traces. Required.
	ServiceName string

	// ServiceVersion is reported as the service version attribute.
	ServiceVersion string

	// OTLPEndpoint is the collector's host:port. When empty, tracing is
	// disabled and InitTracer returns a no-op provider.
	OTLPEndpoint string

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

// sampler returns the sampler this configuration asks for.
//
// SampleNone maps to NeverSample rather than a zero ratio. Wrapping a
// zero-ratio sampler in ParentBased still RECORDS any span whose incoming
// parent carries the sampled bit, so a service that had explicitly opted out
// of sampling went on recording and exporting spans for every distributed
// request that reached it with a sampled parent.
func (c Config) sampler() sdktrace.Sampler {
	if c.SampleNone {
		return sdktrace.NeverSample()
	}
	return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(c.sampleRatio()))
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
