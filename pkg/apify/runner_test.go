package apify

import (
	"context"

	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunSync_PostsToTheActorAndDecodes(t *testing.T) {
	db := testDB(t)
	s := newStore(t, db, []string{"tok-a"}, 5)

	var gotPath, gotBody, gotToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotToken = r.URL.Query().Get("token")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Write([]byte(`[{"id":"a"},{"id":"b"}]`))
	}))
	defer srv.Close()

	r := NewRunner(srv.URL, s, time.Minute)
	raw, err := r.RunSync(context.Background(), "acme~scraper", "job-1", map[string]any{"q": "laptop"})
	if err != nil {
		t.Fatalf("RunSync: %v", err)
	}

	if want := "/acts/acme~scraper/run-sync-get-dataset-items"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
	if gotToken != "tok-a" {
		t.Errorf("token = %q, want tok-a", gotToken)
	}
	if !strings.Contains(gotBody, `"q":"laptop"`) {
		t.Errorf("body = %q, want the input marshalled", gotBody)
	}

	type item struct {
		ID string `json:"id"`
	}
	items, err := DecodeItems[item](raw)
	if err != nil {
		t.Fatalf("DecodeItems: %v", err)
	}
	if len(items) != 2 || items[0].ID != "a" || items[1].ID != "b" {
		t.Errorf("items = %+v, want a and b", items)
	}
	if string(raw[0]) != `{"id":"a"}` {
		t.Errorf("raw[0] = %s, want the original bytes preserved", raw[0])
	}
}

// The whole point of the design: 10 jobs, 2 accounts of 5, and the actor is
// never running more than 10 at once nor more than 5 per account.
func TestRunSync_NeverExceedsPerAccountConcurrency(t *testing.T) {
	db := testDB(t)
	s := newStore(t, db, []string{"tok-a", "tok-b"}, 5)

	var mu sync.Mutex
	inflight := map[string]int{}
	var maxTotal, maxPerToken int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := r.URL.Query().Get("token")

		mu.Lock()
		inflight[tok]++
		total := 0
		for _, n := range inflight {
			total += n
		}
		per := inflight[tok]
		mu.Unlock()

		if int64(total) > atomic.LoadInt64(&maxTotal) {
			atomic.StoreInt64(&maxTotal, int64(total))
		}
		if int64(per) > atomic.LoadInt64(&maxPerToken) {
			atomic.StoreInt64(&maxPerToken, int64(per))
		}

		time.Sleep(120 * time.Millisecond) // hold the slot like a real run

		mu.Lock()
		inflight[tok]--
		mu.Unlock()
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	r := NewRunner(srv.URL, s, time.Minute)

	const jobs = 24
	var wg sync.WaitGroup
	for i := 0; i < jobs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := r.RunSync(context.Background(), "actor", "", nil); err != nil {
				t.Errorf("RunSync: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt64(&maxPerToken); got > 5 {
		t.Errorf("peak concurrency on one account = %d, want <= 5", got)
	}
	if got := atomic.LoadInt64(&maxTotal); got > 10 {
		t.Errorf("peak total concurrency = %d, want <= 10 (2 accounts x 5)", got)
	}
	// Guard against the opposite failure: a cap that holds because nothing
	// ever ran in parallel would pass the checks above but defeat the point.
	if got := atomic.LoadInt64(&maxTotal); got < 6 {
		t.Errorf("peak total concurrency = %d, want real parallelism across both accounts", got)
	}
}

// Queueing behind a full pool is backpressure, not a bad job: it must be
// distinguishable so the consumer requeues instead of dead-lettering.
func TestRunSync_ReturnsErrBusyWhenPoolStaysFull(t *testing.T) {
	db := testDB(t)
	s := newStore(t, db, []string{"tok-a"}, 1)

	if _, err := s.TryAcquire(context.Background(), "actor", "holder"); err != nil {
		t.Fatalf("hold the only slot: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	r := NewRunner(srv.URL, s, time.Minute)
	r.AcquireTimeout = 150 * time.Millisecond

	_, err := r.RunSync(context.Background(), "actor", "waiter", nil)
	var busy *ErrBusy
	if !errors.As(err, &busy) {
		t.Fatalf("err = %v (%T), want *ErrBusy", err, err)
	}
}

func TestRunSync_ReleasesSlotOnActorFailure(t *testing.T) {
	db := testDB(t)
	s := newStore(t, db, []string{"tok-a"}, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"input schema invalid"}}`))
	}))
	defer srv.Close()

	r := NewRunner(srv.URL, s, time.Minute)
	_, err := r.RunSync(context.Background(), "actor", "job-1", nil)
	if err == nil {
		t.Fatal("want an error on HTTP 400")
	}
	if !strings.Contains(err.Error(), "input schema invalid") {
		t.Errorf("err = %v, want Apify's own explanation included", err)
	}

	// A leaked lease would shrink the pool permanently.
	if n := liveLeases(t, db, 0); n != 0 {
		t.Errorf("%d lease(s) still held after a failed run, want 0", n)
	}
}

func TestRunSync_ReleasesSlotOnSuccess(t *testing.T) {
	db := testDB(t)
	s := newStore(t, db, []string{"tok-a"}, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	r := NewRunner(srv.URL, s, time.Minute)
	for i := 0; i < 3; i++ {
		if _, err := r.RunSync(context.Background(), "actor", "", nil); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	if n := liveLeases(t, db, 0); n != 0 {
		t.Errorf("%d lease(s) held after 3 clean runs, want 0", n)
	}
}
