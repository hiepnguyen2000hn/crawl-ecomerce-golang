package apify

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestParseTokens(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{
			name: "single token is the backward-compatible case",
			raw:  "apify_api_aaa",
			want: []string{"apify_api_aaa"},
		},
		{
			name: "comma separated with surrounding spaces",
			raw:  "apify_api_aaa, apify_api_bbb ,apify_api_ccc",
			want: []string{"apify_api_aaa", "apify_api_bbb", "apify_api_ccc"},
		},
		{
			name: "blank entries from a trailing or doubled comma are dropped",
			raw:  "apify_api_aaa,,apify_api_bbb,   ,",
			want: []string{"apify_api_aaa", "apify_api_bbb"},
		},
		{
			name: "duplicates are dropped, first position wins",
			raw:  "apify_api_aaa,apify_api_bbb,apify_api_aaa",
			want: []string{"apify_api_aaa", "apify_api_bbb"},
		},
		{
			name: "empty env yields no tokens",
			raw:  "",
			want: nil,
		},
		{
			name: "whitespace-only env yields no tokens",
			raw:  "   ",
			want: nil,
		},
		{
			// go-zero leaves "${APIFY_API_TOKEN}" verbatim in the config when
			// the env var is unset. Dropping it here means every caller gets
			// an empty list and can use one len()==0 guard, instead of each
			// service re-checking for the "${" marker itself.
			name: "unexpanded env placeholder is not a token",
			raw:  "${APIFY_API_TOKEN}",
			want: nil,
		},
		{
			name: "real tokens survive alongside an unexpanded placeholder",
			raw:  "apify_api_aaa,${APIFY_SECOND_TOKEN}",
			want: []string{"apify_api_aaa"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseTokens(tt.raw)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseTokens(%q) = %#v, want %#v", tt.raw, got, tt.want)
			}
		})
	}
}

/* ---------------------- RotatingTransport ---------------------- */

// tokenRecorder is an httptest handler that records the token query param of
// every request it sees, and answers each token with a caller-chosen status.
type tokenRecorder struct {
	mu       sync.Mutex
	seen     []string
	bodies   []string
	statuses map[string]int // token -> status to return; absent means 200
}

func (tr *tokenRecorder) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		token := r.URL.Query().Get("token")

		tr.mu.Lock()
		tr.seen = append(tr.seen, token)
		tr.bodies = append(tr.bodies, string(body))
		status, ok := tr.statuses[token]
		tr.mu.Unlock()

		if !ok {
			status = http.StatusOK
		}
		w.WriteHeader(status)
		fmt.Fprintf(w, `{"servedBy":%q}`, token)
	}
}

func (tr *tokenRecorder) tokensSeen() []string {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	return append([]string(nil), tr.seen...)
}

// postThrough issues a POST with the given JSON body through client, reads the
// response, and returns status plus body text.
func postThrough(t *testing.T, client *http.Client, url, payload string) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader([]byte(payload)))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func TestRotatingTransport_RotatesOnCredentialErrors(t *testing.T) {
	// Each of these means "this token cannot do the job" — a different token
	// plausibly can, so the transport should move on and retry.
	for _, status := range []int{
		http.StatusUnauthorized,    // 401 invalid/revoked token
		http.StatusPaymentRequired, // 402 out of credits — the common real case
		http.StatusForbidden,       // 403 monthly usage hard limit
		http.StatusTooManyRequests, // 429 rate-limit-exceeded
	} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			rec := &tokenRecorder{statuses: map[string]int{"dead": status}}
			srv := httptest.NewServer(rec.handler())
			defer srv.Close()

			client := &http.Client{Transport: NewRotatingTransport([]string{"dead", "live"}, srv.Client().Transport)}
			got, body := postThrough(t, client, srv.URL, `{"query":"acme"}`)

			if got != http.StatusOK {
				t.Errorf("status = %d, want 200 after rotating past %d", got, status)
			}
			if !strings.Contains(body, `"servedBy":"live"`) {
				t.Errorf("body = %s, want the response served by the live token", body)
			}
			if want := []string{"dead", "live"}; !reflect.DeepEqual(rec.tokensSeen(), want) {
				t.Errorf("tokens tried = %v, want %v", rec.tokensSeen(), want)
			}
		})
	}
}

