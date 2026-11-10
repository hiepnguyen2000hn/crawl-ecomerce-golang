package apify

import (
	"context"
	"encoding/json"
	"fmt"
)

// LinkedInPost is a normalized LinkedIn post record.
type LinkedInPost struct {
	PostID     string
	URL        string
	Content    string
	AuthorName string
	AuthorURL  string
	PostedAt   string
	Likes      int
	Comments   int
	Shares     int
	Raw        json.RawMessage
}

// LinkedInSearchParams drives a harvestapi/linkedin-post-search run.
type LinkedInSearchParams struct {
	Keyword          string
	MaxPosts         int
	Pages            int
	SortBy           string
	PostedLimit      string
	AuthorKeywords   string
	ContentType      string
	AuthorUrls       []string
	AuthorsCompanies []string
}

// LinkedInSearchResult holds the posts matching a search.
type LinkedInSearchResult struct {
	Posts []LinkedInPost
}

type LinkedInClient interface {
	// SearchPosts runs the actor for params.Keyword (+ filters) and returns up
	// to params.MaxPosts posts. jobID is recorded on the concurrency lease so a
	// stuck slot can be traced back to the job holding it.
	SearchPosts(ctx context.Context, jobID string, params LinkedInSearchParams) (LinkedInSearchResult, error)
}

// LinkedInHTTPClient maps the harvestapi/linkedin-post-search actor onto posts.
// The HTTP call, token selection and concurrency slot all live in Runner; what
// stays here is this actor's input shape and output parsing.
type LinkedInHTTPClient struct {
	actorID string
	runner  *Runner
}

func NewLinkedInHTTPClient(actorID string, runner *Runner) *LinkedInHTTPClient {
	if actorID == "" {
		actorID = "harvestapi/linkedin-post-search"
	}
	return &LinkedInHTTPClient{actorID: actorID, runner: runner}
}

// linkedinRunInput mirrors the harvestapi/linkedin-post-search actor input.
// Comments and reactions stay off (scraped separately-priced on Apify).
type linkedinRunInput struct {
	SearchQueries    []string `json:"searchQueries"`
	MaxPosts         int      `json:"maxPosts,omitempty"`
	ScrapePages      int      `json:"scrapePages,omitempty"`
	SortBy           string   `json:"sortBy,omitempty"`
	PostedLimit      string   `json:"postedLimit,omitempty"`
	AuthorUrls       []string `json:"authorUrls,omitempty"`
	AuthorsCompanies []string `json:"authorsCompanies,omitempty"`
	AuthorKeywords   string   `json:"authorKeywords,omitempty"`
	ContentType      string   `json:"contentType,omitempty"`
}

// linkedinItem parses the actor's post output. Field paths are best-effort
// (the full post JSON is always kept in Raw); confirm against a real dataset.
type linkedinItem struct {
	ID          string `json:"id"`
	LinkedinURL string `json:"linkedinUrl"`
	Content     string `json:"content"`
	Author      struct {
		Name        string `json:"name"`
		LinkedinURL string `json:"linkedinUrl"`
	} `json:"author"`
	PostedAt struct {
		Date string `json:"date"`
	} `json:"postedAt"`
	Engagement struct {
		Likes    int `json:"likes"`
		Comments int `json:"comments"`
		Shares   int `json:"shares"`
	} `json:"engagement"`
}

// mapLinkedInItems decodes raw dataset items into posts, keeps the original
// bytes as Raw, and caps at maxPosts. Pure/testable (no network/DB).
func mapLinkedInItems(raw []json.RawMessage, maxPosts int) ([]LinkedInPost, error) {
	items, err := DecodeItems[linkedinItem](raw)
	if err != nil {
		return nil, err
	}
	posts := make([]LinkedInPost, 0, len(items))
	for i, it := range items {
		posts = append(posts, LinkedInPost{
			PostID:     it.ID,
			URL:        it.LinkedinURL,
			Content:    it.Content,
			AuthorName: it.Author.Name,
			AuthorURL:  it.Author.LinkedinURL,
			PostedAt:   it.PostedAt.Date,
			Likes:      it.Engagement.Likes,
			Comments:   it.Engagement.Comments,
			Shares:     it.Engagement.Shares,
			Raw:        raw[i],
		})
	}
	if maxPosts > 0 && len(posts) > maxPosts {
		posts = posts[:maxPosts]
	}
	return posts, nil
}

func (c *LinkedInHTTPClient) SearchPosts(ctx context.Context, jobID string, params LinkedInSearchParams) (LinkedInSearchResult, error) {
	if params.Keyword == "" {
		return LinkedInSearchResult{}, fmt.Errorf("apify: keyword is required")
	}
	maxPosts := params.MaxPosts
	if maxPosts <= 0 {
		maxPosts = 25
	}
	pages := params.Pages
	if pages <= 0 {
		pages = 1
	}
	sortBy := params.SortBy
	if sortBy == "" {
		sortBy = "relevance"
	}

	rawItems, err := c.runner.RunSync(ctx, c.actorID, jobID, linkedinRunInput{
		SearchQueries:    []string{params.Keyword},
		MaxPosts:         maxPosts,
		ScrapePages:      pages,
		SortBy:           sortBy,
		PostedLimit:      params.PostedLimit,
		AuthorUrls:       params.AuthorUrls,
		AuthorsCompanies: params.AuthorsCompanies,
		AuthorKeywords:   params.AuthorKeywords,
		ContentType:      params.ContentType,
	})
	if err != nil {
		return LinkedInSearchResult{}, err
	}

	posts, err := mapLinkedInItems(rawItems, maxPosts)
	if err != nil {
		return LinkedInSearchResult{}, err
	}
	return LinkedInSearchResult{Posts: posts}, nil
}
