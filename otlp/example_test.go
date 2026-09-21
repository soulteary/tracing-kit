package otlp_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	tracing "github.com/soulteary/tracing-kit/v2"
	"github.com/soulteary/tracing-kit/v2/otlp"
)

// Example is the whole of a service's tracing setup: one call at startup, and
// the root package everywhere else.
func Example() {
	tp, err := otlp.InitTracerWithConfig(otlp.Config{
		ServiceName:    "my-service",
		ServiceVersion: "v1.0.0",
		Endpoint:       "collector.internal:4318",
		TLSConfig:      &tls.Config{MinVersion: tls.VersionTLS12},
		SampleRatio:    0.1,
		ExportTimeout:  10 * time.Second,
	})
	if err != nil {
		fmt.Println("init:", err)
		return
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tp.Shutdown(ctx)
		tracing.Uninstall()
	}()

	fmt.Println("tracing enabled:", tracing.IsEnabled())
	// Output:
	// tracing enabled: true
}

// ExampleInitTracerWithConfig_disabled shows the switched-off case. An empty
// Endpoint disables tracing, and the returned provider is still a provider, so
// a deferred Shutdown is safe without a nil check.
func ExampleInitTracerWithConfig_disabled() {
	tp, err := otlp.InitTracerWithConfig(otlp.Config{ServiceName: "my-service"})
	if err != nil {
		fmt.Println("init:", err)
		return
	}
	defer func() {
		_ = tp.Shutdown(context.Background())
		tracing.Uninstall()
	}()

	// Instrumentation runs unchanged; it just records nothing.
	_, span := tracing.StartSpan(context.Background(), "process.request")
	span.End()

	fmt.Println("tracing enabled:", tracing.IsEnabled())
	// Output:
	// tracing enabled: false
}

// ExampleConfig_Sampler reads back the sampler a configuration asks for.
// SampleRatio's zero value is the default ratio, not "sample nothing" --
// leaving the field unset must not silently change behaviour.
func ExampleConfig_Sampler() {
	fmt.Println("unset: ", otlp.Config{}.Sampler().Description())
	fmt.Println("none:  ", otlp.Config{SampleNone: true}.Sampler().Description())
	fmt.Println("default ratio:", otlp.DefaultSampleRatio)
	// Output:
	// unset:  ParentBased{root:TraceIDRatioBased{0.1},remoteParentSampled:AlwaysOnSampler,remoteParentNotSampled:AlwaysOffSampler,localParentSampled:AlwaysOnSampler,localParentNotSampled:AlwaysOffSampler}
	// none:   AlwaysOffSampler
	// default ratio: 0.1
}