func TestRotatingTransport_ReplaysBodyOnRetry(t *testing.T) {
	// The Apify call is a POST carrying the actor input. If the transport
	// retries without rebuilding the body, the second token receives an empty
	// body and the real failure surfaces as a confusing 400 instead.
	rec := &tokenRecorder{statuses: map[string]int{"dead": http.StatusPaymentRequired}}
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()

	payload := `{"query":"acme","maxItems":50}`
	client := &http.Client{Transport: NewRotatingTransport([]string{"dead", "live"}, srv.Client().Transport)}
	postThrough(t, client, srv.URL, payload)

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.bodies) != 2 {
		t.Fatalf("got %d requests, want 2", len(rec.bodies))
	}
	for i, got := range rec.bodies {
		if got != payload {
			t.Errorf("request %d body = %q, want %q", i, got, payload)
		}
	}
}

func TestRotatingTransport_DoesNotRotateOnRequestErrors(t *testing.T) {
	// These are not the token's fault — every token would hit the same wall,
	// so burning the whole pool only wastes credits and buries the real cause.
	for _, status := range []int{
		http.StatusBadRequest,          // 400 bad actor input
		http.StatusRequestTimeout,      // 408 run exceeded 300s
		http.StatusInternalServerError, // 500 Apify-side failure
	} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			rec := &tokenRecorder{statuses: map[string]int{"first": status}}
			srv := httptest.NewServer(rec.handler())
			defer srv.Close()

			client := &http.Client{Transport: NewRotatingTransport([]string{"first", "second"}, srv.Client().Transport)}
			got, _ := postThrough(t, client, srv.URL, `{}`)

			if got != status {
				t.Errorf("status = %d, want %d passed through unchanged", got, status)
			}
			if want := []string{"first"}; !reflect.DeepEqual(rec.tokensSeen(), want) {
				t.Errorf("tokens tried = %v, want %v (no rotation)", rec.tokensSeen(), want)
			}
		})
	}
}

func TestRotatingTransport_StickyAcrossCalls(t *testing.T) {
	// Once a token is known exhausted, later calls must start from the live
	// one. Restarting at tokens[0] every call would spend a doomed request on
	// every single job for the rest of the month.
	rec := &tokenRecorder{statuses: map[string]int{"dead": http.StatusPaymentRequired}}
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()

	client := &http.Client{Transport: NewRotatingTransport([]string{"dead", "live"}, srv.Client().Transport)}
	postThrough(t, client, srv.URL, `{}`)
	postThrough(t, client, srv.URL, `{}`)
	postThrough(t, client, srv.URL, `{}`)

	if want := []string{"dead", "live", "live", "live"}; !reflect.DeepEqual(rec.tokensSeen(), want) {
		t.Errorf("tokens tried = %v, want %v", rec.tokensSeen(), want)
	}
}

func TestRotatingTransport_AllTokensExhausted(t *testing.T) {
	rec := &tokenRecorder{statuses: map[string]int{
		"a": http.StatusPaymentRequired,
		"b": http.StatusPaymentRequired,
		"c": http.StatusPaymentRequired,
	}}
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()

	client := &http.Client{Transport: NewRotatingTransport([]string{"a", "b", "c"}, srv.Client().Transport)}
	got, _ := postThrough(t, client, srv.URL, `{}`)

	if got != http.StatusPaymentRequired {
		t.Errorf("status = %d, want the final 402 surfaced to the caller", got)
	}
	if want := []string{"a", "b", "c"}; !reflect.DeepEqual(rec.tokensSeen(), want) {
		t.Errorf("tokens tried = %v, want each token tried exactly once", rec.tokensSeen())
	}
}

func TestRotatingTransport_WrapsAroundToEarlierTokens(t *testing.T) {
	// Starting sticky on the last token, a failure must wrap to index 0
	// rather than give up at the end of the slice.
	rec := &tokenRecorder{statuses: map[string]int{"b": http.StatusTooManyRequests}}
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()

	rt := NewRotatingTransport([]string{"a", "b"}, srv.Client().Transport)
	rt.current = 1 // pretend an earlier call already rotated onto "b"

	client := &http.Client{Transport: rt}
	got, body := postThrough(t, client, srv.URL, `{}`)

	if got != http.StatusOK {
		t.Errorf("status = %d, want 200 after wrapping to the first token", got)
	}
	if !strings.Contains(body, `"servedBy":"a"`) {
		t.Errorf("body = %s, want the response served by token a", body)
	}
}

