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
	Keywords []string `json:"keywords,omitempty"`
	OfferIDs []string `json:"offer_ids,omitempty"`
	MaxItems int      `json:"max_items,omitempty"`
}

type CompletedMessage struct {
	JobID string `json:"job_id"`
}

// Consumer wires a rabbitmq.Consumer to Apify and Postgres: it reads
// china1688.jobs messages, fetches 1688 wholesale product data, persists it,
// and publishes a crawl.completed.china1688 event.
type Consumer struct {
	DB        *sql.DB
	Apify     apify.China1688Client
	Publisher *rabbitmq.Publisher
}

func (c *Consumer) HandleMessage(body []byte) error {
	var msg JobMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return fmt.Errorf("worker: unmarshal job message: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	result, err := c.Apify.FetchProducts(ctx, apify.China1688Params{
		Keywords: msg.Keywords,
		OfferIDs: msg.OfferIDs,
		MaxItems: msg.MaxItems,
	})
	if err != nil {
		c.markFailed(ctx, msg.JobID)
		return fmt.Errorf("worker: fetch products: %w", err)
	}

	for _, p := range result.Products {
		if _, err := c.DB.ExecContext(ctx,
			`INSERT INTO china1688_raw (job_id, offer_id, title, price_min, price_max, currency, moq, image_url, supplier_name, supplier_province, detail_url, raw)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
			msg.JobID, p.OfferID, p.Title, p.PriceMin, p.PriceMax, p.Currency, p.MOQ, p.ImageURL, p.SupplierName, p.SupplierProvince, p.DetailURL, p.Raw,
		); err != nil {
			c.markFailed(ctx, msg.JobID)
			return fmt.Errorf("worker: insert china1688_raw: %w", err)
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
	if err := c.Publisher.Publish(ctx, "crawl.completed.china1688", payload); err != nil {
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
