package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/hiepnv/crawl-ecomerce-golang/pkg/pipiads"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/rabbitmq"
)

type JobMessage struct {
	JobID       string `json:"job_id"`
	Keyword     string `json:"keyword"`
	SearchType  int    `json:"search_type,omitempty"`
	Sort        int    `json:"sort,omitempty"`
	SortType    string `json:"sort_type,omitempty"`
	CurrentPage int    `json:"current_page,omitempty"`
	PageSize    int    `json:"page_size,omitempty"`
}

type CompletedMessage struct {
	JobID string `json:"job_id"`
}

type Consumer struct {
	DB        *sql.DB
	Client    pipiads.Client
	Registry  *pipiads.AccountRegistry
	Publisher *rabbitmq.Publisher
}

func (c *Consumer) HandleMessage(body []byte) error {
	var msg JobMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return fmt.Errorf("worker: unmarshal job message: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Clear old rows for idempotent re-runs
	if c.DB != nil {
		_, _ = c.DB.ExecContext(ctx, `DELETE FROM pipiads_ads WHERE job_id = $1`, msg.JobID)
	}

	maxRetries := 3
	var ads []pipiads.EcommerceAd
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		account, err := c.Registry.AcquireAccount(ctx)
		if err != nil {
			lastErr = err
			break
		}

		searchReq := pipiads.SearchRequest{
			Keyword:     msg.Keyword,
			SearchType:  msg.SearchType,
			Sort:        msg.Sort,
			SortType:    msg.SortType,
			CurrentPage: msg.CurrentPage,
			PageSize:    msg.PageSize,
		}
		if searchReq.SearchType == 0 {
			searchReq.SearchType = 1
		}
		if searchReq.Sort == 0 {
			searchReq.Sort = 2
		}
		if searchReq.SortType == "" {
			searchReq.SortType = "desc"
		}
		if searchReq.CurrentPage <= 0 {
			searchReq.CurrentPage = 1
		}
		if searchReq.PageSize <= 0 {
			searchReq.PageSize = 20
		}

		ads, err = c.Client.SearchEcommerceAds(ctx, account, searchReq)
		if err != nil {
			log.Printf("worker: pipiads search failed on account %s: %v", account.Email, err)
			if errors.Is(err, pipiads.ErrUnauthorized) {
				_ = c.Registry.MarkAccountDead(ctx, account.ID, true)
			} else if errors.Is(err, pipiads.ErrQuotaExhausted) {
				_ = c.Registry.RecordCreditUsage(ctx, account.ID, 500)
			}
			lastErr = err
			continue
		}

		// Success! Sync quota or record credit usage
		_ = c.Registry.SyncAccountQuota(ctx, account)
		lastErr = nil
		break
	}

	if lastErr != nil {
		if c.DB != nil {
			_, _ = c.DB.ExecContext(ctx, `UPDATE jobs SET status = 'failed', updated_at = now() WHERE id = $1`, msg.JobID)
		}
		return fmt.Errorf("worker: pipiads search failed after retries: %w", lastErr)
	}

	// Persist ads to DB
	if c.DB != nil && len(ads) > 0 {
		stmt, err := c.DB.PrepareContext(ctx, `
			INSERT INTO pipiads_ads (
				job_id, video_id, ad_id, keyword, title, description, app_name, advertiser_name,
				platform, play_count, digg_count, comment_count, share_count,
				cover_url, video_url, landing_page_url, first_seen_at, last_seen_at, raw
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8,
				$9, $10, $11, $12, $13,
				$14, $15, $16, $17, $18, $19
			)
		`)
		if err != nil {
			return fmt.Errorf("worker: prepare insert pipiads_ads: %w", err)
		}
		defer stmt.Close()

		for _, ad := range ads {
			_, _ = stmt.ExecContext(ctx,
				msg.JobID, ad.VideoID, ad.AdID, msg.Keyword, ad.Title, ad.Description, ad.AppName, ad.AdvertiserName,
				ad.Platform, ad.PlayCount, ad.DiggCount, ad.CommentCount, ad.ShareCount,
				ad.CoverURL, "", ad.LandingPageURL, ad.FirstTime, ad.LastTime, ad.Raw,
			)
		}

		_, _ = c.DB.ExecContext(ctx, `UPDATE jobs SET status = 'crawled', updated_at = now() WHERE id = $1`, msg.JobID)
	} else if c.DB != nil {
		_, _ = c.DB.ExecContext(ctx, `UPDATE jobs SET status = 'crawled', updated_at = now() WHERE id = $1`, msg.JobID)
	}

	// Publish completion message
	if c.Publisher != nil {
		comp := CompletedMessage{JobID: msg.JobID}
		data, _ := json.Marshal(comp)
		_ = c.Publisher.Publish(ctx, "crawl.completed.pipiads", data)
	}

	return nil
}
