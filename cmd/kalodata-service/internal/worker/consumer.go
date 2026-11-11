package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/hiepnv/crawl-ecomerce-golang/pkg/kalodata"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/rabbitmq"
)

type JobMessage struct {
	JobID            string `json:"job_id"`
	Country          string `json:"country,omitempty"`
	StartDate        string `json:"start_date,omitempty"`
	EndDate          string `json:"end_date,omitempty"`
	PageNo           int    `json:"page_no,omitempty"`
	PageSize         int    `json:"page_size,omitempty"`
	SortField        string `json:"sort_field,omitempty"`
	SortType         string `json:"sort_type,omitempty"`
	CategoryIDs      []int  `json:"category_ids,omitempty"`
	Revenue          string `json:"revenue,omitempty"`
	UnitPrice        string `json:"unit_price,omitempty"`
	SalesChannel     string `json:"sales_channel,omitempty"`
	CommissionRate   string `json:"commission_rate,omitempty"`
	Creator          string `json:"creator,omitempty"`
	WithProducts     *bool  `json:"with_products,omitempty"`
	WithVideos       *bool  `json:"with_videos,omitempty"`
	WithVideoDetail  bool   `json:"with_video_detail,omitempty"`
	VideoType        string `json:"video_type,omitempty"`
	Duration         string `json:"duration,omitempty"`
	CreatorFollowers string `json:"creator_followers,omitempty"`
}

type CompletedMessage struct {
	JobID string `json:"job_id"`
}

// Consumer reads kalodata.jobs messages, crawls Kalodata products and videos via
// the cookie-rotating client, persists them, and publishes crawl.completed.kalodata.
type Consumer struct {
	DB        *sql.DB
	Client    kalodata.Client
	Publisher *rabbitmq.Publisher
}

func enabled(p *bool) bool { return p == nil || *p }

func (c *Consumer) HandleMessage(body []byte) error {
	var msg JobMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return fmt.Errorf("worker: unmarshal job message: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Idempotent re-run: a redelivered message (crash before ack) must not
	// duplicate rows, so clear any prior rows for this job before inserting.
	if c.DB != nil {
		if _, err := c.DB.ExecContext(ctx, `DELETE FROM kalodata_products WHERE job_id = $1`, msg.JobID); err != nil {
			return fmt.Errorf("worker: clear kalodata_products: %w", err)
		}
		if _, err := c.DB.ExecContext(ctx, `DELETE FROM kalodata_videos WHERE job_id = $1`, msg.JobID); err != nil {
			return fmt.Errorf("worker: clear kalodata_videos: %w", err)
		}
	}

	country := msg.Country
	if country == "" {
		country = "VN"
	}
	pageNo := msg.PageNo
	if pageNo <= 0 {
		pageNo = 1
	}
	pageSize := msg.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	sortField := msg.SortField
	if sortField == "" {
		sortField = "revenue"
	}
	sortType := msg.SortType
	if sortType == "" {
		sortType = "DESC"
	}

	startDate, endDate := msg.StartDate, msg.EndDate
	if startDate == "" || endDate == "" {
		if day, err := c.Client.AllLastDay(ctx, country); err == nil && day != "" {
			if startDate == "" {
				startDate = day
			}
			if endDate == "" {
				endDate = day
			}
		}
	}

	if enabled(msg.WithProducts) {
		products, err := c.Client.SearchProducts(ctx, kalodata.ProductParams{
			Country: country, StartDate: startDate, EndDate: endDate,
			PageNo: pageNo, PageSize: pageSize, SortField: sortField, SortType: sortType,
			CategoryIDs: msg.CategoryIDs, Revenue: msg.Revenue, UnitPrice: msg.UnitPrice,
			SalesChannel: msg.SalesChannel, CommissionRate: msg.CommissionRate, Creator: msg.Creator,
		})
		if stop := c.handleCrawlErr(ctx, msg.JobID, err); stop != nil {
			return stop
		}
		for _, p := range products {
			if err := c.insertProduct(ctx, msg.JobID, country, p); err != nil {
				c.markFailed(ctx, msg.JobID)
				return fmt.Errorf("worker: insert kalodata_products: %w", err)
			}
		}
	}

	if enabled(msg.WithVideos) {
		videos, err := c.Client.SearchVideos(ctx, kalodata.VideoParams{
			Country: country, StartDate: startDate, EndDate: endDate,
			PageNo: pageNo, PageSize: pageSize, SortField: sortField, SortType: sortType,
			VideoType: msg.VideoType, Duration: msg.Duration, CreatorFollowers: msg.CreatorFollowers,
		})
		if stop := c.handleCrawlErr(ctx, msg.JobID, err); stop != nil {
			return stop
		}
		for i := range videos {
			if msg.WithVideoDetail {
				detail, derr := c.Client.VideoDetail(ctx, country, videos[i].VideoID, startDate, endDate)
				if errors.Is(derr, kalodata.ErrQuotaExhausted) || errors.Is(derr, kalodata.ErrNoCookies) {
					// Out of detail quota mid-run: stop fetching detail, keep videos.
					msg.WithVideoDetail = false
				} else if derr == nil {
					videos[i].Detail = detail
				}
			}
			if err := c.insertVideo(ctx, msg.JobID, country, videos[i]); err != nil {
				c.markFailed(ctx, msg.JobID)
				return fmt.Errorf("worker: insert kalodata_videos: %w", err)
			}
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
	if err := c.Publisher.Publish(ctx, "crawl.completed.kalodata", payload); err != nil {
		return fmt.Errorf("worker: publish completed event: %w", err)
	}
	return nil
}

// handleCrawlErr maps a crawl error to a terminal action. ErrQuotaExhausted and
// ErrNoCookies both mark the job failed: the daily quota stays exhausted for
// hours, and the rabbitmq wrapper requeues immediately (no delay), so requeuing
// here would busy-loop against Postgres until the reset. Operator resubmits once
// quota resets / fresh cookies are injected. Returns nil to continue.
func (c *Consumer) handleCrawlErr(ctx context.Context, jobID string, err error) error {
	if err == nil {
		return nil
	}
	c.markFailed(ctx, jobID)
	return fmt.Errorf("worker: kalodata crawl: %w", err)
}

func (c *Consumer) insertProduct(ctx context.Context, jobID, country string, p kalodata.Product) error {
	_, err := c.DB.ExecContext(ctx,
		`INSERT INTO kalodata_products (
			job_id, country, product_id, title, revenue, gmv, sales, rating,
			unit_price, commission_rate, thumbnail_url, product_url, raw
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		jobID, country, p.ProductID, p.Title, p.Revenue, p.GMV, p.Sales, p.Rating,
		p.UnitPrice, p.CommissionRate, p.ThumbnailURL, p.ProductURL, p.Raw,
	)
	return err
}

func (c *Consumer) insertVideo(ctx context.Context, jobID, country string, v kalodata.Video) error {
	_, err := c.DB.ExecContext(ctx,
		`INSERT INTO kalodata_videos (
			job_id, country, video_id, handle, title, views, revenue, sales,
			duration, thumbnail_url, tiktok_url, detail, raw
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		jobID, country, v.VideoID, v.Handle, v.Title, v.Views, v.Revenue, v.Sales,
		v.Duration, v.ThumbnailURL, v.TikTokURL, nullJSON(v.Detail), v.Raw,
	)
	return err
}

func nullJSON(b json.RawMessage) interface{} {
	if len(b) == 0 {
		return nil
	}
	return b
}

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
