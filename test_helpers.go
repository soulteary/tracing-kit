package tracing

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// SetupTestTracer sets up a tracer for testing
func SetupTestTracer(t *testing.T) (*sdktrace.TracerProvider, *tracetest.InMemoryExporter) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	// Set package-level variables to ensure GetTracer() works
	tracerProvider = tp
	tracer = tp.Tracer("test-service")
	serviceName = "test-service"

	return tp, exporter
}

// TeardownTestTracer cleans up the tracer
func TeardownTestTracer() {
	tracer = nil
	tracerProvider = nil
	serviceName = ""
	otel.SetTracerProvider(nil)
}

// ShutdownTracerProvider safely shuts down a tracer provider, ignoring errors in test cleanup
func ShutdownTracerProvider(tp *sdktrace.TracerProvider) {
	if tp != nil {
		_ = tp.Shutdown(context.Background())
	}
}

// ForceFlushTracerProvider safely flushes a tracer provider, ignoring errors in test cleanup
func ForceFlushTracerProvider(tp *sdktrace.TracerProvider) {
	if tp != nil {
		_ = tp.ForceFlush(context.Background())
	}
}
