package pipiads_test

import (
	"context"
	"os"
	"testing"

	"github.com/hiepnv/crawl-ecomerce-golang/pkg/pipiads"
)

func TestPipiadsLiveSearch(t *testing.T) {
	token := os.Getenv("PIPIADS_TEST_TOKEN")
	devID := os.Getenv("PIPIADS_TEST_DEVICE_ID")
	proxy := os.Getenv("PIPIADS_TEST_PROXY")
	if token == "" || devID == "" {
		t.Skip("PIPIADS_TEST_TOKEN or PIPIADS_TEST_DEVICE_ID not set; skipping live test")
	}

	client := pipiads.NewHTTPClient("https://www.pipiads.com")
	acc := &pipiads.Account{
		Email:       "test@example.com",
		AccessToken: token,
		DeviceID:    devID,
		Proxy:       proxy,
	}

	ctx := context.Background()

	// 1. Check Usage / Quota
	usage, err := client.GetMemberUsage(ctx, acc)
	if err != nil {
		t.Fatalf("GetMemberUsage failed: %v", err)
	}
	if usage.TotalCredits <= 0 {
		t.Errorf("expected TotalCredits > 0, got %d", usage.TotalCredits)
	}

	// 2. Search E-commerce Ads
	ads, err := client.SearchEcommerceAds(ctx, acc, pipiads.SearchRequest{
		Keyword:     "shoes",
		CurrentPage: 1,
		PageSize:    5,
	})
	if err != nil {
		t.Fatalf("SearchEcommerceAds failed: %v", err)
	}

	if len(ads) == 0 {
		t.Errorf("expected > 0 ads for keyword 'shoes'")
	}
}
