// Package tracingtest sets up an in-memory tracer for testing code that uses
// tracing-kit.
//
// It lives in its own package because it needs the SDK's tracetest exporter.
// In the root package those helpers linked tracetest into every production
// binary that depended on this library; here, only test binaries that import
// tracingtest pay for it.
//
//	func TestMyTracedFunction(t *testing.T) {
//		tp, exporter := tracingtest.Setup(t)
//
//		doTracedWork(context.Background())
//
//		tracingtest.ForceFlush(tp)
//		if len(exporter.GetSpans()) == 0 {
//			t.Fatal("expected at least one span")
//		}
//	}
//
// [Setup] registers its own cleanup, so nothing has to be deferred by hand.
package tracingtest

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	tracing "github.com/soulteary/tracing-kit/v2"
)

// ServiceName is the name [Setup] installs the test tracer under.
const ServiceName = "test-service"

// TestingT is the subset of *testing.T that [Setup] accepts.
//
// Declaring the two methods needed keeps *testing.T callers compiling exactly
// as before, while leaving a custom harness free to pass its own.
type TestingT interface {
	Helper()
	Cleanup(func())
}

// Setup installs a tracer that records spans into memory and returns it
// together with the exporter holding them.
//
// It also installs the W3C trace context propagator, so [tracing.InjectTraceContext]
// and [tracing.ExtractTraceContext] work under test.
//
// When t is non-nil, Setup registers a cleanup that shuts the provider down
// and calls [Teardown]; a test that uses it needs no defer of its own. Pass
// nil to manage that by hand.
func Setup(t TestingT) (*sdktrace.TracerProvider, *tracetest.InMemoryExporter) {
	if t != nil {
		t.Helper()
	}

	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
	)

	tracing.Install(tp, ServiceName)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	if t != nil {
		t.Cleanup(func() {
			Shutdown(tp)
			Teardown()
		})
	}

	return tp, exporter
}

// Teardown drops the installed tracer, returning the process to the state it
// was in before [Setup]: a no-op tracer, and tracing reported disabled.
func Teardown() {
	tracing.Uninstall()
}

// Shutdown shuts a tracer provider down, ignoring the error in test cleanup.
func Shutdown(tp *sdktrace.TracerProvider) {
	if tp != nil {
		_ = tp.Shutdown(context.Background())
	}
}

// ForceFlush flushes a tracer provider's pending spans, ignoring the error in
// test cleanup. Call it before reading the exporter when the provider batches.
func ForceFlush(tp *sdktrace.TracerProvider) {
	if tp != nil {
		_ = tp.ForceFlush(context.Background())
	}
}
