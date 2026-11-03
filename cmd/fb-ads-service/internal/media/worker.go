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
	MarkDone(ctx context.Context, id, storageKey, contentType string, size int64) error
	MarkFailed(ctx context.Context, id string, expired bool, errMsg string) error
}

// Worker handles crawl.completed.fbads: it downloads the pending media of
// the ads that job crawled. A download failure is recorded on its row and
// never fails the message -- the crawl itself already succeeded.
type Worker struct {
	Repo        Repo
	Fetcher     Fetcher
	Concurrency int
	Retries     int

	sleep func(time.Duration) // nil = time.Sleep; tests stub it out
}

func (w *Worker) HandleMessage(body []byte) error {
	var msg struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(body, &msg); err != nil || msg.JobID == "" {
		return fmt.Errorf("media: bad completed message %q: %v", body, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	rows, err := w.Repo.PendingForJob(ctx, msg.JobID)
	if err != nil {
		return fmt.Errorf("%w: %v", rabbitmq.ErrRequeue, err)
	}

	concurrency := w.Concurrency
	if concurrency <= 0 {
		concurrency = 4
	}
	var (
		wg                    sync.WaitGroup
		sem                   = make(chan struct{}, concurrency)
		done, expired, failed atomic.Int32
	)
	for _, r := range rows {
		wg.Add(1)
		sem <- struct{}{}
		go func(r Row) {
			defer wg.Done()
			defer func() { <-sem }()
			switch w.download(ctx, r) {
			case "done":
				done.Add(1)
			case "expired":
				expired.Add(1)
			default:
				failed.Add(1)
			}
		}(r)
	}
	wg.Wait()

	fmt.Printf("fbads media: job %s: %d done, %d expired, %d failed\n", msg.JobID, done.Load(), expired.Load(), failed.Load())
	return nil
}

// download fetches one row with retry/backoff and records the outcome.
func (w *Worker) download(ctx context.Context, r Row) string {
	retries := w.Retries
	if retries <= 0 {
		retries = 3
	}
	sleep := w.sleep
	if sleep == nil {
		sleep = time.Sleep
	}

	var err error
	for attempt := 0; attempt < retries; attempt++ {
		if attempt > 0 {
			sleep(time.Duration(1<<(attempt-1)) * time.Second)
		}
		var res Result
		res, err = w.Fetcher.Fetch(ctx, r)
		if err == nil {
			if mErr := w.Repo.MarkDone(ctx, r.ID, res.StorageKey, res.ContentType, res.Size); mErr != nil {
				fmt.Println("fbads media:", mErr)
			}
			return "done"
		}
		if errors.Is(err, ErrExpired) {
			if mErr := w.Repo.MarkFailed(ctx, r.ID, true, err.Error()); mErr != nil {
				fmt.Println("fbads media:", mErr)
			}
			return "expired"
		}
	}
	if mErr := w.Repo.MarkFailed(ctx, r.ID, false, err.Error()); mErr != nil {
		fmt.Println("fbads media:", mErr)
	}
	return "failed"
}
