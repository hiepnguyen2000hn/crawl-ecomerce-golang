package apify

import (
	"context"
	"encoding/json"
	"fmt"
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
	// JobID is recorded on the concurrency lease so a stuck slot can be
	// traced back to the job holding it.
	JobID string
}

type China1688ProductsResult struct {
	Products []China1688Product
}

type China1688Client interface {
	FetchProducts(ctx context.Context, params China1688Params) (China1688ProductsResult, error)
}

// China1688HTTPClient maps the 1688 wholesale actor onto China1688Product.
// The HTTP call, token selection and concurrency slot all live in Runner;
// what stays here is specific to this actor.
type China1688HTTPClient struct {
	actorID string
	runner  *Runner
}

func NewChina1688HTTPClient(actorID string, runner *Runner) *China1688HTTPClient {
	if actorID == "" {
		actorID = "zen-studio~1688-wholesale-scraper"
	}
	return &China1688HTTPClient{actorID: actorID, runner: runner}
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
	rawItems, err := c.runner.RunSync(ctx, c.actorID, params.JobID, input)
	if err != nil {
		return China1688ProductsResult{}, err
	}
	items, err := DecodeItems[china1688ProductItem](rawItems)
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
