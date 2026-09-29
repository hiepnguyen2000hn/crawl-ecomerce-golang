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

// China1688Product is a normalized 1688 wholesale product record.
type China1688Product struct {
	OfferID          string
	Title            string
	PriceMin         float64
	PriceMax         float64
	Currency         string
	MOQ              int
	ImageURL         string
	SupplierName     string
	SupplierProvince string
	DetailURL        string
	Raw              json.RawMessage
}

// China1688Params mirrors the zen-studio/1688-wholesale-scraper actor's input
// schema. Either Keywords or OfferIDs must be set; OfferIDs bypasses keyword
// search when provided.
type China1688Params struct {
	Keywords []string
	OfferIDs []string
	MaxItems int
}

type China1688ProductsResult struct {
	Products []China1688Product
}

type China1688Client interface {
	FetchProducts(ctx context.Context, params China1688Params) (China1688ProductsResult, error)
}

type China1688HTTPClient struct {
	apiToken   string
	actorID    string
	baseURL    string
	httpClient *http.Client
}

func NewChina1688HTTPClient(apiToken, actorID, baseURL string, httpClient *http.Client) *China1688HTTPClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if actorID == "" {
		actorID = "zen-studio~1688-wholesale-scraper"
	}
	return &China1688HTTPClient{apiToken: apiToken, actorID: actorID, baseURL: baseURL, httpClient: httpClient}
}

// china1688RunInput mirrors the zen-studio/1688-wholesale-scraper actor's
// input schema: keyword search via keywords, or direct product lookup via
// offerIds (which bypasses keyword search when provided).
type china1688RunInput struct {
	Keywords   []string `json:"keywords,omitempty"`
	OfferIDs   []string `json:"offerIds,omitempty"`
	MaxResults int      `json:"maxResults,omitempty"`
}

type china1688Price struct {
	Min      float64 `json:"min"`
	Max      float64 `json:"max"`
	Currency string  `json:"currency"`
}

type china1688Supplier struct {
	CompanyName string `json:"companyName"`
}

type china1688ProductItem struct {
	OfferID          string            `json:"offerId"`
	Title            string            `json:"title"`
	Price            china1688Price    `json:"price"`
	MinOrderQuantity int               `json:"minOrderQuantity"`
	Images           []string          `json:"images"`
	Supplier         china1688Supplier `json:"supplier"`
	Province         string            `json:"province"`
	DetailURL        string            `json:"detailUrl"`
	Error            string            `json:"error"`
}

func (c *China1688HTTPClient) FetchProducts(ctx context.Context, params China1688Params) (China1688ProductsResult, error) {
	maxItems := params.MaxItems
	if maxItems <= 0 {
		maxItems = 50
	}
	input := china1688RunInput{MaxResults: maxItems}
	if len(params.OfferIDs) > 0 {
		input.OfferIDs = params.OfferIDs
	} else {
		input.Keywords = params.Keywords
	}
	if len(input.Keywords) == 0 && len(input.OfferIDs) == 0 {
		return China1688ProductsResult{}, fmt.Errorf("apify: either keywords or offer_ids is required")
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return China1688ProductsResult{}, fmt.Errorf("apify: marshal input: %w", err)
	}

	q := url.Values{}
	q.Set("token", c.apiToken)
	reqURL := fmt.Sprintf("%s/acts/%s/run-sync-get-dataset-items?%s", c.baseURL, c.actorID, q.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(payload))
	if err != nil {
		return China1688ProductsResult{}, fmt.Errorf("apify: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return China1688ProductsResult{}, fmt.Errorf("apify: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return China1688ProductsResult{}, fmt.Errorf("apify: returned status %d: %s", resp.StatusCode, string(body))
	}

	items, rawItems, err := decodeChina1688Items(resp.Body)
	if err != nil {
		return China1688ProductsResult{}, err
	}

	products := make([]China1688Product, 0, len(items))
	for i, it := range items {
		if it.Error != "" {
			continue
		}
		var imageURL string
		if len(it.Images) > 0 {
			imageURL = it.Images[0]
		}
		products = append(products, China1688Product{
			OfferID:          it.OfferID,
			Title:            it.Title,
			PriceMin:         it.Price.Min,
			PriceMax:         it.Price.Max,
			Currency:         it.Price.Currency,
			MOQ:              it.MinOrderQuantity,
			ImageURL:         imageURL,
			SupplierName:     it.Supplier.CompanyName,
			SupplierProvince: it.Province,
			DetailURL:        it.DetailURL,
			Raw:              rawItems[i],
		})
	}
	return China1688ProductsResult{Products: products}, nil
}

// decodeChina1688Items reads the full response body, keeping each dataset
// item's raw bytes (for the China1688Product.Raw JSONB column) alongside its
// typed decode.
func decodeChina1688Items(body io.Reader) ([]china1688ProductItem, []json.RawMessage, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, nil, fmt.Errorf("apify: read response body: %w", err)
	}
	var rawItems []json.RawMessage
	if err := json.Unmarshal(data, &rawItems); err != nil {
		return nil, nil, fmt.Errorf("apify: unmarshal raw items: %w", err)
	}
	items := make([]china1688ProductItem, len(rawItems))
	for i, raw := range rawItems {
		if err := json.Unmarshal(raw, &items[i]); err != nil {
			return nil, nil, fmt.Errorf("apify: unmarshal item %d: %w", i, err)
		}
	}
	return items, rawItems, nil
}
