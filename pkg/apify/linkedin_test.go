package apify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLinkedInHTTPClient_SearchPosts(t *testing.T) {
	var gotPath, gotToken, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotToken = r.URL.Query().Get("token")
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		gotBody = string(buf)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[{"id":"p1","linkedinUrl":"https://linkedin.com/p1","content":"hi",` +
			`"author":{"name":"Ann"},"engagement":{"likes":3}}]`))
	}))
	defer srv.Close()

	db := testDB(t) // skips without APIFY_SLOTS_TEST_DSN
	s := newStore(t, db, []string{"test-token"}, 5)
	c := NewLinkedInHTTPClient("harvestapi/linkedin-post-search", NewRunner(srv.URL, s, time.Minute))

	res, err := c.SearchPosts(context.Background(), "job-1", LinkedInSearchParams{Keyword: "golang", MaxPosts: 5})
	if err != nil {
		t.Fatalf("SearchPosts: %v", err)
	}
	if want := "/acts/harvestapi~linkedin-post-search/run-sync-get-dataset-items"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
	if gotToken != "test-token" {
		t.Errorf("token = %q", gotToken)
	}
	if !strings.Contains(gotBody, `"searchQueries":["golang"]`) {
		t.Errorf("body missing searchQueries: %q", gotBody)
	}
	if len(res.Posts) != 1 || res.Posts[0].PostID != "p1" || res.Posts[0].AuthorName != "Ann" || res.Posts[0].Likes != 3 {
		t.Errorf("parsed posts wrong: %+v", res.Posts)
	}
}

func TestLinkedInHTTPClient_SearchPosts_RequiresKeyword(t *testing.T) {
	c := NewLinkedInHTTPClient("", NewRunner("http://unused", nil, time.Minute))
	if _, err := c.SearchPosts(context.Background(), "job-1", LinkedInSearchParams{}); err == nil {
		t.Fatal("expected error when keyword is empty")
	}
}

func TestMapLinkedInItems(t *testing.T) {
	raw := []json.RawMessage{
		json.RawMessage(`{"id":"p1","linkedinUrl":"https://linkedin.com/p1","content":"hello",` +
			`"author":{"name":"Ann","linkedinUrl":"https://linkedin.com/in/ann"},` +
			`"postedAt":{"date":"2026-01-02 10:00:00"},` +
			`"engagement":{"likes":5,"comments":2,"shares":1}}`),
	}
	posts, err := mapLinkedInItems(raw, 10)
	if err != nil {
		t.Fatalf("mapLinkedInItems: %v", err)
	}
	if len(posts) != 1 {
		t.Fatalf("want 1 post, got %d", len(posts))
	}
	p := posts[0]
	if p.PostID != "p1" || p.URL != "https://linkedin.com/p1" || p.Content != "hello" {
		t.Errorf("post basic fields wrong: %+v", p)
	}
	if p.AuthorName != "Ann" || p.AuthorURL != "https://linkedin.com/in/ann" {
		t.Errorf("author fields wrong: %+v", p)
	}
	if p.PostedAt != "2026-01-02 10:00:00" {
		t.Errorf("postedAt wrong: %q", p.PostedAt)
	}
	if p.Likes != 5 || p.Comments != 2 || p.Shares != 1 {
		t.Errorf("engagement wrong: %+v", p)
	}
	if string(p.Raw) != string(raw[0]) {
		t.Errorf("raw not preserved")
	}
}

func TestMapLinkedInItems_CapsMaxPosts(t *testing.T) {
	raw := []json.RawMessage{
		json.RawMessage(`{"id":"a"}`),
		json.RawMessage(`{"id":"b"}`),
		json.RawMessage(`{"id":"c"}`),
	}
	posts, err := mapLinkedInItems(raw, 2)
	if err != nil {
		t.Fatalf("mapLinkedInItems: %v", err)
	}
	if len(posts) != 2 || posts[0].PostID != "a" || posts[1].PostID != "b" {
		t.Errorf("cap wrong: %+v", posts)
	}
}
