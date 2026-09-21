package tracing

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestInstall(t *testing.T) {
	tp, exporter := setupTracer(t)

	if !IsEnabled() {
		t.Error("IsEnabled() = false after Install, want true")
	}
	if otel.GetTracerProvider() != trace.TracerProvider(tp) {
		t.Error("Install did not make the provider the global one; otel.Tracer would not reach it")
	}

	_, span := GetTracer().Start(context.Background(), "op")
	span.End()
	forceFlush(tp)

	if got := len(exporter.GetSpans()); got != 1 {
		t.Errorf("recorded %d spans, want 1", got)
	}
}

// TestInstallRetiresTheProviderItReplaces is the regression test for replacing
// only the GLOBAL provider. A tracer handle obtained before reconfiguration --
// the usual package-level `var tracer = otel.Tracer("x")` -- still belongs to
// the provider installed when it was taken, so it kept recording and exporting
// through the old endpoint while IsEnabled() reported the new state.
func TestInstallRetiresTheProviderItReplaces(t *testing.T) {
	t.Cleanup(Uninstall)

	previous, exporter := setupTracer(t)
	handle := otel.Tracer("held-before-reconfiguration")

	_, span := handle.Start(context.Background(), "before")
	span.End()
	if got := len(exporter.GetSpans()); got != 1 {
		t.Fatalf("setup recorded %d spans, want 1", got)
	}

	next := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.NeverSample()))
	t.Cleanup(func() { shutdownProvider(next) })
	Install(next, "svc")

	before := len(exporter.GetSpans())
	_, span = handle.Start(context.Background(), "after")
	span.End()
	if got := len(exporter.GetSpans()); got != before {
		t.Errorf("a tracer handle held across reconfiguration recorded %d more spans; %T is still live", got-before, previous)
	}
}

// TestInstallSameProviderTwiceKeepsItLive: Install shuts down the provider it
// replaces, so reinstalling the one already installed must not shut down the
// provider it is installing.
func TestInstallSameProviderTwiceKeepsItLive(t *testing.T) {
	tp, exporter := setupTracer(t)

	Install(tp, "test-service")

	_, span := GetTracer().Start(context.Background(), "after-reinstall")
	span.End()
	forceFlush(tp)

	if got := len(exporter.GetSpans()); got != 1 {
		t.Errorf("recorded %d spans after reinstalling the same provider, want 1; it was shut down on the way in", got)
	}
}

// TestInstallDisabledInstallsButReportsDisabled: a provider that records
// nothing is still a non-nil provider, so presence alone cannot answer
// IsEnabled -- and it still has to replace whatever was installed before.
func TestInstallDisabledInstallsButReportsDisabled(t *testing.T) {
	t.Cleanup(Uninstall)

	previous, _ := setupTracer(t)
	if otel.GetTracerProvider() != trace.TracerProvider(previous) {
		t.Fatal("setup did not install its provider globally")
	}

	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.NeverSample()))
	t.Cleanup(func() { shutdownProvider(tp) })
	InstallDisabled(tp, "svc")

	if IsEnabled() {
		t.Error("IsEnabled() = true after InstallDisabled, want false")
	}
	if otel.GetTracerProvider() == trace.TracerProvider(previous) {
		t.Error("the previous provider is still installed globally; otel.Tracer keeps exporting through it")
	}
	if otel.GetTracerProvider() != trace.TracerProvider(tp) {
		t.Error("InstallDisabled did not install its provider globally")
	}

	// The tracer still works, it just records nothing.
	_, span := GetTracer().Start(context.Background(), "op")
	span.End()
}

// TestInstallNilChangesNothing: Provider is an interface, so a nil
// *sdktrace.TracerProvider stored in one is not == nil, and taking a tracer
// from it panics. An observability library that takes the process down is
// worse than one that records nothing -- and a caller whose argument came out
// nil has said nothing about the provider already running, so that one keeps
// working.
func TestInstallNilChangesNothing(t *testing.T) {
	var handled []error
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		handled = append(handled, err)
	}))
	t.Cleanup(func() { otel.SetErrorHandler(otel.ErrorHandlerFunc(func(error) {})) })

	for name, p := range map[string]Provider{
		"untyped nil": nil,
		"typed nil":   (*sdktrace.TracerProvider)(nil),
	} {
		t.Run(name, func(t *testing.T) {
			Uninstall()
			handled = nil

			Install(p, "svc")

			if IsEnabled() {
				t.Error("IsEnabled() = true after installing a nil provider")
			}
			if len(handled) == 0 {
				t.Error("installing a nil provider was not reported to the OpenTelemetry error handler")
			}

			// GetTracer must still hand back something usable.
			_, span := GetTracer().Start(context.Background(), "op")
			span.End()

			// And a provider that was already working keeps working.
			tp, exporter := setupTracer(t)
			handled = nil

			Install(p, "svc")

			if !IsEnabled() {
				t.Error("installing a nil provider disabled the tracing that was working before it")
			}
			_, span = GetTracer().Start(context.Background(), "op")
			span.End()
			forceFlush(tp)
			if got := len(exporter.GetSpans()); got != 1 {
				t.Errorf("recorded %d spans after a nil install, want 1", got)
			}
			if len(handled) == 0 {
				t.Error("installing a nil provider was not reported to the OpenTelemetry error handler")
			}
		})
	}
}

