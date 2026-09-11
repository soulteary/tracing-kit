package tracing

import (
	"context"
	"crypto/tls"
	"sync"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// TestSampleRatioZeroMeansDefault: a zero value must not silently change
// sampling behaviour. AlwaysSample() was hard-coded before, recording 100% of
// traces on every deployment with no way to change it.
func TestSampleRatioZeroMeansDefault(t *testing.T) {
	if got := (Config{}).sampleRatio(); got != DefaultSampleRatio {
		t.Errorf("zero SampleRatio = %v, want the default %v", got, DefaultSampleRatio)
	}
	if got := (Config{SampleNone: true}).sampleRatio(); got != 0 {
		t.Errorf("SampleNone ratio = %v, want 0", got)
	}
	if got := (Config{SampleRatio: 0.25}).sampleRatio(); got != 0.25 {
		t.Errorf("SampleRatio = %v, want 0.25", got)
	}
	if got := (Config{SampleRatio: 5}).sampleRatio(); got != 1 {
		t.Errorf("SampleRatio above 1 = %v, want it clamped to 1", got)
	}
	if got := (Config{SampleRatio: -1}).sampleRatio(); got != DefaultSampleRatio {
		t.Errorf("negative SampleRatio = %v, want the default", got)
	}
}

// TestTLSIsConfigurable: WithInsecure() was hard-coded with a comment telling
// the reader to use TLS in production, and no way to do so -- every deployment
// exported traces in cleartext.
func TestTLSIsConfigurable(t *testing.T) {
	TeardownTestTracer()
	defer TeardownTestTracer()

	tp, err := InitTracerWithConfig(Config{
		ServiceName:  "svc",
		OTLPEndpoint: "collector.internal:4318",
		TLSConfig:    &tls.Config{MinVersion: tls.VersionTLS12},
	})
	if err != nil {
		t.Fatalf("InitTracerWithConfig with TLS error = %v", err)
	}
	if tp == nil {
		t.Fatal("InitTracerWithConfig returned no provider")
	}
	ShutdownTracerProvider(tp)
}

// TestNoEndpointYieldsUsableProvider: (nil, nil) made the idiomatic
// "defer tp.Shutdown(ctx)" a nil dereference.
func TestNoEndpointYieldsUsableProvider(t *testing.T) {
	TeardownTestTracer()
	defer TeardownTestTracer()

	tp, err := InitTracerWithConfig(Config{ServiceName: "svc"})
	if err != nil {
		t.Fatalf("InitTracerWithConfig error = %v", err)
	}
	if tp == nil {
		t.Fatal("provider is nil; a deferred Shutdown would panic")
	}
	if err := tp.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown error = %v", err)
	}

	// The tracer still works, it just records nothing.
	_, span := GetTracer().Start(context.Background(), "op")
	span.End()
}

// TestGlobalStateIsRaceFree: the package-level tracer state was unsynchronised,
// so InitTracer racing with GetTracer was a data race. The existing tests never
// caught it because they run sequentially. Run this with -race.
func TestGlobalStateIsRaceFree(t *testing.T) {
	TeardownTestTracer()
	defer TeardownTestTracer()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = GetTracer()
				_ = IsEnabled()
			}
		}()
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				tp, err := InitTracerWithConfig(Config{ServiceName: "svc"})
				if err != nil {
					t.Error(err)
					return
				}
				ShutdownTracerProvider(tp)
			}
		}()
	}
	wg.Wait()
}

// --- Codex review follow-ups (PR #2) ---

// TestEmptyEndpointIsReportedDisabled is the regression test for the
// no-op-provider path. A no-op provider is still a non-nil provider, so
// IsEnabled reported true even though the method and the Config docs both say
// an empty OTLPEndpoint disables tracing.
func TestEmptyEndpointIsReportedDisabled(t *testing.T) {
	t.Cleanup(TeardownTestTracer)

	tp, err := InitTracerWithConfig(Config{ServiceName: "svc", OTLPEndpoint: ""})
	if err != nil {
		t.Fatalf("InitTracerWithConfig error = %v", err)
	}
	if tp == nil {
		t.Fatal("InitTracerWithConfig returned a nil provider; deferring Shutdown must be safe")
	}
	if IsEnabled() {
		t.Error("IsEnabled() = true with an empty OTLPEndpoint, want false")
	}
}

// TestEmptyEndpointReplacesTheGlobalProvider: reconfiguring a process with an
// empty endpoint used to return before otel.SetTracerProvider, leaving the
// previously configured provider installed -- so instrumentation reaching for
// otel.Tracer kept exporting through it.
func TestEmptyEndpointReplacesTheGlobalProvider(t *testing.T) {
	t.Cleanup(TeardownTestTracer)

	// Stand in for "a provider was already configured".
	previous, _ := SetupTestTracer(t)
	if otel.GetTracerProvider() != trace.TracerProvider(previous) {
		t.Fatal("setup did not install its provider globally")
	}

	tp, err := InitTracerWithConfig(Config{ServiceName: "svc", OTLPEndpoint: ""})
	if err != nil {
		t.Fatalf("InitTracerWithConfig error = %v", err)
	}
	if otel.GetTracerProvider() == trace.TracerProvider(previous) {
		t.Error("the previously configured provider is still installed globally; otel.Tracer keeps exporting through it")
	}
	if otel.GetTracerProvider() != trace.TracerProvider(tp) {
		t.Error("the no-op provider was not installed globally")
	}
}

// TestSampleNoneOverridesASampledParent is the regression test for SampleNone
// being implemented as a zero ratio. ParentBased records any span whose
// incoming parent carries the sampled bit, so a service that explicitly opted
// out went on recording spans for every distributed request that reached it
// already sampled.
func TestSampleNoneOverridesASampledParent(t *testing.T) {
	sampler := Config{SampleNone: true}.sampler()

	traceID, _ := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	spanID, _ := trace.SpanIDFromHex("00f067aa0ba902b7")
	sampledParent := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled, // the parent IS sampled
		Remote:     true,
	}))

	got := sampler.ShouldSample(sdktrace.SamplingParameters{
		ParentContext: sampledParent,
		TraceID:       traceID,
		Name:          "child",
	})
	if got.Decision != sdktrace.Drop {
		t.Errorf("SampleNone decision = %v with a sampled parent, want Drop", got.Decision)
	}

	// The ratio path still defers to the parent.
	ratio := Config{SampleRatio: 0.5}.sampler()
	if d := ratio.ShouldSample(sdktrace.SamplingParameters{
		ParentContext: sampledParent,
		TraceID:       traceID,
		Name:          "child",
	}).Decision; d != sdktrace.RecordAndSample {
		t.Errorf("ratio sampler decision = %v with a sampled parent, want RecordAndSample", d)
	}
}

// TestDocumentedTestHelpersAreImportable mirrors the README's testing example
// verbatim: these four helpers are documented public API and must not become
// test-only.
func TestDocumentedTestHelpersAreImportable(t *testing.T) {
	tp, exporter := SetupTestTracer(t)
	defer func() {
		ShutdownTracerProvider(tp)
		TeardownTestTracer()
	}()

	_, span := GetTracer().Start(context.Background(), "example")
	span.End()

	ForceFlushTracerProvider(tp)

	if len(exporter.GetSpans()) != 1 {
		t.Errorf("recorded %d spans, want 1", len(exporter.GetSpans()))
	}
}
