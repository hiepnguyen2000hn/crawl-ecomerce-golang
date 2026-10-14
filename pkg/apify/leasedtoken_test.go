package apify

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLeasedTokenTransport_SendsTheLeasedToken(t *testing.T) {
	var gotToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.URL.Query().Get("token")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := &http.Client{Transport: &LeasedTokenTransport{Base: srv.Client().Transport}}
	req, _ := http.NewRequestWithContext(
		WithLease(context.Background(), &Lease{TokenIndex: 1, Token: "tok-b"}),
		http.MethodPost, srv.URL, nil)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	resp.Body.Close()

	if gotToken != "tok-b" {
		t.Errorf("token = %q, want %q -- the request must use the account its slot was charged to", gotToken, "tok-b")
	}
}

// A request with no lease was never counted against any account's budget, so
// letting it through would reintroduce the over-subscription this prevents.
func TestLeasedTokenTransport_RefusesRequestWithoutLease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("request reached the server without a lease")
	}))
	defer srv.Close()

	client := &http.Client{Transport: &LeasedTokenTransport{Base: srv.Client().Transport}}
	req, _ := http.NewRequest(http.MethodPost, srv.URL, nil)

	if _, err := client.Do(req); err == nil {
		t.Fatal("Do succeeded without a lease, want an error")
	} else if !strings.Contains(err.Error(), "no lease") {
		t.Errorf("err = %v, want it to name the missing lease", err)
	}
}

func TestLeasedTokenTransport_ParksTokenOnCredentialFailure(t *testing.T) {
	db := testDB(t)
	s := newStore(t, db, []string{"tok-a", "tok-b"}, 5)

	for _, tc := range []struct {
		name   string
		status int
		body   string
		parked bool
		why    string
	}{
		{"402", http.StatusPaymentRequired, "", true, "402 is out of credits: non-transient"},
		{"403", http.StatusForbidden, "", true, "403 is a hard usage limit: non-transient"},
		{"401", http.StatusUnauthorized, "", true, "401 is a dead token"},
		{"429", http.StatusTooManyRequests, "", false, "429 is backpressure -- parking would retire a healthy account the moment it got busy"},
		{"400 bad input", http.StatusBadRequest, `{"error":{"type":"invalid-input","message":"bad field"}}`, false, "a genuine input error must not park a healthy token"},
		// Measured against the real API on a drained free account: a
		// pay-per-result actor reports an empty wallet as 400, not 402.
		{"400 out of credit", http.StatusBadRequest, `{"error":{"type":"max-items-must-be-greater-than-zero","message":"Maximum charged results must be greater than zero"}}`, true, "a drained account answers 400; classifying by status alone would never park it"},
		{"400 monthly limit", http.StatusBadRequest, `{"error":{"type":"monthly-usage-hard-limit-exceeded"}}`, true, "same wallet problem, different error type"},
		{"200", http.StatusOK, "[]", false, "success"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := db.Exec(`UPDATE apify_tokens SET exhausted_until = NULL`); err != nil {
				t.Fatalf("reset: %v", err)
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			client := &http.Client{Transport: &LeasedTokenTransport{
				Slots: s, Base: srv.Client().Transport, ExhaustFor: time.Hour,
			}}
			req, _ := http.NewRequestWithContext(
				WithLease(context.Background(), &Lease{TokenIndex: 0, Token: "tok-a"}),
				http.MethodPost, srv.URL, nil)

			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("Do: %v", err)
			}
			got, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if string(got) != tc.body {
				t.Errorf("body = %q, want %q -- classification must not consume it", got, tc.body)
			}

			var parked bool
			if err := db.QueryRow(
				`SELECT exhausted_until IS NOT NULL AND exhausted_until > now()
				   FROM apify_tokens WHERE token_index = 0`).Scan(&parked); err != nil {
				t.Fatalf("query: %v", err)
			}
			if parked != tc.parked {
				t.Errorf("parked = %v, want %v (%s)", parked, tc.parked, tc.why)
			}
		})
	}
}

// The response body must survive the transport untouched -- callers build
// their error messages from it.
func TestLeasedTokenTransport_PassesBodyThrough(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Write([]byte("echo:" + string(body)))
	}))
	defer srv.Close()

	client := &http.Client{Transport: &LeasedTokenTransport{Base: srv.Client().Transport}}
	req, _ := http.NewRequestWithContext(
		WithLease(context.Background(), &Lease{Token: "tok-a"}),
		http.MethodPost, srv.URL, strings.NewReader(`{"q":"x"}`))

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	got, _ := io.ReadAll(resp.Body)
	if string(got) != `echo:{"q":"x"}` {
		t.Errorf("body = %q, want the request body echoed back", got)
	}
}
