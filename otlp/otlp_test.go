package otlp

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"

	tracing "github.com/soulteary/tracing-kit/v2"
	"github.com/soulteary/tracing-kit/v2/tracingtest"
)

func TestInitTracer_WithEndpoint(t *testing.T) {
	t.Cleanup(tracing.Uninstall)

	tp, err := InitTracer("test-service", "v1.0.0", "http://localhost:4318")
	if err != nil {
		t.Fatalf("InitTracer failed: %v", err)
	}
	if tp == nil {
		t.Fatal("TracerProvider should not be nil")
	}
	t.Cleanup(func() { tracingtest.Shutdown(tp) })

	if !tracing.IsEnabled() {
		t.Error("IsEnabled() = false after InitTracer with an endpoint")
	}
	if otel.GetTracerProvider() == nil {
		t.Fatal("Global tracer provider should be set")
	}
}

// TestInitTracerKeepsItsPreviousBehaviour: the three-argument form is the one
// existing callers depend on, so it must keep exporting in cleartext and
// sampling everything. Tightening either would silently drop their traces --
// or, for TLS, fail against a collector that only speaks plaintext.
func TestInitTracerKeepsItsPreviousBehaviour(t *testing.T) {
	cfg := legacyConfig("svc", "v1", "localhost:4318")

	if !cfg.Insecure {
		t.Error("InitTracer must stay plaintext; existing callers have no TLS collector")
	}
	if got := cfg.sampleRatio(); got != 1 {
		t.Errorf("InitTracer sample ratio = %v, want 1 (everything)", got)
	}
	if cfg.SampleNone {
		t.Error("InitTracer must not disable sampling")
	}
	if cfg.ServiceName != "svc" || cfg.ServiceVersion != "v1" || cfg.Endpoint != "localhost:4318" {
		t.Errorf("InitTracer arguments landed wrong: %+v", cfg)
	}
}

func TestInitTracer_WithoutEndpoint(t *testing.T) {
	t.Cleanup(tracing.Uninstall)

	// With no endpoint, tracing is disabled -- but a non-nil provider is
	// returned so the usual "tp, err := InitTracer(...); defer tp.Shutdown(ctx)"
	// does not panic on a nil dereference, which (nil, nil) guaranteed.
	tp, err := InitTracer("test-service", "v1.0.0", "")
	if err != nil {
		t.Fatalf("InitTracer should not return error with empty endpoint: %v", err)
	}
	if tp == nil {
		t.Fatal("TracerProvider should record nothing, not be nil, when the endpoint is empty")
	}
	if err := tp.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown on the records-nothing provider error = %v", err)
	}
}

func TestInitTracer_InvalidEndpoint(t *testing.T) {
	t.Cleanup(tracing.Uninstall)

	// otlptracehttp.NewClient is lenient, so a malformed endpoint is accepted
	// here and fails later, on export. Either outcome is acceptable; what is
	// not acceptable is a panic.
	tp, err := InitTracer("test-service", "v1.0.0", "invalid-url-format")
	if err != nil {
		t.Logf("InitTracer with invalid URL returned error (expected): %v", err)
		return
	}
	tracingtest.Shutdown(tp)
}

func TestInitTracer_MultipleSequential(t *testing.T) {
	t.Cleanup(tracing.Uninstall)

	for i := 0; i < 2; i++ {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("InitTracer panicked: %v", r)
				}
			}()
			tp, err := InitTracer("test-service", "v1.0.0", "http://localhost:4318")
			if err != nil {
				t.Errorf("InitTracer error = %v", err)
				return
			}
			tracingtest.Shutdown(tp)
		}()
	}
}

func TestInitTracer_GlobalPropagator(t *testing.T) {
	t.Cleanup(tracing.Uninstall)

	// Start from a propagator that carries nothing, so the assertion below
	// cannot pass on something an earlier test installed.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator())

	tp, err := InitTracer("test-service", "v1.0.0", "http://localhost:4318")
	if err != nil {
		t.Fatalf("InitTracer failed: %v", err)
	}
	t.Cleanup(func() { tracingtest.Shutdown(tp) })

	fields := otel.GetTextMapPropagator().Fields()
	for _, want := range []string{"traceparent", "baggage"} {
		if !containsString(fields, want) {
			t.Errorf("global propagator fields = %v, want it to carry %q", fields, want)
		}
	}
}

// TestEnvironmentOverridesTheConfiguredServiceName pins down which name wins
// where. resource.WithFromEnv runs after the configured attributes and
// replaces them, so a collector sees OTEL_SERVICE_NAME -- that is
// OpenTelemetry's documented precedence, and deployments rely on it to rename
// a service without a rebuild. Config.ServiceName still names the tracer
// itself, which is what shows up as the instrumentation scope.
func TestEnvironmentOverridesTheConfiguredServiceName(t *testing.T) {
	cfg := Config{ServiceName: "test-service", ServiceVersion: "v1.0.0"}

	res, err := cfg.Resource(context.Background())
	if err != nil {
		t.Fatalf("Resource error = %v", err)
	}
	if got := serviceNameOf(res); got != "test-service" {
		t.Fatalf("service.name = %q with no OTEL_SERVICE_NAME set, want %q", got, "test-service")
	}

	t.Setenv("OTEL_SERVICE_NAME", "env-service")

	res, err = cfg.Resource(context.Background())
	if err != nil {
		t.Fatalf("Resource error = %v", err)
	}
	if got := serviceNameOf(res); got != "env-service" {
		t.Errorf("service.name = %q with OTEL_SERVICE_NAME set, want %q", got, "env-service")
	}
}

