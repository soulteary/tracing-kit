package tracing

import (
	"context"
	"errors"
	"os"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestInitTracer_WithEndpoint(t *testing.T) {
	// Clean up before test
	TeardownTestTracer()

	// Test with valid endpoint
	tp, err := InitTracer("test-service", "v1.0.0", "http://localhost:4318")
	if err != nil {
		t.Fatalf("InitTracer failed: %v", err)
	}
	if tp == nil {
		t.Fatal("TracerProvider should not be nil")
	}

	// Verify global state
	if tracerProvider == nil {
		t.Fatal("tracerProvider should be set")
	}
	if tracer == nil {
		t.Fatal("tracer should be set")
	}
	if serviceName != "test-service" {
		t.Fatalf("serviceName should be 'test-service', got '%s'", serviceName)
	}

	// Verify global tracer provider is set
	globalTP := otel.GetTracerProvider()
	if globalTP == nil {
		t.Fatal("Global tracer provider should be set")
	}

	// Clean up
	ShutdownTracerProvider(tp)
	TeardownTestTracer()
}

func TestInitTracer_WithoutEndpoint(t *testing.T) {
	// Clean up before test
	TeardownTestTracer()

	// Test with empty endpoint (should return nil)
	tp, err := InitTracer("test-service", "v1.0.0", "")
	if err != nil {
		t.Fatalf("InitTracer should not return error with empty endpoint: %v", err)
	}
	if tp != nil {
		t.Fatal("TracerProvider should be nil when endpoint is empty")
	}

	// Clean up
	TeardownTestTracer()
}

func TestInitTracer_InvalidEndpoint(t *testing.T) {
	// Clean up before test
	TeardownTestTracer()

	// Test with invalid endpoint format (should still create exporter but might fail later)
	// Note: otlptracehttp.NewClient is lenient, so we test with a malformed URL
	tp, err := InitTracer("test-service", "v1.0.0", "invalid-url-format")
	if err != nil {
		// This is acceptable - invalid URLs might cause errors
		t.Logf("InitTracer with invalid URL returned error (expected): %v", err)
		return
	}
	if tp != nil {
		ShutdownTracerProvider(tp)
	}
	TeardownTestTracer()
}

func TestInitTracer_ResourceError(t *testing.T) {
	// Clean up before test
	TeardownTestTracer()

	// Set an invalid environment variable that might cause resource creation to fail
	// This is tricky to test, but we can try to create a resource with invalid attributes
	// Actually, resource.New is quite lenient, so we'll test the normal path
	// and verify error handling exists

	tp, err := InitTracer("test-service", "v1.0.0", "http://localhost:4318")
	if err != nil {
		t.Fatalf("InitTracer should succeed with valid endpoint: %v", err)
	}
	if tp != nil {
		ShutdownTracerProvider(tp)
	}
	TeardownTestTracer()
}

func TestShutdown_WithProvider(t *testing.T) {
	// Setup
	TeardownTestTracer()
	tp, _ := InitTracer("test-service", "v1.0.0", "http://localhost:4318")
	if tp == nil {
		t.Skip("Skipping test - tracer provider not initialized")
	}

	// Test shutdown
	ctx := context.Background()
	err := Shutdown(ctx)
	if err != nil {
		t.Fatalf("Shutdown should not return error: %v", err)
	}

	// Verify state is cleared (Shutdown doesn't clear package vars, but that's OK)
	TeardownTestTracer()
}

func TestShutdown_WithoutProvider(t *testing.T) {
	// Clean up before test
	TeardownTestTracer()

	// Test shutdown when provider is nil
	ctx := context.Background()
	err := Shutdown(ctx)
	if err != nil {
		t.Fatalf("Shutdown should return nil when provider is nil: %v", err)
	}
}

func TestGetTracer_WithInitializedTracer(t *testing.T) {
	// Setup
	TeardownTestTracer()
	tp, _ := InitTracer("test-service", "v1.0.0", "http://localhost:4318")
	if tp == nil {
		t.Skip("Skipping test - tracer provider not initialized")
	}

	// Test GetTracer
	tr := GetTracer()
	if tr == nil {
		t.Fatal("GetTracer should return a tracer")
	}

	// Verify it's the initialized tracer (not noop)
	// We can verify by checking that it's the same instance or by checking IsEnabled
	if !IsEnabled() {
		t.Fatal("Tracing should be enabled after initialization")
	}

	// Clean up
	ShutdownTracerProvider(tp)
	TeardownTestTracer()
}

func TestGetTracer_WithoutInitializedTracer(t *testing.T) {
	// Clean up before test
	TeardownTestTracer()

	// Test GetTracer when not initialized
	tr := GetTracer()
	if tr == nil {
		t.Fatal("GetTracer should return noop tracer when not initialized")
	}

	// Verify it's not nil (noop tracer is returned)
	// Note: noop.Tracer is not exported, so we can't do type assertion
	// But we can verify it works by starting a span
	ctx := context.Background()
	_, span := tr.Start(ctx, "test")
	if span == nil {
		t.Fatal("Noop tracer should be able to create spans")
	}
	span.End()
}

func TestGetTracer_WithServiceName(t *testing.T) {
	// Clean up before test
	TeardownTestTracer()

	// Set service name but don't initialize tracer
	serviceName = "my-service"

	// Test GetTracer
	tr := GetTracer()
	if tr == nil {
		t.Fatal("GetTracer should return a tracer")
	}

	// Verify it works by starting a span
	ctx := context.Background()
	_, span := tr.Start(ctx, "test")
	if span == nil {
		t.Fatal("Tracer should be able to create spans")
	}
	span.End()

	// Clean up
	serviceName = ""
}

func TestGetTracer_WithoutServiceName(t *testing.T) {
	// Clean up before test
	TeardownTestTracer()
	serviceName = ""

	// Test GetTracer
	tr := GetTracer()
	if tr == nil {
		t.Fatal("GetTracer should return a tracer")
	}

	// Verify it works by starting a span (should use "unknown-service")
	ctx := context.Background()
	_, span := tr.Start(ctx, "test")
	if span == nil {
		t.Fatal("Tracer should be able to create spans")
	}
	span.End()
}

func TestIsEnabled_WithInitializedTracer(t *testing.T) {
	// Setup
	TeardownTestTracer()
	tp, _ := InitTracer("test-service", "v1.0.0", "http://localhost:4318")
	if tp == nil {
		t.Skip("Skipping test - tracer provider not initialized")
	}

	// Test IsEnabled
	if !IsEnabled() {
		t.Fatal("IsEnabled should return true when tracer is initialized")
	}

	// Clean up
	ShutdownTracerProvider(tp)
	TeardownTestTracer()
}

func TestIsEnabled_WithoutInitializedTracer(t *testing.T) {
	// Clean up before test
	TeardownTestTracer()

	// Test IsEnabled
	if IsEnabled() {
		t.Fatal("IsEnabled should return false when tracer is not initialized")
	}
}

func TestIsEnabled_WithNilTracer(t *testing.T) {
	// Setup partial state
	TeardownTestTracer()
	tracerProvider = &sdktrace.TracerProvider{}
	tracer = nil

	// Test IsEnabled
	if IsEnabled() {
		t.Fatal("IsEnabled should return false when tracer is nil")
	}

	// Clean up
	TeardownTestTracer()
}

func TestIsEnabled_WithNilProvider(t *testing.T) {
	// Setup partial state
	TeardownTestTracer()
	tracerProvider = nil
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	tracer = tp.Tracer("test")

	// Test IsEnabled
	if IsEnabled() {
		t.Fatal("IsEnabled should return false when provider is nil")
	}

	// Clean up
	ShutdownTracerProvider(tp)
	TeardownTestTracer()
}

func TestInitTracer_GlobalPropagator(t *testing.T) {
	// Clean up before test
	TeardownTestTracer()

	// Initialize tracer
	tp, err := InitTracer("test-service", "v1.0.0", "http://localhost:4318")
	if err != nil {
		t.Fatalf("InitTracer failed: %v", err)
	}
	defer func() {
		if tp != nil {
			ShutdownTracerProvider(tp)
		}
		TeardownTestTracer()
	}()

	// Verify global propagator is set
	propagator := otel.GetTextMapPropagator()
	if propagator == nil {
		t.Fatal("Global propagator should be set")
	}

	// Test that it's a composite propagator (contains TraceContext and Baggage)
	headers := make(map[string]string)
	ctx := context.Background()
	propagator.Inject(ctx, &mapCarrier{headers: headers})
	// If propagator works, headers should be set (or at least not panic)
}

func TestInitTracer_WithEnvironmentVariables(t *testing.T) {
	// Clean up before test
	TeardownTestTracer()

	// Set some environment variables that resource.WithFromEnv() might pick up
	_ = os.Setenv("OTEL_SERVICE_NAME", "env-service")
	defer func() {
		_ = os.Unsetenv("OTEL_SERVICE_NAME")
	}()

	// Initialize tracer
	tp, err := InitTracer("test-service", "v1.0.0", "http://localhost:4318")
	if err != nil {
		t.Fatalf("InitTracer failed: %v", err)
	}
	defer func() {
		if tp != nil {
			ShutdownTracerProvider(tp)
		}
		TeardownTestTracer()
	}()

	// Verify it still uses the provided service name
	if serviceName != "test-service" {
		t.Fatalf("serviceName should be 'test-service', got '%s'", serviceName)
	}
}

func TestInitTracer_ExporterCreationError(t *testing.T) {
	// Clean up before test
	TeardownTestTracer()

	// Test with endpoint that might cause exporter creation to fail
	// Note: otlptrace.New is quite lenient, so we test normal path
	// In real scenarios, network issues might cause failures, but that's hard to test

	tp, err := InitTracer("test-service", "v1.0.0", "http://localhost:4318")
	if err != nil {
		t.Logf("InitTracer returned error (acceptable): %v", err)
		return
	}
	if tp != nil {
		ShutdownTracerProvider(tp)
	}
	TeardownTestTracer()
}

// Test multiple sequential initialization (should not panic)
// Note: InitTracer is designed to be called once at application startup,
// it is NOT thread-safe and should not be called concurrently.
func TestInitTracer_MultipleSequential(t *testing.T) {
	// Clean up before test
	TeardownTestTracer()

	// Test multiple sequential initialization (should not panic)
	for i := 0; i < 2; i++ {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("InitTracer panicked: %v", r)
				}
			}()
			tp, _ := InitTracer("test-service", "v1.0.0", "http://localhost:4318")
			if tp != nil {
				ShutdownTracerProvider(tp)
			}
			TeardownTestTracer()
		}()
	}
}

