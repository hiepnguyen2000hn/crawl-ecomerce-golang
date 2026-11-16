package pipiads

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	ErrUnauthorized   = errors.New("pipiads: unauthorized (token invalid or expired)")
	ErrQuotaExhausted = errors.New("pipiads: ad credits quota exhausted")
)

// Client talks to PiPiAds REST APIs
type Client interface {
	GetMemberUsage(ctx context.Context, account *Account) (*MemberUsage, error)
	GetAiCredits(ctx context.Context, account *Account) (int, error)
	SearchEcommerceAds(ctx context.Context, account *Account, req SearchRequest) ([]EcommerceAd, error)
	Login(ctx context.Context, account *Account) (string, error)
}

type HTTPClient struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewHTTPClient(baseURL string) *HTTPClient {
	if baseURL == "" {
		baseURL = "https://www.pipiads.com"
	}
	return &HTTPClient{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

type MemberUsage struct {
	TotalCredits int `json:"total"`
	UsedCredits  int `json:"usage"`
}

type SearchRequest struct {
	Keyword     string `json:"search,omitempty"`
	SearchType  int    `json:"search_type"`  // default 1
	Sort        int    `json:"sort"`         // default 2 (Hot/Winning)
	SortType    string `json:"sort_type"`    // default "desc"
	CurrentPage int    `json:"current_page"` // default 1
	PageSize    int    `json:"page_size"`    // default 20
}

type EcommerceAd struct {
	VideoID        string          `json:"video_id"`
	AdID           string          `json:"ad_id"`
	Title          string          `json:"title"`
	Description    string          `json:"desc"`
	AppName        string          `json:"app_name"`
	AdvertiserName string          `json:"advertiser_name"`
	Platform       int             `json:"platform"`
	PlayCount      int64           `json:"play_count"`
	DiggCount      int64           `json:"digg_count"`
	CommentCount   int64           `json:"comment_count"`
	ShareCount     int64           `json:"share_count"`
	CoverURL       string          `json:"cover"`
	ImageURL       string          `json:"image"`
	LandingPageURL string          `json:"landing_page_url"`
	FirstTime      int64           `json:"first_time"`
	LastTime       int64           `json:"last_put_time"`
	PutDays        int             `json:"put_days"`
	Raw            json.RawMessage `json:"raw"`
}

func (c *HTTPClient) createTransport(proxyStr string) *http.Transport {
	transport := &http.Transport{
		MaxIdleConns:        100,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression: false,
	}
	if proxyStr != "" {
		if !strings.Contains(proxyStr, "://") {
			proxyStr = "http://" + proxyStr
		}
		if proxyURL, err := url.Parse(proxyStr); err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}
	return transport
}

func (c *HTTPClient) setHeaders(req *http.Request, acc *Account) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Origin", c.BaseURL)
	req.Header.Set("Referer", c.BaseURL+"/vi/ad-search/ecom_ads")
	req.Header.Set("timezone_offset", "-540")
	req.Header.Set("time_zone_id", "Asia/Tokyo")
	req.Header.Set("language_code", "vi")

	if acc != nil {
		if acc.AccessToken != "" {
			req.Header.Set("access_token", acc.AccessToken)
		}
		if acc.DeviceID != "" {
			req.Header.Set("device_id", acc.DeviceID)
		}
		cookieHeader := fmt.Sprintf("language=vi; uid=%s", acc.Email)
		if acc.AccessToken != "" {
			info := fmt.Sprintf(`{"access_token":"%s","level":"FREE"}`, acc.AccessToken)
			cookieHeader += fmt.Sprintf("; PP-userInfo=%s", url.QueryEscape(info))
		}
		req.Header.Set("Cookie", cookieHeader)
	}
}

// GetMemberUsage queries /v1/api/member/usage to inspect available ad_credits_extra
func (c *HTTPClient) GetMemberUsage(ctx context.Context, account *Account) (*MemberUsage, error) {
	reqURL := fmt.Sprintf("%s/v1/api/member/usage", c.BaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req, account)

	client := &http.Client{
		Transport: c.createTransport(account.Proxy),
		Timeout:   30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pipiads: request member usage: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("pipiads: read member usage body: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == 401 {
		return nil, ErrUnauthorized
	}

	var parsed struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Result  struct {
			Data struct {
				AdCreditsExtra MemberUsage `json:"ad_credits_extra"`
			} `json:"data"`
		} `json:"result"`
	}

	if err := json.Unmarshal(bodyBytes, &parsed); err != nil {
		return nil, fmt.Errorf("pipiads: parse member usage json: %w (body: %s)", err, string(bodyBytes))
	}

	if parsed.Code == 4000021 || parsed.Code == 4000022 {
		return nil, ErrUnauthorized
	}

	return &parsed.Result.Data.AdCreditsExtra, nil
}

// GetAiCredits queries /v3/api/ai/credits
func (c *HTTPClient) GetAiCredits(ctx context.Context, account *Account) (int, error) {
	reqURL := fmt.Sprintf("%s/v3/api/ai/credits", c.BaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return 0, err
	}
	c.setHeaders(req, account)

	client := &http.Client{
		Transport: c.createTransport(account.Proxy),
		Timeout:   30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("pipiads: request ai credits: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("pipiads: read ai credits body: %w", err)
	}

	var parsed struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Credits      int `json:"credits"`
			TotalCredits int `json:"total_credits"`
			UsedCredits  int `json:"used_credits"`
		} `json:"data"`
	}

	if err := json.Unmarshal(bodyBytes, &parsed); err != nil {
		return 0, fmt.Errorf("pipiads: parse ai credits json: %w", err)
	}
	if parsed.Code == 4000021 || parsed.Code == 4000022 {
		return 0, ErrUnauthorized
	}

	return parsed.Data.Credits, nil
}

