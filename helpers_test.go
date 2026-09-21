package tracing

// Test-only plumbing for this package's own tests.
//
// It is deliberately a near-copy of what the tracingtest subpackage offers
// consumers: tracingtest imports this package, so an internal test here
// cannot import tracingtest back without an import cycle. tracingtest has its
// own tests.

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// testingT is the part of *testing.T setupTracer needs.
type testingT interface {
	Helper()
	Cleanup(func())
}

// setupTracer installs a provider recording into memory, and registers its
// teardown.
func setupTracer(t testingT) (*sdktrace.TracerProvider, *tracetest.InMemoryExporter) {
	t.Helper()

	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	Install(tp, "test-service")
	otel.SetTextMapPropagator(propagation.TraceContext{})

	t.Cleanup(func() {
		shutdownProvider(tp)
		Uninstall()
	})

	return tp, exporter
}

func shutdownProvider(tp *sdktrace.TracerProvider) {
	if tp != nil {
		_ = tp.Shutdown(context.Background())
	}
}

func forceFlush(tp *sdktrace.TracerProvider) {
	if tp != nil {
		_ = tp.ForceFlush(context.Background())
	}
}
