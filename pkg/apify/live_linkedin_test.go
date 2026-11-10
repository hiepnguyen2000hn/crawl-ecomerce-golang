package apify

import (
	"context"
	"os"
	"testing"
	"time"
)

// Live end-to-end test against the real Apify API and the real
// harvestapi/linkedin-post-search actor, through the same client the
// linkedin-service worker uses. It confirms the actor runs AND that the
// assumed output field paths (author.name, postedAt.date, engagement.*) are
// correct — any field logged empty means the path needs fixing in linkedin.go.
//
// It costs real Apify credit, so it is opt-in:
//
//	docker compose up -d postgres && docker compose run --rm migrate
//	APIFY_SLOTS_TEST_DSN='postgres://crawl:crawl@127.0.0.1:5433/crawl?sslmode=disable' \
//	APIFY_LIVE_TOKEN="$APIFY_API_TOKEN" \
//	go test ./pkg/apify/ -run TestLive_LinkedIn -v -timeout 10m
func TestLive_LinkedIn(t *testing.T) {
	// Accept either APIFY_LIVE_TOKEN (explicit opt-in) or APIFY_API_TOKEN
	// (the same name used in .env / docker-compose), so the token from the
	// environment just works.
	raw := os.Getenv("APIFY_LIVE_TOKEN")
	if raw == "" {
		raw = os.Getenv("APIFY_API_TOKEN")
	}
	if raw == "" {
		t.Skip("neither APIFY_LIVE_TOKEN nor APIFY_API_TOKEN set; skipping the live Apify test (it spends real credit)")
	}
	tokens := ParseTokens(raw)
	if len(tokens) == 0 {
		t.Fatal("APIFY_LIVE_TOKEN parsed to zero usable tokens")
	}

	db := testDB(t)
	slots := NewSlotStore(db, tokens, "live-linkedin-test", 10*time.Minute)
	if err := slots.SyncTokens(context.Background(), 5); err != nil {
		t.Fatalf("SyncTokens: %v", err)
	}

	keyword := os.Getenv("APIFY_LIVE_KEYWORD")
	if keyword == "" {
		keyword = "golang developer"
	}

	client := NewLinkedInHTTPClient("", NewRunner("https://api.apify.com/v2", slots, 10*time.Minute))
	res, err := client.SearchPosts(context.Background(), "live-linkedin", LinkedInSearchParams{
		Keyword:  keyword,
		MaxPosts: 5,
		SortBy:   "date",
	})
	if err != nil {
		t.Fatalf("SearchPosts: %v", err)
	}
	t.Logf("keyword %q -> %d post(s)", keyword, len(res.Posts))
	for i, p := range res.Posts {
		t.Logf("  [%d] id=%q author=%q postedAt=%q likes=%d comments=%d shares=%d url=%q rawBytes=%d",
			i+1, p.PostID, p.AuthorName, p.PostedAt, p.Likes, p.Comments, p.Shares, p.URL, len(p.Raw))
	}
	if len(res.Posts) == 0 {
		t.Fatal("no posts returned — check the keyword or actor availability")
	}
}
