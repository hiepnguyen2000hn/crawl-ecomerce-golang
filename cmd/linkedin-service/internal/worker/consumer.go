package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/hiepnv/crawl-ecomerce-golang/pkg/apify"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/rabbitmq"
)

type JobMessage struct {
	JobID            string   `json:"job_id"`
	Keyword          string   `json:"keyword"`
	MaxPosts         int      `json:"max_posts,omitempty"`
	Pages            int      `json:"pages,omitempty"`
	SortBy           string   `json:"sort_by,omitempty"`
	PostedLimit      string   `json:"posted_limit,omitempty"`
	AuthorUrls       []string `json:"author_urls,omitempty"`
	AuthorsCompanies []string `json:"authors_companies,omitempty"`
	AuthorKeywords   string   `json:"author_keywords,omitempty"`
	ContentType      string   `json:"content_type,omitempty"`
}

type CompletedMessage struct {
	JobID string `json:"job_id"`
}

// Consumer wires a rabbitmq.Consumer to Apify and Postgres: it reads
// linkedin.jobs messages, searches LinkedIn posts via the
// harvestapi/linkedin-post-search actor, persists them to linkedin_posts_raw,
// and publishes a crawl.completed.linkedin event.
type Consumer struct {
	DB        *sql.DB
	Apify     apify.LinkedInClient
	Publisher *rabbitmq.Publisher
}

func (c *Consumer) HandleMessage(body []byte) error {
	var msg JobMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return fmt.Errorf("worker: unmarshal job message: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	params := apify.LinkedInSearchParams{
		Keyword:          msg.Keyword,
		MaxPosts:         msg.MaxPosts,
		Pages:            msg.Pages,
		SortBy:           msg.SortBy,
		PostedLimit:      msg.PostedLimit,
		AuthorUrls:       msg.AuthorUrls,
		AuthorsCompanies: msg.AuthorsCompanies,
		AuthorKeywords:   msg.AuthorKeywords,
		ContentType:      msg.ContentType,
	}
	if params.MaxPosts <= 0 {
		params.MaxPosts = 25
	}
	if params.Pages <= 0 {
		params.Pages = 1
	}
	if params.SortBy == "" {
		params.SortBy = "relevance"
	}

	result, err := c.Apify.SearchPosts(ctx, msg.JobID, params)
	var busy *apify.ErrBusy
	if errors.As(err, &busy) {
		// Every Apify account is at its concurrency limit. The job is fine;
		// requeue instead of dead-lettering valid work.
		return fmt.Errorf("%w: %v", rabbitmq.ErrRequeue, err)
	}
	if err != nil {
		c.markFailed(ctx, msg.JobID)
		return fmt.Errorf("worker: search posts: %w", err)
	}

	for _, p := range result.Posts {
		if _, err := c.DB.ExecContext(ctx,
			`INSERT INTO linkedin_posts_raw (
				job_id, post_id, url, content, author_name, author_url,
				posted_at, likes, comments_count, shares, raw
			) VALUES (
				$1, $2, $3, $4, $5, $6,
				NULLIF($7, '')::timestamptz, $8, $9, $10, $11
			)`,
			msg.JobID, p.PostID, p.URL, p.Content, p.AuthorName, p.AuthorURL,
			p.PostedAt, p.Likes, p.Comments, p.Shares, p.Raw,
		); err != nil {
			c.markFailed(ctx, msg.JobID)
			return fmt.Errorf("worker: insert linkedin_posts_raw: %w", err)
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
	if err := c.Publisher.Publish(ctx, "crawl.completed.linkedin", payload); err != nil {
		return fmt.Errorf("worker: publish completed event: %w", err)
	}
	return nil
}

// markFailed best-effort sets jobs.status = 'failed'; errors are logged and
// swallowed so they never mask the original failure.
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
