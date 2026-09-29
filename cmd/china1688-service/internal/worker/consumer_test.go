package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/hiepnv/crawl-ecomerce-golang/pkg/apify"
)

type fakeApify struct {
	called   bool
	keywords []string
	err      error
}

func (f *fakeApify) FetchProducts(ctx context.Context, params apify.China1688Params) (apify.China1688ProductsResult, error) {
	f.called = true
	f.keywords = params.Keywords
	if f.err != nil {
		return apify.China1688ProductsResult{}, f.err
	}
	return apify.China1688ProductsResult{}, nil
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
	msg, _ := json.Marshal(JobMessage{JobID: "11111111-1111-1111-1111-111111111111", Keywords: []string{"wireless earbuds"}})
	err := c.HandleMessage(msg)
	if err == nil {
		t.Fatal("expected error propagated from Apify, got nil")
	}
	if !fake.called || len(fake.keywords) != 1 || fake.keywords[0] != "wireless earbuds" {
		t.Errorf("expected Apify.FetchProducts called with keywords=[wireless earbuds], got called=%v keywords=%v", fake.called, fake.keywords)
	}
}
