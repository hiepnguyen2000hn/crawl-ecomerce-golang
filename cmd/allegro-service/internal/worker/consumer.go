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
	JobID        string   `json:"job_id"`
	Keyword      string   `json:"keyword,omitempty"`
	URL          string   `json:"url,omitempty"`
	URLs         []string `json:"urls,omitempty"`
	Pages        int      `json:"pages,omitempty"`
	MaxItems     int      `json:"max_items,omitempty"`
	WithComments *bool    `json:"with_comments,omitempty"`
	MaxComments  int      `json:"max_comments,omitempty"`
}

// commentsEnabled defaults to true when with_comments is absent from the
// message, so existing/older clients get comments now that the feature exists.
func commentsEnabled(msg JobMessage) bool {
	return msg.WithComments == nil || *msg.WithComments
}

// commentsLimit defaults to 5 comments per product when unset or non-positive.
func commentsLimit(msg JobMessage) int {
	if msg.MaxComments <= 0 {
		return 5
	}
	return msg.MaxComments
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
			URL:          targetURL,
			Pages:        pages,
			MaxItems:     msg.MaxItems,
			WithComments: commentsEnabled(msg),
			MaxComments:  commentsLimit(msg),
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

			var allegroRawID string
			if err := c.DB.QueryRowContext(ctx,
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
				) RETURNING id`,
				msg.JobID, o.OfferID, o.ProductID, o.Title, o.Brand, o.Model, o.Condition, o.Seller,
				o.Price, o.PriceWithDelivery, o.Currency, o.Rating, o.ReviewCount, o.BoughtRecently,
				o.SuperSeller, o.Smart, o.Delivery, o.Sponsored, o.Position, page, o.ImageURL, o.URL,
				specsJSON, raw,
			).Scan(&allegroRawID); err != nil {
				c.markFailed(ctx, msg.JobID)
				return fmt.Errorf("worker: insert allegro_raw: %w", err)
			}

			// Comments are best-effort: a failed insert must not fail the job.
			for _, cm := range o.Comments {
				cmRaw, err := json.Marshal(cm)
				if err != nil {
					cmRaw = []byte("{}")
				}
				if _, err := c.DB.ExecContext(ctx,
					`INSERT INTO allegro_comments (
						allegro_raw_id, job_id, offer_id, product_id, author, rating, content,
						review_date, confirmed_purchase, helpful_count, pros, cons, position, raw
					) VALUES (
						$1, $2, $3, $4, $5, $6, $7,
						$8, $9, $10, $11, $12, $13, $14
					)`,
					allegroRawID, msg.JobID, o.OfferID, o.ProductID, cm.Author, cm.Rating, cm.Content,
					cm.ReviewDate, cm.ConfirmedPurchase, cm.HelpfulCount, cm.Pros, cm.Cons, cm.Position, cmRaw,
				); err != nil {
					fmt.Println("worker: insert allegro_comments error:", err)
				}
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
