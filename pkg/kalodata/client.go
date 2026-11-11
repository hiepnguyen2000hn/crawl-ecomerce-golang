package kalodata

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	defaultBaseURL = "https://www.kalodata.com"
	defaultCDNBase = "https://img.kalocdn.com"
	userAgent      = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0 Safari/537.36"
)

type ProductParams struct {
	Country, StartDate, EndDate     string
	PageNo, PageSize                int
	SortField, SortType             string
	CategoryIDs                     []int
	Revenue, UnitPrice, SalesChannel string
	Strategy, CommissionRate        string
	Creator, LaunchDate             string
}

type VideoParams struct {
	Country, StartDate, EndDate string
	PageNo, PageSize            int
	SortField, SortType         string
	ProductID, VideoType        string
	Duration, CreatorFollowers  string
}

// Product/Video field types match the live Kalodata API: revenue/unit_price/
// commission/views/duration are formatted display strings ("฿2.57m", "1%",
// "103.14k", "44s"); GMV is the numeric product revenue; sale/rating are numeric.
type Product struct {
	ProductID      string
	Title          string  // product_title
	Revenue        string  // "฿2.57m"
	GMV            float64 // gmv_A (numeric)
	Sales          int     // sale
	Rating         float64 // product_rating
	UnitPrice      string  // "฿18.77k"
	CommissionRate string  // "1%"
	ThumbnailURL   string
	ProductURL     string
	Raw            json.RawMessage
}

type Video struct {
	VideoID      string
	Handle       string
	Title        string // description
	Views        string // "103.14k"
	Revenue      string // "฿1.14m"
	Sales        int    // sale
	Duration     string // "44s"
	ThumbnailURL string
	TikTokURL    string
	Detail       json.RawMessage
	Raw          json.RawMessage
}

type Client interface {
	AllLastDay(ctx context.Context, country string) (string, error)
	SearchProducts(ctx context.Context, p ProductParams) ([]Product, error)
	SearchVideos(ctx context.Context, p VideoParams) ([]Video, error)
	VideoDetail(ctx context.Context, country, id, start, end string) (json.RawMessage, error)
}

type HTTPClient struct {
	baseURL  string
	cdnBase  string
	registry *CookieRegistry
	retries  int
	httpc    *http.Client
	mu       sync.Mutex
	proxied  map[string]*http.Client
}

func NewHTTPClient(baseURL, cdnBase string, reg *CookieRegistry, retries int) *HTTPClient {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	if cdnBase == "" {
		cdnBase = defaultCDNBase
	}
	if retries <= 0 {
		retries = 3
	}
	return &HTTPClient{
		baseURL:  baseURL,
		cdnBase:  cdnBase,
		registry: reg,
		retries:  retries,
		httpc:    &http.Client{Timeout: 30 * time.Second},
		proxied:  map[string]*http.Client{},
	}
}

// listEnvelope matches Kalodata's {"data": [...]} list responses.
type listEnvelope struct {
	Data []json.RawMessage `json:"data"`
}

func asFloat(m map[string]json.RawMessage, keys ...string) float64 {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			var f float64
			if json.Unmarshal(v, &f) == nil {
				return f
			}
		}
	}
	return 0
}

func asInt(m map[string]json.RawMessage, keys ...string) int {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			var i int
			if json.Unmarshal(v, &i) == nil {
				return i
			}
		}
	}
	return 0
}

func asStr(m map[string]json.RawMessage, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			var s string
			if json.Unmarshal(v, &s) == nil {
				return s
			}
		}
	}
	return ""
}

