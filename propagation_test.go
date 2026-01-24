package tracing

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestExtractTraceContext(t *testing.T) {
	// Setup test tracer
	tp, _ := SetupTestTracer(t)
	defer func() {
		ShutdownTracerProvider(tp)
		TeardownTestTracer()
	}()

	// Create a span and inject it
	ctx := context.Background()
	_, span := StartSpan(ctx, "test.operation")
	defer span.End()

	// Inject trace context
	headers := make(map[string]string)
	InjectTraceContext(ctx, headers)

	// Extract trace context
	newCtx := ExtractTraceContext(context.Background(), headers)

	// Verify context is not nil
	if newCtx == nil {
		t.Fatal("ExtractTraceContext should return a context")
	}
}

func TestExtractTraceContext_WithTraceparent(t *testing.T) {
	// Setup test tracer
	tp, _ := SetupTestTracer(t)
	defer func() {
		ShutdownTracerProvider(tp)
		TeardownTestTracer()
	}()

	// Create headers with traceparent
	headers := map[string]string{
		"traceparent": "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		"tracestate":  "rojo=00f067aa0ba902b7",
	}

	// Extract trace context
	ctx := ExtractTraceContext(context.Background(), headers)

	// Verify context is not nil
	if ctx == nil {
		t.Fatal("ExtractTraceContext should return a context")
	}

	// Verify span context is extracted
	spanCtx := trace.SpanContextFromContext(ctx)
	if !spanCtx.IsValid() {
		t.Fatal("Extracted span context should be valid")
	}
}

func TestExtractTraceContext_EmptyHeaders(t *testing.T) {
	// Setup test tracer
	tp, _ := SetupTestTracer(t)
	defer func() {
		ShutdownTracerProvider(tp)
		TeardownTestTracer()
	}()

	// Test with empty headers
	headers := make(map[string]string)
	ctx := ExtractTraceContext(context.Background(), headers)

	// Should not panic and return a context
	if ctx == nil {
		t.Fatal("ExtractTraceContext should return a context even with empty headers")
	}
}

func TestExtractTraceContext_NilHeaders(t *testing.T) {
	// Setup test tracer
	tp, _ := SetupTestTracer(t)
	defer func() {
		ShutdownTracerProvider(tp)
		TeardownTestTracer()
	}()

	// Test with nil headers (should not panic)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ExtractTraceContext panicked with nil headers: %v", r)
		}
	}()
	ctx := ExtractTraceContext(context.Background(), nil)
	if ctx == nil {
		t.Fatal("ExtractTraceContext should return a context")
	}
}

func TestExtractTraceContext_InvalidTraceparent(t *testing.T) {
	// Setup test tracer
	tp, _ := SetupTestTracer(t)
	defer func() {
		ShutdownTracerProvider(tp)
		TeardownTestTracer()
	}()

	// Test with invalid traceparent
	headers := map[string]string{
		"traceparent": "invalid-format",
	}

	// Should not panic
	ctx := ExtractTraceContext(context.Background(), headers)
	if ctx == nil {
		t.Fatal("ExtractTraceContext should return a context")
	}
}

func TestInjectTraceContext(t *testing.T) {
	// Setup test tracer
	tp, _ := SetupTestTracer(t)
	defer func() {
		ShutdownTracerProvider(tp)
		TeardownTestTracer()
	}()

	// Create a span
	ctx := context.Background()
	newCtx, span := StartSpan(ctx, "test.operation")
	defer span.End()

	// Inject trace context
	headers := make(map[string]string)
	InjectTraceContext(newCtx, headers)

	// Verify headers are set
	if len(headers) == 0 {
		t.Fatal("InjectTraceContext should set headers")
	}

	// Verify traceparent is set
	if _, ok := headers["traceparent"]; !ok {
		t.Fatal("traceparent header should be set")
	}
}

func TestInjectTraceContext_EmptyContext(t *testing.T) {
	// Setup test tracer
	tp, _ := SetupTestTracer(t)
	defer func() {
		ShutdownTracerProvider(tp)
		TeardownTestTracer()
	}()

	// Test with context without span
	ctx := context.Background()
	headers := make(map[string]string)
	InjectTraceContext(ctx, headers)

	// Headers might be empty if no span in context, which is acceptable
}

