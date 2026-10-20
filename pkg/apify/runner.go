package apify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Runner is the one place that talks to Apify's run-sync endpoint.
//
// Each site client (fb-ads, amazon, 1688, reddit) used to carry its own copy
// of the URL format, request build, status check and raw-item decode. They
// differ only in actor ID, input shape and item shape, so everything else
// lives here and they keep just the parts that are actually theirs.
//
// It also owns the concurrency slot: a run is acquired before the request and
// released after, so callers cannot forget and over-subscribe an account.
type Runner struct {
	baseURL string
	http    *http.Client
	slots   *SlotStore

	// AcquireTimeout bounds how long a job waits for a free slot, separately
	// from how long the run itself may take. They must not share one deadline:
	// queueing behind a busy pool is backpressure and should be retried, while
	// a run that overruns is a failure.
	AcquireTimeout time.Duration
}

func NewRunner(baseURL string, slots *SlotStore, runTimeout time.Duration) *Runner {
	if runTimeout <= 0 {
		runTimeout = 5 * time.Minute
	}
	return &Runner{
		baseURL: baseURL,
		slots:   slots,
		http: &http.Client{
			Timeout:   runTimeout,
			Transport: &LeasedTokenTransport{Slots: slots},
		},
		AcquireTimeout: 30 * time.Minute,
	}
}

// ErrBusy wraps a failure that says "not now", not "not ever": no slot came
// free before AcquireTimeout, or the account that did answer has no budget
// left. Callers should requeue on it rather than dead-letter -- nothing is
// wrong with the job, and the next attempt lands on a token that still works.
type ErrBusy struct{ Err error }

func (e *ErrBusy) Error() string { return "apify: cannot run right now: " + e.Err.Error() }
func (e *ErrBusy) Unwrap() error { return e.Err }

// exhaustionProbeBytes is how much of a failed response Runner reads. Apify's
// billing errors put error.type in the first object, but a validation error
// can precede it with a long schema dump, so this is well past the 512 bytes
// the error message itself shows.
const exhaustionProbeBytes = 8 << 10

// RunSync runs an actor and returns its dataset items as raw JSON, one
// element per item, so callers can both decode into their own type and keep
// the original bytes for a JSONB column.
func (r *Runner) RunSync(ctx context.Context, actorID, jobID string, input any) ([]json.RawMessage, error) {
	payload, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("apify: marshal input: %w", err)
	}

	acquireCtx, cancel := context.WithTimeout(ctx, r.AcquireTimeout)
	lease, err := r.slots.Acquire(acquireCtx, actorID, jobID)
	cancel()
	if err != nil {
		return nil, &ErrBusy{Err: err}
	}
	// Released even when the run panics or ctx is already cancelled -- a
	// dropped lease stays booked until its TTL, silently shrinking the pool.
	defer func() {
		if err := r.slots.Release(context.WithoutCancel(ctx), lease); err != nil && r.slots.Logf != nil {
			r.slots.Logf("apify: releasing slot %d: %v", lease.ID, err)
		}
	}()

	reqURL := fmt.Sprintf("%s/acts/%s/run-sync-get-dataset-items", r.baseURL, actorID)
	req, err := http.NewRequestWithContext(WithLease(ctx, lease), http.MethodPost, reqURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("apify: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("apify: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Include a snippet of the body: Apify explains input-schema
		// rejections there, and a bare status number sends you to the
		// dashboard to find out what you already had in hand. Read past the
		// snippet length so a billing error.type near the end of a verbose
		// body is still there to classify.
		body, _ := io.ReadAll(io.LimitReader(resp.Body, exhaustionProbeBytes))
		body = bytes.TrimSpace(body)
		snippet := body
		if len(snippet) > 512 {
			snippet = snippet[:512]
		}
		err := fmt.Errorf("apify: actor %s returned status %d: %s", actorID, resp.StatusCode, snippet)
		if isExhaustion(resp.StatusCode, body) {
			// The account is out of budget, which says nothing about this
			// job. The transport has already parked the token, so requeueing
			// sends the job to a token that still has credit instead of
			// dead-lettering perfectly valid work.
			return nil, &ErrBusy{Err: err}
		}
		return nil, err
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("apify: read response body: %w", err)
	}
	var items []json.RawMessage
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("apify: unmarshal dataset items: %w", err)
	}
	return items, nil
}

// DecodeItems decodes raw dataset items into T, keeping each item's original
// bytes alongside so callers can persist the untouched payload.
func DecodeItems[T any](raw []json.RawMessage) ([]T, error) {
	out := make([]T, len(raw))
	for i, r := range raw {
		if err := json.Unmarshal(r, &out[i]); err != nil {
			return nil, fmt.Errorf("apify: unmarshal item %d: %w", i, err)
		}
	}
	return out, nil
}
