package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/hiepnv/crawl-ecomerce-golang/pkg/apify"
)

type fakeApify struct {
	called bool
	query  string
	err    error
}

func (f *fakeApify) FetchAds(ctx context.Context, params apify.AdParams) (apify.AdsResult, error) {
	f.called = true
	f.query = params.Query
	if f.err != nil {
		return apify.AdsResult{}, f.err
	}
	return apify.AdsResult{}, nil
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
	msg, _ := json.Marshal(JobMessage{JobID: "11111111-1111-1111-1111-111111111111", Query: "nike"})
	err := c.HandleMessage(msg)
	if err == nil {
		t.Fatal("expected error propagated from Apify, got nil")
	}
	if !fake.called || fake.query != "nike" {
		t.Errorf("expected Apify.FetchAds called with query=nike, got called=%v query=%q", fake.called, fake.query)
	}
}
