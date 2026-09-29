package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hiepnv/crawl-ecomerce-golang/pkg/rabbitmq"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/redditapi"
)

const (
	maxPosts    = 3
	maxComments = 100
)

type JobMessage struct {
	JobID   string `json:"job_id"`
	Keyword string `json:"keyword"`
}

type CompletedMessage struct {
	JobID string `json:"job_id"`
}

// Consumer wires a rabbitmq.Consumer to Reddit's official OAuth API and
// Postgres: it reads reddit-api.jobs messages, searches Reddit for the top
// posts matching a keyword, fetches each post's top-scoring comments,
// persists both (to the same tables reddit-service uses), and publishes a
// crawl.completed.reddit_api event.
type Consumer struct {
	DB        *sql.DB
	Reddit    redditapi.RedditClient
	Publisher *rabbitmq.Publisher
}

func (c *Consumer) HandleMessage(body []byte) error {
	var msg JobMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return fmt.Errorf("worker: unmarshal job message: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	result, err := c.Reddit.SearchTopPostsWithComments(ctx, msg.Keyword, maxPosts, maxComments)
	if err != nil {
		c.markFailed(ctx, msg.JobID)
		return fmt.Errorf("worker: search posts: %w", err)
	}

	for _, p := range result.Posts {
		if _, err := c.DB.ExecContext(ctx,
			`INSERT INTO reddit_posts_raw (job_id, post_id, title, url, community_name, upvotes, num_comments, created_at_reddit, raw)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, '')::timestamptz, $9)`,
			msg.JobID, p.PostID, p.Title, p.URL, p.CommunityName, p.UpVotes, p.NumComments, p.CreatedAt, p.Raw,
		); err != nil {
			c.markFailed(ctx, msg.JobID)
			return fmt.Errorf("worker: insert reddit_posts_raw: %w", err)
		}
	}

	for _, cm := range result.Comments {
		if _, err := c.DB.ExecContext(ctx,
			`INSERT INTO reddit_comments_raw (job_id, post_id, comment_id, username, body, upvotes, created_at_reddit, raw)
			 VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, '')::timestamptz, $8)`,
			msg.JobID, cm.PostID, cm.CommentID, cm.Username, cm.Body, cm.UpVotes, cm.CreatedAt, cm.Raw,
		); err != nil {
			c.markFailed(ctx, msg.JobID)
			return fmt.Errorf("worker: insert reddit_comments_raw: %w", err)
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
	if err := c.Publisher.Publish(ctx, "crawl.completed.reddit_api", payload); err != nil {
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
