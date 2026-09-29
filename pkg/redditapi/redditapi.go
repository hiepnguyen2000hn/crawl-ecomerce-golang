// Package redditapi is a client for Reddit's official OAuth API
// (https://oauth.reddit.com), used as a free (no per-result billing)
// alternative to the Apify-based reddit-service. It authenticates via the
// "client_credentials" grant (app-only, no Reddit user login required) and
// is suitable for read-only access to public data.
package redditapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
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

type RedditSearchResult struct {
	Posts    []RedditPost
	Comments []RedditComment
}

type RedditClient interface {
	SearchTopPostsWithComments(ctx context.Context, keyword string, maxPosts, maxCommentsPerPost int) (RedditSearchResult, error)
}

type RedditHTTPClient struct {
	clientID     string
	clientSecret string
	userAgent    string
	authURL      string
	apiURL       string
	httpClient   *http.Client

	mu          sync.Mutex
	token       string
	tokenExpiry time.Time
}

func NewRedditHTTPClient(clientID, clientSecret, userAgent, authURL, apiURL string, httpClient *http.Client) *RedditHTTPClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if authURL == "" {
		authURL = "https://www.reddit.com"
	}
	if apiURL == "" {
		apiURL = "https://oauth.reddit.com"
	}
	return &RedditHTTPClient{
		clientID:     clientID,
		clientSecret: clientSecret,
		userAgent:    userAgent,
		authURL:      authURL,
		apiURL:       apiURL,
		httpClient:   httpClient,
	}
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

