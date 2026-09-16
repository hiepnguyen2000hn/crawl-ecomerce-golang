package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/hiepnv/crawl-ecomerce-golang/pkg/aiproviders"
)

type CompletedMessage struct {
	JobID string `json:"job_id"`
}

// Consumer reads crawl.completed.trend events, analyzes the trend data
// with an aiproviders.Provider, and persists the result.
type Consumer struct {
	DB       *sql.DB
	Provider aiproviders.Provider
}

func (c *Consumer) HandleMessage(body []byte) error {
	var msg CompletedMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return fmt.Errorf("worker: unmarshal completed message: %w", err)
	}

	ctx := context.Background()

	var trendData []byte
	err := c.DB.QueryRowContext(ctx,
		`SELECT trend_data FROM trend_raw WHERE job_id = $1 ORDER BY fetched_at DESC LIMIT 1`, msg.JobID,
	).Scan(&trendData)
	if err != nil {
		return fmt.Errorf("worker: load trend_raw: %w", err)
	}

	if _, err := c.DB.ExecContext(ctx,
		`UPDATE jobs SET status = 'ai_processing', updated_at = now() WHERE id = $1`, msg.JobID,
	); err != nil {
		return fmt.Errorf("worker: mark ai_processing: %w", err)
	}

	prompt := fmt.Sprintf("Analyze this Google Trends data and summarize the key insight in 2-3 sentences:\n%s", string(trendData))
	result, err := c.Provider.Complete(ctx, prompt)
	if err != nil {
		c.markFailed(ctx, msg.JobID)
		return fmt.Errorf("worker: ai completion: %w", err)
	}

	output, err := json.Marshal(map[string]string{"content": result.Content})
	if err != nil {
		return fmt.Errorf("worker: marshal ai output: %w", err)
	}

	if _, err := c.DB.ExecContext(ctx,
		`INSERT INTO ai_results (job_id, provider, model, prompt_type, output) VALUES ($1, 'openrouter', $2, 'analysis', $3)`,
		msg.JobID, result.Model, output,
	); err != nil {
		c.markFailed(ctx, msg.JobID)
		return fmt.Errorf("worker: insert ai_results: %w", err)
	}

	if _, err := c.DB.ExecContext(ctx,
		`UPDATE jobs SET status = 'done', updated_at = now() WHERE id = $1`, msg.JobID,
	); err != nil {
		return fmt.Errorf("worker: mark done: %w", err)
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
