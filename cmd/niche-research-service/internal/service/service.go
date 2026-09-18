// Package service implements Step 1 (Market Demand & Customer Insights) of
// the niche research flow described in LevelUp_SRS_Research.md.
//
// Google Ads API (absolute search volume) and Meta Marketing API (audience
// size) are not wired up yet — there is no credential for either in this
// project. Reddit VOC scraping (S1.4) is likewise not implemented. Only two
// of the four SRS sub-scores are computed from real data today:
//   - Localized keyword + narrative insight: OpenRouter (already used
//     elsewhere in this repo).
//   - Seasonality (S1.3) and an approximate demand-level score (S1.2 proxy):
//     computed from the real 12-month Google Trends interest-over-time
//     series already fetched via pkg/serpapi for trend-service. Note this is
//     a *relative interest index* (0-100), not an absolute monthly search
//     count — it stands in for real Keyword Planner data until Google Ads
//     API access is configured.
//
// Audience size (S1.1) and Reddit VOC (S1.4) are left null; step1_score is
// the average of just the two available sub-scores.
package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/hiepnv/crawl-ecomerce-golang/pkg/aiproviders"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/serpapi"
)

var countryNames = map[string]string{
	"NL": "Hà Lan", "DE": "Đức", "BE": "Bỉ", "FR": "Pháp", "ES": "Tây Ban Nha",
	"IT": "Ý", "PT": "Bồ Đào Nha", "PL": "Ba Lan", "CZ": "Séc", "SE": "Thuỵ Điển",
	"US": "Mỹ", "CA": "Canada", "GB": "Anh", "VN": "Việt Nam", "SG": "Singapore",
}

type Country struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// Session mirrors the shape the web/ frontend (web/app.jsx, web/niche.jsx)
// already expects from its mock API, so the frontend needs no changes
// beyond pointing its api() wrapper at this service.
type Session struct {
	ID          string    `json:"id"`
	RawKeyword  string    `json:"raw_keyword"`
	Countries   []Country `json:"countries"`
	CountryCode string    `json:"country_code"`
	CountryName string    `json:"country_name"`

	Status   string `json:"status"`
	Progress int    `json:"progress"`
	Stage    string `json:"stage"`

	Step1Score         *float64 `json:"step1_score"`
	AvgMonthlySearches *int     `json:"avg_monthly_searches"`
	DemandType         *string  `json:"demand_type"`
	DemandLabel        *string  `json:"demand_label"`
	KwSource           *string  `json:"kw_source"`
	VolumeSource       *string  `json:"volume_source"`
	AiSource           *string  `json:"ai_source"`

	PeakMonths       []string `json:"peak_months"`
	LowMonths        []string `json:"low_months"`
	TrendDirection   *string  `json:"trend_direction"`
	Volatility       *string  `json:"volatility"`
	VolatilityLabel  *string  `json:"volatility_label"`
	FluctuationRatio *float64 `json:"fluctuation_ratio"`
	SeasonalityNote  *string  `json:"seasonality_note"`

	AiSummary *string  `json:"ai_summary"`
	AiRisks   []string `json:"ai_risks"`
	AiActions []string `json:"ai_actions"`

	LastRunAt *time.Time `json:"last_run_at"`
}

type Service struct {
	DB     *sql.DB
	AI     aiproviders.Provider
	Trends serpapi.Client
}

const insightSchemaJSON = `{
  "type": "object",
  "properties": {
    "localized_keyword": {"type": "string"},
    "summary": {"type": "string"},
    "risks": {"type": "array", "items": {"type": "string"}},
    "actions": {"type": "array", "items": {"type": "string"}}
  },
  "required": ["localized_keyword", "summary", "risks", "actions"],
  "additionalProperties": false
}`

type insightResult struct {
	LocalizedKeyword string   `json:"localized_keyword"`
	Summary          string   `json:"summary"`
	Risks            []string `json:"risks"`
	Actions          []string `json:"actions"`
}

