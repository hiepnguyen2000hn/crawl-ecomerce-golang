package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hiepnv/crawl-ecomerce-golang/pkg/rabbitmq"
)

type Fetcher interface {
	Fetch(ctx context.Context, r Row) (Result, error)
}

type Repo interface {
	PendingForJob(ctx context.Context, jobID string) ([]Row, error)
	Retryable(ctx context.Context, limit int) ([]Row, error)
	MarkDone(ctx context.Context, id, storageKey, contentType string, size int64) error
	MarkFailed(ctx context.Context, id string, expired bool, errMsg string) error
}

// Worker downloads pending media. HandleMessage serves crawl.completed.fbads
// for one job; Sweep is the periodic safety net for rows whose message was
// lost or that failed transiently. A download failure is recorded on its row
// and never fails the crawl itself.
type Worker struct {
	Repo        Repo
	Fetcher     Fetcher
	Concurrency int
	Retries     int
	Timeout     time.Duration // per job message; 0 = 10 minutes

	sleep func(time.Duration) // nil = context-aware sleep; tests stub it out
}

const (
	sweepBatch     = 200
	requeueBackoff = 5 * time.Second
)

type outcome int

const (
	outDone outcome = iota
	outExpired
	outFailed
	outAborted // context ended first; row left pending for a retry
)

func (w *Worker) HandleMessage(body []byte) error {
	var msg struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(body, &msg); err != nil || msg.JobID == "" {
		return fmt.Errorf("media: bad completed message %q: %v", body, err)
	}

	timeout := w.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	rows, err := w.Repo.PendingForJob(ctx, msg.JobID)
	if err != nil {
		// Requeue puts the message straight back at the head of the queue;
		// pause first so a DB outage does not become a hot loop.
		w.wait(context.Background(), requeueBackoff)
		return fmt.Errorf("%w: %v", rabbitmq.ErrRequeue, err)
	}

	aborted := w.process(ctx, "job "+msg.JobID, rows)
	if aborted > 0 {
		// Rows still pending: redeliver, PendingForJob picks up only what is left.
		return fmt.Errorf("%w: job %s: %d media left after %s", rabbitmq.ErrRequeue, msg.JobID, aborted, timeout)
	}
	return nil
}

// Sweep retries stale pending rows and transiently failed ones whose URL is
// still valid. Run it periodically.
func (w *Worker) Sweep(ctx context.Context) error {
	rows, err := w.Repo.Retryable(ctx, sweepBatch)
	if err != nil {
		return err
	}
	if len(rows) > 0 {
		w.process(ctx, "sweep", rows)
	}
	return nil
}

// process downloads rows in parallel and returns how many were not attempted
// to completion because ctx ended.
func (w *Worker) process(ctx context.Context, label string, rows []Row) (aborted int) {
	concurrency := w.Concurrency
	if concurrency <= 0 {
		concurrency = 4
	}
	var (
		wg     sync.WaitGroup
		sem    = make(chan struct{}, concurrency)
		counts [4]atomic.Int32
	)
	for i, r := range rows {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
		}
		if ctx.Err() != nil {
			counts[outAborted].Add(int32(len(rows) - i))
			break
		}
		wg.Add(1)
		go func(r Row) {
			defer wg.Done()
			defer func() { <-sem }()
			counts[w.download(ctx, r)].Add(1)
		}(r)
	}
	wg.Wait()

	fmt.Printf("fbads media: %s: %d done, %d expired, %d failed, %d left pending\n", label,
		counts[outDone].Load(), counts[outExpired].Load(), counts[outFailed].Load(), counts[outAborted].Load())
	return int(counts[outAborted].Load())
}

// download fetches one row with retry/backoff and records the outcome. If the
// context ends first the row is left untouched (still pending).
func (w *Worker) download(ctx context.Context, r Row) outcome {
	retries := w.Retries
	if retries <= 0 {
		retries = 3
	}

	var err error
	for attempt := 0; attempt < retries; attempt++ {
		if attempt > 0 && !w.wait(ctx, time.Duration(1<<(attempt-1))*time.Second) {
			return outAborted
		}
		var res Result
		res, err = w.Fetcher.Fetch(ctx, r)
		if err == nil {
			if mErr := w.Repo.MarkDone(ctx, r.ID, res.StorageKey, res.ContentType, res.Size); mErr != nil {
				fmt.Println("fbads media:", mErr)
			}
			return outDone
		}
		if ctx.Err() != nil {
			return outAborted
		}
		if errors.Is(err, ErrExpired) {
			if mErr := w.Repo.MarkFailed(ctx, r.ID, true, err.Error()); mErr != nil {
				fmt.Println("fbads media:", mErr)
			}
			return outExpired
		}
	}
	if mErr := w.Repo.MarkFailed(ctx, r.ID, false, err.Error()); mErr != nil {
		fmt.Println("fbads media:", mErr)
	}
	return outFailed
}

// wait pauses for d unless ctx ends first; it reports whether to carry on.
func (w *Worker) wait(ctx context.Context, d time.Duration) bool {
	if ctx.Err() != nil {
		return false
	}
	if w.sleep != nil {
		w.sleep(d)
		return ctx.Err() == nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}
