package service

import (
	"strings"
	"testing"
	"time"

	"github.com/hiepnv/crawl-ecomerce-golang/pkg/serpapi"
)

func TestTwentyFourMonthRange(t *testing.T) {
	r := twentyFourMonthRange()
	parts := strings.Split(r, " ")
	if len(parts) != 2 {
		t.Fatalf("twentyFourMonthRange() = %q, want two space-separated dates", r)
	}
	start, err := time.Parse("2006-01-02", parts[0])
	if err != nil {
		t.Fatalf("start date %q not parseable: %v", parts[0], err)
	}
	end, err := time.Parse("2006-01-02", parts[1])
	if err != nil {
		t.Fatalf("end date %q not parseable: %v", parts[1], err)
	}
	gotMonths := int(end.Sub(start).Hours() / 24 / 30)
	if gotMonths < 22 || gotMonths > 26 {
		t.Errorf("range spans ~%d months, want ~24", gotMonths)
	}
}

func TestApplyTrendData_Evergreen(t *testing.T) {
	points := []serpapi.TrendPoint{
		{Date: "Jan 2026", Value: 50}, {Date: "Feb 2026", Value: 55},
		{Date: "Mar 2026", Value: 60}, {Date: "Apr 2026", Value: 45},
	}
	var sess Session
	applyTrendData(&sess, points)

	if sess.DemandType == nil || *sess.DemandType != "evergreen" {
		t.Fatalf("DemandType = %v, want evergreen", sess.DemandType)
	}
	if sess.VolumeSource == nil || *sess.VolumeSource != "google_trends_index" {
		t.Fatalf("VolumeSource = %v, want google_trends_index", sess.VolumeSource)
	}
	if sess.Step1Score == nil || *sess.Step1Score < 8 {
		t.Fatalf("Step1Score = %v, want a high score for stable high-interest demand", sess.Step1Score)
	}
}

func TestApplyTrendData_Spike(t *testing.T) {
	points := []serpapi.TrendPoint{
		{Date: "Jan 2026", Value: 2}, {Date: "Feb 2026", Value: 3},
		{Date: "Mar 2026", Value: 100}, {Date: "Apr 2026", Value: 5},
	}
	var sess Session
	applyTrendData(&sess, points)

	if sess.DemandType == nil || *sess.DemandType != "spike" {
		t.Fatalf("DemandType = %v, want spike", sess.DemandType)
	}
	if sess.PeakMonths == nil || sess.PeakMonths[0] != "Mar" {
		t.Fatalf("PeakMonths = %v, want [Mar]", sess.PeakMonths)
	}
	if sess.LowMonths == nil || sess.LowMonths[0] != "Jan" {
		t.Fatalf("LowMonths = %v, want [Jan]", sess.LowMonths)
	}
}

func TestApplyTrendData_AllZero(t *testing.T) {
	points := []serpapi.TrendPoint{{Date: "Jan 2026", Value: 0}, {Date: "Feb 2026", Value: 0}}
	var sess Session
	applyTrendData(&sess, points)

	if sess.FluctuationRatio == nil || *sess.FluctuationRatio != 0 {
		t.Fatalf("FluctuationRatio = %v, want 0 (guarded against div-by-zero)", sess.FluctuationRatio)
	}
}

// The year-over-year change (which CreateAndRun computes from a 24-month
// fetch and stores in the same FluctuationRatio field, replacing the
// within-year ratio applyTrendData itself would have put there) follows
// this formula; this test locks in the arithmetic independent of the AI/DB
// wiring in CreateAndRun.
func TestAvgRange_YoyFormula(t *testing.T) {
	priorYear := []serpapi.TrendPoint{{Value: 40}, {Value: 60}}  // avg 50
	recentYear := []serpapi.TrendPoint{{Value: 70}, {Value: 90}} // avg 80

	avgPrior := avgRange(priorYear)
	avgRecent := avgRange(recentYear)
	if avgPrior != 50 || avgRecent != 80 {
		t.Fatalf("avgRange = %v/%v, want 50/80", avgPrior, avgRecent)
	}
	yoy := (avgRecent - avgPrior) / avgPrior
	if yoy != 0.6 {
		t.Fatalf("yoy = %v, want 0.6 (i.e. +60%% so với năm trước)", yoy)
	}
}