func TestInjectTraceContext_NilHeaders(t *testing.T) {
	// Setup test tracer
	tp, _ := SetupTestTracer(t)
	defer func() {
		ShutdownTracerProvider(tp)
		TeardownTestTracer()
	}()

	// Create a span
	ctx := context.Background()
	newCtx, span := StartSpan(ctx, "test.operation")
	defer span.End()

	// Test with nil headers (should panic or handle gracefully)
	defer func() {
		if r := recover(); r != nil {
			// Panic is acceptable for nil map
			t.Logf("InjectTraceContext panicked with nil headers (expected): %v", r)
		}
	}()
	InjectTraceContext(newCtx, nil)
}

func TestInjectAndExtract_RoundTrip(t *testing.T) {
	// Setup test tracer
	tp, _ := SetupTestTracer(t)
	defer func() {
		ShutdownTracerProvider(tp)
		TeardownTestTracer()
	}()

	// Create a span
	ctx := context.Background()
	newCtx, span := StartSpan(ctx, "test.operation")
	defer span.End()

	// Get original span context
	originalSpanCtx := trace.SpanContextFromContext(newCtx)
	if !originalSpanCtx.IsValid() {
		t.Fatal("Original span context should be valid")
	}

	// Inject trace context
	headers := make(map[string]string)
	InjectTraceContext(newCtx, headers)

	// Extract trace context
	extractedCtx := ExtractTraceContext(context.Background(), headers)

	// Verify extracted span context matches original
	extractedSpanCtx := trace.SpanContextFromContext(extractedCtx)
	if !extractedSpanCtx.IsValid() {
		t.Fatal("Extracted span context should be valid")
	}

	// Verify trace IDs match
	if originalSpanCtx.TraceID() != extractedSpanCtx.TraceID() {
		t.Fatal("Trace IDs should match after round trip")
	}

	if originalSpanCtx.SpanID() != extractedSpanCtx.SpanID() {
		t.Fatal("Span IDs should match after round trip")
	}
}

func TestMapCarrier_Get(t *testing.T) {
	// Test mapCarrier.Get
	headers := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}
	carrier := &mapCarrier{headers: headers}

	// Test Get with existing key
	val := carrier.Get("key1")
	if val != "value1" {
		t.Fatalf("Get should return 'value1', got '%s'", val)
	}

	// Test Get with non-existing key
	val = carrier.Get("nonexistent")
	if val != "" {
		t.Fatalf("Get should return empty string for non-existing key, got '%s'", val)
	}
}

func TestMapCarrier_Set(t *testing.T) {
	// Test mapCarrier.Set
	headers := make(map[string]string)
	carrier := &mapCarrier{headers: headers}

	// Test Set
	carrier.Set("key1", "value1")
	if headers["key1"] != "value1" {
		t.Fatalf("Set should set value, got '%s'", headers["key1"])
	}

	// Test Set with existing key (should overwrite)
	carrier.Set("key1", "value2")
	if headers["key1"] != "value2" {
		t.Fatalf("Set should overwrite existing value, got '%s'", headers["key1"])
	}
}

func TestMapCarrier_Keys(t *testing.T) {
	// Test mapCarrier.Keys
	headers := map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}
	carrier := &mapCarrier{headers: headers}

	// Test Keys
	keys := carrier.Keys()
	if len(keys) != 3 {
		t.Fatalf("Keys should return 3 keys, got %d", len(keys))
	}

	// Verify all keys are present
	keyMap := make(map[string]bool)
	for _, k := range keys {
		keyMap[k] = true
	}
	if !keyMap["key1"] || !keyMap["key2"] || !keyMap["key3"] {
		t.Fatal("Keys should contain all header keys")
	}
}

func TestMapCarrier_Keys_Empty(t *testing.T) {
	// Test mapCarrier.Keys with empty map
	headers := make(map[string]string)
	carrier := &mapCarrier{headers: headers}

	// Test Keys
	keys := carrier.Keys()
	if len(keys) != 0 {
		t.Fatalf("Keys should return empty slice for empty map, got %d keys", len(keys))
	}
}

