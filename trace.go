package goclikit

import (
	"context"
	"net/http"

	"go.opentelemetry.io/contrib/propagators/envcar"
	"go.opentelemetry.io/otel/propagation"
)

// tracePropagator reads and writes W3C trace context and baggage, the default
// of OTEL_PROPAGATORS.
var tracePropagator = propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})

// withInheritedTrace is ctx carrying the trace and baggage the process that
// started this one handed it in TRACEPARENT, TRACESTATE and BAGGAGE.
//
// A variable that is set replaces what ctx already carries, and one that is
// unset leaves it alone, so with none of the three set ctx comes back as it was.
func withInheritedTrace(ctx context.Context) context.Context {
	return tracePropagator.Extract(ctx, &envcar.Carrier{})
}

// TraceForwardingTransport sends the trace and baggage each request's context
// carries as its traceparent, tracestate and baggage headers, then hands the
// request to base. A nil base is [http.DefaultTransport].
//
// [Execute] runs every command under the trace the CLI was started under, so a
// request built from the command's context names that trace:
//
//	httpClient := oauth2.NewClient(ctx, source)
//	httpClient.Transport = goclikit.TraceForwardingTransport(httpClient.Transport)
//
//	req, err := http.NewRequestWithContext(cmd.Context(), method, url, body)
//
// The receiving API's span for the request then sits directly under whatever
// ran the CLI. The transport starts no span of its own and needs no
// OpenTelemetry SDK, because a CLI that records nothing has no span to insert.
//
// Wrap the client that calls the CLI's own API, and leave every other client
// alone. A token endpoint or a third-party service given this transport would
// receive the trace id and every baggage member. The oauth2 client above
// fetches its tokens through a client of its own, so they stay unwrapped.
//
// A request whose context carries no trace and no baggage passes through
// untouched. A request that does is cloned before its headers are set, so the
// caller's request is never modified.
func TraceForwardingTransport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return traceForwardingTransport{base: base}
}

type traceForwardingTransport struct {
	base http.RoundTripper
}

func (t traceForwardingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	headers := propagation.HeaderCarrier{}
	tracePropagator.Inject(req.Context(), headers)
	if len(headers) == 0 {
		return t.base.RoundTrip(req)
	}

	forwarded := req.Clone(req.Context())
	if forwarded.Header == nil {
		forwarded.Header = http.Header{}
	}
	for key, values := range headers {
		forwarded.Header[key] = values
	}
	return t.base.RoundTrip(forwarded)
}
