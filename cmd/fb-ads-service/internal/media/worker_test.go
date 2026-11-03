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
	w := &Worker{Repo: &fakeRepo{pendErr: errors.New("db down")}, Fetcher: &fakeFetcher{}}
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
