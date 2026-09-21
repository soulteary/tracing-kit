package tracing

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// Provider is the part of an OpenTelemetry tracer provider this package uses:
// it hands out tracers, and it can be shut down.
//
// *go.opentelemetry.io/otel/sdk/trace.TracerProvider satisfies it, and so does
// any wrapper around one. Taking the interface rather than the SDK type is
// what keeps this package's dependencies down to the OpenTelemetry API: code
// that only starts spans links no SDK and no exporter, and the application
// decides what to install. See the otlp subpackage for the usual choice.
type Provider interface {
	trace.TracerProvider

	// Shutdown flushes and releases the provider. [Shutdown] calls it, and so
	// does [Install] on the provider it replaces.
	Shutdown(ctx context.Context) error
}

var (
	// mu guards every variable below it. Without it Install racing with
	// GetTracer or IsEnabled is a data race; sequential tests never catch it.
	mu sync.RWMutex

	provider    Provider
	tracer      trace.Tracer
	serviceName string

	// enabled records whether the installed provider is expected to export.
	// A provider that records nothing is still a non-nil provider, so
	// presence alone cannot answer IsEnabled.
	enabled bool
)

// shutdownTimeout bounds the shutdown of a provider being replaced: a wedged
// exporter must not hang reconfiguration.
const shutdownTimeout = 5 * time.Second

// Install makes p the tracer provider for this process, both for this package
// and for the global OpenTelemetry API, and reports tracing as enabled.
//
// The provider being replaced, if any, is shut down. Replacing the global
// alone is not enough: callers hold tracer handles obtained from whatever
// provider was installed when they asked -- a package-level
// `var tracer = otel.Tracer("svc")` is the common shape -- and those handles
// keep their own provider alive, recording and exporting to the old
// destination while [IsEnabled] reports the new state.
//
// svcName names the tracer taken from p, and is what [GetTracer] returns
// spans from.
//
// Installing a nil provider changes nothing and is reported through
// [go.opentelemetry.io/otel.Handle], rather than panicking: an observability
// library that takes the process down is worse than one that records nothing,
// and a caller that got its argument wrong has not said anything about the
// provider already running.
func Install(p Provider, svcName string) {
	install(p, svcName, true)
}

// InstallDisabled installs p exactly as [Install] does, but reports tracing as
// disabled: [IsEnabled] returns false.
//
// It is for the "tracing is switched off" provider -- one that records
// nothing, yet is a real provider, so a caller can `defer tp.Shutdown(ctx)`
// unconditionally, and so that whatever was installed before is retired
// instead of being left running.
func InstallDisabled(p Provider, svcName string) {
	install(p, svcName, false)
}

func install(p Provider, svcName string, on bool) {
	if isNil(p) {
		// Nothing is installed and nothing is retired: a bad argument is a
		// failed call, and a failed call leaves the working configuration
		// alone -- the same rule otlp.InitTracerWithConfig follows when the
		// exporter cannot be built.
		otel.Handle(errors.New("tracing: a nil Provider was passed in; nothing was installed"))
		return
	}

	mu.Lock()
	defer mu.Unlock()

	previous := provider
	otel.SetTracerProvider(p)

	provider = p
	tracer = p.Tracer(svcName)
	serviceName = svcName
	enabled = on

	if previous != nil && !same(previous, p) {
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := previous.Shutdown(ctx); err != nil {
			otel.Handle(err)
		}
	}
}

// Uninstall drops the installed provider without shutting it down, and points
// the global OpenTelemetry API back at a no-op provider.
//
// After it returns [GetTracer] hands back a no-op tracer and [IsEnabled]
// reports false. The provider itself is left alone, because the caller holds
// it and is the one that can decide when flushing is finished; shut it down
// first if you want its buffered spans exported.
func Uninstall() {
	mu.Lock()
	provider, tracer, serviceName, enabled = nil, nil, "", false
	mu.Unlock()

	otel.SetTracerProvider(noop.NewTracerProvider())
}

// Shutdown gracefully shuts down the installed provider. It is a no-op when
// nothing is installed.
func Shutdown(ctx context.Context) error {
	mu.RLock()
	p := provider
	mu.RUnlock()

	if p != nil {
		return p.Shutdown(ctx)
	}
	return nil
}

// GetTracer returns the tracer of the installed provider, or a no-op tracer
// when nothing is installed -- so instrumentation written against this package
// runs unchanged in a process that never configures tracing.
func GetTracer() trace.Tracer {
	mu.RLock()
	t, name := tracer, serviceName
	mu.RUnlock()

	if t == nil {
		if name == "" {
			name = "unknown-service"
		}
		return noop.NewTracerProvider().Tracer(name)
	}
	return t
}

// IsEnabled reports whether a provider expected to export is installed, i.e.
// whether it arrived through [Install] rather than [InstallDisabled].
func IsEnabled() bool {
	mu.RLock()
	defer mu.RUnlock()
	return enabled && provider != nil && tracer != nil
}

// isNil reports whether there is no provider to install. Provider is an
// interface, so a plain p == nil misses the case that actually reaches here: a
// nil *sdktrace.TracerProvider stored in it, which a caller gets from an
// unassigned field or a constructor that returned early. Taking a tracer from
// one panics.
func isNil(p Provider) bool {
	if p == nil {
		return true
	}
	v := reflect.ValueOf(p)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

// same reports whether a and b are the same provider, so that reinstalling the
// one already installed does not shut it down on the way in.
//
// a == b would do it for the pointer types providers really are, but Provider
// is an interface and comparing two interfaces holding an uncomparable type --
// a struct with a slice field, say -- panics at run time. Answering "not the
// same" for those is the safe way to be wrong: the caller passed a value that
// cannot alias anything, so nothing installed is retired by mistake.
func same(a, b Provider) bool {
	va, vb := reflect.ValueOf(a), reflect.ValueOf(b)
	if va.Type() != vb.Type() || !va.Comparable() || !vb.Comparable() {
		return false
	}
	return va.Equal(vb)
}
