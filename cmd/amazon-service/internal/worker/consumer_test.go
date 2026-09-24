package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/hiepnv/crawl-ecomerce-golang/pkg/apify"
)

type fakeApify struct {
	called  bool
	keyword string
	err     error
}

func (f *fakeApify) FetchProducts(ctx context.Context, params apify.ProductParams) (apify.ProductsResult, error) {
	f.called = true
	f.keyword = params.Keyword
	if f.err != nil {
		return apify.ProductsResult{}, f.err
	}
	return apify.ProductsResult{}, nil
}

func TestHandleMessage_UnmarshalError(t *testing.T) {
	c := &Consumer{Apify: &fakeApify{}}
	err := c.HandleMessage([]byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestHandleMessage_ApifyError(t *testing.T) {
	fake := &fakeApify{err: errors.New("boom")}
	c := &Consumer{Apify: fake}
	msg, _ := json.Marshal(JobMessage{JobID: "11111111-1111-1111-1111-111111111111", Keyword: "wireless earbuds"})
	err := c.HandleMessage(msg)
	if err == nil {
		t.Fatal("expected error propagated from Apify, got nil")
	}
	if !fake.called || fake.keyword != "wireless earbuds" {
		t.Errorf("expected Apify.FetchProducts called with keyword=wireless earbuds, got called=%v keyword=%q", fake.called, fake.keyword)
	}
}
