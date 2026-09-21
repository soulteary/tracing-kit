// Package tracing is a small OpenTelemetry tracing toolkit: span helpers,
// trace-context propagation over plain string maps, and management of the
// process-wide tracer provider.
//
// # Layout
//
// The root package depends on the OpenTelemetry API only --
// go.opentelemetry.io/otel and go.opentelemetry.io/otel/trace. It does not
// import the SDK, and it does not import an exporter. That is the split
// OpenTelemetry itself asks for: instrumentation depends on the API, and the
// application chooses the SDK and the exporter.
//
// Everything that needs more than the API lives in a subpackage, so importing
// the root package never links a transport the service does not use:
//
//   - github.com/soulteary/tracing-kit/v2/otlp -- the OTLP/HTTP exporter and
//     tracer initialisation, and with them the OpenTelemetry SDK, gRPC and
//     protobuf.
//   - github.com/soulteary/tracing-kit/v2/tracingtest -- the in-memory test
//     tracer, and with it the SDK's tracetest package.
//
// A library that only starts spans pays nothing for either one existing.
//
// # Getting started
//
// The application wires up an exporter once, at startup:
//
//	tp, err := otlp.InitTracerWithConfig(otlp.Config{
//		ServiceName: "my-service",
//		Endpoint:    "collector.internal:4318",
//		TLSConfig:   &tls.Config{MinVersion: tls.VersionTLS12},
//	})
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer tp.Shutdown(context.Background())
//
// Everything after that -- including code in libraries that never import the
// otlp subpackage -- goes through the root package:
//
//	ctx, span := tracing.StartSpan(ctx, "process.request")
//	defer span.End()
//
// Until something is installed, [GetTracer] hands back a no-op tracer, so
// instrumentation written against this package is safe to run in a process
// that never configures tracing at all.
//
// # Bringing your own exporter
//
// [Install] takes a [Provider], not a concrete SDK type, so any tracer
// provider can be installed -- a stdout exporter for local debugging, a
// vendor SDK, or an SDK provider assembled by hand:
//
//	tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exp))
//	tracing.Install(tp, "my-service")
//	tracing.SetDefaultPropagator()
//
// [SetDefaultPropagator] is a separate call on purpose. OpenTelemetry's
// global propagator propagates nothing by default, and a service that exports
// no spans of its own still has to pass an incoming trace context on to the
// next hop -- so propagation is worth switching on even where tracing is off.
package tracing
