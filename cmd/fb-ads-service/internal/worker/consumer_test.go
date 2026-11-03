package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/fb-ads-service/internal/media"
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

type fakeMedia struct {
	adIDs []string
	refs  [][]media.Ref
	err   error
}

func (f *fakeMedia) UpsertPending(ctx context.Context, adArchiveID string, refs []media.Ref) error {
	f.adIDs = append(f.adIDs, adArchiveID)
	f.refs = append(f.refs, refs)
	return f.err
}

func TestRecordMedia_UpsertsExtractedRefs(t *testing.T) {
	fm := &fakeMedia{}
	c := &Consumer{Media: fm}
	c.recordMedia(context.Background(), apify.Ad{
		AdArchiveID: "ad-1",
		Raw:         json.RawMessage(`{"snapshot":{"images":[{"resized_image_url":"https://x/a.jpg"}],"videos":[{"video_sd_url":"https://x/v.mp4","video_preview_image_url":"https://x/c.jpg"}]}}`),
	})
	if len(fm.adIDs) != 1 || fm.adIDs[0] != "ad-1" || len(fm.refs[0]) != 2 {
		t.Fatalf("upserts = %v %+v", fm.adIDs, fm.refs)
	}
}

func TestRecordMedia_FailuresAreSwallowed(t *testing.T) {
	// Neither a store error nor undecodable raw JSON may panic or surface:
	// recordMedia has no error return on purpose, so a media problem can
	// never fail (and re-run) the Apify crawl.
	c := &Consumer{Media: &fakeMedia{err: errors.New("db down")}}
	c.recordMedia(context.Background(), apify.Ad{AdArchiveID: "ad-1", Raw: json.RawMessage(`{"snapshot":{"images":[{"resized_image_url":"https://x/a.jpg"}]}}`)})
	c.recordMedia(context.Background(), apify.Ad{AdArchiveID: "ad-2", Raw: json.RawMessage(`not json`)})

	(&Consumer{}).recordMedia(context.Background(), apify.Ad{AdArchiveID: "ad-3"}) // nil Media: no-op
}
