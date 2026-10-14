package apify

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
)

// ParseTokens splits a comma-separated APIFY_API_TOKEN value into usable
// tokens, dropping blanks, duplicates, and unexpanded "${...}" placeholders.
// A single token parses to a one-element list, so the multi-token form stays
// backward compatible with a plain one-token env value.
func ParseTokens(raw string) []string {
	var tokens []string
	seen := make(map[string]bool)
	for _, part := range strings.Split(raw, ",") {
		token := strings.TrimSpace(part)
		if token == "" || strings.Contains(token, "${") || seen[token] {
			continue
		}
		seen[token] = true
		tokens = append(tokens, token)
	}
	return tokens
}

// credentialScoped reports whether a status means "this token cannot do the
// job", as opposed to "this job cannot be done". Only the former is worth
// retrying with a different token.
//
//	401 token invalid or revoked
//	402 out of credits / usage limit exceeded — the common exhaustion case
//	403 monthly usage hard limit reached
//	429 rate-limit-exceeded (counted per user, so another account has its own)
//
// Everything else (400 bad input, 408 run over 300s, 5xx on Apify's side)
// would fail identically on every token, so rotating would only spend the
// whole pool on a doomed request and bury the real cause under repeats.
func credentialScoped(status int) bool {
	switch status {
	case http.StatusUnauthorized, http.StatusPaymentRequired, http.StatusForbidden, http.StatusTooManyRequests:
		return true
	default:
		return false
	}
}

// RotatingTransport injects an Apify API token into each request's "token"
// query param and retries with the next token when the current one is out of
// credits or rate limited.
//
// It owns the token entirely — the Apify clients no longer set the query
// param themselves, so a client is only ever as authenticated as the
// transport it was given.
//
// The chosen index is sticky: once a token is known exhausted, later requests
// start from the live one instead of re-spending a doomed request on every
// job. State is per-process, so each service discovers an exhausted token
// independently — one wasted request each, in exchange for needing no shared
// store.
type RotatingTransport struct {
	mu      sync.Mutex
	tokens  []string
	current int
	base    http.RoundTripper

	// Logf reports rotations. Nil disables logging; NewRotatingTransport
	// defaults it to log.Printf.
	Logf func(format string, args ...any)
}

func NewRotatingTransport(tokens []string, base http.RoundTripper) *RotatingTransport {
	if base == nil {
		base = http.DefaultTransport
	}
	return &RotatingTransport{tokens: tokens, base: base, Logf: log.Printf}
}

// nextToken returns the token to try now, along with its index.
func (t *RotatingTransport) nextToken() (string, int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.tokens[t.current], t.current
}

// advance moves the sticky index past the token at idx, wrapping around.
// Concurrent callers that already rotated away from idx win, so a burst of
// parallel jobs hitting the same exhausted token only advances once.
func (t *RotatingTransport) advance(idx int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.current == idx {
		t.current = (idx + 1) % len(t.tokens)
	}
}

func (t *RotatingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// RoundTrip owns the request body under the http.RoundTripper contract,
	// and must close it even on error.
	var body []byte
	if req.Body != nil {
		var err error
		body, err = io.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("apify: buffer request body: %w", err)
		}
	}

	if len(t.tokens) == 0 {
		return nil, errors.New("apify: no Apify tokens configured")
	}

	var resp *http.Response
	for attempt := 0; attempt < len(t.tokens); attempt++ {
		token, idx := t.nextToken()

		var err error
		resp, err = t.base.RoundTrip(cloneWithToken(req, token, body))
		if err != nil {
			// A network failure says nothing about the token; retrying the
			// pool against an unreachable host only multiplies the wait.
			return nil, err
		}
		if !credentialScoped(resp.StatusCode) {
			return resp, nil
		}

		t.advance(idx)

		// On the last attempt there is nothing left to rotate to, so hand
		// this response back unread — the caller needs its body for the
		// error message it builds.
		if attempt == len(t.tokens)-1 {
			if t.Logf != nil {
				t.Logf("apify: all %d token(s) exhausted, last status HTTP %d", len(t.tokens), resp.StatusCode)
			}
			return resp, nil
		}

		// This response is never handed to the caller. Drain and close it so
		// the connection goes back to the pool instead of leaking.
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		if t.Logf != nil {
			t.Logf("apify: token %d/%d got HTTP %d, rotating to the next token", idx+1, len(t.tokens), resp.StatusCode)
		}
	}

	return resp, nil
}

func cloneWithToken(req *http.Request, token string, body []byte) *http.Request {
	clone := req.Clone(req.Context())

	q := clone.URL.Query()
	q.Set("token", token)
	clone.URL.RawQuery = q.Encode()

	if body != nil {
		clone.Body = io.NopCloser(bytes.NewReader(body))
		clone.ContentLength = int64(len(body))
		clone.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(body)), nil
		}
	}
	return clone
}
