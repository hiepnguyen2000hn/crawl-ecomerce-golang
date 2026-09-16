package crawl4ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// CrawlResult is the crawl4ai-service response for one URL. Success=false
// represents a crawl-level failure (bad URL, timeout, blocked page) reported
// by the service itself, distinct from a Go-level transport/HTTP error.
type CrawlResult struct {
	Markdown string
	Success  bool
	Error    string
}

type Client interface {
	Crawl(ctx context.Context, url string) (CrawlResult, error)
}

type HTTPClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewHTTPClient(baseURL string, httpClient *http.Client) *HTTPClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &HTTPClient{baseURL: baseURL, httpClient: httpClient}
}

type crawlRequest struct {
	URL string `json:"url"`
}

func (c *HTTPClient) Crawl(ctx context.Context, url string) (CrawlResult, error) {
	payload, err := json.Marshal(crawlRequest{URL: url})
	if err != nil {
		return CrawlResult{}, fmt.Errorf("crawl4ai: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/crawl", bytes.NewReader(payload))
	if err != nil {
		return CrawlResult{}, fmt.Errorf("crawl4ai: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return CrawlResult{}, fmt.Errorf("crawl4ai: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return CrawlResult{}, fmt.Errorf("crawl4ai: returned status %d", resp.StatusCode)
	}

	var result CrawlResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return CrawlResult{}, fmt.Errorf("crawl4ai: decode response: %w", err)
	}
	return result, nil
}
