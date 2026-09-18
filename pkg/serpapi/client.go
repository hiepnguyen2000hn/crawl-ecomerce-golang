package serpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type TrendPoint struct {
	Date  string
	Value int
}

type TrendResult struct {
	Keyword string
	Points  []TrendPoint
	Raw     json.RawMessage
}

type Client interface {
	// FetchTrend fetches Google Trends interest-over-time data for keyword,
	// optionally scoped to a country via geo (a Google Trends location code
	// such as "VN", "US"; empty means worldwide) and to a Google Trends
	// date range via dateRange (e.g. "today 12-m", "today 24-m"; empty
	// defaults to "today 12-m").
	FetchTrend(ctx context.Context, keyword, geo, dateRange string) (TrendResult, error)
}

type HTTPClient struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func NewHTTPClient(apiKey, baseURL string, httpClient *http.Client) *HTTPClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &HTTPClient{apiKey: apiKey, baseURL: baseURL, httpClient: httpClient}
}

type serpApiResponse struct {
	InterestOverTime struct {
		TimelineData []struct {
			Date   string `json:"date"`
			Values []struct {
				// SerpAPI returns Value as a string (e.g. "45", sometimes
				// "<1"); ExtractedValue is the numeric form and is what we
				// actually want.
				Value          string `json:"value"`
				ExtractedValue int    `json:"extracted_value"`
			} `json:"values"`
		} `json:"timeline_data"`
	} `json:"interest_over_time"`
}

func (c *HTTPClient) FetchTrend(ctx context.Context, keyword, geo, dateRange string) (TrendResult, error) {
	q := url.Values{}
	q.Set("engine", "google_trends")
	q.Set("q", keyword)
	q.Set("api_key", c.apiKey)
	if dateRange == "" {
		// Explicitly request the trailing 12 months rather than relying on
		// SerpAPI's default (currently also "today 12-m", but the API
		// could change its default without notice).
		dateRange = "today 12-m"
	}
	q.Set("date", dateRange)
	if geo != "" {
		q.Set("geo", geo)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/search.json?"+q.Encode(), nil)
	if err != nil {
		return TrendResult{}, fmt.Errorf("serpapi: build request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return TrendResult{}, fmt.Errorf("serpapi: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return TrendResult{}, fmt.Errorf("serpapi: returned status %d", resp.StatusCode)
	}

	var body serpApiResponse
	bodyBytes, err := readAndDecode(resp, &body)
	if err != nil {
		return TrendResult{}, err
	}

	result := TrendResult{Keyword: keyword, Raw: bodyBytes}
	for _, tl := range body.InterestOverTime.TimelineData {
		val := 0
		if len(tl.Values) > 0 {
			val = tl.Values[0].ExtractedValue
		}
		result.Points = append(result.Points, TrendPoint{Date: tl.Date, Value: val})
	}
	return result, nil
}

func readAndDecode(resp *http.Response, out *serpApiResponse) (json.RawMessage, error) {
	dec := json.NewDecoder(resp.Body)
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("serpapi: decode raw response: %w", err)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return nil, fmt.Errorf("serpapi: decode response: %w", err)
	}
	return raw, nil
}