// mapProducts/mapVideos parse raw items into normalized records, keeping the
// original bytes as Raw. Field paths are best-effort; confirm against a live
// dataset (raw always holds the full object).
func mapProducts(raw []json.RawMessage, cdn string) []Product {
	out := make([]Product, 0, len(raw))
	for _, r := range raw {
		var m map[string]json.RawMessage
		_ = json.Unmarshal(r, &m)
		id := asStr(m, "id")
		out = append(out, Product{
			ProductID:      id,
			Title:          asStr(m, "product_title"),
			Revenue:        asStr(m, "revenue"),
			GMV:            asFloat(m, "gmv_A", "gmv"),
			Sales:          asInt(m, "sale"),
			Rating:         asFloat(m, "product_rating"),
			UnitPrice:      asStr(m, "unit_price"),
			CommissionRate: asStr(m, "commission_rate"),
			ThumbnailURL:   productCover(cdn, id),
			Raw:            r,
		})
	}
	return out
}

func mapVideos(raw []json.RawMessage, cdn string) []Video {
	out := make([]Video, 0, len(raw))
	for _, r := range raw {
		var m map[string]json.RawMessage
		_ = json.Unmarshal(r, &m)
		id := asStr(m, "id")
		handle := asStr(m, "handle")
		v := Video{
			VideoID:      id,
			Handle:       handle,
			Title:        asStr(m, "description", "title"),
			Views:        asStr(m, "views"),
			Revenue:      asStr(m, "revenue"),
			Sales:        asInt(m, "sale"),
			Duration:     asStr(m, "duration"),
			ThumbnailURL: videoCover(cdn, id),
			Raw:          r,
		}
		if id != "" && handle != "" {
			v.TikTokURL = fmt.Sprintf("https://www.tiktok.com/@%s/video/%s", handle, id)
		}
		out = append(out, v)
	}
	return out
}

func productCover(cdn, id string) string {
	if id == "" {
		return ""
	}
	return cdn + "/tiktok.product/" + id + "/cover.png"
}

func videoCover(cdn, id string) string {
	if id == "" {
		return ""
	}
	return cdn + "/tiktok.video/" + id + "/cover.png"
}