// SearchEcommerceAds queries /v3/api/search4/at/video/search-e-commerce
func (c *HTTPClient) SearchEcommerceAds(ctx context.Context, account *Account, req SearchRequest) ([]EcommerceAd, error) {
	reqURL := fmt.Sprintf("%s/v3/api/search4/at/video/search-e-commerce", c.BaseURL)

	if req.SearchType == 0 {
		req.SearchType = 1
	}
	if req.Sort == 0 {
		req.Sort = 2
	}
	if req.SortType == "" {
		req.SortType = "desc"
	}
	if req.CurrentPage <= 0 {
		req.CurrentPage = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 20
	}

	type searchPayload struct {
		IsParticiple bool   `json:"is_participle"`
		SearchType   int    `json:"search_type"`
		Search       string `json:"search,omitempty"`
		Sort         int    `json:"sort"`
		SortType     string `json:"sort_type"`
		CurrentPage  int    `json:"current_page"`
		PageSize     int    `json:"page_size"`
	}

	payload := searchPayload{
		IsParticiple: false,
		SearchType:   req.SearchType,
		Search:       req.Keyword,
		Sort:         req.Sort,
		SortType:     req.SortType,
		CurrentPage:  req.CurrentPage,
		PageSize:     req.PageSize,
	}

	reqBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("pipiads: marshal search payload: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(reqBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	c.setHeaders(httpReq, account)

	client := &http.Client{
		Transport: c.createTransport(account.Proxy),
		Timeout:   30 * time.Second,
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("pipiads: search request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("pipiads: read search body: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == 401 {
		return nil, ErrUnauthorized
	}

	var parsed struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Result  struct {
			Data []json.RawMessage `json:"data"`
		} `json:"result"`
	}

	if err := json.Unmarshal(bodyBytes, &parsed); err != nil {
		return nil, fmt.Errorf("pipiads: parse search response: %w (body: %s)", err, string(bodyBytes))
	}

	if parsed.Code == 4000021 || parsed.Code == 4000022 {
		return nil, ErrUnauthorized
	}
	if parsed.Code == 4000163 {
		return nil, ErrQuotaExhausted
	}

	var ads []EcommerceAd
	for _, raw := range parsed.Result.Data {
		var item EcommerceAd
		if err := json.Unmarshal(raw, &item); err == nil {
			item.Raw = raw
			if item.CoverURL == "" && item.ImageURL != "" {
				item.CoverURL = item.ImageURL
			}
			ads = append(ads, item)
		}
	}

	return ads, nil
}

// Login performs PUT /v1/api/member/login
func (c *HTTPClient) Login(ctx context.Context, account *Account) (string, error) {
	reqURL := fmt.Sprintf("%s/v1/api/member/login", c.BaseURL)

	payload := map[string]interface{}{
		"email":     account.Email,
		"password":  account.Password,
		"device_id": account.DeviceID,
		"uuid":      "583e7915-9c7c-41f7-aa44-ca670c864d9d",
	}

	reqBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPut, reqURL, bytes.NewReader(reqBytes))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	c.setHeaders(httpReq, account)

	client := &http.Client{
		Transport: c.createTransport(account.Proxy),
		Timeout:   30 * time.Second,
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("pipiads: login request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var parsed struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Result  struct {
			Data struct {
				AccessToken string `json:"access_token"`
			} `json:"data"`
		} `json:"result"`
	}

	if err := json.Unmarshal(bodyBytes, &parsed); err != nil {
		return "", fmt.Errorf("pipiads: login response json: %w", err)
	}

	if parsed.Result.Data.AccessToken != "" {
		return parsed.Result.Data.AccessToken, nil
	}

	return "", fmt.Errorf("pipiads: login failed: %s (code %d)", parsed.Message, parsed.Code)
}
