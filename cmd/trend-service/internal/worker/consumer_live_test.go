//go:build live

// Live tests for trend-service: they crawl Google Trends through the real
// SerpAPI and spend quota (one search per test), so they are excluded from
// a plain `go test ./...` by the `live` build tag. Run them explicitly:
//
//	go test -tags live -v -run Live ./cmd/trend-service/internal/worker/
//
// Besides the shared variables in internal/livetest:
//
//	TREND_LIVE_KEYWORD   keyword to fetch (default "sneakers")
//	TREND_LIVE_GEO       Google Trends geo code (default "US"; "" = worldwide)
package worker

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/hiepnv/crawl-ecomerce-golang/internal/livetest"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/rabbitmq"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/serpapi"
)

// liveTrendResponse is the subset of the SerpAPI google_trends response the
// assertions need; serpapi.HTTPClient only decodes timeline values, not the
// metadata that proves the request was honoured.
type liveTrendResponse struct {
	Error          string `json:"error"`
	SearchMetadata struct {
		Status string `json:"status"`
	} `json:"search_metadata"`
	SearchParameters struct {
		Q    string `json:"q"`
		Geo  string `json:"geo"`
		Date string `json:"date"`
	} `json:"search_parameters"`
	InterestOverTime struct {
		TimelineData []struct {
			Date        string `json:"date"`
			PartialData bool   `json:"partial_data"`
			Values      []struct {
				ExtractedValue int `json:"extracted_value"`
			} `json:"values"`
		} `json:"timeline_data"`
	} `json:"interest_over_time"`
}

// checkLiveTrend asserts the invariants Google Trends guarantees for a
// "today 12-m" interest-over-time series, and returns the decoded response
// for logging.
func checkLiveTrend(t *testing.T, raw []byte, keyword, geo string) liveTrendResponse {
	t.Helper()
	var resp liveTrendResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("decode raw SerpAPI response: %v", err)
	}
	// SerpAPI reports "no results" as HTTP 200 with an error field, which
	// serpapi.HTTPClient does not check.
	if resp.Error != "" {
		t.Fatalf("SerpAPI returned error: %s", resp.Error)
	}
	if resp.SearchMetadata.Status != "Success" {
		t.Errorf("search_metadata.status = %q, want Success", resp.SearchMetadata.Status)
	}
	if resp.SearchParameters.Q != keyword {
		t.Errorf("search_parameters.q = %q, want %q", resp.SearchParameters.Q, keyword)
	}
	// Proves the geo filter actually reached Google — a job created with
	// geo "" silently yields worldwide data.
	if resp.SearchParameters.Geo != geo {
		t.Errorf("search_parameters.geo = %q, want %q", resp.SearchParameters.Geo, geo)
	}
	if resp.SearchParameters.Date != "today 12-m" {
		t.Errorf("search_parameters.date = %q, want %q", resp.SearchParameters.Date, "today 12-m")
	}

	tl := resp.InterestOverTime.TimelineData
	// 12 months at weekly granularity is 52 or 53 buckets.
	if len(tl) < 50 || len(tl) > 54 {
		t.Fatalf("got %d timeline points, want ~52 weekly points", len(tl))
	}
	seen := make(map[string]bool, len(tl))
	maxV := 0
	for i, p := range tl {
		if p.Date == "" {
			t.Errorf("point %d has empty date", i)
		}
		if seen[p.Date] {
			t.Errorf("duplicate date %q", p.Date)
		}
		seen[p.Date] = true
		if len(p.Values) != 1 {
			t.Errorf("point %d (%s) has %d values, want 1 for a single-keyword query", i, p.Date, len(p.Values))
			continue
		}
		v := p.Values[0].ExtractedValue
		if v < 0 || v > 100 {
			t.Errorf("point %d (%s) value %d outside 0..100", i, p.Date, v)
		}
		maxV = max(maxV, v)
		if p.PartialData && i != len(tl)-1 {
			t.Errorf("point %d (%s) is partial_data but is not the last point", i, p.Date)
		}
	}
	// Google Trends normalises each series so its peak is exactly 100.
	if maxV != 100 {
		t.Errorf("series peak = %d, want 100 (Google Trends normalisation)", maxV)
	}
	return resp
}

func trendFileName(keyword, geo string) string {
	if geo == "" {
		geo = "WORLD"
	}
	return "trend_" + keyword + "_" + geo
}

func logSeries(t *testing.T, resp liveTrendResponse) {
	t.Helper()
	for _, p := range resp.InterestOverTime.TimelineData {
		mark := ""
		if p.PartialData {
			mark = "  (partial)"
		}
		t.Logf("  %-30s %3d%s", p.Date, p.Values[0].ExtractedValue, mark)
	}
}

