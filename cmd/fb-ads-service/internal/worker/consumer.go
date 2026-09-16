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
	JobID        string   `json:"job_id"`
	Query        string   `json:"query,omitempty"`
	PageID       string   `json:"page_id,omitempty"`
	Country      string   `json:"country,omitempty"`
	Category     string   `json:"category,omitempty"`
	MediaType    string   `json:"media_type,omitempty"`
	SortBy       string   `json:"sort_by,omitempty"`
	ActiveStatus string   `json:"active_status,omitempty"`
	MinDate      string   `json:"min_date,omitempty"`
	MaxDate      string   `json:"max_date,omitempty"`
	MaxItems     int      `json:"max_items,omitempty"`
	Advertisers  []string `json:"advertisers,omitempty"`
	FetchDetails bool     `json:"fetch_details,omitempty"`
}

type CompletedMessage struct {
	JobID string `json:"job_id"`
}

// Consumer wires a rabbitmq.Consumer to Apify and Postgres: it reads
// fbads.jobs messages, fetches Facebook Ads Library data, persists it, and
// publishes a crawl.completed.fbads event.
type Consumer struct {
	DB        *sql.DB
	Apify     apify.Client
	Publisher *rabbitmq.Publisher
}

func (c *Consumer) HandleMessage(body []byte) error {
	var msg JobMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return fmt.Errorf("worker: unmarshal job message: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	result, err := c.Apify.FetchAds(ctx, apify.AdParams{
		Query:        msg.Query,
		PageID:       msg.PageID,
		Country:      msg.Country,
		Category:     msg.Category,
		MediaType:    msg.MediaType,
		SortBy:       msg.SortBy,
		ActiveStatus: msg.ActiveStatus,
		MinDate:      msg.MinDate,
		MaxDate:      msg.MaxDate,
		MaxItems:     msg.MaxItems,
		Advertisers:  msg.Advertisers,
		FetchDetails: msg.FetchDetails,
	})
	if err != nil {
		c.markFailed(ctx, msg.JobID)
		return fmt.Errorf("worker: fetch ads: %w", err)
	}

	for _, ad := range result.Ads {
		if _, err := c.DB.ExecContext(ctx,
			`INSERT INTO fbads_raw (job_id, ad_archive_id, page_id, page_name, is_active, start_date, end_date, body, title, cta_text, cta_type, link_url, raw)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
			msg.JobID, ad.AdArchiveID, ad.PageID, ad.PageName, ad.IsActive, ad.StartDate, ad.EndDate, ad.Body, ad.Title, ad.CtaText, ad.CtaType, ad.LinkURL, ad.Raw,
		); err != nil {
			c.markFailed(ctx, msg.JobID)
			return fmt.Errorf("worker: insert fbads_raw: %w", err)
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
	if err := c.Publisher.Publish(ctx, "crawl.completed.fbads", payload); err != nil {
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
