package apify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// AccountLimits is what GET /users/me/limits reports for one account.
//
// Field names follow Apify's, which are not all obvious:
//   - the concurrency cap is maxConcurrentActorJobs, not ...ActorRuns
//   - monthlyUsageCycle is a rolling 30 days from the account's own start
//     date, so two accounts created a week apart reset a week apart. There is
//     no calendar-month reset to wait for.
type AccountLimits struct {
	MaxConcurrentJobs int
	MaxMonthlyUsageUS float64
	MonthlyUsageUSD   float64
	ActiveJobCount    int
	CycleStartsAt     time.Time
	CycleEndsAt       time.Time
}

// Remaining reports the unspent budget. Negative is possible: Apify settles
// pay-per-result charges asynchronously, so an account can end up slightly
// past its own limit.
func (l AccountLimits) Remaining() float64 {
	return l.MaxMonthlyUsageUS - l.MonthlyUsageUSD
}

// UsedFraction is 0..1 (or above 1 after async settlement overshoots).
func (l AccountLimits) UsedFraction() float64 {
	if l.MaxMonthlyUsageUS <= 0 {
		return 0
	}
	return l.MonthlyUsageUSD / l.MaxMonthlyUsageUS
}

type limitsResponse struct {
	Data struct {
		MonthlyUsageCycle struct {
			StartAt time.Time `json:"startAt"`
			EndAt   time.Time `json:"endAt"`
		} `json:"monthlyUsageCycle"`
		Limits struct {
			MaxMonthlyUsageUsd     float64 `json:"maxMonthlyUsageUsd"`
			MaxConcurrentActorJobs int     `json:"maxConcurrentActorJobs"`
		} `json:"limits"`
		Current struct {
			MonthlyUsageUsd     float64 `json:"monthlyUsageUsd"`
			ActiveActorJobCount int     `json:"activeActorJobCount"`
		} `json:"current"`
	} `json:"data"`
}

// LimitsClient reads account limits. It takes the token per call rather than
// holding one, because its whole job is to ask about many different accounts.
type LimitsClient struct {
	BaseURL string
	HTTP    *http.Client
}

func NewLimitsClient(baseURL string) *LimitsClient {
	if baseURL == "" {
		baseURL = "https://api.apify.com/v2"
	}
	return &LimitsClient{BaseURL: baseURL, HTTP: &http.Client{Timeout: 20 * time.Second}}
}

func (c *LimitsClient) Fetch(ctx context.Context, token string) (AccountLimits, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/users/me/limits", nil)
	if err != nil {
		return AccountLimits{}, fmt.Errorf("apify: build limits request: %w", err)
	}
	q := req.URL.Query()
	q.Set("token", token)
	req.URL.RawQuery = q.Encode()

	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return AccountLimits{}, fmt.Errorf("apify: limits request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return AccountLimits{}, fmt.Errorf("apify: limits returned status %d: %s", resp.StatusCode, body)
	}

	var payload limitsResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return AccountLimits{}, fmt.Errorf("apify: decode limits: %w", err)
	}
	d := payload.Data
	return AccountLimits{
		MaxConcurrentJobs: d.Limits.MaxConcurrentActorJobs,
		MaxMonthlyUsageUS: d.Limits.MaxMonthlyUsageUsd,
		MonthlyUsageUSD:   d.Current.MonthlyUsageUsd,
		ActiveJobCount:    d.Current.ActiveActorJobCount,
		CycleStartsAt:     d.MonthlyUsageCycle.StartAt,
		CycleEndsAt:       d.MonthlyUsageCycle.EndAt,
	}, nil
}