// TestLive_FetchTrend calls the real SerpAPI exactly the way HandleMessage
// does (same client, empty dateRange) and checks the data that comes back.
func TestLive_FetchTrend(t *testing.T) {
	key := livetest.Secret(t, "SERPAPI_API_KEY")
	keyword := livetest.EnvOr("TREND_LIVE_KEYWORD", "sneakers")
	geo := livetest.EnvOr("TREND_LIVE_GEO", "US")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client := serpapi.NewHTTPClient(key, "https://serpapi.com", &http.Client{Timeout: 60 * time.Second})
	result, err := client.FetchTrend(ctx, keyword, geo, "")
	if err != nil {
		t.Fatalf("FetchTrend(%q, %q) error = %v", keyword, geo, err)
	}

	resp := checkLiveTrend(t, result.Raw, keyword, geo)

	// The parsed points HandleMessage relies on must mirror the raw series.
	tl := resp.InterestOverTime.TimelineData
	if len(result.Points) != len(tl) {
		t.Fatalf("parsed %d points, raw has %d", len(result.Points), len(tl))
	}
	for i, p := range result.Points {
		if p.Date != tl[i].Date || p.Value != tl[i].Values[0].ExtractedValue {
			t.Errorf("point %d parsed as {%s %d}, raw is {%s %d}",
				i, p.Date, p.Value, tl[i].Date, tl[i].Values[0].ExtractedValue)
		}
	}

	livetest.Save(t, trendFileName(keyword, geo), "json", result.Raw)
	t.Logf("keyword=%q geo=%q points=%d", keyword, geo, len(result.Points))
	logSeries(t, resp)
}

// TestLive_HandleMessage runs the real worker end to end: real SerpAPI,
// real Postgres (trend_raw + jobs) and a real RabbitMQ publish. It needs
// the docker compose stack up and cleans up the rows it creates.
func TestLive_HandleMessage(t *testing.T) {
	key := livetest.Secret(t, "SERPAPI_API_KEY")
	dsn, amqpURL := livetest.Stack(t)
	keyword := livetest.EnvOr("TREND_LIVE_KEYWORD", "sneakers")
	geo := livetest.EnvOr("TREND_LIVE_GEO", "US")
	ctx := context.Background()
	conn := livetest.DB(t, dsn)

	params, _ := json.Marshal(map[string]string{"keyword": keyword, "geo": geo})
	var jobID string
	if err := conn.QueryRowContext(ctx,
		`INSERT INTO jobs (type, status, params) VALUES ('trend', 'pending', $1) RETURNING id`, params,
	).Scan(&jobID); err != nil {
		t.Fatalf("insert job: %v", err)
	}
	t.Cleanup(func() {
		conn.ExecContext(ctx, `DELETE FROM trend_raw WHERE job_id = $1`, jobID)
		conn.ExecContext(ctx, `DELETE FROM jobs WHERE id = $1`, jobID)
	})

	publisher, err := rabbitmq.NewPublisher(rabbitmq.Config{URL: amqpURL, Exchange: livetest.Exchange})
	if err != nil {
		t.Fatalf("connect rabbitmq publisher: %v", err)
	}
	t.Cleanup(func() { publisher.Close() })
	awaitEvent := livetest.CaptureEvent(t, amqpURL, "crawl.completed.trend")

	c := &Consumer{
		DB:        conn,
		SerpApi:   serpapi.NewHTTPClient(key, "https://serpapi.com", &http.Client{Timeout: 60 * time.Second}),
		Publisher: publisher,
	}
	msg, _ := json.Marshal(JobMessage{JobID: jobID, Keyword: keyword, Geo: geo})
	if err := c.HandleMessage(msg); err != nil {
		t.Fatalf("HandleMessage() error = %v", err)
	}

	var status string
	if err := conn.QueryRowContext(ctx, `SELECT status FROM jobs WHERE id = $1`, jobID).Scan(&status); err != nil {
		t.Fatalf("read job status: %v", err)
	}
	if status != "crawled" {
		t.Errorf("jobs.status = %q, want crawled", status)
	}

	var storedKeyword string
	var raw []byte
	if err := conn.QueryRowContext(ctx,
		`SELECT keyword, trend_data FROM trend_raw WHERE job_id = $1`, jobID,
	).Scan(&storedKeyword, &raw); err != nil {
		t.Fatalf("read trend_raw: %v", err)
	}
	if storedKeyword != keyword {
		t.Errorf("trend_raw.keyword = %q, want %q", storedKeyword, keyword)
	}
	resp := checkLiveTrend(t, raw, keyword, geo)

	var event CompletedMessage
	if err := json.Unmarshal(awaitEvent(5*time.Second), &event); err != nil {
		t.Fatalf("decode completion event: %v", err)
	}
	if event.JobID != jobID {
		t.Errorf("completion event job_id = %q, want %q", event.JobID, jobID)
	}

	livetest.Save(t, trendFileName(keyword, geo), "json", raw)
	t.Logf("job %s crawled: %d points stored in trend_raw", jobID, len(resp.InterestOverTime.TimelineData))
}
