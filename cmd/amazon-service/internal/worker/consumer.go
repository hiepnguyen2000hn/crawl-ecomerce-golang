package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hiepnv/crawl-ecomerce-golang/pkg/apify"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/rabbitmq"
)

type JobMessage struct {
	JobID    string   `json:"job_id"`
	Keyword  string   `json:"keyword,omitempty"`
	URLs     []string `json:"urls,omitempty"`
	Country  string   `json:"country,omitempty"`
	MaxItems int      `json:"max_items,omitempty"`
}

type CompletedMessage struct {
	JobID string `json:"job_id"`
}

// Consumer wires a rabbitmq.Consumer to Apify and Postgres: it reads
// amazon.jobs messages, fetches Amazon product data, persists it, and
// publishes a crawl.completed.amazon event.
type Consumer struct {
	DB        *sql.DB
	Apify     apify.ProductClient
	Publisher *rabbitmq.Publisher
}

func (c *Consumer) HandleMessage(body []byte) error {
	var msg JobMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return fmt.Errorf("worker: unmarshal job message: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	result, err := c.Apify.FetchProducts(ctx, apify.ProductParams{
		Keyword:  msg.Keyword,
		URLs:     msg.URLs,
		Country:  msg.Country,
		MaxItems: msg.MaxItems,
	})
	if err != nil {
		c.markFailed(ctx, msg.JobID)
		return fmt.Errorf("worker: fetch products: %w", err)
	}

	for _, p := range result.Products {
		if _, err := c.DB.ExecContext(ctx,
			`INSERT INTO amazon_raw (job_id, asin, title, price, currency, rating, reviews_count, brand, image_url, product_url, raw)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			msg.JobID, p.ASIN, p.Title, p.Price, p.Currency, p.Rating, p.ReviewsCount, p.Brand, p.ImageURL, p.ProductURL, p.Raw,
		); err != nil {
			c.markFailed(ctx, msg.JobID)
			return fmt.Errorf("worker: insert amazon_raw: %w", err)
		}
	}

	if _, err := c.DB.ExecContext(ctx,
		`UPDATE jobs SET status = 'crawled', updated_at = now() WHERE id = $1`, msg.JobID,
	); err != nil {
		c.markFailed(ctx, msg.JobID)
		return fmt.Errorf("worker: update job status: %w", err)
	}

	payload, err := json.Marshal(CompletedMessage{JobID: msg.JobID})
	if err != nil {
		return fmt.Errorf("worker: marshal completed message: %w", err)
	}
	if err := c.Publisher.Publish(ctx, "crawl.completed.amazon", payload); err != nil {
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
		fmt.Println("worker: markFailed error:", err)
	}
}