// CreateAndRun creates a niche research session and immediately runs Step 1
// against it (synchronous — a single AI call plus a single SerpAPI call are
// fast enough not to need a queue/worker for now).
func (s *Service) CreateAndRun(ctx context.Context, rawKeyword string, countryCodes []string) (Session, error) {
	rawKeyword = strings.TrimSpace(rawKeyword)
	if rawKeyword == "" {
		return Session{}, fmt.Errorf("service: raw_keyword is required")
	}
	if len(countryCodes) == 0 {
		return Session{}, fmt.Errorf("service: at least one country_code is required")
	}

	countries := make([]Country, len(countryCodes))
	for i, code := range countryCodes {
		code = strings.ToUpper(strings.TrimSpace(code))
		name := countryNames[code]
		if name == "" {
			name = code
		}
		countries[i] = Country{Code: code, Name: name}
	}
	primary := countries[0]

	sess := Session{
		RawKeyword:  rawKeyword,
		Countries:   countries,
		CountryCode: primary.Code,
		CountryName: primary.Name,
		Status:      "done",
		Progress:    100,
		Stage:       "Hoàn tất",
		PeakMonths:  []string{},
		LowMonths:   []string{},
		AiRisks:     []string{},
		AiActions:   []string{},
	}

	prompt := fmt.Sprintf(
		"You are an e-commerce market researcher. A team wants to sell \"%s\" in %s (country code %s).\n"+
			"Return a native, e-commerce-appropriate search keyword for that country's language (localized_keyword), "+
			"a 2-3 sentence market summary in Vietnamese aimed at a Vietnamese dropshipping team (summary), "+
			"2-3 short risk bullets in Vietnamese (risks), and 2-3 short next-action bullets in Vietnamese (actions).",
		rawKeyword, primary.Name, primary.Code,
	)
	trendKeyword := rawKeyword
	raw, err := s.AI.CompleteJSON(ctx, prompt, "niche_insight", []byte(insightSchemaJSON))
	if err == nil {
		var insight insightResult
		if jsonErr := json.Unmarshal(raw, &insight); jsonErr == nil {
			kwSource := "openrouter"
			aiSource := "openrouter"
			sess.KwSource = &kwSource
			sess.AiSource = &aiSource
			if insight.Summary != "" {
				sess.AiSummary = &insight.Summary
			}
			if len(insight.Risks) > 0 {
				sess.AiRisks = insight.Risks
			}
			if len(insight.Actions) > 0 {
				sess.AiActions = insight.Actions
			}
			if insight.LocalizedKeyword != "" {
				// Google Trends has little to no data for a raw Vietnamese
				// keyword in a non-Vietnamese market — use the AI-localized
				// term instead.
				trendKeyword = insight.LocalizedKeyword
			}
		} else {
			fmt.Println("niche-research-service: unmarshal AI insight failed:", jsonErr, "raw:", string(raw))
		}
	} else {
		fmt.Println("niche-research-service: AI insight call failed:", err)
	}
	if sess.AiSummary == nil {
		fallback := "Không sinh được insight AI (lỗi gọi OpenRouter) — dùng dữ liệu Google Trends thô bên dưới."
		sess.AiSummary = &fallback
		note := "demo"
		sess.KwSource = &note
		sess.AiSource = &note
		sess.AiRisks = append(sess.AiRisks, "AI insight tạm thời không khả dụng: "+errString(err))
	}

	// Seasonality + approximate demand-level score from real Google Trends
	// data (see package doc comment for why this is a proxy, not S1.2's
	// real absolute search volume). Fetching 24 months lets us compute a
	// year-over-year % change (like the "+60% so với năm trước" badge on
	// trends.google.com) in addition to the within-year seasonality score;
	// only the most recent 12-month half feeds the score, unchanged from
	// before.
	result, trendErr := s.Trends.FetchTrend(ctx, trendKeyword, primary.Code, twentyFourMonthRange())
	if trendErr == nil && len(result.Points) >= 2 {
		half := len(result.Points) / 2
		priorYear, recentYear := result.Points[:half], result.Points[half:]
		applyTrendData(&sess, recentYear)
		if avgPrior := avgRange(priorYear); avgPrior > 0 {
			yoy := (avgRange(recentYear) - avgPrior) / avgPrior
			sess.FluctuationRatio = &yoy
		}
	} else {
		volSrc := "demo"
		sess.VolumeSource = &volSrc
		note := "Chưa lấy được dữ liệu Google Trends"
		if trendErr != nil {
			note += ": " + trendErr.Error()
		}
		sess.SeasonalityNote = &note
	}

	now := time.Now().UTC()
	sess.LastRunAt = &now

	countriesJSON, err := json.Marshal(countries)
	if err != nil {
		return Session{}, fmt.Errorf("service: marshal countries: %w", err)
	}
	peakJSON, _ := json.Marshal(sess.PeakMonths)
	lowJSON, _ := json.Marshal(sess.LowMonths)
	risksJSON, _ := json.Marshal(sess.AiRisks)
	actionsJSON, _ := json.Marshal(sess.AiActions)

	err = s.DB.QueryRowContext(ctx, `
		INSERT INTO niche_sessions (
			raw_keyword, countries, status, step1_score, avg_monthly_searches,
			demand_type, demand_label, kw_source, volume_source, ai_source,
			peak_months, low_months, trend_direction, volatility, volatility_label,
			fluctuation_ratio, seasonality_note, ai_summary, ai_risks, ai_actions, last_run_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)
		RETURNING id
	`,
		sess.RawKeyword, countriesJSON, sess.Status, sess.Step1Score, sess.AvgMonthlySearches,
		sess.DemandType, sess.DemandLabel, sess.KwSource, sess.VolumeSource, sess.AiSource,
		peakJSON, lowJSON, sess.TrendDirection, sess.Volatility, sess.VolatilityLabel,
		sess.FluctuationRatio, sess.SeasonalityNote, sess.AiSummary, risksJSON, actionsJSON, sess.LastRunAt,
	).Scan(&sess.ID)
	if err != nil {
		return Session{}, fmt.Errorf("service: insert niche_session: %w", err)
	}

	return sess, nil
}

