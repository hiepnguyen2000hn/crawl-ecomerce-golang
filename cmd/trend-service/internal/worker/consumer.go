package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/hiepnv/crawl-ecomerce-golang/pkg/rabbitmq"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/serpapi"
)

type JobMessage struct {
	JobID   string `json:"job_id"`
	Keyword string `json:"keyword"`
	Geo     string `json:"geo"`
}

type CompletedMessage struct {
	JobID string `json:"job_id"`
}

// Consumer wires a rabbitmq.Consumer to SerpApi and Postgres: it reads
// trend.jobs messages, fetches trend data, persists it, and publishes a
// crawl.completed.trend event.
type Consumer struct {
	DB        *sql.DB
	SerpApi   serpapi.Client
	Publisher *rabbitmq.Publisher
}

func (c *Consumer) HandleMessage(body []byte) error {
	var msg JobMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return fmt.Errorf("worker: unmarshal job message: %w", err)
	}

	ctx := context.Background()

	result, err := c.SerpApi.FetchTrend(ctx, msg.Keyword, msg.Geo)
	if err != nil {
		c.markFailed(ctx, msg.JobID)
		return fmt.Errorf("worker: fetch trend: %w", err)
	}

	if _, err := c.DB.ExecContext(ctx,
		`INSERT INTO trend_raw (job_id, keyword, trend_data) VALUES ($1, $2, $3)`,
		msg.JobID, msg.Keyword, result.Raw,
	); err != nil {
		c.markFailed(ctx, msg.JobID)
		return fmt.Errorf("worker: insert trend_raw: %w", err)
	}

	if _, err := c.DB.ExecContext(ctx,
		`UPDATE jobs SET status = 'crawled', updated_at = now() WHERE id = $1`, msg.JobID,
	); err != nil {
		return fmt.Errorf("worker: update job status: %w", err)
	}

	payload, err := json.Marshal(CompletedMessage{JobID: msg.JobID})
	if err != nil {
		return fmt.Errorf("worker: marshal completed message: %w", err)
	}
	if err := c.Publisher.Publish(ctx, "crawl.completed.trend", payload); err != nil {
		return fmt.Errorf("worker: publish completed event: %w", err)
	}
	return nil
}

// markFailed best-effort sets jobs.status = 'failed' for the given job. Any
// error from this update is logged and swallowed so it never masks the
// original error that triggered the failure.
func (c *Consumer) markFailed(ctx context.Context, jobID string) {
	if c.DB == nil {
		return
	}
	if _, err := c.DB.ExecContext(ctx,
		`UPDATE jobs SET status = 'failed', updated_at = now() WHERE id = $1`, jobID,
	); err != nil {
		fmt.Println("worker: failed to mark job as failed:", err)
	}
}
