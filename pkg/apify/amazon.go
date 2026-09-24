package apify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// Product is a normalized Amazon product record.
type Product struct {
	ASIN         string
	Title        string
	Price        float64
	Currency     string
	Rating       float64
	ReviewsCount int
	Brand        string
	ImageURL     string
	ProductURL   string
	Raw          json.RawMessage
}

// ProductParams mirrors the junglee/amazon-crawler actor's input schema.
// Either Keyword or URLs (product/category/ASIN URLs) must be set.
type ProductParams struct {
	Keyword  string
	URLs     []string
	Country  string
	MaxItems int
}

type ProductsResult struct {
	Products []Product
}

type ProductClient interface {
	FetchProducts(ctx context.Context, params ProductParams) (ProductsResult, error)
}

type AmazonHTTPClient struct {
	apiToken   string
	actorID    string
	baseURL    string
	httpClient *http.Client
}

func NewAmazonHTTPClient(apiToken, actorID, baseURL string, httpClient *http.Client) *AmazonHTTPClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if actorID == "" {
		actorID = "junglee~amazon-crawler"
	}
	return &AmazonHTTPClient{apiToken: apiToken, actorID: actorID, baseURL: baseURL, httpClient: httpClient}
}

type amazonCategoryOrProductURL struct {
	URL string `json:"url"`
}

// amazonRunInput mirrors the junglee/amazon-crawler actor's actual input
// schema (fetched from its /builds/default endpoint): the actor has no
// standalone "search"/"maxItems"/"country" fields — a keyword search is
// expressed as an Amazon search-results URL inside categoryOrProductUrls,
// and the per-URL item cap is maxItemsPerStartUrl.
type amazonRunInput struct {
	CategoryOrProductURLs []amazonCategoryOrProductURL `json:"categoryOrProductUrls"`
	MaxItemsPerStartURL   int                           `json:"maxItemsPerStartUrl,omitempty"`
}

type amazonPrice struct {
	Value    float64 `json:"value"`
	Currency string  `json:"currency"`
}

type amazonProductItem struct {
	ASIN         string      `json:"asin"`
	Title        string      `json:"title"`
	Price        amazonPrice `json:"price"`
	Stars        float64     `json:"stars"`
	ReviewsCount int         `json:"reviewsCount"`
	Brand        string      `json:"brand"`
	Thumbnail    string      `json:"thumbnailImage"`
	URL          string      `json:"url"`
}

// amazonDomains maps a country code to the Amazon storefront domain used to
// build search URLs. Unrecognized/empty codes fall back to amazon.com.
var amazonDomains = map[string]string{
	"US": "amazon.com",
	"UK": "amazon.co.uk",
	"GB": "amazon.co.uk",
	"DE": "amazon.de",
	"CA": "amazon.ca",
	"FR": "amazon.fr",
	"IT": "amazon.it",
	"ES": "amazon.es",
	"JP": "amazon.co.jp",
	"IN": "amazon.in",
}

func (c *AmazonHTTPClient) FetchProducts(ctx context.Context, params ProductParams) (ProductsResult, error) {
	maxItems := params.MaxItems
	if maxItems <= 0 {
		maxItems = 50
	}
	input := amazonRunInput{MaxItemsPerStartURL: maxItems}
	if params.Keyword != "" {
		domain := amazonDomains[params.Country]
		if domain == "" {
			domain = "amazon.com"
		}
		searchURL := fmt.Sprintf("https://www.%s/s?k=%s", domain, url.QueryEscape(params.Keyword))
		input.CategoryOrProductURLs = append(input.CategoryOrProductURLs, amazonCategoryOrProductURL{URL: searchURL})
	}
	for _, u := range params.URLs {
		input.CategoryOrProductURLs = append(input.CategoryOrProductURLs, amazonCategoryOrProductURL{URL: u})
	}
	if len(input.CategoryOrProductURLs) == 0 {
		return ProductsResult{}, fmt.Errorf("apify: either keyword or urls is required")
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return ProductsResult{}, fmt.Errorf("apify: marshal input: %w", err)
	}

	q := url.Values{}
	q.Set("token", c.apiToken)
	reqURL := fmt.Sprintf("%s/acts/%s/run-sync-get-dataset-items?%s", c.baseURL, c.actorID, q.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(payload))
	if err != nil {
		return ProductsResult{}, fmt.Errorf("apify: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ProductsResult{}, fmt.Errorf("apify: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return ProductsResult{}, fmt.Errorf("apify: returned status %d: %s", resp.StatusCode, string(body))
	}

	items, rawItems, err := decodeAmazonItems(resp.Body)
	if err != nil {
		return ProductsResult{}, err
	}

	products := make([]Product, 0, len(items))
	for i, it := range items {
		products = append(products, Product{
			ASIN:         it.ASIN,
			Title:        it.Title,
			Price:        it.Price.Value,
			Currency:     it.Price.Currency,
			Rating:       it.Stars,
			ReviewsCount: it.ReviewsCount,
			Brand:        it.Brand,
			ImageURL:     it.Thumbnail,
			ProductURL:   it.URL,
			Raw:          rawItems[i],
		})
	}
	return ProductsResult{Products: products}, nil
}

// decodeAmazonItems reads the full response body, keeping each dataset
// item's raw bytes (for the Product.Raw JSONB column) alongside its typed
// decode.
func decodeAmazonItems(body io.Reader) ([]amazonProductItem, []json.RawMessage, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, nil, fmt.Errorf("apify: read response body: %w", err)
	}
	var rawItems []json.RawMessage
	if err := json.Unmarshal(data, &rawItems); err != nil {
		return nil, nil, fmt.Errorf("apify: unmarshal raw items: %w", err)
	}
	items := make([]amazonProductItem, len(rawItems))
	for i, raw := range rawItems {
		if err := json.Unmarshal(raw, &items[i]); err != nil {
			return nil, nil, fmt.Errorf("apify: unmarshal item %d: %w", i, err)
		}
	}
	return items, rawItems, nil
}
