package apify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"
)

type leaseCtxKey struct{}

// WithLease pins a request to the token whose concurrency slot it holds.
//
// Without this the transport picks its own token, which would double-count:
// SlotStore charged the run to one account while the request went out on
// another, leaving the first under-used and the second over-subscribed.
func WithLease(ctx context.Context, l *Lease) context.Context {
	if l == nil {
		return ctx
	}
	return context.WithValue(ctx, leaseCtxKey{}, l)
}

// LeaseFrom returns the lease pinned to ctx, if any.
func LeaseFrom(ctx context.Context) (*Lease, bool) {
	l, ok := ctx.Value(leaseCtxKey{}).(*Lease)
	return l, ok
}

// LeasedTokenTransport sends each request under the token of the lease on its
// context, and reports credential failures back to the SlotStore so the other
// services stop picking a dead account.
//
// It deliberately does not retry on another token. Retrying would spend a slot
// that was never acquired for that account; when a token really is dead, the
// next Acquire skips it because MarkExhausted is visible to every process.
type LeasedTokenTransport struct {
	Slots *SlotStore
	Base  http.RoundTripper

	// ExhaustFor is how long a credential failure parks a token. Long enough
	// that the pool isn't rechecked every job, short enough that a topped-up
	// account comes back on its own.
	ExhaustFor time.Duration
}

func (t *LeasedTokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}

	lease, ok := LeaseFrom(req.Context())
	if !ok {
		// Failing closed is the point: a request with no lease has not been
		// counted against anyone's concurrency budget, and letting it through
		// would quietly reintroduce the over-subscription this package exists
		// to prevent.
		return nil, errors.New("apify: request has no lease on its context; call SlotStore.Acquire first")
	}

	resp, err := base.RoundTrip(withToken(req, lease.Token))
	if err != nil {
		return nil, err
	}

	exhausted, body := classifyExhaustion(resp)
	// Reading the body consumes it, so hand the caller an identical one --
	// error messages are built from it downstream.
	resp.Body = io.NopCloser(bytes.NewReader(body))

	if exhausted {
		d := t.ExhaustFor
		if d <= 0 {
			d = 15 * time.Minute
		}
		// The job's context may already be cancelled by the failure; parking
		// the token still has to happen, or every other service rediscovers
		// this the hard way.
		_ = t.Slots.MarkExhausted(context.WithoutCancel(req.Context()), lease.TokenIndex, d)
	}
	return resp, nil
}

// exhaustionErrorTypes are Apify error.type values that mean "this account
// has no budget left", regardless of the HTTP status carrying them.
//
// This is not theoretical. A drained free account calling a pay-per-result
// actor answers 400, not 402:
//
//	HTTP 400 {"error":{"type":"max-items-must-be-greater-than-zero",
//	          "message":"Maximum charged results must be greater than zero"}}
//
// Classifying by status alone would leave that token unparked forever, so
// every service keeps picking it, every job fails, and the failures look like
// bad input instead of an empty wallet.
var exhaustionErrorTypes = map[string]bool{
	"max-items-must-be-greater-than-zero": true,
	"monthly-usage-hard-limit-exceeded":   true,
	"insufficient-credits":                true,
	"usage-limit-exceeded":                true,
}

// classifyExhaustion reports whether the response means the account is out of
// budget, and returns the body it consumed so the caller can restore it.
//
// 429 is deliberately not exhaustion. It means "too many requests right now",
// which under per-token slot accounting is transient backpressure -- parking
// on 429 would retire a healthy account the moment it got busy, and one burst
// would retire the whole pool.
func classifyExhaustion(resp *http.Response) (bool, []byte) {
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return isExhaustion(resp.StatusCode, body), body
}

// isExhaustion is classifyExhaustion's rule on its own, over a status and the
// bytes of a body. Runner needs the same verdict to decide whether a failed
// run is the job's fault or the wallet's, and it has already read the body by
// then, so the rule cannot live inside a function that consumes a response.
func isExhaustion(status int, body []byte) bool {
	switch status {
	case http.StatusUnauthorized, http.StatusPaymentRequired, http.StatusForbidden:
		return true
	}
	// A 400 is normally the caller's own bad input and must not park anyone,
	// so only a known billing error.type counts.
	if status == http.StatusBadRequest {
		var payload struct {
			Error struct {
				Type string `json:"type"`
			} `json:"error"`
		}
		if json.Unmarshal(body, &payload) == nil && exhaustionErrorTypes[payload.Error.Type] {
			return true
		}
	}
	return false
}

func withToken(req *http.Request, token string) *http.Request {
	clone := req.Clone(req.Context())
	q := clone.URL.Query()
	q.Set("token", token)
	clone.URL.RawQuery = q.Encode()
	return clone
}
