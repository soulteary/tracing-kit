package tracing

// This file is compiled only during tests.
//
// It used to be test_helpers.go, a normal source file that imported "testing".
// That linked the testing package into every production binary depending on
// this library -- registering its -test.* flags into flag.CommandLine -- and
// exposed the exporter hooks below as public API, letting anything in the
// process swap the trace exporter at runtime.
//
// The four helpers the README documents for consumers (SetupTestTracer,
// TeardownTestTracer, ShutdownTracerProvider, ForceFlushTracerProvider) stay
// in normal package code, in testhelper.go; they avoid the "testing" import by
// accepting the TestingT interface instead of *testing.T. Only the hooks are
// test-only, because nothing outside this package should be able to replace
// the exporter.

import (
	"context"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/sdk/resource"
)

// ResetHooks resets the hook functions to their default implementations
func ResetHooks() {
	resourceNewFunc = resource.New
	otlptraceNewFunc = otlptrace.New
}

// SetResourceNewFunc sets a custom resource.New function for testing
func SetResourceNewFunc(fn func(ctx context.Context, opts ...resource.Option) (*resource.Resource, error)) {
	resourceNewFunc = fn
}

// SetOtlptraceNewFunc sets a custom otlptrace.New function for testing
func SetOtlptraceNewFunc(fn func(ctx context.Context, client otlptrace.Client) (*otlptrace.Exporter, error)) {
	otlptraceNewFunc = fn
}
