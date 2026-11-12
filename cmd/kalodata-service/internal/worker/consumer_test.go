package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/hiepnv/crawl-ecomerce-golang/pkg/kalodata"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/rabbitmq"
)

type fakeKalodata struct {
	productErr error
	lastProd   kalodata.ProductParams
}

func (f *fakeKalodata) AllLastDay(ctx context.Context, country string) (string, error) {
	return "2026-11-10", nil
}
func (f *fakeKalodata) SearchProducts(ctx context.Context, p kalodata.ProductParams) ([]kalodata.Product, error) {
	f.lastProd = p
	return nil, f.productErr
}
func (f *fakeKalodata) SearchVideos(ctx context.Context, p kalodata.VideoParams) ([]kalodata.Video, error) {
	return nil, nil
}
func (f *fakeKalodata) VideoDetail(ctx context.Context, country, id, start, end string) (json.RawMessage, error) {
	return nil, nil
}

func TestHandleMessage_UnmarshalError(t *testing.T) {
	c := &Consumer{Client: &fakeKalodata{}}
	if err := c.HandleMessage([]byte("not json")); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestHandleMessage_SearchErrorForwardsDefaults(t *testing.T) {
	fake := &fakeKalodata{productErr: errors.New("boom")}
	c := &Consumer{Client: fake}
	msg, _ := json.Marshal(JobMessage{JobID: "11111111-1111-1111-1111-111111111111"})
	if err := c.HandleMessage(msg); err == nil {
		t.Fatal("expected error propagated")
	}
	p := fake.lastProd
	if p.Country != "VN" || p.PageNo != 1 || p.PageSize != 10 || p.SortField != "revenue" || p.SortType != "DESC" {
		t.Fatalf("defaults not applied: %+v", p)
	}
}

func TestHandleMessage_QuotaExhaustedFailsNotRequeue(t *testing.T) {
	// Quota exhaustion must NOT requeue: an immediate requeue busy-loops until
	// the daily reset. The job fails (operator resubmits after reset).
	fake := &fakeKalodata{productErr: kalodata.ErrQuotaExhausted}
	c := &Consumer{Client: fake}
	msg, _ := json.Marshal(JobMessage{JobID: "11111111-1111-1111-1111-111111111111"})
	err := c.HandleMessage(msg)
	if err == nil {
		t.Fatal("expected an error")
	}
	if errors.Is(err, rabbitmq.ErrRequeue) {
		t.Fatalf("quota exhaustion must not requeue (busy-loop); got %v", err)
	}
}
