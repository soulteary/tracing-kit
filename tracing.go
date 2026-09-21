package tracing

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

var (
	tracerProvider *sdktrace.TracerProvider
	tracer         trace.Tracer
	serviceName    string // Store service name for fallback

	// enabled records whether an exporter was actually configured. A no-op
	// provider is still a non-nil provider, so presence alone cannot answer
	// IsEnabled.
	enabled bool

	// Hooks for testing - allows injecting errors for testing error paths
	resourceNewFunc  = resource.New
	otlptraceNewFunc = otlptrace.New
)

// globalMu guards the package-level tracer state above. Without it InitTracer
// racing with GetTracer or IsEnabled is a data race; the existing tests never
// caught it because they run sequentially.
var globalMu sync.RWMutex

// InitTracer initializes the OpenTelemetry tracer with default settings.
//
// It is equivalent to InitTracerWithConfig with Insecure set, preserving the
// behaviour callers already depend on. New code should use
// InitTracerWithConfig: exporting traces over plaintext HTTP is only
// appropriate for a collector on loopback or a trusted local network.
//
// When otlpEndpoint is empty, tracing is disabled. The returned provider is a
// no-op provider rather than nil, so the usual
//
//	tp, err := InitTracer(...)
//	defer tp.Shutdown(ctx)
//
// does not panic; it used to return (nil, nil) and the deferred Shutdown
// dereferenced it.
func InitTracer(svcName, serviceVersion, otlpEndpoint string) (*sdktrace.TracerProvider, error) {
	return InitTracerWithConfig(Config{
		ServiceName:    svcName,
		ServiceVersion: serviceVersion,
		OTLPEndpoint:   otlpEndpoint,
		Insecure:       true,
		SampleRatio:    1,
	})
}

// InitTracerWithConfig initializes the OpenTelemetry tracer.
func InitTracerWithConfig(cfg Config) (*sdktrace.TracerProvider, error) {
	globalMu.Lock()
	defer globalMu.Unlock()

	serviceName = cfg.ServiceName

	// The disabled path is decided FIRST, before resource discovery.
	//
	// Resource discovery reads OTEL_RESOURCE_ATTRIBUTES and can fail on a
	// malformed value. Running it first meant that failure returned before
	// the no-op provider was installed, so reconfiguring a process to an
	// empty endpoint left the previous exporter active instead of disabling
	// tracing -- and a disabled tracer needs no export resource anyway.
	if cfg.OTLPEndpoint == "" {
		// No exporter configured: hand back a no-op provider so callers can
		// defer Shutdown unconditionally.
		tp := sdktrace.NewTracerProvider(
			sdktrace.WithSampler(sdktrace.NeverSample()),
		)

		// Retire the provider being replaced, and install the new one
		// globally like the configured path does.
		//
		// Returning early without installing left a previously configured
		// provider in place, so instrumentation using otel.Tracer kept
		// exporting through it. Replacing the global is not enough either:
		// a tracer handle obtained BEFORE reconfiguration -- the usual
		// package-level `var tracer = otel.Tracer("x")` -- still belongs to
		// the old SDK provider and goes on recording and exporting through
		// the old endpoint. Shutting it down stops that.
		retirePreviousProvider(tp)

		tracerProvider = tp
		tracer = tp.Tracer(serviceName)
		// Tracing is disabled: IsEnabled must say so, whatever the docs say
		// about the provider being non-nil.
		enabled = false
		return tp, nil
	}

	// Create resource with service information
	res, err := resourceNewFunc(context.Background(),
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.ServiceVersion),
		),
		resource.WithFromEnv(), // Automatically detect resource attributes from environment
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	opts := []otlptracehttp.Option{otlptracehttp.WithEndpoint(cfg.OTLPEndpoint)}
	if cfg.Insecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	} else if cfg.TLSConfig != nil {
		opts = append(opts, otlptracehttp.WithTLSClientConfig(cfg.TLSConfig))
	}
	if cfg.ExportTimeout > 0 {
		opts = append(opts, otlptracehttp.WithTimeout(cfg.ExportTimeout))
	}

	otlpExporter, err := otlptraceNewFunc(context.Background(), otlptracehttp.NewClient(opts...))
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
	}

	// Create tracer provider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(otlpExporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(cfg.sampler()),
	)

	// Set global tracer provider, retiring the one being replaced.
	retirePreviousProvider(tp)

	// Set global propagator
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	tracerProvider = tp
	tracer = tp.Tracer(serviceName)
	enabled = true

	return tp, nil
}

// retirePreviousProvider installs next as the global provider and shuts the
// previous SDK provider down.
//
// Callers hold tracer handles obtained from whatever provider was installed
// when they asked -- a package-level `var tracer = otel.Tracer("svc")` is the
// common shape -- and those handles keep their provider alive. Swapping the
// global alone therefore leaves the old pipeline recording and exporting to
// the old endpoint while IsEnabled reports the new state.
//
// Must be called with globalMu held.
func retirePreviousProvider(next *sdktrace.TracerProvider) {
	previous := tracerProvider

	otel.SetTracerProvider(next)

	if previous != nil && previous != next {
		// Bounded: a wedged exporter must not hang reconfiguration.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := previous.Shutdown(ctx); err != nil {
			log.Printf("[tracing] shutting down the replaced tracer provider: %v", err)
		}
	}
}

// Shutdown gracefully shuts down the tracer provider
func Shutdown(ctx context.Context) error {
	globalMu.RLock()
	tp := tracerProvider
	globalMu.RUnlock()

	if tp != nil {
		return tp.Shutdown(ctx)
	}
	return nil
}

// GetTracer returns the global tracer
func GetTracer() trace.Tracer {
	globalMu.RLock()
	t, name := tracer, serviceName
	globalMu.RUnlock()

	if t == nil {
		// Return noop tracer if not initialized
		if name == "" {
			name = "unknown-service"
		}
		return noop.NewTracerProvider().Tracer(name)
	}
	return t
}

// IsEnabled reports whether tracing is enabled, i.e. an OTLP endpoint was
// configured and an exporter installed.
//
// An empty OTLPEndpoint yields a no-op provider so callers can defer Shutdown
// unconditionally; that provider is non-nil, so its presence is not the
// question being asked here.
func IsEnabled() bool {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return enabled && tracerProvider != nil && tracer != nil
}
