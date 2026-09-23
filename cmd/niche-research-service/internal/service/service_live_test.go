//go:build live

// Live tests for niche-research-service: they crawl 24 months of Google
// Trends through the real SerpAPI and, end to end, call the real OpenRouter
// model, so they are excluded from a plain `go test ./...` by the `live`
// build tag. Run them explicitly:
//
//	go test -tags live -v -run Live ./cmd/niche-research-service/internal/service/
//
// Besides the shared variables in internal/livetest:
//
//	NICHE_LIVE_KEYWORD   keyword to research (default "sneakers")
//	NICHE_LIVE_COUNTRY   country code (default "US")
//	OPENROUTER_MODEL     model for the insight call (default: etc/niche.yaml's)
package service

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/hiepnv/crawl-ecomerce-golang/internal/livetest"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/aiproviders"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/serpapi"
)

func liveTrends(t *testing.T) serpapi.Client {
	t.Helper()
	key := livetest.Secret(t, "SERPAPI_API_KEY")
	return serpapi.NewHTTPClient(key, "https://serpapi.com", &http.Client{Timeout: 60 * time.Second})
}

// wantDemandType is the SRS band for a within-year fluctuation ratio.
func wantDemandType(ratio float64) string {
	switch {
	case ratio <= 0.30:
		return "evergreen"
	case ratio <= 0.70:
		return "seasonal"
	default:
		return "spike"
	}
}

func logSession(t *testing.T, s Session) {
	t.Helper()
	deref := func(p *string) string {
		if p == nil {
			return "<nil>"
		}
		return *p
	}
	if s.Step1Score != nil {
		t.Logf("step1_score=%.1f", *s.Step1Score)
	}
	if s.AvgMonthlySearches != nil {
		t.Logf("avg_monthly_searches (trends index)=%d", *s.AvgMonthlySearches)
	}
	if s.FluctuationRatio != nil {
		t.Logf("fluctuation_ratio=%.3f", *s.FluctuationRatio)
	}
	t.Logf("demand_type=%s volatility=%s trend_direction=%s peak=%v low=%v",
		deref(s.DemandType), deref(s.Volatility), deref(s.TrendDirection), s.PeakMonths, s.LowMonths)
	t.Logf("volume_source=%s kw_source=%s ai_source=%s",
		deref(s.VolumeSource), deref(s.KwSource), deref(s.AiSource))
	t.Logf("seasonality_note=%s", deref(s.SeasonalityNote))
}

// TestLive_TrendsTwentyFourMonths makes the same SerpAPI call CreateAndRun
// makes (custom 24-month range, primary country as geo) and runs the
// service's scoring on the result, without touching the database.
func TestLive_TrendsTwentyFourMonths(t *testing.T) {
	trends := liveTrends(t)
	keyword := livetest.EnvOr("NICHE_LIVE_KEYWORD", "sneakers")
	geo := livetest.EnvOr("NICHE_LIVE_COUNTRY", "US")
	dateRange := twentyFourMonthRange()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	result, err := trends.FetchTrend(ctx, keyword, geo, dateRange)
	if err != nil {
		t.Fatalf("FetchTrend(%q, %q, %q) error = %v", keyword, geo, dateRange, err)
	}
	livetest.Save(t, "niche_trend24m_"+keyword+"_"+geo, "json", result.Raw)

	var meta struct {
		Error            string `json:"error"`
		SearchParameters struct {
			Geo  string `json:"geo"`
			Date string `json:"date"`
		} `json:"search_parameters"`
	}
	if err := json.Unmarshal(result.Raw, &meta); err != nil {
		t.Fatalf("decode raw SerpAPI response: %v", err)
	}
	if meta.Error != "" {
		t.Fatalf("SerpAPI returned error: %s", meta.Error)
	}
	if meta.SearchParameters.Geo != geo {
		t.Errorf("search_parameters.geo = %q, want %q", meta.SearchParameters.Geo, geo)
	}
	if meta.SearchParameters.Date != dateRange {
		t.Errorf("search_parameters.date = %q, want %q", meta.SearchParameters.Date, dateRange)
	}

	// Google Trends serves a 2-year range at weekly granularity: ~104 points.
	// CreateAndRun splits the series in half assuming exactly that.
	if n := len(result.Points); n < 100 || n > 108 {
		t.Fatalf("got %d points for a 24-month range, want ~104 weekly points", n)
	}
	maxV := 0
	for _, p := range result.Points {
		if p.Value < 0 || p.Value > 100 {
			t.Errorf("%s value %d outside 0..100", p.Date, p.Value)
		}
		maxV = max(maxV, p.Value)
	}
	if maxV != 100 {
		t.Errorf("series peak = %d, want 100 (Google Trends normalisation)", maxV)
	}

	half := len(result.Points) / 2
	recent := result.Points[half:]
	t.Logf("recent year: %s … %s (%d points)", recent[0].Date, recent[len(recent)-1].Date, len(recent))

	var sess Session
	applyTrendData(&sess, recent)
	logSession(t, sess)

	if sess.FluctuationRatio == nil || sess.DemandType == nil {
		t.Fatal("applyTrendData left fluctuation_ratio or demand_type unset")
	}
	if want := wantDemandType(*sess.FluctuationRatio); *sess.DemandType != want {
		t.Errorf("demand_type = %s, but fluctuation %.3f falls in the %s band",
			*sess.DemandType, *sess.FluctuationRatio, want)
	}
	if sess.Step1Score == nil || *sess.Step1Score < 0 || *sess.Step1Score > 10 {
		t.Errorf("step1_score = %v, want a value in 0..10", sess.Step1Score)
	}
}

