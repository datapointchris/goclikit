package goclikit

import (
	"context"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/trace"

	"github.com/datapointchris/goselfupdate/autoupdate"
)

type callerKey struct{}

// runCall runs `demo call` through Execute under ctx. The command sends one
// request through TraceForwardingTransport, built from its own context. It
// returns that context and the headers the server received.
func runCall(t *testing.T, ctx context.Context) (context.Context, http.Header) {
	t.Helper()
	var received http.Header
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		received = r.Header.Clone()
	}))
	t.Cleanup(server.Close)

	var seen context.Context
	root := &cobra.Command{Use: "demo", SilenceErrors: true, SilenceUsage: true}
	root.AddCommand(&cobra.Command{Use: "call", RunE: func(cmd *cobra.Command, _ []string) error {
		seen = cmd.Context()
		req, err := http.NewRequestWithContext(cmd.Context(), http.MethodGet, server.URL, nil)
		if err != nil {
			return err
		}
		resp, err := (&http.Client{Transport: TraceForwardingTransport(nil)}).Do(req)
		if err != nil {
			return err
		}
		return resp.Body.Close()
	}})

	withArgs(t, "call")
	if err := Execute(ctx, root, autoupdate.Config{Suppress: true}); err != nil {
		t.Fatal(err)
	}
	return seen, received
}

// sameBaggage reports whether two baggage headers hold the same members,
// compared parsed because baggage serializes them in no fixed order.
func sameBaggage(t *testing.T, got, want string) bool {
	t.Helper()
	parse := func(header string) map[string]bool {
		bag, err := baggage.Parse(header)
		if err != nil {
			t.Fatalf("baggage %q: %v", header, err)
		}
		set := map[string]bool{}
		for _, member := range bag.Members() {
			set[member.String()] = true
		}
		return set
	}
	return maps.Equal(parse(got), parse(want))
}

// The trace a CLI was started under reaches the command's context, and the
// request carries the same trace id and every baggage member.
func TestExecuteForwardsTheInheritedTraceOnEachRequest(t *testing.T) {
	const traceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	const inherited = "run.id=r-42,note=two%20words;origin=job"
	t.Setenv("TRACEPARENT", "00-"+traceID+"-00f067aa0ba902b7-01")
	t.Setenv("TRACESTATE", "vendor=opaque")
	t.Setenv("BAGGAGE", inherited)

	seen, received := runCall(t, context.Background())

	if got := trace.SpanContextFromContext(seen).TraceID().String(); got != traceID {
		t.Errorf("command's trace id = %q, want %q", got, traceID)
	}
	if got := baggage.FromContext(seen).String(); !sameBaggage(t, got, inherited) {
		t.Errorf("command's baggage = %q, want %q", got, inherited)
	}

	if got := received.Get("traceparent"); !strings.HasPrefix(got, "00-"+traceID+"-") {
		t.Errorf("traceparent = %q, want trace id %s", got, traceID)
	}
	if got := received.Get("tracestate"); got != "vendor=opaque" {
		t.Errorf("tracestate = %q, want vendor=opaque", got)
	}
	if got := received.Get("baggage"); !sameBaggage(t, got, inherited) {
		t.Errorf("baggage = %q, want %q", got, inherited)
	}
}

// With none of the variables set, the command runs under the very context the
// caller passed, and the request goes out with no trace headers. The test
// process may itself run under a trace, so each variable is unset rather than
// assumed.
func TestExecuteWithNoInheritedTraceSendsNone(t *testing.T) {
	for _, name := range []string{"TRACEPARENT", "TRACESTATE", "BAGGAGE"} {
		t.Setenv(name, "")
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
	}
	caller := context.WithValue(context.Background(), callerKey{}, "caller")

	seen, received := runCall(t, caller)

	if seen != caller {
		t.Errorf("command ran under a context other than the caller's")
	}
	for _, header := range []string{"traceparent", "tracestate", "baggage"} {
		if got := received.Get(header); got != "" {
			t.Errorf("%s = %q, want none", header, got)
		}
	}
}
