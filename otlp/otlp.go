// Package otlp initialises OpenTelemetry tracing with an OTLP/HTTP exporter.
//
// It lives in its own package so that importing the root package does not drag
// the OpenTelemetry SDK, gRPC and protobuf into binaries that never export a
// span. A library that only starts spans, or a service whose traces go
// somewhere else entirely, pays nothing for OTLP support existing; only
// importing this package links it in.
//
// The usual shape is one call at startup, and the root package everywhere
// else:
//
//	tp, err := otlp.InitTracerWithConfig(otlp.Config{
//		ServiceName: "my-service",
//		Endpoint:    "collector.internal:4318",
//		TLSConfig:   &tls.Config{MinVersion: tls.VersionTLS12},
//	})
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer tp.Shutdown(context.Background())
//
// [InitTracerWithConfig] is a composition of parts this package also exports
// on their own -- [Config.Resource], [NewExporter], [Config.Sampler] and
// tracing.Install -- so a provider that needs one piece done differently can
// be assembled without reimplementing the rest.
package otlp

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	tracing "github.com/soulteary/tracing-kit/v2"
)

// Hooks for testing - allows injecting errors for testing error paths.
var (
	resourceNewFunc  = resource.New
	otlptraceNewFunc = otlptrace.New
)

// InitTracer initialises tracing with default settings.
//
// It is equivalent to [InitTracerWithConfig] with Insecure set, preserving the
// behaviour callers already depend on. New code should use
// InitTracerWithConfig: exporting traces over plaintext HTTP is only
// appropriate for a collector on loopback or a trusted local network.
//
// When endpoint is empty, tracing is disabled. The returned provider records
// nothing rather than being nil, so the usual
//
//	tp, err := otlp.InitTracer(...)
//	defer tp.Shutdown(ctx)
//
// does not panic.
func InitTracer(svcName, serviceVersion, endpoint string) (*sdktrace.TracerProvider, error) {
	return InitTracerWithConfig(legacyConfig(svcName, serviceVersion, endpoint))
}

// legacyConfig is the Config [InitTracer] stands for. It is a function of its
// own so that the two defaults existing callers depend on -- plaintext export
// and sampling everything -- are stated once and can be asserted directly.
func legacyConfig(svcName, serviceVersion, endpoint string) Config {
	return Config{
		ServiceName:    svcName,
		ServiceVersion: serviceVersion,
		Endpoint:       endpoint,
		Insecure:       true,
		SampleRatio:    1,
	}
}

// InitTracerWithConfig initialises tracing and installs the resulting provider
// globally, with [tracing.Install] for a configured endpoint and
// [tracing.InstallDisabled] for an empty one.
//
// On error nothing is installed and the previously installed provider, if any,
// keeps running: a failed reconfiguration must not leave a process with no
// tracing at all and no way to tell.
func InitTracerWithConfig(cfg Config) (*sdktrace.TracerProvider, error) {
	// The disabled path is decided FIRST, before resource discovery.
	//
	// Resource discovery reads OTEL_RESOURCE_ATTRIBUTES and can fail on a
	// malformed value. Running it first meant that failure returned before
	// the recording-nothing provider was installed, so reconfiguring a
	// process to an empty endpoint left the previous exporter active instead
	// of disabling tracing -- and a disabled tracer needs no export resource
	// anyway.
	if cfg.Endpoint == "" {
		tp := sdktrace.NewTracerProvider(
			sdktrace.WithSampler(sdktrace.NeverSample()),
		)

		// Installed, not merely returned: leaving the previous provider in
		// place would keep instrumentation that reaches for otel.Tracer
		// exporting through it, and would keep a tracer handle taken before
		// the reconfiguration recording against the old endpoint.
		tracing.InstallDisabled(tp, cfg.ServiceName)
		return tp, nil
	}

	ctx := context.Background()

	res, err := cfg.Resource(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	exporter, err := NewExporter(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(cfg.Sampler()),
	)

	tracing.Install(tp, cfg.ServiceName)
	tracing.SetDefaultPropagator()

	return tp, nil
}

// NewExporter builds the OTLP/HTTP span exporter [InitTracerWithConfig] uses.
//
// Only cfg's transport fields are read -- Endpoint, Insecure, TLSConfig and
// ExportTimeout. Use it to put this exporter behind a provider you assemble
// yourself, then hand that provider to tracing.Install.
func NewExporter(ctx context.Context, cfg Config) (*otlptrace.Exporter, error) {
	opts := []otlptracehttp.Option{otlptracehttp.WithEndpoint(cfg.Endpoint)}
	if cfg.Insecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	} else if cfg.TLSConfig != nil {
		opts = append(opts, otlptracehttp.WithTLSClientConfig(cfg.TLSConfig))
	}
	if cfg.ExportTimeout > 0 {
		opts = append(opts, otlptracehttp.WithTimeout(cfg.ExportTimeout))
	}

	return otlptraceNewFunc(ctx, otlptracehttp.NewClient(opts...))
}
