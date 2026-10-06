package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/hiepnv/crawl-ecomerce-golang/pkg/aiproviders"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/crawl4ai"
)

type CompletedMessage struct {
	JobID string `json:"job_id"`
}

const productSchemaJSON = `{
  "type": "object",
  "properties": {
    "products": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "product_name": {"type": "string"},
          "price": {"type": "number"},
          "currency": {"type": "string"},
          "sku": {"type": "string"}
        },
        "required": ["product_name", "price", "currency", "sku"],
        "additionalProperties": false
      }
    }
  },
  "required": ["products"],
  "additionalProperties": false
}`

type extractedProduct struct {
	ProductName string  `json:"product_name"`
	Price       float64 `json:"price"`
	Currency    string  `json:"currency"`
	SKU         string  `json:"sku"`
}

type extractedProducts struct {
	Products []extractedProduct `json:"products"`
}

// Consumer reads crawl.completed.fbads events (the same event ai-service
// consumes) and, for each distinct link_url in the job's crawled ads,
// crawls the destination page and extracts product info via structured
// AI output. It never modifies jobs.status — that belongs to ai-service.
// Per-URL failures (crawl or extraction) are logged and skipped; they
// never fail the whole message, since one broken destination link should
// not block extraction for the job's other URLs.
type Consumer struct {
	DB       *sql.DB
	Crawler  crawl4ai.Client
	Provider aiproviders.Provider
	// Concurrency = số URL của 1 job được crawl+extract cùng lúc. <= 0 → defaultConcurrency.
	Concurrency int
	// URLTimeout chặn trần crawl4ai + LLM cho MỖI URL, để 1 trang/1 lời gọi treo không giữ cả job.
	// <= 0 → defaultURLTimeout.
	URLTimeout time.Duration
}

type urlAd struct {
	URL  string
	AdID string
}

const (
	defaultConcurrency = 4
	defaultURLTimeout  = 90 * time.Second
)

func (c *Consumer) HandleMessage(body []byte) error {
	var msg CompletedMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return fmt.Errorf("worker: unmarshal completed message: %w", err)
	}

	ctx := context.Background()

	rows, err := c.DB.QueryContext(ctx,
		`SELECT DISTINCT link_url, (array_agg(id))[1] FROM fbads_raw WHERE job_id = $1 AND link_url IS NOT NULL AND link_url != '' GROUP BY link_url`,
		msg.JobID,
	)
	if err != nil {
		return fmt.Errorf("worker: load distinct link_urls: %w", err)
	}

	var urls []urlAd
	for rows.Next() {
		var u urlAd
		if err := rows.Scan(&u.URL, &u.AdID); err != nil {
			rows.Close()
			return fmt.Errorf("worker: scan link_url row: %w", err)
		}
		urls = append(urls, u)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("worker: iterate link_url rows: %w", err)
	}

	c.extractAll(ctx, msg.JobID, urls)

	// Báo cho bên đọc dữ liệu (service research) biết products của job này đã đủ. Lỗi ở đây
	// chỉ log: trả error sẽ khiến message bị xử lý lại và trích trùng sản phẩm; bên đọc có
	// timeout riêng.
	if _, err := c.DB.ExecContext(ctx,
		`UPDATE jobs SET products_done_at = now() WHERE id = $1`, msg.JobID,
	); err != nil {
		fmt.Println("worker: failed to mark products_done_at for", msg.JobID, ":", err)
	}
	return nil
}

// extractAll chạy extractOne cho mọi URL, tối đa c.Concurrency URL cùng lúc, mỗi URL có
// trần thời gian riêng. Trước đây các URL chạy tuần tự (crawl4ai + LLM mỗi URL) nên job có
// 10–15 trang đích mất vài phút và là đường găng của service research.
func (c *Consumer) extractAll(ctx context.Context, jobID string, urls []urlAd) {
	limit := c.Concurrency
	if limit <= 0 {
		limit = defaultConcurrency
	}
	timeout := c.URLTimeout
	if timeout <= 0 {
		timeout = defaultURLTimeout
	}
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for _, u := range urls {
		wg.Add(1)
		sem <- struct{}{}
		go func(u urlAd) {
			defer wg.Done()
			defer func() { <-sem }()
			uctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			c.extractOne(uctx, jobID, u.AdID, u.URL)
		}(u)
	}
	wg.Wait()
}

// extractOne handles a single URL's crawl+extract+save. Any failure is
// logged and swallowed — see the Consumer doc comment on why this never
// returns an error to HandleMessage's caller.
func (c *Consumer) extractOne(ctx context.Context, jobID, adID, url string) {
	crawlResult, err := c.Crawler.Crawl(ctx, url)
	if err != nil {
		fmt.Println("worker: crawl4ai request failed for", url, ":", err)
		return
	}
	if !crawlResult.Success {
		fmt.Println("worker: crawl4ai reported failure for", url, ":", crawlResult.Error)
		return
	}
	if crawlResult.Markdown == "" {
		fmt.Println("worker: crawl4ai returned empty markdown for", url)
		return
	}

	// 18000 runes (not the full 24000 originally used) — verified empirically
	// against the live OpenRouter API: prompts built from markdown longer
	// than ~20000-22000 runes get rejected by this model's upstream
	// provider with a generic HTTP 400 "bad request" (a free-tier backend
	// limit well below the model's advertised context window), so 18000
	// leaves headroom for the fixed instruction text around it.
	md := crawlResult.Markdown
	if r := []rune(md); len(r) > 18000 {
		md = string(r[:18000])
	}

	prompt := fmt.Sprintf("You are extracting product listings from a web page converted to Markdown.\n\n"+
		"First decide whether this page is actually a product or e-commerce listing page. If it is NOT — for example an app store listing, a blog post, a login or error page, a category index, or a social media profile — return {\"products\": []} and nothing else. Do not derive products from navigation menus, footers, related-item widgets, breadcrumbs, or other page furniture.\n\n"+
		"If it IS a product page, extract every distinct product actually offered for sale on it. Include a product only if its price is explicitly stated on the page; if no price is stated, omit that product entirely rather than guessing. If no SKU is stated, derive a short stable identifier from the product name.\n\n"+
		"Page content (Markdown):\n%s", md)
	raw, err := c.Provider.CompleteJSON(ctx, prompt, "product_extraction", []byte(productSchemaJSON))
	if err != nil {
		fmt.Println("worker: AI extraction failed for", url, ":", err)
		return
	}

	var extracted extractedProducts
	if err := json.Unmarshal(raw, &extracted); err != nil {
		fmt.Println("worker: failed to parse extracted products for", url, ":", err)
		return
	}

	for _, p := range extracted.Products {
		productJSON, err := json.Marshal(p)
		if err != nil {
			fmt.Println("worker: failed to marshal product for storage:", err)
			continue
		}
		if _, err := c.DB.ExecContext(ctx,
			`INSERT INTO products (job_id, ad_id, url, product_name, price, currency, sku, raw) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			jobID, adID, url, p.ProductName, p.Price, p.Currency, p.SKU, productJSON,
		); err != nil {
			fmt.Println("worker: failed to insert product for", url, ":", err)
		}
	}
}
