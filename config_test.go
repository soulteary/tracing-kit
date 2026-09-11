package tracing

import (
	"context"
	"crypto/tls"
	"sync"
	"testing"
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
