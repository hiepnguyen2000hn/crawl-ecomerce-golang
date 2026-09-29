package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/hiepnv/crawl-ecomerce-golang/pkg/redditapi"
)

type fakeReddit struct {
	err     error
	keyword string
	result  redditapi.RedditSearchResult
}

func (f *fakeReddit) SearchTopPostsWithComments(ctx context.Context, keyword string, maxPosts, maxCommentsPerPost int) (redditapi.RedditSearchResult, error) {
	f.keyword = keyword
	if f.err != nil {
		return redditapi.RedditSearchResult{}, f.err
	}
	return f.result, nil
}

func TestHandleMessage_UnmarshalError(t *testing.T) {
	c := &Consumer{Reddit: &fakeReddit{}}
	err := c.HandleMessage([]byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestHandleMessage_SearchError(t *testing.T) {
	fake := &fakeReddit{err: errors.New("boom")}
	c := &Consumer{Reddit: fake}
	msg, _ := json.Marshal(JobMessage{JobID: "11111111-1111-1111-1111-111111111111", Keyword: "wireless earbuds"})
	err := c.HandleMessage(msg)
	if err == nil {
		t.Fatal("expected error propagated from Reddit client, got nil")
	}
	if fake.keyword != "wireless earbuds" {
		t.Errorf("expected Reddit.SearchTopPostsWithComments called with keyword=wireless earbuds, got %q", fake.keyword)
	}
}
