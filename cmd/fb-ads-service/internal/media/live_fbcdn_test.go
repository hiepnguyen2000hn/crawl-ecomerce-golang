package media

import (
	"context"
	"errors"
	"net/http"
	"os"
	"testing"
	"time"
)

// TestLive_ExpiredFbcdnURLsAreExpired checks the assumption the worker is
// built on: once its `oe` has passed, fbcdn answers a status the downloader
// classifies as ErrExpired (so the row is not retried). It hits the real CDN
// with the long-expired URLs in testdata, so it only runs on request:
//
//	FBADS_MEDIA_LIVE=1 go test -run Live ./cmd/fb-ads-service/internal/media/
func TestLive_ExpiredFbcdnURLsAreExpired(t *testing.T) {
	if os.Getenv("FBADS_MEDIA_LIVE") == "" {
		t.Skip("FBADS_MEDIA_LIVE not set; skipping live fbcdn check")
	}
	d := &Downloader{HTTP: &http.Client{Timeout: 20 * time.Second}, Dir: t.TempDir(), MaxBytes: 10 << 20}
	for _, raw := range loadSample(t) {
		refs, err := ExtractRefs(raw)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range refs {
			if r.ExpiresAt.After(time.Now()) {
				t.Logf("%s not expired yet (oe %v); skipping", r.JSONPath, r.ExpiresAt)
				continue
			}
			_, err := d.Fetch(context.Background(), Row{ID: "live", AdArchiveID: "live", JSONPath: r.JSONPath, URL: r.URL})
			if !errors.Is(err, ErrExpired) {
				t.Errorf("%s: err = %v, want ErrExpired", r.JSONPath, err)
			} else {
				t.Logf("%s: %v", r.JSONPath, err)
			}
		}
	}
}
