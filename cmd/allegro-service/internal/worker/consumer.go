package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/allegro-service/internal/crawler"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/rabbitmq"
)

type JobMessage struct {
	JobID    string   `json:"job_id"`
	Keyword  string   `json:"keyword,omitempty"`
	URL      string   `json:"url,omitempty"`
	URLs     []string `json:"urls,omitempty"`
	Pages    int      `json:"pages,omitempty"`
	MaxItems int      `json:"max_items,omitempty"`
}

type CompletedMessage struct {
	JobID string `json:"job_id"`
}

// Consumer wires a rabbitmq.Consumer to Camoufox+YOLO26 and Postgres:
// it reads allegro.jobs messages, fetches Allegro listing products,
// extracts title/brand/model/specs/price, saves them to allegro_raw,
// and publishes crawl.completed.allegro.
type Consumer struct {
	DB        *sql.DB
	Crawler   crawler.Client
	Publisher *rabbitmq.Publisher
	Timeout   time.Duration
}

func (c *Consumer) HandleMessage(body []byte) error {
	var msg JobMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return fmt.Errorf("worker: unmarshal job message: %w", err)
	}

	targets := c.resolveTargets(msg)
	if len(targets) == 0 {
		c.markFailed(context.Background(), msg.JobID)
		return fmt.Errorf("worker: job %s has neither keyword nor url(s)", msg.JobID)
	}

	timeout := c.Timeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	pages := msg.Pages
	if pages <= 0 {
		pages = 1
	}

	for _, targetURL := range targets {
		res, err := c.Crawler.Fetch(ctx, crawler.FetchParams{
			URL:      targetURL,
			Pages:    pages,
			MaxItems: msg.MaxItems,
		})
		if err != nil {
			c.markFailed(ctx, msg.JobID)
			return fmt.Errorf("worker: fetch allegro listing %s: %w", targetURL, err)
		}

		for _, o := range res.Offers {
			raw, err := json.Marshal(o)
			if err != nil {
				c.markFailed(ctx, msg.JobID)
				return fmt.Errorf("worker: marshal offer: %w", err)
			}
			specsJSON, err := json.Marshal(o.Specs)
			if err != nil {
				specsJSON = []byte("{}")
			}

			page := o.Page
			if page <= 0 {
				page = 1
			}

			if _, err := c.DB.ExecContext(ctx,
				`INSERT INTO allegro_raw (
					job_id, offer_id, product_id, title, brand, model, condition, seller,
					price, price_with_delivery, currency, rating, reviews_count, bought_recently,
					super_seller, smart, delivery, sponsored, position, page, image_url, product_url,
					specs, raw
				) VALUES (
					$1, $2, $3, $4, $5, $6, $7, $8,
					$9, $10, $11, $12, $13, $14,
					$15, $16, $17, $18, $19, $20, $21, $22,
					$23, $24
				)`,
				msg.JobID, o.OfferID, o.ProductID, o.Title, o.Brand, o.Model, o.Condition, o.Seller,
				o.Price, o.PriceWithDelivery, o.Currency, o.Rating, o.ReviewCount, o.BoughtRecently,
				o.SuperSeller, o.Smart, o.Delivery, o.Sponsored, o.Position, page, o.ImageURL, o.URL,
				specsJSON, raw,
			); err != nil {
				c.markFailed(ctx, msg.JobID)
				return fmt.Errorf("worker: insert allegro_raw: %w", err)
			}
		}
	}

	if _, err := c.DB.ExecContext(ctx,
		`UPDATE jobs SET status = 'crawled', updated_at = now() WHERE id = $1`, msg.JobID,
	); err != nil {
		c.markFailed(ctx, msg.JobID)
		return fmt.Errorf("worker: update job status: %w", err)
	}

	if c.Publisher != nil {
		payload, err := json.Marshal(CompletedMessage{JobID: msg.JobID})
		if err != nil {
			return fmt.Errorf("worker: marshal completed message: %w", err)
		}
		if err := c.Publisher.Publish(ctx, "crawl.completed.allegro", payload); err != nil {
			return fmt.Errorf("worker: publish completed event: %w", err)
		}
	}
	return nil
}

func (c *Consumer) resolveTargets(msg JobMessage) []string {
	var out []string
	if msg.URL != "" {
		out = append(out, msg.URL)
	}
	for _, u := range msg.URLs {
		if u != "" {
			out = append(out, u)
		}
	}
	if len(out) == 0 && msg.Keyword != "" {
		q := url.Values{}
		q.Set("string", msg.Keyword)
		out = append(out, "https://allegro.pl/listing?"+q.Encode())
	}
	return out
}

func (c *Consumer) markFailed(ctx context.Context, jobID string) {
	if c.DB == nil || jobID == "" {
		return
	}
	if _, err := c.DB.ExecContext(ctx,
		`UPDATE jobs SET status = 'failed', updated_at = now() WHERE id = $1`, jobID,
	); err != nil {
		fmt.Println("worker: markFailed error:", err)
	}
}
