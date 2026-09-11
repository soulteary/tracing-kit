package tracing

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// TestingT is the subset of *testing.T that SetupTestTracer accepts.
//
// These helpers are documented in the README as part of this package's public
// API, so they have to live in normal package code -- but importing "testing"
// from normal package code links it into every production binary depending on
// this library and registers its -test.* flags into flag.CommandLine.
// Declaring the two methods needed keeps *testing.T callers compiling exactly
// as before without that cost.
type TestingT interface {
	Helper()
	Cleanup(func())
}

// SetupTestTracer sets up a tracer for testing, returning the provider and an
// in-memory exporter holding the spans it records.
func SetupTestTracer(t TestingT) (*sdktrace.TracerProvider, *tracetest.InMemoryExporter) {
	if t != nil {
		t.Helper()
	}

	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	// Set package-level variables to ensure GetTracer() works
	globalMu.Lock()
	tracerProvider = tp
	tracer = tp.Tracer("test-service")
	serviceName = "test-service"
	enabled = true
	globalMu.Unlock()

	return tp, exporter
}

// TeardownTestTracer cleans up the tracer.
func TeardownTestTracer() {
	globalMu.Lock()
	tracer = nil
	tracerProvider = nil
	serviceName = ""
	enabled = false
	globalMu.Unlock()

	otel.SetTracerProvider(nil)
}

// ShutdownTracerProvider safely shuts down a tracer provider, ignoring errors
// in test cleanup.
func ShutdownTracerProvider(tp *sdktrace.TracerProvider) {
	if tp != nil {
		_ = tp.Shutdown(context.Background())
	}
}

// ForceFlushTracerProvider safely flushes a tracer provider, ignoring errors
// in test cleanup.
func ForceFlushTracerProvider(tp *sdktrace.TracerProvider) {
	if tp != nil {
		_ = tp.ForceFlush(context.Background())
	}
}