// getToken returns a cached app-only access token, fetching a new one via
// the client_credentials grant if missing or close to expiry.
func (c *RedditHTTPClient) getToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.token != "" && time.Now().Before(c.tokenExpiry) {
		return c.token, nil
	}

	form := url.Values{}
	form.Set("grant_type", "client_credentials")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.authURL+"/api/v1/access_token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("redditapi: build token request: %w", err)
	}
	req.SetBasicAuth(c.clientID, c.clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("redditapi: token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("redditapi: read token response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("redditapi: token endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", fmt.Errorf("redditapi: unmarshal token response: %w", err)
	}
	if tr.AccessToken == "" {
		return "", fmt.Errorf("redditapi: token response missing access_token: %s", string(body))
	}

	c.token = tr.AccessToken
	c.tokenExpiry = time.Now().Add(time.Duration(tr.ExpiresIn-60) * time.Second)
	return c.token, nil
}

func (c *RedditHTTPClient) doGet(ctx context.Context, path string) ([]byte, error) {
	token, err := c.getToken(ctx)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("redditapi: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("redditapi: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("redditapi: read response body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("redditapi: returned status %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

// listingResponse mirrors Reddit's generic "Listing" wrapper used by both
// the search endpoint and the post half of the comments endpoint.
type listingResponse struct {
	Data struct {
		Children []struct {
			Kind string          `json:"kind"`
			Data json.RawMessage `json:"data"`
		} `json:"children"`
	} `json:"data"`
}

type postData struct {
	Name        string  `json:"name"`
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Permalink   string  `json:"permalink"`
	Subreddit   string  `json:"subreddit"`
	Score       int     `json:"score"`
	NumComments int     `json:"num_comments"`
	CreatedUTC  float64 `json:"created_utc"`
}

func (c *RedditHTTPClient) searchTopPosts(ctx context.Context, keyword string, maxPosts int) ([]RedditPost, error) {
	q := url.Values{}
	q.Set("q", keyword)
	q.Set("sort", "top")
	q.Set("type", "link")
	q.Set("limit", strconv.Itoa(maxPosts))

	body, err := c.doGet(ctx, "/search?"+q.Encode())
	if err != nil {
		return nil, fmt.Errorf("redditapi: search posts: %w", err)
	}

	var listing listingResponse
	if err := json.Unmarshal(body, &listing); err != nil {
		return nil, fmt.Errorf("redditapi: unmarshal search response: %w", err)
	}

	posts := make([]RedditPost, 0, len(listing.Data.Children))
	for _, child := range listing.Data.Children {
		if child.Kind != "t3" {
			continue
		}
		var pd postData
		if err := json.Unmarshal(child.Data, &pd); err != nil {
			return nil, fmt.Errorf("redditapi: unmarshal post data: %w", err)
		}
		posts = append(posts, RedditPost{
			PostID:        pd.Name,
			Title:         pd.Title,
			URL:           "https://www.reddit.com" + pd.Permalink,
			CommunityName: "r/" + pd.Subreddit,
			UpVotes:       pd.Score,
			NumComments:   pd.NumComments,
			CreatedAt:     time.Unix(int64(pd.CreatedUTC), 0).UTC().Format(time.RFC3339),
			Raw:           child.Data,
		})
		if len(posts) >= maxPosts {
			break
		}
	}
	return posts, nil
}

type commentData struct {
	ID         string          `json:"id"`
	Author     string          `json:"author"`
	Body       string          `json:"body"`
	Score      int             `json:"score"`
	CreatedUTC float64         `json:"created_utc"`
	Replies    json.RawMessage `json:"replies"`
}

// flattenComments walks a comment Listing (including nested "replies"
// listings) and appends every t1 (comment) node it finds. "more" stub nodes
// (additional replies not included inline) are skipped rather than
// expanded via a further API call, trading some depth for simplicity.
func flattenComments(postID string, raw json.RawMessage, out *[]RedditComment) error {
	if len(raw) == 0 {
		return nil
	}
	var listing listingResponse
	if err := json.Unmarshal(raw, &listing); err != nil {
		// "replies" is "" (empty string) when a comment has no children.
		return nil
	}
	for _, child := range listing.Data.Children {
		if child.Kind != "t1" {
			continue
		}
		var cd commentData
		if err := json.Unmarshal(child.Data, &cd); err != nil {
			return fmt.Errorf("redditapi: unmarshal comment data: %w", err)
		}
		*out = append(*out, RedditComment{
			PostID:    postID,
			CommentID: cd.ID,
			Username:  cd.Author,
			Body:      cd.Body,
			UpVotes:   cd.Score,
			CreatedAt: time.Unix(int64(cd.CreatedUTC), 0).UTC().Format(time.RFC3339),
			Raw:       child.Data,
		})
		if err := flattenComments(postID, cd.Replies, out); err != nil {
			return err
		}
	}
	return nil
}

func (c *RedditHTTPClient) fetchTopComments(ctx context.Context, post RedditPost, maxCommentsPerPost int) ([]RedditComment, error) {
	postID36 := strings.TrimPrefix(post.PostID, "t3_")

	q := url.Values{}
	q.Set("sort", "top")
	q.Set("depth", "10")
	// The official API has no per-result cost, so requesting a larger
	// buffer than needed is free; it just widens the pool we sort/truncate
	// locally to find the true top-scoring comments.
	q.Set("limit", strconv.Itoa(maxCommentsPerPost*2))

	body, err := c.doGet(ctx, "/comments/"+postID36+"?"+q.Encode())
	if err != nil {
		return nil, fmt.Errorf("redditapi: fetch comments for post %s: %w", post.PostID, err)
	}

	var listings []listingResponse
	if err := json.Unmarshal(body, &listings); err != nil {
		return nil, fmt.Errorf("redditapi: unmarshal comments response: %w", err)
	}
	if len(listings) < 2 {
		return nil, nil
	}

	var comments []RedditComment
	for _, child := range listings[1].Data.Children {
		if child.Kind != "t1" {
			continue
		}
		var cd commentData
		if err := json.Unmarshal(child.Data, &cd); err != nil {
			return nil, fmt.Errorf("redditapi: unmarshal comment data: %w", err)
		}
		comments = append(comments, RedditComment{
			PostID:    post.PostID,
			CommentID: cd.ID,
			Username:  cd.Author,
			Body:      cd.Body,
			UpVotes:   cd.Score,
			CreatedAt: time.Unix(int64(cd.CreatedUTC), 0).UTC().Format(time.RFC3339),
			Raw:       child.Data,
		})
		if err := flattenComments(post.PostID, cd.Replies, &comments); err != nil {
			return nil, err
		}
	}

	sort.Slice(comments, func(i, j int) bool { return comments[i].UpVotes > comments[j].UpVotes })
	if len(comments) > maxCommentsPerPost {
		comments = comments[:maxCommentsPerPost]
	}
	return comments, nil
}

func (c *RedditHTTPClient) SearchTopPostsWithComments(ctx context.Context, keyword string, maxPosts, maxCommentsPerPost int) (RedditSearchResult, error) {
	if keyword == "" {
		return RedditSearchResult{}, fmt.Errorf("redditapi: keyword is required")
	}
	if maxPosts <= 0 {
		maxPosts = 3
	}
	if maxCommentsPerPost <= 0 {
		maxCommentsPerPost = 100
	}

	posts, err := c.searchTopPosts(ctx, keyword, maxPosts)
	if err != nil {
		return RedditSearchResult{}, err
	}

	var allComments []RedditComment
	for _, p := range posts {
		comments, err := c.fetchTopComments(ctx, p, maxCommentsPerPost)
		if err != nil {
			return RedditSearchResult{}, err
		}
		allComments = append(allComments, comments...)
	}

	return RedditSearchResult{Posts: posts, Comments: allComments}, nil
}