func TestMapCarrier_Get_NilHeaders(t *testing.T) {
	// Test mapCarrier.Get with nil headers
	// Note: Reading from nil map returns zero value, doesn't panic
	carrier := &mapCarrier{headers: nil}
	val := carrier.Get("key")
	if val != "" {
		t.Fatalf("Get should return empty string for nil map, got '%s'", val)
	}
}

func TestMapCarrier_Set_NilHeaders(t *testing.T) {
	// Test mapCarrier.Set with nil headers (should panic)
	defer func() {
		if r := recover(); r != nil {
			// Panic is expected
			t.Logf("mapCarrier.Set panicked with nil headers (expected): %v", r)
		}
	}()
	carrier := &mapCarrier{headers: nil}
	carrier.Set("key", "value")
	t.Fatal("Set should panic with nil headers")
}

func TestMapCarrier_Keys_NilHeaders(t *testing.T) {
	// Test mapCarrier.Keys with nil headers
	// Note: Range over nil map doesn't panic, returns empty slice
	carrier := &mapCarrier{headers: nil}
	keys := carrier.Keys()
	if len(keys) != 0 {
		t.Fatalf("Keys should return empty slice for nil map, got %d keys", len(keys))
	}
}

func TestExtractTraceContext_WithBaggage(t *testing.T) {
	// Setup test tracer with baggage propagator
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(tracetest.NewInMemoryExporter()))
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	defer func() {
		ShutdownTracerProvider(tp)
		otel.SetTracerProvider(nil)
	}()

	// Create headers with baggage
	headers := map[string]string{
		"baggage": "key1=value1,key2=value2",
	}

	// Extract trace context
	ctx := ExtractTraceContext(context.Background(), headers)

	// Verify context is not nil
	if ctx == nil {
		t.Fatal("ExtractTraceContext should return a context")
	}
}

func TestInjectTraceContext_WithBaggage(t *testing.T) {
	// Setup test tracer with baggage propagator
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(tracetest.NewInMemoryExporter()))
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	defer func() {
		ShutdownTracerProvider(tp)
		otel.SetTracerProvider(nil)
	}()

	// Create a span
	ctx := context.Background()
	tracer := tp.Tracer("test")
	newCtx, span := tracer.Start(ctx, "test.operation")
	defer span.End()

	// Inject trace context
	headers := make(map[string]string)
	InjectTraceContext(newCtx, headers)

	// Verify headers are set
	if len(headers) == 0 {
		t.Fatal("InjectTraceContext should set headers")
	}
}

func TestExtractTraceContext_MultipleHeaders(t *testing.T) {
	// Setup test tracer
	tp, _ := SetupTestTracer(t)
	defer func() {
		ShutdownTracerProvider(tp)
		TeardownTestTracer()
	}()

	// Create headers with multiple trace-related headers
	headers := map[string]string{
		"traceparent": "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		"tracestate":  "rojo=00f067aa0ba902b7",
		"custom":      "value",
		"other":       "header",
	}

	// Extract trace context
	ctx := ExtractTraceContext(context.Background(), headers)

	// Verify context is not nil
	if ctx == nil {
		t.Fatal("ExtractTraceContext should return a context")
	}
}

func TestInjectTraceContext_MultipleHeaders(t *testing.T) {
	// Setup test tracer
	tp, _ := SetupTestTracer(t)
	defer func() {
		ShutdownTracerProvider(tp)
		TeardownTestTracer()
	}()

	// Create a span
	ctx := context.Background()
	newCtx, span := StartSpan(ctx, "test.operation")
	defer span.End()

	// Inject trace context into headers with existing values
	headers := map[string]string{
		"custom": "value",
		"other":  "header",
	}
	InjectTraceContext(newCtx, headers)

	// Verify trace headers are added
	if _, ok := headers["traceparent"]; !ok {
		t.Fatal("traceparent header should be set")
	}

	// Verify existing headers are preserved
	if headers["custom"] != "value" {
		t.Fatal("Existing headers should be preserved")
	}
}
