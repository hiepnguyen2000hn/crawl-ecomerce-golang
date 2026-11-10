package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/hiepnv/crawl-ecomerce-golang/pkg/apify"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/rabbitmq"
)

type fakeLinkedIn struct {
	err    error
	jobID  string
	params apify.LinkedInSearchParams
	result apify.LinkedInSearchResult
}

func (f *fakeLinkedIn) SearchPosts(ctx context.Context, jobID string, p apify.LinkedInSearchParams) (apify.LinkedInSearchResult, error) {
	f.jobID = jobID
	f.params = p
	return f.result, f.err
}

func TestHandleMessage_UnmarshalError(t *testing.T) {
	c := &Consumer{Apify: &fakeLinkedIn{}}
	if err := c.HandleMessage([]byte("not json")); err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestHandleMessage_SearchErrorForwardsParamsAndDefaults(t *testing.T) {
	fake := &fakeLinkedIn{err: errors.New("boom")}
	c := &Consumer{Apify: fake}
	msg, _ := json.Marshal(JobMessage{JobID: "11111111-1111-1111-1111-111111111111", Keyword: "golang developer"})
	if err := c.HandleMessage(msg); err == nil {
		t.Fatal("expected error propagated from Apify, got nil")
	}
	if fake.params.Keyword != "golang developer" {
		t.Errorf("keyword = %q, want 'golang developer'", fake.params.Keyword)
	}
	if fake.params.MaxPosts != 25 || fake.params.Pages != 1 || fake.params.SortBy != "relevance" {
		t.Errorf("defaults wrong: %+v", fake.params)
	}
}

func TestHandleMessage_ExplicitParams(t *testing.T) {
	fake := &fakeLinkedIn{err: errors.New("boom")}
	c := &Consumer{Apify: fake}
	msg, _ := json.Marshal(JobMessage{
		JobID: "11111111-1111-1111-1111-111111111111", Keyword: "k",
		MaxPosts: 7, Pages: 3, SortBy: "date", PostedLimit: "week",
	})
	_ = c.HandleMessage(msg)
	if fake.params.MaxPosts != 7 || fake.params.Pages != 3 || fake.params.SortBy != "date" || fake.params.PostedLimit != "week" {
		t.Errorf("explicit params not forwarded: %+v", fake.params)
	}
}

func TestHandleMessage_BusyRequeues(t *testing.T) {
	fake := &fakeLinkedIn{err: &apify.ErrBusy{Err: errors.New("pool full")}}
	c := &Consumer{Apify: fake}
	msg, _ := json.Marshal(JobMessage{JobID: "11111111-1111-1111-1111-111111111111", Keyword: "k"})
	err := c.HandleMessage(msg)
	if err == nil || !errors.Is(err, rabbitmq.ErrRequeue) {
		t.Fatalf("expected ErrRequeue wrap, got %v", err)
	}
}
