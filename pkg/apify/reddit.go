package apify

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
)

// RedditPost is a normalized Reddit post record.
type RedditPost struct {
	PostID        string
	Title         string
	URL           string
	CommunityName string
	UpVotes       int
	NumComments   int
	CreatedAt     string
	Raw           json.RawMessage
}

// RedditComment is a normalized Reddit comment record.
type RedditComment struct {
	PostID    string
	CommentID string
	Username  string
	Body      string
	UpVotes   int
	CreatedAt string
	Raw       json.RawMessage
}

// RedditSearchResult holds the top posts for a keyword and, for each post,
// its top-scoring comments (already sorted and truncated).
type RedditSearchResult struct {
	Posts    []RedditPost
	Comments []RedditComment
}

type RedditClient interface {
	// SearchTopPostsWithComments searches Reddit for keyword, returns the
	// top maxPosts posts sorted by score, and for each post its top
	// maxCommentsPerPost comments sorted by score descending.
	// jobID is recorded on the concurrency lease so a stuck slot can be
	// traced back to the job holding it.
	SearchTopPostsWithComments(ctx context.Context, jobID, keyword string, maxPosts, maxCommentsPerPost int) (RedditSearchResult, error)
}

// RedditHTTPClient maps the Reddit scraper actor onto posts and comments.
// The HTTP call, token selection and concurrency slot all live in Runner;
// what stays here is specific to this actor, including the local re-sorting
// the actor's output requires.
type RedditHTTPClient struct {
	actorID string
	runner  *Runner
}

func NewRedditHTTPClient(actorID string, runner *Runner) *RedditHTTPClient {
	if actorID == "" {
		actorID = "harshmaur~reddit-scraper"
	}
	return &RedditHTTPClient{actorID: actorID, runner: runner}
}

// redditRunInput mirrors the harshmaur/reddit-scraper actor's input schema
// for a combined keyword-search + per-post-comment-crawl run.
type redditRunInput struct {
	SearchTerms          []string `json:"searchTerms,omitempty"`
	SearchPosts          bool     `json:"searchPosts,omitempty"`
	SearchComments       bool     `json:"searchComments,omitempty"`
	SearchCommunities    bool     `json:"searchCommunities,omitempty"`
	SearchSort           string   `json:"searchSort,omitempty"`
	MaxPostsCount        int      `json:"maxPostsCount,omitempty"`
	CrawlCommentsPerPost bool     `json:"crawlCommentsPerPost,omitempty"`
	MaxCommentsPerPost   int      `json:"maxCommentsPerPost,omitempty"`
}

type redditItem struct {
	DataType         string          `json:"dataType"`
	ID               string          `json:"id"`
	PostID           string          `json:"postId"`
	Title            string          `json:"title"`
	PostURL          string          `json:"postUrl"`
	CommunityName    string          `json:"communityName"`
	UpVotes          int             `json:"upVotes"`
	CommentsCount    int             `json:"commentsCount"`
	CreatedAt        string          `json:"createdAt"`
	AuthorName       string          `json:"authorName"`
	Body             string          `json:"body"`
	Score            int             `json:"score"`
	CommentCreatedAt string          `json:"commentCreatedAt"`
	Raw              json.RawMessage `json:"-"`
}

func (c *RedditHTTPClient) SearchTopPostsWithComments(ctx context.Context, jobID, keyword string, maxPosts, maxCommentsPerPost int) (RedditSearchResult, error) {
	if keyword == "" {
		return RedditSearchResult{}, fmt.Errorf("apify: keyword is required")
	}
	if maxPosts <= 0 {
		maxPosts = 3
	}
	if maxCommentsPerPost <= 0 {
		maxCommentsPerPost = 100
	}
	// Request a larger buffer than needed since comments are not returned
	// pre-sorted by score (they follow Reddit's default traversal order);
	// we re-sort locally per post and truncate to maxCommentsPerPost below.
	fetchBuffer := maxCommentsPerPost * 3

	rawItems, err := c.runner.RunSync(ctx, c.actorID, jobID, redditRunInput{
		SearchTerms:          []string{keyword},
		SearchPosts:          true,
		SearchSort:           "top",
		MaxPostsCount:        maxPosts,
		CrawlCommentsPerPost: true,
		MaxCommentsPerPost:   fetchBuffer,
	})
	if err != nil {
		return RedditSearchResult{}, err
	}
	items, err := DecodeItems[redditItem](rawItems)
	if err != nil {
		return RedditSearchResult{}, err
	}

	var posts []RedditPost
	commentsByPost := map[string][]RedditComment{}
	for i, it := range items {
		switch it.DataType {
		case "post":
			posts = append(posts, RedditPost{
				PostID:        it.ID,
				Title:         it.Title,
				URL:           it.PostURL,
				CommunityName: it.CommunityName,
				UpVotes:       it.UpVotes,
				NumComments:   it.CommentsCount,
				CreatedAt:     it.CreatedAt,
				Raw:           rawItems[i],
			})
		case "comment":
			commentsByPost[it.PostID] = append(commentsByPost[it.PostID], RedditComment{
				PostID:    it.PostID,
				CommentID: it.ID,
				Username:  it.AuthorName,
				Body:      it.Body,
				UpVotes:   it.Score,
				CreatedAt: it.CommentCreatedAt,
				Raw:       rawItems[i],
			})
		}
	}
	if len(posts) > maxPosts {
		posts = posts[:maxPosts]
	}

	var allComments []RedditComment
	for _, p := range posts {
		cs := commentsByPost[p.PostID]
		sort.Slice(cs, func(i, j int) bool { return cs[i].UpVotes > cs[j].UpVotes })
		if len(cs) > maxCommentsPerPost {
			cs = cs[:maxCommentsPerPost]
		}
		allComments = append(allComments, cs...)
	}

	return RedditSearchResult{Posts: posts, Comments: allComments}, nil
}