// postLeased POSTs payload to path, leasing a cookie for op and rotating on
// cookie failure (401/403 → MarkDead, then try another). Returns the raw body.
func (c *HTTPClient) postLeased(ctx context.Context, op, path string, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for attempt := 0; attempt < c.retries; attempt++ {
		lease, err := c.registry.Lease(ctx, op)
		if err != nil {
			return nil, err // ErrNoCookies / ErrQuotaExhausted propagate
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
		c.setHeaders(req, lease.Cookie)
		resp, err := c.doVia(lease.Proxy, req)
		if err != nil {
			lastErr = err
			continue
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		switch {
		case resp.StatusCode == http.StatusOK:
			return data, nil
		case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
			// Cookie rejected (logged out / invalid) → retire it, try another.
			_ = c.registry.MarkDead(ctx, lease.ID)
			lastErr = fmt.Errorf("kalodata: %s %d (cookie retired)", path, resp.StatusCode)
			continue
		case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
			// Transient (rate limit / server error) → rotate to another cookie
			// and retry rather than failing the whole job on a blip.
			lastErr = fmt.Errorf("kalodata: %s %d (transient)", path, resp.StatusCode)
			continue
		default:
			return nil, fmt.Errorf("kalodata: %s returned %d: %s", path, resp.StatusCode, trim(data))
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("kalodata: %s exhausted %d attempts", path, c.retries)
	}
	return nil, lastErr
}

// doVia sends req through the account's residential proxy (empty = direct). A
// per-proxy http.Client is cached so each account keeps its own egress IP.
func (c *HTTPClient) doVia(proxy string, req *http.Request) (*http.Response, error) {
	if proxy == "" {
		return c.httpc.Do(req)
	}
	c.mu.Lock()
	hc := c.proxied[proxy]
	if hc == nil {
		pu, err := url.Parse(proxy)
		if err != nil {
			c.mu.Unlock()
			return nil, fmt.Errorf("kalodata: bad proxy %q: %w", proxy, err)
		}
		hc = &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{Proxy: http.ProxyURL(pu)}}
		c.proxied[proxy] = hc
	}
	c.mu.Unlock()
	return hc.Do(req)
}

func (c *HTTPClient) setHeaders(req *http.Request, cookie string) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", cookie)
	req.Header.Set("Origin", c.baseURL)
	req.Header.Set("Referer", c.baseURL+"/product")
	req.Header.Set("User-Agent", userAgent)
}

func trim(b []byte) string {
	s := string(b)
	if len(s) > 300 {
		return s[:300] + "..."
	}
	return s
}

func (c *HTTPClient) AllLastDay(ctx context.Context, country string) (string, error) {
	lease, err := c.registry.Lease(ctx, "search")
	if err != nil {
		return "", err
	}
	u := c.baseURL + "/api/allLastDay?" + url.Values{"country": {country}}.Encode()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	c.setHeaders(req, lease.Cookie)
	resp, err := c.doVia(lease.Proxy, req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("kalodata: allLastDay %d: %s", resp.StatusCode, trim(data))
	}
	// Shape (verified live): {"success":true,"data":[{"country":"VN","lastDay":"2026-11-10"},...]}
	var env struct {
		Data []struct {
			Country string `json:"country"`
			LastDay string `json:"lastDay"`
		} `json:"data"`
	}
	_ = json.Unmarshal(data, &env)
	for _, d := range env.Data {
		if strings.EqualFold(d.Country, country) {
			return d.LastDay, nil
		}
	}
	return "", nil
}

func (c *HTTPClient) SearchProducts(ctx context.Context, p ProductParams) ([]Product, error) {
	payload := map[string]any{
		"country":     p.Country,
		"startDate":   p.StartDate,
		"endDate":     p.EndDate,
		"pageNo":      p.PageNo,
		"pageSize":    p.PageSize,
		"sort":        []map[string]string{{"field": p.SortField, "type": p.SortType}},
		"cateIds":     intsOrEmpty(p.CategoryIDs),
		"showCateIds": intsOrEmpty(p.CategoryIDs),
	}
	putIf(payload, "revenue", p.Revenue)
	putIf(payload, "unitPrice", p.UnitPrice)
	putIf(payload, "salesChannel", p.SalesChannel)
	putIf(payload, "strategy", p.Strategy)
	putIf(payload, "commissionRate", p.CommissionRate)
	putIf(payload, "creator", p.Creator)
	putIf(payload, "launchDate", p.LaunchDate)

	data, err := c.postLeased(ctx, "search", "/product/queryList", payload)
	if err != nil {
		return nil, err
	}
	var env listEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("kalodata: decode products: %w", err)
	}
	return mapProducts(env.Data, c.cdnBase), nil
}

func (c *HTTPClient) SearchVideos(ctx context.Context, p VideoParams) ([]Video, error) {
	payload := map[string]any{
		"country":   p.Country,
		"startDate": p.StartDate,
		"endDate":   p.EndDate,
		"pageNo":    p.PageNo,
		"pageSize":  p.PageSize,
		"sort":      []map[string]string{{"field": p.SortField, "type": p.SortType}},
	}
	putIf(payload, "productId", p.ProductID)
	putIf(payload, "videoType", p.VideoType)
	putIf(payload, "duration", p.Duration)
	putIf(payload, "creatorFollowers", p.CreatorFollowers)

	data, err := c.postLeased(ctx, "search", "/video/queryList", payload)
	if err != nil {
		return nil, err
	}
	var env listEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("kalodata: decode videos: %w", err)
	}
	return mapVideos(env.Data, c.cdnBase), nil
}

func (c *HTTPClient) VideoDetail(ctx context.Context, country, id, start, end string) (json.RawMessage, error) {
	payload := map[string]any{"id": id, "country": country, "startDate": start, "endDate": end}
	data, err := c.postLeased(ctx, "detail", "/video/detail", payload)
	if err != nil {
		return nil, err
	}
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("kalodata: decode video detail: %w", err)
	}
	return env.Data, nil
}

func putIf(m map[string]any, key, val string) {
	if val != "" {
		m[key] = val
	}
}

func intsOrEmpty(ids []int) []int {
	if ids == nil {
		return []int{}
	}
	return ids
}
