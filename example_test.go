package tracing_test

import (
	"context"
	"fmt"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	tracing "github.com/soulteary/tracing-kit/v2"
)

// Example instruments a function the way a library would: the root package
// only, with no SDK and no exporter in sight. Until the application installs
// a provider, the spans go nowhere and nothing has to guard against it.
func Example() {
	ctx, span := tracing.StartSpan(context.Background(), "process.request")
	defer span.End()

	tracing.SetSpanAttributes(span, map[string]string{"request.id": "12345"})
	tracing.SetSpanAttributesFromMap(span, map[string]interface{}{
		"user.id": 42,
		"cached":  true,
	})

	fmt.Println("tracing enabled:", tracing.IsEnabled())
	fmt.Println("span recording:", tracing.GetSpanFromContext(ctx).IsRecording())
	// Output:
	// tracing enabled: false
	// span recording: false
}

// ExampleInstall wires up a provider the application built itself -- here one
// that records into memory, but a stdout or vendor exporter goes in the same
// way. The otlp subpackage is the shortcut for the OTLP/HTTP case.
func ExampleInstall() {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))

	tracing.Install(tp, "my-service")
	tracing.SetDefaultPropagator()
	defer tracing.Uninstall()

	_, span := tracing.StartSpan(context.Background(), "process.request")
	span.End()

	fmt.Println("tracing enabled:", tracing.IsEnabled())
	fmt.Println("spans recorded:", len(exporter.GetSpans()))
	// Output:
	// tracing enabled: true
	// spans recorded: 1
}

// ExampleInstallDisabled installs the provider a process uses when tracing is
// switched off: it records nothing, but it is a real provider, so deferring
// Shutdown is safe and whatever was installed before is retired.
func ExampleInstallDisabled() {
	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.NeverSample()))

	tracing.InstallDisabled(tp, "my-service")
	defer tracing.Uninstall()

	_, span := tracing.StartSpan(context.Background(), "process.request")
	span.End()

	fmt.Println("tracing enabled:", tracing.IsEnabled())
	fmt.Println("shutdown error:", tracing.Shutdown(context.Background()))
	// Output:
	// tracing enabled: false
	// shutdown error: <nil>
}

// ExampleInjectTraceContext carries a trace across a service boundary. The
// propagator has to be installed for this to do anything -- OpenTelemetry's
// default propagates nothing -- which is why SetDefaultPropagator is worth
// calling even where no exporter is configured.
func ExampleInjectTraceContext() {
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(tracetest.NewInMemoryExporter()))
	tracing.Install(tp, "service-a")
	tracing.SetDefaultPropagator()
	defer tracing.Uninstall()

	ctx, span := tracing.StartSpan(context.Background(), "service.a.operation")
	defer span.End()

	// Service A: inject into the outgoing request's headers.
	headers := map[string]string{}
	tracing.InjectTraceContext(ctx, headers)

	// Service B: extract from the incoming request's headers and carry on.
	received := tracing.ExtractTraceContext(context.Background(), headers)

	sent := tracing.GetSpanFromContext(ctx).SpanContext().TraceID()
	fmt.Println("traceparent sent:", headers["traceparent"] != "")
	fmt.Println("same trace on the other side:", tracing.GetSpanFromContext(received).SpanContext().TraceID() == sent)
	// Output:
	// traceparent sent: true
	// same trace on the other side: true
}

// ExampleRecordError marks a span as failed. A nil error is ignored, so the
// usual "record whatever came back" call needs no branch around it.
func ExampleRecordError() {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	tracing.Install(tp, "my-service")
	defer tracing.Uninstall()

	_, span := tracing.StartSpan(context.Background(), "do.work")
	tracing.RecordError(span, fmt.Errorf("upstream refused the connection"))
	span.End()

	recorded := exporter.GetSpans()[0]
	fmt.Println("status:", recorded.Status.Code)
	fmt.Println("description:", recorded.Status.Description)
	// Output:
	// status: Error
	// description: upstream refused the connection
}