// TestLive_CreateAndRun runs Step 1 end to end: real OpenRouter insight,
// real 24-month SerpAPI crawl and a real niche_sessions insert. It needs
// the docker compose Postgres and deletes the session it creates.
func TestLive_CreateAndRun(t *testing.T) {
	trends := liveTrends(t)
	aiKey := livetest.Secret(t, "OPENROUTER_API_KEY")
	dsn := livetest.PostgresDSN(t)
	keyword := livetest.EnvOr("NICHE_LIVE_KEYWORD", "sneakers")
	country := livetest.EnvOr("NICHE_LIVE_COUNTRY", "US")
	ctx := context.Background()

	s := &Service{
		DB: livetest.DB(t, dsn),
		AI: aiproviders.NewOpenRouter(aiKey,
			livetest.EnvOr("OPENROUTER_MODEL", "dots-studio/dots-3-note-preview:free"),
			"https://openrouter.ai/api/v1", http.DefaultClient,
			"nvidia/nemotron-3-super-120b-a12b:free"),
		Trends: trends,
	}

	sess, err := s.CreateAndRun(ctx, keyword, []string{country})
	if err != nil {
		t.Fatalf("CreateAndRun(%q, %q) error = %v", keyword, country, err)
	}
	t.Cleanup(func() { s.DB.ExecContext(ctx, `DELETE FROM niche_sessions WHERE id = $1`, sess.ID) })
	livetest.SaveJSON(t, "niche_session_"+keyword+"_"+country, sess)
	logSession(t, sess)
	if sess.AiSummary != nil {
		t.Logf("ai_summary=%s", *sess.AiSummary)
	}

	// Both external sources must actually have answered: CreateAndRun
	// swallows their errors and marks the session "demo" instead.
	if sess.VolumeSource == nil || *sess.VolumeSource != "google_trends_index" {
		t.Errorf("volume_source = %v, want google_trends_index (Google Trends call failed?)", sess.VolumeSource)
	}
	if sess.AiSource == nil || *sess.AiSource != "openrouter" {
		t.Errorf("ai_source = %v, want openrouter (AI insight call failed?)", sess.AiSource)
	}
	if len(sess.AiRisks) == 0 || len(sess.AiActions) == 0 {
		t.Errorf("got %d risks and %d actions, want both non-empty", len(sess.AiRisks), len(sess.AiActions))
	}

	// The stored fluctuation_ratio must be the one demand_type was derived
	// from, or readers applying the SRS bands get a different label.
	if sess.FluctuationRatio != nil && sess.DemandType != nil {
		if want := wantDemandType(*sess.FluctuationRatio); *sess.DemandType != want {
			t.Errorf("stored fluctuation_ratio %.3f is in the %s band, but demand_type = %s",
				*sess.FluctuationRatio, want, *sess.DemandType)
		}
	}

	var stored int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM niche_sessions WHERE id = $1`, sess.ID).Scan(&stored); err != nil {
		t.Fatalf("read niche_sessions: %v", err)
	}
	if stored != 1 {
		t.Errorf("niche_sessions rows for %s = %d, want 1", sess.ID, stored)
	}
}
