package tracing

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// SetDefaultPropagator installs the propagator [ExtractTraceContext] and
// [InjectTraceContext] are meant to be used with: W3C trace context plus W3C
// baggage.
//
// OpenTelemetry's global propagator propagates nothing until something sets
// it, so a process that never calls this -- directly, or through
// otlp.InitTracerWithConfig -- injects empty headers and extracts nothing, and
// traces break at every service boundary.
//
// It is deliberately independent of whether any exporter is configured. A
// service that records no spans of its own still has to pass the incoming
// trace context on to the next hop, or it breaks the trace for the services
// behind it.
func SetDefaultPropagator() {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
}

// ExtractTraceContext extracts trace context from headers
func ExtractTraceContext(ctx context.Context, headers map[string]string) context.Context {
	propagator := otel.GetTextMapPropagator()
	carrier := &mapCarrier{headers: headers}
	return propagator.Extract(ctx, carrier)
}

// InjectTraceContext injects trace context into headers
func InjectTraceContext(ctx context.Context, headers map[string]string) {
	propagator := otel.GetTextMapPropagator()
	carrier := &mapCarrier{headers: headers}
	propagator.Inject(ctx, carrier)
}

// mapCarrier implements the TextMapCarrier interface for map[string]string
type mapCarrier struct {
	headers map[string]string
}

func (c *mapCarrier) Get(key string) string {
	return c.headers[key]
}

func (c *mapCarrier) Set(key, value string) {
	c.headers[key] = value
}

func (c *mapCarrier) Keys() []string {
	keys := make([]string, 0, len(c.headers))
	for k := range c.headers {
		keys = append(keys, k)
	}
	return keys
}
