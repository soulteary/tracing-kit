package tracingtest_test

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"

	tracing "github.com/soulteary/tracing-kit/v2"
	"github.com/soulteary/tracing-kit/v2/tracingtest"
)

// TestSetupMirrorsTheDocumentedExample follows the README's testing example
// verbatim: these helpers are documented API and have to keep working.
func TestSetupMirrorsTheDocumentedExample(t *testing.T) {
	tp, exporter := tracingtest.Setup(t)

	_, span := tracing.StartSpan(context.Background(), "test.operation")
	span.End()

	tracingtest.ForceFlush(tp)

	if len(exporter.GetSpans()) != 1 {
		t.Errorf("recorded %d spans, want 1", len(exporter.GetSpans()))
	}
	if !tracing.IsEnabled() {
		t.Error("IsEnabled() = false after Setup")
	}
}

// TestSetupCleansUpAfterItself: Setup registers its own cleanup, so a test
// that uses it needs no defer. That is why TestingT requires Cleanup.
func TestSetupCleansUpAfterItself(t *testing.T) {
	t.Run("inner", func(t *testing.T) {
		tracingtest.Setup(t)
		if !tracing.IsEnabled() {
			t.Fatal("IsEnabled() = false inside the test that called Setup")
		}
	})

	if tracing.IsEnabled() {
		t.Error("tracing is still installed after the test that called Setup returned")
	}
}

// TestSetupInstallsTheTraceContextPropagator: Inject and Extract read the
// global propagator, which propagates nothing until something sets it.
func TestSetupInstallsTheTraceContextPropagator(t *testing.T) {
	tracingtest.Setup(t)

	ctx, span := tracing.StartSpan(context.Background(), "op")
	defer span.End()

	headers := map[string]string{}
	tracing.InjectTraceContext(ctx, headers)

	if headers["traceparent"] == "" {
		t.Errorf("no traceparent injected; headers = %v", headers)
	}
}

// TestSetupWithNilT leaves cleanup to the caller.
func TestSetupWithNilT(t *testing.T) {
	tp, exporter := tracingtest.Setup(nil)
	defer func() {
		tracingtest.Shutdown(tp)
		tracingtest.Teardown()
	}()

	_, span := tracing.StartSpan(context.Background(), "op")
	span.End()
	tracingtest.ForceFlush(tp)

	if len(exporter.GetSpans()) != 1 {
		t.Errorf("recorded %d spans, want 1", len(exporter.GetSpans()))
	}
}

func TestTeardownRestoresTheNoopTracer(t *testing.T) {
	tp, _ := tracingtest.Setup(nil)
	tracingtest.Shutdown(tp)
	tracingtest.Teardown()

	if tracing.IsEnabled() {
		t.Error("IsEnabled() = true after Teardown")
	}
	if otel.GetTracerProvider() == nil {
		t.Error("Teardown left no global tracer provider; otel.Tracer would panic")
	}

	// Must not panic.
	_, span := tracing.StartSpan(context.Background(), "op")
	span.End()
}

// TestShutdownAndForceFlushTolerateNil: they run in cleanup, where the
// provider may never have been built.
func TestShutdownAndForceFlushTolerateNil(t *testing.T) {
	tracingtest.Shutdown(nil)
	tracingtest.ForceFlush(nil)
}
