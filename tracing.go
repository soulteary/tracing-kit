package tracing

import (
	"context"
	"fmt"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

var (
	tracerProvider *sdktrace.TracerProvider
	tracer         trace.Tracer
	serviceName    string // Store service name for fallback

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

	if cfg.OTLPEndpoint == "" {
		// No exporter configured: hand back a no-op provider so callers can
		// defer Shutdown unconditionally.
		tp := sdktrace.NewTracerProvider(
			sdktrace.WithResource(res),
			sdktrace.WithSampler(sdktrace.NeverSample()),
		)
		tracerProvider = tp
		tracer = tp.Tracer(serviceName)
		return tp, nil
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
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.sampleRatio()))),
	)

	// Set global tracer provider
	otel.SetTracerProvider(tp)

	// Set global propagator
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	tracerProvider = tp
	tracer = tp.Tracer(serviceName)

	return tp, nil
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

// IsEnabled returns whether tracing is enabled
func IsEnabled() bool {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return tracerProvider != nil && tracer != nil
}
