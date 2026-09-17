package apify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Ad is a normalized Facebook Ads Library record.
type Ad struct {
	AdArchiveID string
	PageID      string
	PageName    string
	IsActive    bool
	StartDate   time.Time
	EndDate     time.Time
	Body        string
	Title       string
	CtaText     string
	CtaType     string
	LinkURL     string
	Raw         json.RawMessage
}

// AdParams mirrors the Apify actor's input schema. All fields are
// optional except MaxItems, which the client always sends (defaulting
// to 50 when unset) since the actor requires it.
type AdParams struct {
	Query        string
	PageID       string
	Country      string
	Category     string
	MediaType    string
	SortBy       string
	ActiveStatus string
	MinDate      string
	MaxDate      string
	MaxItems     int
	Advertisers  []string
	FetchDetails bool
}

type AdsResult struct {
	Ads []Ad
}

type Client interface {
	FetchAds(ctx context.Context, params AdParams) (AdsResult, error)
}

type HTTPClient struct {
	apiToken   string
	actorID    string
	baseURL    string
	httpClient *http.Client
}

func NewHTTPClient(apiToken, actorID, baseURL string, httpClient *http.Client) *HTTPClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if actorID == "" {
		actorID = "igolaizola~facebook-ad-library-scraper"
	}
	return &HTTPClient{apiToken: apiToken, actorID: actorID, baseURL: baseURL, httpClient: httpClient}
}

type apifyRunInput struct {
	Query        string   `json:"query,omitempty"`
	PageID       string   `json:"pageId,omitempty"`
	Country      string   `json:"country,omitempty"`
	Category     string   `json:"category,omitempty"`
	MediaType    string   `json:"mediaType,omitempty"`
	SortBy       string   `json:"sortBy,omitempty"`
	ActiveStatus string   `json:"activeStatus,omitempty"`
	MinDate      string   `json:"minDate,omitempty"`
	MaxDate      string   `json:"maxDate,omitempty"`
	MaxItems     int      `json:"maxItems"`
	Advertisers  []string `json:"advertisers,omitempty"`
	FetchDetails bool     `json:"fetchDetails,omitempty"`
}

type apifySnapshot struct {
	Body struct {
		Text string `json:"text"`
	} `json:"body"`
	Title   string `json:"title"`
	CtaText string `json:"cta_text"`
	CtaType string `json:"cta_type"`
	LinkURL string `json:"link_url"`
}

type apifyAdItem struct {
	AdArchiveID string        `json:"ad_archive_id"`
	PageID      string        `json:"page_id"`
	PageName    string        `json:"page_name"`
	IsActive    bool          `json:"is_active"`
	StartDate   *int64        `json:"start_date"`
	EndDate     *int64        `json:"end_date"`
	Snapshot    apifySnapshot `json:"snapshot"`
}

func (c *HTTPClient) FetchAds(ctx context.Context, params AdParams) (AdsResult, error) {
	maxItems := params.MaxItems
	if maxItems <= 0 {
		maxItems = 50
	}
	input := apifyRunInput{
		Query:        params.Query,
		PageID:       params.PageID,
		Country:      params.Country,
		Category:     params.Category,
		MediaType:    params.MediaType,
		SortBy:       params.SortBy,
		ActiveStatus: params.ActiveStatus,
		MinDate:      params.MinDate,
		MaxDate:      params.MaxDate,
		MaxItems:     maxItems,
		Advertisers:  params.Advertisers,
		FetchDetails: params.FetchDetails,
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return AdsResult{}, fmt.Errorf("apify: marshal input: %w", err)
	}

	q := url.Values{}
	q.Set("token", c.apiToken)
	reqURL := fmt.Sprintf("%s/acts/%s/run-sync-get-dataset-items?%s", c.baseURL, c.actorID, q.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(payload))
	if err != nil {
		return AdsResult{}, fmt.Errorf("apify: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return AdsResult{}, fmt.Errorf("apify: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return AdsResult{}, fmt.Errorf("apify: returned status %d", resp.StatusCode)
	}

	items, rawItems, err := decodeItems(resp.Body)
	if err != nil {
		return AdsResult{}, err
	}

	ads := make([]Ad, 0, len(items))
	for i, it := range items {
		ads = append(ads, Ad{
			AdArchiveID: it.AdArchiveID,
			PageID:      it.PageID,
			PageName:    it.PageName,
			IsActive:    it.IsActive,
			StartDate:   unixPtrToTime(it.StartDate),
			EndDate:     unixPtrToTime(it.EndDate),
			Body:        it.Snapshot.Body.Text,
			Title:       it.Snapshot.Title,
			CtaText:     it.Snapshot.CtaText,
			CtaType:     it.Snapshot.CtaType,
			LinkURL:     it.Snapshot.LinkURL,
			Raw:         rawItems[i],
		})
	}
	return AdsResult{Ads: ads}, nil
}

// unixPtrToTime converts a possibly-nil unix-seconds pointer (Apify sends
// JSON null for start_date/end_date on ads with no known date) into a
// time.Time, returning the zero value when nil rather than misrepresenting
// "unknown" as the 1970 epoch.
func unixPtrToTime(sec *int64) time.Time {
	if sec == nil {
		return time.Time{}
	}
	return time.Unix(*sec, 0).UTC()
}

// decodeItems reads the full response body, keeping each dataset item's
// raw bytes (for the Ad.Raw JSONB column) alongside its typed decode.
func decodeItems(body io.Reader) ([]apifyAdItem, []json.RawMessage, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, nil, fmt.Errorf("apify: read response body: %w", err)
	}
	var rawItems []json.RawMessage
	if err := json.Unmarshal(data, &rawItems); err != nil {
		return nil, nil, fmt.Errorf("apify: unmarshal raw items: %w", err)
	}
	items := make([]apifyAdItem, len(rawItems))
	for i, raw := range rawItems {
		if err := json.Unmarshal(raw, &items[i]); err != nil {
			return nil, nil, fmt.Errorf("apify: unmarshal item %d: %w", i, err)
		}
	}
	return items, rawItems, nil
}
