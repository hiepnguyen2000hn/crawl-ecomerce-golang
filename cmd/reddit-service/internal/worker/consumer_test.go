package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/hiepnv/crawl-ecomerce-golang/pkg/apify"
)

type fakeApify struct {
	err     error
	keyword string
	result  apify.RedditSearchResult
}

func (f *fakeApify) SearchTopPostsWithComments(ctx context.Context, keyword string, maxPosts, maxCommentsPerPost int) (apify.RedditSearchResult, error) {
	f.keyword = keyword
	if f.err != nil {
		return apify.RedditSearchResult{}, f.err
	}
	return f.result, nil
}

func TestHandleMessage_UnmarshalError(t *testing.T) {
	c := &Consumer{Apify: &fakeApify{}}
	err := c.HandleMessage([]byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestHandleMessage_SearchError(t *testing.T) {
	fake := &fakeApify{err: errors.New("boom")}
	c := &Consumer{Apify: fake}
	msg, _ := json.Marshal(JobMessage{JobID: "11111111-1111-1111-1111-111111111111", Keyword: "wireless earbuds"})
	err := c.HandleMessage(msg)
	if err == nil {
		t.Fatal("expected error propagated from Apify, got nil")
	}
	if fake.keyword != "wireless earbuds" {
		t.Errorf("expected Apify.SearchTopPostsWithComments called with keyword=wireless earbuds, got %q", fake.keyword)
	}
}
