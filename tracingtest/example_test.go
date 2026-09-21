package tracingtest_test

import (
	"context"
	"fmt"

	tracing "github.com/soulteary/tracing-kit/v2"
	"github.com/soulteary/tracing-kit/v2/tracingtest"
)

// Example shows the shape a test takes. Inside a real test the argument is
// the *testing.T, and Setup registers its own cleanup, so there is nothing to
// defer:
//
//	func TestCheckout(t *testing.T) {
//		tp, exporter := tracingtest.Setup(t)
//		...
//	}
//
// Passing nil, as here, leaves the cleanup to the caller.
func Example() {
	tp, exporter := tracingtest.Setup(nil)
	defer func() {
		tracingtest.Shutdown(tp)
		tracingtest.Teardown()
	}()

	// The code under test, using nothing but the root package.
	_, span := tracing.StartSpan(context.Background(), "checkout.charge")
	tracing.SetSpanAttributes(span, map[string]string{"order.id": "A-1"})
	span.End()

	tracingtest.ForceFlush(tp)

	recorded := exporter.GetSpans()
	fmt.Println("spans:", len(recorded))
	fmt.Println("name: ", recorded[0].Name)
	// Output:
	// spans: 1
	// name:  checkout.charge
}