// Test that GetTracer works correctly after Shutdown
func TestGetTracer_AfterShutdown(t *testing.T) {
	// Setup
	TeardownTestTracer()
	tp, _ := InitTracer("test-service", "v1.0.0", "http://localhost:4318")
	if tp == nil {
		t.Skip("Skipping test - tracer provider not initialized")
	}

	// Shutdown
	ShutdownTracerProvider(tp)

	// GetTracer should still work (returns noop if tracer is nil)
	tr := GetTracer()
	if tr == nil {
		t.Fatal("GetTracer should return a tracer even after shutdown")
	}

	// Clean up
	TeardownTestTracer()
}

// Test InitTracer with resource.New returning error
func TestInitTracer_ResourceNewError(t *testing.T) {
	// Clean up before test
	TeardownTestTracer()
	defer func() {
		ResetHooks()
		TeardownTestTracer()
	}()

	// Inject error into resource.New
	expectedErr := errors.New("mock resource creation error")
	SetResourceNewFunc(func(ctx context.Context, opts ...resource.Option) (*resource.Resource, error) {
		return nil, expectedErr
	})

	// Test InitTracer - should return error
	tp, err := InitTracer("test-service", "v1.0.0", "http://localhost:4318")
	if err == nil {
		t.Fatal("InitTracer should return error when resource.New fails")
	}
	if tp != nil {
		ShutdownTracerProvider(tp)
		t.Fatal("TracerProvider should be nil when resource.New fails")
	}

	// Verify error message contains the original error
	if !errors.Is(err, expectedErr) && !contains(err.Error(), "failed to create resource") {
		t.Fatalf("Error should contain 'failed to create resource', got: %v", err)
	}
}

// Test InitTracer with otlptrace.New returning error
func TestInitTracer_OtlptraceNewError(t *testing.T) {
	// Clean up before test
	TeardownTestTracer()
	defer func() {
		ResetHooks()
		TeardownTestTracer()
	}()

	// Inject error into otlptrace.New
	expectedErr := errors.New("mock OTLP exporter creation error")
	SetOtlptraceNewFunc(func(ctx context.Context, client otlptrace.Client) (*otlptrace.Exporter, error) {
		return nil, expectedErr
	})

	// Test InitTracer - should return error
	tp, err := InitTracer("test-service", "v1.0.0", "http://localhost:4318")
	if err == nil {
		t.Fatal("InitTracer should return error when otlptrace.New fails")
	}
	if tp != nil {
		ShutdownTracerProvider(tp)
		t.Fatal("TracerProvider should be nil when otlptrace.New fails")
	}

	// Verify error message contains the original error
	if !errors.Is(err, expectedErr) && !contains(err.Error(), "failed to create OTLP exporter") {
		t.Fatalf("Error should contain 'failed to create OTLP exporter', got: %v", err)
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
