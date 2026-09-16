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
	FetchTrend(ctx context.Context, keyword string) (TrendResult, error)
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
				Value int `json:"value"`
			} `json:"values"`
		} `json:"timeline_data"`
	} `json:"interest_over_time"`
}

func (c *HTTPClient) FetchTrend(ctx context.Context, keyword string) (TrendResult, error) {
	q := url.Values{}
	q.Set("engine", "google_trends")
	q.Set("q", keyword)
	q.Set("api_key", c.apiKey)

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
			val = tl.Values[0].Value
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