func serviceNameOf(res *resource.Resource) string {
	for _, attr := range res.Attributes() {
		if attr.Key == semconv.ServiceNameKey {
			return attr.Value.AsString()
		}
	}
	return ""
}

func TestConfigResource_CarriesNameAndVersion(t *testing.T) {
	res, err := Config{ServiceName: "svc", ServiceVersion: "v2.3.4"}.Resource(context.Background())
	if err != nil {
		t.Fatalf("Resource error = %v", err)
	}

	want := map[attribute.Key]string{
		semconv.ServiceNameKey:    "svc",
		semconv.ServiceVersionKey: "v2.3.4",
	}
	got := map[attribute.Key]string{}
	for _, attr := range res.Attributes() {
		if _, ok := want[attr.Key]; ok {
			got[attr.Key] = attr.Value.AsString()
		}
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func TestNewExporter_BuildsWithoutAnEndpointReachable(t *testing.T) {
	exp, err := NewExporter(context.Background(), Config{
		Endpoint: "collector.internal:4318",
		Insecure: true,
	})
	if err != nil {
		t.Fatalf("NewExporter error = %v", err)
	}
	if exp == nil {
		t.Fatal("NewExporter returned no exporter")
	}
	_ = exp.Shutdown(context.Background())
}

// TestInitTracer_ResourceNewError: resource discovery reads the environment
// and can fail; the failure has to reach the caller rather than yielding a
// provider with no service identity.
func TestInitTracer_ResourceNewError(t *testing.T) {
	t.Cleanup(func() {
		ResetHooks()
		tracing.Uninstall()
	})

	expectedErr := errors.New("mock resource creation error")
	SetResourceNewFunc(func(ctx context.Context, opts ...resource.Option) (*resource.Resource, error) {
		return nil, expectedErr
	})

	tp, err := InitTracer("test-service", "v1.0.0", "http://localhost:4318")
	if err == nil {
		t.Fatal("InitTracer should return error when resource.New fails")
	}
	if tp != nil {
		tracingtest.Shutdown(tp)
		t.Fatal("TracerProvider should be nil when resource.New fails")
	}
	if !errors.Is(err, expectedErr) || !strings.Contains(err.Error(), "failed to create resource") {
		t.Fatalf("error = %v, want it to wrap %v under 'failed to create resource'", err, expectedErr)
	}
}

func TestInitTracer_OtlptraceNewError(t *testing.T) {
	t.Cleanup(func() {
		ResetHooks()
		tracing.Uninstall()
	})

	expectedErr := errors.New("mock OTLP exporter creation error")
	SetOtlptraceNewFunc(func(ctx context.Context, client otlptrace.Client) (*otlptrace.Exporter, error) {
		return nil, expectedErr
	})

	tp, err := InitTracer("test-service", "v1.0.0", "http://localhost:4318")
	if err == nil {
		t.Fatal("InitTracer should return error when otlptrace.New fails")
	}
	if tp != nil {
		tracingtest.Shutdown(tp)
		t.Fatal("TracerProvider should be nil when otlptrace.New fails")
	}
	if !errors.Is(err, expectedErr) || !strings.Contains(err.Error(), "failed to create OTLP exporter") {
		t.Fatalf("error = %v, want it to wrap %v under 'failed to create OTLP exporter'", err, expectedErr)
	}
}

// TestFailedInitLeavesTheRunningProviderAlone: a failed reconfiguration must
// not leave a process with no tracing at all and no way to tell.
func TestFailedInitLeavesTheRunningProviderAlone(t *testing.T) {
	t.Cleanup(func() {
		ResetHooks()
		tracing.Uninstall()
	})

	_, exporter := tracingtest.Setup(t)

	SetResourceNewFunc(func(context.Context, ...resource.Option) (*resource.Resource, error) {
		return nil, errors.New("malformed OTEL_RESOURCE_ATTRIBUTES")
	})

	if _, err := InitTracer("svc", "v1", "http://localhost:4318"); err == nil {
		t.Fatal("InitTracer should have failed")
	}

	if !tracing.IsEnabled() {
		t.Error("a failed InitTracer disabled the tracing that was working before it")
	}
	_, span := tracing.GetTracer().Start(context.Background(), "after-failed-init")
	span.End()
	if got := len(exporter.GetSpans()); got != 1 {
		t.Errorf("recorded %d spans after a failed InitTracer, want 1", got)
	}
}

func TestMain(m *testing.M) {
	// Keep the OpenTelemetry error handler quiet: several tests install
	// deliberately broken providers.
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(error) {}))
	os.Exit(m.Run())
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