// applyTrendData fills in the seasonality fields (real) and an approximate
// demand-level score (proxy) from a Google Trends interest-over-time
// series. Points are assumed ordered chronologically, as SerpAPI returns
// them.
func applyTrendData(sess *Session, points []serpapi.TrendPoint) {
	volSrc := "google_trends_index"
	sess.VolumeSource = &volSrc

	minV, maxV := points[0].Value, points[0].Value
	minIdx, maxIdx := 0, 0
	sum := 0
	for i, p := range points {
		sum += p.Value
		if p.Value < minV {
			minV, minIdx = p.Value, i
		}
		if p.Value > maxV {
			maxV, maxIdx = p.Value, i
		}
	}
	avg := sum / len(points)
	sess.AvgMonthlySearches = &avg

	ratio := 0.0
	if maxV > 0 {
		ratio = float64(maxV-minV) / float64(maxV)
	}
	sess.FluctuationRatio = &ratio

	var demandType, demandLabel, volatility, volatilityLabel string
	var s13 float64
	switch {
	case ratio <= 0.30:
		demandType, demandLabel = "evergreen", "Nhu cầu ổn định quanh năm — evergreen"
		volatility, volatilityLabel, s13 = "low", "Biến động thấp", 9.5
	case ratio <= 0.70:
		demandType, demandLabel = "seasonal", "Nhu cầu theo mùa — seasonal"
		volatility, volatilityLabel, s13 = "medium", "Biến động trung bình", 6
	default:
		demandType, demandLabel = "spike", "Trend ngắn hạn, rủi ro tồn kho — spike"
		volatility, volatilityLabel, s13 = "high", "Biến động cao", 2.5
	}
	sess.DemandType, sess.DemandLabel = &demandType, &demandLabel
	sess.Volatility, sess.VolatilityLabel = &volatility, &volatilityLabel

	sess.PeakMonths = []string{monthLabel(points[maxIdx].Date)}
	sess.LowMonths = []string{monthLabel(points[minIdx].Date)}

	firstHalf, secondHalf := avgRange(points[:len(points)/2]), avgRange(points[len(points)/2:])
	direction := "stable"
	if secondHalf > firstHalf*1.1 {
		direction = "up"
	} else if secondHalf < firstHalf*0.9 {
		direction = "down"
	}
	sess.TrendDirection = &direction

	// S1.2 proxy: demand level inferred from the average relative interest
	// index (0-100) — not the SRS's real absolute search-volume thresholds.
	var s12 float64
	switch {
	case avg >= 50:
		s12 = 9
	case avg >= 25:
		s12 = 7
	case avg >= 10:
		s12 = 5
	default:
		s12 = 2
	}
	total := math.Round((s12+s13)/2*10) / 10
	sess.Step1Score = &total

	note := fmt.Sprintf(
		"Điểm S1 hiện chỉ dựa trên 2/4 tiêu chí thật (Search Interest proxy %.0f/100 trung bình 12 tháng theo Google Trends + Biến động mùa vụ %.0f%%). "+
			"Audience Size (Meta) và Reddit VOC chưa có credential nên chưa tính vào điểm.",
		float64(avg), ratio*100,
	)
	sess.SeasonalityNote = &note
}

