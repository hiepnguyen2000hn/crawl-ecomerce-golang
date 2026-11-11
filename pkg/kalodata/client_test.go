package kalodata

import (
	"encoding/json"
	"testing"
)

func TestMapProducts(t *testing.T) {
	raw := []json.RawMessage{
		json.RawMessage(`{"id":"p1","product_title":"Serum","revenue":"฿2.57m","gmv_A":7683222.5,"sale":42,"product_rating":4.8,"unit_price":"฿18.77k","commission_rate":"1%"}`),
	}
	got := mapProducts(raw, "https://img.kalocdn.com")
	if len(got) != 1 {
		t.Fatalf("want 1, got %d", len(got))
	}
	p := got[0]
	if p.ProductID != "p1" || p.Title != "Serum" || p.Revenue != "฿2.57m" || p.GMV != 7683222.5 {
		t.Errorf("product fields wrong: %+v", p)
	}
	if p.Sales != 42 || p.Rating != 4.8 || p.UnitPrice != "฿18.77k" || p.CommissionRate != "1%" {
		t.Errorf("product numeric/str fields wrong: %+v", p)
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
		json.RawMessage(`{"id":"v1","handle":"creator1","description":"Unboxing","views":"103.14k","revenue":"฿1.14m","sale":12,"duration":"44s"}`),
	}
	got := mapVideos(raw, "https://img.kalocdn.com")
	if len(got) != 1 {
		t.Fatalf("want 1, got %d", len(got))
	}
	v := got[0]
	if v.VideoID != "v1" || v.Handle != "creator1" || v.Title != "Unboxing" || v.Views != "103.14k" || v.Duration != "44s" {
		t.Errorf("video fields wrong: %+v", v)
	}
	if v.Revenue != "฿1.14m" || v.Sales != 12 {
		t.Errorf("video revenue/sale wrong: %+v", v)
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

func TestAccountCookieHeader(t *testing.T) {
	a := Account{SessionCookie: "abc", DeviceID: "dev123"}
	if got := a.cookieHeader(); got != "SESSION=abc; deviceId=dev123" {
		t.Fatalf("cookie header wrong: %q", got)
	}
}
