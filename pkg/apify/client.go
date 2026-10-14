package apify

import (
	"context"
	"encoding/json"
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
	// JobID is recorded on the concurrency lease so a stuck slot can be
	// traced back to the job holding it.
	JobID string
}

type AdsResult struct {
	Ads []Ad
}

type Client interface {
	FetchAds(ctx context.Context, params AdParams) (AdsResult, error)
}

// HTTPClient maps the Facebook Ads Library actor onto Ad. The HTTP call,
// token selection and concurrency slot all live in Runner; what stays here is
// the part that is actually specific to this actor -- its input shape, its
// item shape, and the mapping to our domain type.
type HTTPClient struct {
	actorID string
	runner  *Runner
}

func NewHTTPClient(actorID string, runner *Runner) *HTTPClient {
	if actorID == "" {
		actorID = "igolaizola~facebook-ad-library-scraper"
	}
	return &HTTPClient{actorID: actorID, runner: runner}
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

	rawItems, err := c.runner.RunSync(ctx, c.actorID, params.JobID, input)
	if err != nil {
		return AdsResult{}, err
	}
	items, err := DecodeItems[apifyAdItem](rawItems)
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