func TestUninstall(t *testing.T) {
	tp, _ := setupTracer(t)

	Uninstall()

	if IsEnabled() {
		t.Error("IsEnabled() = true after Uninstall")
	}
	if otel.GetTracerProvider() == trace.TracerProvider(tp) {
		t.Error("the uninstalled provider is still the global one")
	}

	// Uninstall does not shut the provider down: the caller holds it and
	// decides when flushing is finished.
	if err := tp.ForceFlush(context.Background()); err != nil {
		t.Errorf("Uninstall shut the provider down: ForceFlush = %v", err)
	}

	_, span := GetTracer().Start(context.Background(), "op")
	span.End()
}

func TestShutdown_WithProvider(t *testing.T) {
	setupTracer(t)

	if err := Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown error = %v", err)
	}
}

func TestShutdown_WithoutProvider(t *testing.T) {
	Uninstall()

	if err := Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown with nothing installed = %v, want nil", err)
	}
}

func TestGetTracer_WithInstalledProvider(t *testing.T) {
	setupTracer(t)

	if GetTracer() == nil {
		t.Fatal("GetTracer returned nil")
	}
	if !IsEnabled() {
		t.Fatal("IsEnabled() = false after Install")
	}
}

func TestGetTracer_WithoutInstalledProvider(t *testing.T) {
	Uninstall()

	tr := GetTracer()
	if tr == nil {
		t.Fatal("GetTracer should return a no-op tracer when nothing is installed")
	}

	// Verify it works by starting a span. noop.Tracer is not exported, so
	// there is nothing to type-assert against.
	_, span := tr.Start(context.Background(), "test")
	span.End()
	if span.SpanContext().IsSampled() {
		t.Error("the fallback tracer recorded a sampled span")
	}
}

func TestGetTracer_WithServiceName(t *testing.T) {
	Uninstall()

	// A service name without a provider: the fallback tracer is named after
	// it rather than after "unknown-service".
	mu.Lock()
	serviceName = "my-service"
	mu.Unlock()
	t.Cleanup(Uninstall)

	_, span := GetTracer().Start(context.Background(), "test")
	span.End()
}

func TestGetTracer_WithoutServiceName(t *testing.T) {
	Uninstall()

	_, span := GetTracer().Start(context.Background(), "test")
	span.End()
}

func TestGetTracer_AfterShutdown(t *testing.T) {
	tp, _ := setupTracer(t)
	shutdownProvider(tp)

	if GetTracer() == nil {
		t.Fatal("GetTracer should return a tracer even after shutdown")
	}
}

func TestIsEnabled_WithoutInstalledProvider(t *testing.T) {
	Uninstall()

	if IsEnabled() {
		t.Fatal("IsEnabled() = true with nothing installed")
	}
}

func TestIsEnabled_WithPartialState(t *testing.T) {
	t.Cleanup(Uninstall)

	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	t.Cleanup(func() { shutdownProvider(tp) })

	Uninstall()
	mu.Lock()
	provider, tracer, enabled = tp, nil, true
	mu.Unlock()
	if IsEnabled() {
		t.Error("IsEnabled() = true with a nil tracer")
	}

	Uninstall()
	mu.Lock()
	provider, tracer, enabled = nil, tp.Tracer("test"), true
	mu.Unlock()
	if IsEnabled() {
		t.Error("IsEnabled() = true with a nil provider")
	}
}

// TestSameHandlesUncomparableProviders: comparing two interfaces holding an
// uncomparable type panics, and Install compares the provider it is given
// against the installed one.
func TestSameHandlesUncomparableProviders(t *testing.T) {
	t.Cleanup(Uninstall)

	a, b := uncomparableProvider{}, uncomparableProvider{}
	if same(a, b) {
		t.Error("same reported two uncomparable providers as the same")
	}

	Install(a, "svc")
	Install(b, "svc")

	if !IsEnabled() {
		t.Error("IsEnabled() = false after installing uncomparable providers")
	}
}

// uncomparableProvider has a field that makes == panic on two interfaces
// holding it.
type uncomparableProvider struct {
	_ []int
	noop.TracerProvider
}

func (uncomparableProvider) Shutdown(context.Context) error { return nil }

func TestShutdownReportsTheProviderError(t *testing.T) {
	t.Cleanup(Uninstall)

	want := errors.New("boom")
	Install(failingProvider{err: want}, "svc")

	if got := Shutdown(context.Background()); !errors.Is(got, want) {
		t.Errorf("Shutdown error = %v, want %v", got, want)
	}
}

type failingProvider struct {
	noop.TracerProvider
	err error
}

func (p failingProvider) Shutdown(context.Context) error { return p.err }

// TestInstallReportsAFailingRetirement: the provider being replaced is shut
// down on the way out, and that shutdown can fail -- a wedged exporter, a
// closed connection. It must not fail the install, and it must not be
// swallowed either.
func TestInstallReportsAFailingRetirement(t *testing.T) {
	t.Cleanup(Uninstall)

	var handled []error
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		handled = append(handled, err)
	}))
	t.Cleanup(func() { otel.SetErrorHandler(otel.ErrorHandlerFunc(func(error) {})) })

	want := errors.New("exporter is wedged")
	Install(failingProvider{err: want}, "svc")

	next := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.NeverSample()))
	t.Cleanup(func() { shutdownProvider(next) })
	Install(next, "svc")

	if !IsEnabled() {
		t.Error("a failing retirement failed the install")
	}
	if otel.GetTracerProvider() != trace.TracerProvider(next) {
		t.Error("the new provider was not installed")
	}
	if !errors.Is(errors.Join(handled...), want) {
		t.Errorf("errors reported = %v, want the retirement error %v", handled, want)
	}
}
