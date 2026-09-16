package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/hiepnv/crawl-ecomerce-golang/pkg/serpapi"
)

type fakeSerpApi struct {
	called  bool
	keyword string
	err     error
}

func (f *fakeSerpApi) FetchTrend(ctx context.Context, keyword string) (serpapi.TrendResult, error) {
	f.called = true
	f.keyword = keyword
	if f.err != nil {
		return serpapi.TrendResult{}, f.err
	}
	return serpapi.TrendResult{Keyword: keyword, Raw: json.RawMessage(`{}`)}, nil
}

func TestHandleMessage_UnmarshalError(t *testing.T) {
	c := &Consumer{SerpApi: &fakeSerpApi{}}
	err := c.HandleMessage([]byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestHandleMessage_SerpApiError(t *testing.T) {
	fake := &fakeSerpApi{err: errors.New("boom")}
	c := &Consumer{SerpApi: fake}
	msg, _ := json.Marshal(JobMessage{JobID: "11111111-1111-1111-1111-111111111111", Keyword: "golang"})
	err := c.HandleMessage(msg)
	if err == nil {
		t.Fatal("expected error propagated from SerpApi, got nil")
	}
	if !fake.called || fake.keyword != "golang" {
		t.Errorf("expected SerpApi.FetchTrend called with keyword=golang, got called=%v keyword=%q", fake.called, fake.keyword)
	}
}
