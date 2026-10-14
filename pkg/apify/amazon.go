package apify

import (
	"context"
	"encoding/json"
	"fmt"
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
	// JobID is recorded on the concurrency lease so a stuck slot can be
	// traced back to the job holding it.
	JobID string
}

type ProductsResult struct {
	Products []Product
}

type ProductClient interface {
	FetchProducts(ctx context.Context, params ProductParams) (ProductsResult, error)
}

// AmazonHTTPClient maps the Amazon crawler actor onto Product. The HTTP
// call, token selection and concurrency slot all live in Runner; what stays
// here is specific to this actor -- its input shape, its item shape, and the
// mapping to our domain type.
type AmazonHTTPClient struct {
	actorID string
	runner  *Runner
}

func NewAmazonHTTPClient(actorID string, runner *Runner) *AmazonHTTPClient {
	if actorID == "" {
		actorID = "junglee~amazon-crawler"
	}
	return &AmazonHTTPClient{actorID: actorID, runner: runner}
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
	MaxItemsPerStartURL   int                          `json:"maxItemsPerStartUrl,omitempty"`
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
	rawItems, err := c.runner.RunSync(ctx, c.actorID, params.JobID, input)
	if err != nil {
		return ProductsResult{}, err
	}
	items, err := DecodeItems[amazonProductItem](rawItems)
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