func TestRotatingTransport_NoTokens(t *testing.T) {
	client := &http.Client{Transport: NewRotatingTransport(nil, http.DefaultTransport)}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://api.apify.com/v2/acts/x/run-sync-get-dataset-items", strings.NewReader(`{}`))

	_, err := client.Do(req)
	if err == nil {
		t.Fatal("Do() error = nil, want an error when no tokens are configured")
	}
	if !strings.Contains(err.Error(), "no Apify tokens") {
		t.Errorf("error = %v, want it to name the missing-token cause", err)
	}
}

// closeTrackingTransport hands back a canned response whose body records
// whether it was closed.
type closeTrackingTransport struct {
	status int
	closed *int
	calls  *int
}

type trackedBody struct {
	io.Reader
	closed *int
}

func (b *trackedBody) Close() error {
	*b.closed++
	return nil
}

func (t *closeTrackingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	*t.calls++
	status := t.status
	if *t.calls > 1 {
		status = http.StatusOK
	}
	return &http.Response{
		StatusCode: status,
		Body:       &trackedBody{Reader: strings.NewReader(`{}`), closed: t.closed},
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func TestRotatingTransport_ClosesDiscardedResponses(t *testing.T) {
	// A rotated-past response is never handed to the caller, so if the
	// transport drops it without closing, that connection leaks.
	var closed, calls int
	base := &closeTrackingTransport{status: http.StatusPaymentRequired, closed: &closed, calls: &calls}

	client := &http.Client{Transport: NewRotatingTransport([]string{"a", "b"}, base)}
	resp, err := client.Do(mustPost(t, "https://apify.test/x"))
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()

	if calls != 2 {
		t.Fatalf("base transport calls = %d, want 2", calls)
	}
	if closed != 1 {
		t.Errorf("discarded bodies closed = %d, want 1", closed)
	}
}

type errTransport struct{ calls *int }

func (t *errTransport) RoundTrip(*http.Request) (*http.Response, error) {
	*t.calls++
	return nil, errors.New("dial tcp: connection refused")
}

func TestRotatingTransport_DoesNotRotateOnTransportError(t *testing.T) {
	// A network failure says nothing about the token; retrying the whole pool
	// against an unreachable host just multiplies the wait.
	var calls int
	client := &http.Client{Transport: NewRotatingTransport([]string{"a", "b", "c"}, &errTransport{calls: &calls})}

	if _, err := client.Do(mustPost(t, "https://apify.test/x")); err == nil {
		t.Fatal("Do() error = nil, want the transport error surfaced")
	}
	if calls != 1 {
		t.Errorf("base transport calls = %d, want 1 (no rotation on network error)", calls)
	}
}

func mustPost(t *testing.T, url string) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	return req
}

func TestRotatingTransport_ConcurrentRequestsAdvanceOnce(t *testing.T) {
	// Several workers share one transport. If each failure advanced the index
	// blindly, a burst of parallel jobs hitting the same exhausted token would
	// skip right over the live one — with two tokens, advancing twice lands
	// back on the dead one.
	rec := &tokenRecorder{statuses: map[string]int{"dead": http.StatusPaymentRequired}}
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()

	rt := NewRotatingTransport([]string{"dead", "live"}, srv.Client().Transport)
	rt.Logf = nil
	client := &http.Client{Transport: rt}

	const workers = 12
	var wg sync.WaitGroup
	statuses := make([]int, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL, strings.NewReader(`{}`))
			if err != nil {
				t.Errorf("build request: %v", err)
				return
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Errorf("Do() error = %v", err)
				return
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			statuses[i] = resp.StatusCode
		}(i)
	}
	wg.Wait()

	for i, got := range statuses {
		if got != http.StatusOK {
			t.Errorf("worker %d status = %d, want 200", i, got)
		}
	}

	rt.mu.Lock()
	current := rt.current
	rt.mu.Unlock()
	if current != 1 {
		t.Errorf("sticky index = %d, want 1 (parked on the live token)", current)
	}
}