// twentyFourMonthRange builds a SerpAPI custom date range ("YYYY-MM-DD
// YYYY-MM-DD") covering the trailing 24 months. Google Trends' relative
// presets only go up to "today 12-m" before jumping to "today 5-y" — there
// is no built-in "24 months ago to today" preset, so a custom range is
// required to compute a year-over-year comparison.
func twentyFourMonthRange() string {
	end := time.Now().UTC()
	start := end.AddDate(-2, 0, 0)
	return start.Format("2006-01-02") + " " + end.Format("2006-01-02")
}

func avgRange(points []serpapi.TrendPoint) float64 {
	if len(points) == 0 {
		return 0
	}
	sum := 0
	for _, p := range points {
		sum += p.Value
	}
	return float64(sum) / float64(len(points))
}

// monthLabel best-effort extracts a month token (e.g. "Sep") from a
// SerpAPI trend point date string, whose exact format varies (weekly vs
// monthly buckets). Falls back to the raw string if no match is found.
func monthLabel(date string) string {
	months := []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
	for _, m := range months {
		if strings.Contains(date, m) {
			return m
		}
	}
	return date
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// ListSessions returns all niche research sessions, most recently run
// first.
func (s *Service) ListSessions(ctx context.Context) ([]Session, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, raw_keyword, countries, status, step1_score, avg_monthly_searches,
			demand_type, demand_label, kw_source, volume_source, ai_source,
			peak_months, low_months, trend_direction, volatility, volatility_label,
			fluctuation_ratio, seasonality_note, ai_summary, ai_risks, ai_actions, last_run_at
		FROM niche_sessions
		ORDER BY last_run_at DESC NULLS LAST, created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("service: query niche_sessions: %w", err)
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var sess Session
		var countriesJSON, peakJSON, lowJSON, risksJSON, actionsJSON []byte
		if err := rows.Scan(
			&sess.ID, &sess.RawKeyword, &countriesJSON, &sess.Status, &sess.Step1Score, &sess.AvgMonthlySearches,
			&sess.DemandType, &sess.DemandLabel, &sess.KwSource, &sess.VolumeSource, &sess.AiSource,
			&peakJSON, &lowJSON, &sess.TrendDirection, &sess.Volatility, &sess.VolatilityLabel,
			&sess.FluctuationRatio, &sess.SeasonalityNote, &sess.AiSummary, &risksJSON, &actionsJSON, &sess.LastRunAt,
		); err != nil {
			return nil, fmt.Errorf("service: scan niche_session: %w", err)
		}
		json.Unmarshal(countriesJSON, &sess.Countries)
		json.Unmarshal(peakJSON, &sess.PeakMonths)
		json.Unmarshal(lowJSON, &sess.LowMonths)
		json.Unmarshal(risksJSON, &sess.AiRisks)
		json.Unmarshal(actionsJSON, &sess.AiActions)
		if len(sess.Countries) > 0 {
			sess.CountryCode = sess.Countries[0].Code
			sess.CountryName = sess.Countries[0].Name
		}
		sess.Progress = 100
		sess.Stage = "Hoàn tất"
		sessions = append(sessions, sess)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("service: iterate niche_sessions: %w", err)
	}
	if sessions == nil {
		sessions = []Session{}
	}
	return sessions, nil
}
