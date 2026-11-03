package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/hiepnv/crawl-ecomerce-golang/pkg/rabbitmq"
)

type fakeRepo struct {
	mu        sync.Mutex
	rows      []Row
	pendErr   error
	jobID     string
	done      []string
	failed    map[string]bool // id -> expired
	failedMsg map[string]string
	retryable []Row
}

func (f *fakeRepo) Retryable(ctx context.Context, limit int) ([]Row, error) {
	return f.retryable, nil
}

func (f *fakeRepo) PendingForJob(ctx context.Context, jobID string) ([]Row, error) {
	f.jobID = jobID
	return f.rows, f.pendErr
}

func (f *fakeRepo) MarkDone(ctx context.Context, id, key, ct string, size int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.done = append(f.done, id)
	return nil
}

func (f *fakeRepo) MarkFailed(ctx context.Context, id string, expired bool, msg string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failed == nil {
		f.failed, f.failedMsg = map[string]bool{}, map[string]string{}
	}
	f.failed[id] = expired
	f.failedMsg[id] = msg
	return nil
}

type fakeFetcher struct {
	mu    sync.Mutex
	calls map[string]int
	errs  map[string]error // by row ID; missing = success
}

func (f *fakeFetcher) Fetch(ctx context.Context, r Row) (Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[r.ID]++
	if err := f.errs[r.ID]; err != nil {
		return Result{}, err
	}
	return Result{StorageKey: "fbads/x/" + r.ID + ".jpg", ContentType: "image/jpeg", Size: 1}, nil
}

func jobMsg(id string) []byte {
	b, _ := json.Marshal(map[string]string{"job_id": id})
	return b
}

func TestWorker_MixedOutcomes(t *testing.T) {
	repo := &fakeRepo{rows: []Row{{ID: "ok"}, {ID: "gone"}, {ID: "flaky"}}}
	fetch := &fakeFetcher{errs: map[string]error{
		"gone":  fmt.Errorf("%w: http 403", ErrExpired),
		"flaky": errors.New("connection reset"),
	}}
	var slept []time.Duration
	var sleptMu sync.Mutex
	w := &Worker{Repo: repo, Fetcher: fetch, Concurrency: 2, Retries: 3,
		sleep: func(d time.Duration) { sleptMu.Lock(); slept = append(slept, d); sleptMu.Unlock() }}

	if err := w.HandleMessage(jobMsg("job-1")); err != nil {
		t.Fatalf("HandleMessage = %v, want nil", err)
	}
	if repo.jobID != "job-1" {
		t.Errorf("PendingForJob job = %q", repo.jobID)
	}
	if len(repo.done) != 1 || repo.done[0] != "ok" {
		t.Errorf("done = %v", repo.done)
	}
	if exp, ok := repo.failed["gone"]; !ok || !exp {
		t.Errorf("gone not marked expired: %v", repo.failed)
	}
	if exp, ok := repo.failed["flaky"]; !ok || exp || repo.failedMsg["flaky"] != "connection reset" {
		t.Errorf("flaky not marked failed: %v %v", repo.failed, repo.failedMsg)
	}
	if fetch.calls["gone"] != 1 || fetch.calls["flaky"] != 3 || fetch.calls["ok"] != 1 {
		t.Errorf("fetch calls = %v, want ok:1 gone:1 flaky:3", fetch.calls)
	}
	if len(slept) != 2 || slept[0] != time.Second || slept[1] != 2*time.Second {
		t.Errorf("backoff = %v, want [1s 2s]", slept)
	}
}

func TestWorker_RepoErrorRequeues(t *testing.T) {
	w := &Worker{Repo: &fakeRepo{pendErr: errors.New("db down")}, Fetcher: &fakeFetcher{}, sleep: func(time.Duration) {}}
	if err := w.HandleMessage(jobMsg("job-1")); !errors.Is(err, rabbitmq.ErrRequeue) {
		t.Fatalf("err = %v, want ErrRequeue", err)
	}
}

func TestWorker_BadMessageIsDropped(t *testing.T) {
	w := &Worker{Repo: &fakeRepo{}, Fetcher: &fakeFetcher{}}
	if err := w.HandleMessage([]byte("not json")); err == nil || errors.Is(err, rabbitmq.ErrRequeue) {
		t.Fatalf("err = %v, want a non-requeue error", err)
	}
}

// blockingFetcher waits for the context to end, like a stalled fbcdn request.
type blockingFetcher struct {
	mu    sync.Mutex
	calls int
}

func (f *blockingFetcher) Fetch(ctx context.Context, r Row) (Result, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	<-ctx.Done()
	return Result{}, ctx.Err()
}

func TestWorker_TimeoutLeavesRowsPendingAndRequeues(t *testing.T) {
	repo := &fakeRepo{rows: []Row{{ID: "a"}, {ID: "b"}, {ID: "c"}}}
	fetch := &blockingFetcher{}
	var slept int
	w := &Worker{Repo: repo, Fetcher: fetch, Concurrency: 1, Retries: 3, Timeout: 50 * time.Millisecond,
		sleep: func(time.Duration) { slept++ }}

	err := w.HandleMessage(jobMsg("job-1"))
	if !errors.Is(err, rabbitmq.ErrRequeue) {
		t.Fatalf("err = %v, want ErrRequeue so the rest is retried", err)
	}
	if len(repo.failed) != 0 || len(repo.done) != 0 {
		t.Errorf("rows were marked after the deadline: done=%v failed=%v", repo.done, repo.failed)
	}
	if fetch.calls != 1 {
		t.Errorf("fetch calls = %d, want 1 (no new work after the deadline)", fetch.calls)
	}
	if slept != 0 {
		t.Errorf("backoff slept %d times after the deadline", slept)
	}
}

func TestWorker_RepoErrorBacksOffBeforeRequeue(t *testing.T) {
	var slept []time.Duration
	w := &Worker{Repo: &fakeRepo{pendErr: errors.New("db down")}, Fetcher: &fakeFetcher{},
		sleep: func(d time.Duration) { slept = append(slept, d) }}
	if err := w.HandleMessage(jobMsg("job-1")); !errors.Is(err, rabbitmq.ErrRequeue) {
		t.Fatalf("err = %v", err)
	}
	if len(slept) != 1 || slept[0] < 5*time.Second {
		t.Fatalf("slept = %v, want one pause of >= 5s before requeue", slept)
	}
}

func TestWorker_SweepRetriesStaleRows(t *testing.T) {
	repo := &fakeRepo{retryable: []Row{{ID: "stale"}, {ID: "gone"}}}
	fetch := &fakeFetcher{errs: map[string]error{"gone": ErrExpired}}
	w := &Worker{Repo: repo, Fetcher: fetch, sleep: func(time.Duration) {}}

	if err := w.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(repo.done) != 1 || repo.done[0] != "stale" || !repo.failed["gone"] {
		t.Fatalf("done=%v failed=%v", repo.done, repo.failed)
	}
}
