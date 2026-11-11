package kalodata

import (
	"encoding/json"
	"testing"
)

func TestMapProducts(t *testing.T) {
	raw := []json.RawMessage{
		json.RawMessage(`{"id":"p1","title":"Serum","revenue":1250.5,"sales":42,"rating":4.8,"price":9.9,"commissionRate":20}`),
	}
	got := mapProducts(raw, "https://img.kalocdn.com")
	if len(got) != 1 {
		t.Fatalf("want 1, got %d", len(got))
	}
	p := got[0]
	if p.ProductID != "p1" || p.Title != "Serum" || p.Revenue != 1250.5 || p.Sales != 42 || p.Rating != 4.8 {
		t.Errorf("product fields wrong: %+v", p)
	}
	if p.ThumbnailURL != "https://img.kalocdn.com/tiktok.product/p1/cover.png" {
		t.Errorf("thumbnail wrong: %q", p.ThumbnailURL)
	}
	if string(p.Raw) != string(raw[0]) {
		t.Error("raw not preserved")
	}
}

func TestMapVideos(t *testing.T) {
	raw := []json.RawMessage{
		json.RawMessage(`{"id":"v1","handle":"creator1","title":"Unboxing","views":10000,"revenue":500,"sales":12,"duration":45}`),
	}
	got := mapVideos(raw, "https://img.kalocdn.com")
	if len(got) != 1 {
		t.Fatalf("want 1, got %d", len(got))
	}
	v := got[0]
	if v.VideoID != "v1" || v.Handle != "creator1" || v.Title != "Unboxing" || v.Views != 10000 || v.Duration != 45 {
		t.Errorf("video fields wrong: %+v", v)
	}
	if v.ThumbnailURL != "https://img.kalocdn.com/tiktok.video/v1/cover.png" {
		t.Errorf("thumbnail wrong: %q", v.ThumbnailURL)
	}
	if v.TikTokURL != "https://www.tiktok.com/@creator1/video/v1" {
		t.Errorf("tiktok url wrong: %q", v.TikTokURL)
	}
	if string(v.Raw) != string(raw[0]) {
		t.Error("raw not preserved")
	}
}
