package media

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func loadSample(t *testing.T) []json.RawMessage {
	t.Helper()
	b, err := os.ReadFile("testdata/apify_ads_sample.json")
	if err != nil {
		t.Fatal(err)
	}
	var items []json.RawMessage
	if err := json.Unmarshal(b, &items); err != nil {
		t.Fatal(err)
	}
	return items
}

func TestExtract_RealSample(t *testing.T) {
	byAd := map[string][]Ref{}
	total := 0
	for _, raw := range loadSample(t) {
		var head struct {
			AdArchiveID string `json:"ad_archive_id"`
		}
		if err := json.Unmarshal(raw, &head); err != nil {
			t.Fatal(err)
		}
		refs, err := ExtractRefs(raw)
		if err != nil {
			t.Fatalf("ExtractRefs(%s): %v", head.AdArchiveID, err)
		}
		byAd[head.AdArchiveID] = refs
		total += len(refs)
		for _, r := range refs {
			for _, bad := range []string{".mp4", "s60x60", "t39.30808"} {
				if strings.Contains(r.URL, bad) {
					t.Errorf("ad %s: ref %s has unwanted URL %q", head.AdArchiveID, r.JSONPath, r.URL)
				}
			}
		}
	}
	if total != 4 {
		t.Fatalf("total refs = %d, want 4", total)
	}

	video := byAd["1083605804695644"]
	if len(video) != 1 || video[0].JSONPath != "snapshot.videos[0].video_preview_image_url" || video[0].Kind != KindVideoCover {
		t.Errorf("video ad refs = %+v", video)
	}

	img := byAd["2179652559429754"]
	if len(img) != 1 || img[0].JSONPath != "snapshot.images[0].resized_image_url" || img[0].Kind != KindImage {
		t.Fatalf("image ad refs = %+v", img)
	}
	if !strings.HasPrefix(img[0].URL, "https://scontent.fakl5-2.fna.fbcdn.net/v/t39.35426-6/864828879_") {
		t.Errorf("image URL = %q", img[0].URL)
	}
	if want := time.Date(2026, 11, 2, 18, 11, 57, 0, time.UTC); !img[0].ExpiresAt.Equal(want) {
		t.Errorf("ExpiresAt = %v, want %v", img[0].ExpiresAt, want)
	}
}

func TestExtract_EmptyResizedFallsBackToOriginal(t *testing.T) {
	refs, err := ExtractRefs(json.RawMessage(`{"snapshot":{"images":[{"resized_image_url":"","original_image_url":"https://x/o.jpg?oe=6AE8D26D"}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].JSONPath != "snapshot.images[0].original_image_url" || refs[0].URL != "https://x/o.jpg?oe=6AE8D26D" {
		t.Fatalf("refs = %+v", refs)
	}

	refs, err = ExtractRefs(json.RawMessage(`{"snapshot":{"images":[{"resized_image_url":null}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 0 {
		t.Fatalf("null resized: refs = %+v, want none", refs)
	}
}

func TestExtract_CardsYieldBoth(t *testing.T) {
	refs, err := ExtractRefs(json.RawMessage(`{"snapshot":{"cards":[{"resized_image_url":"https://x/a.jpg","video_preview_image_url":"https://x/b.jpg","video_sd_url":"https://x/c.mp4"}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Kind{}
	for _, r := range refs {
		got[r.JSONPath] = r.Kind
	}
	if len(refs) != 2 ||
		got["snapshot.cards[0].resized_image_url"] != KindImage ||
		got["snapshot.cards[0].video_preview_image_url"] != KindVideoCover {
		t.Fatalf("refs = %+v", refs)
	}
}

func TestExtract_NoSnapshot(t *testing.T) {
	refs, err := ExtractRefs(json.RawMessage(`{"ad_archive_id":"1"}`))
	if err != nil || len(refs) != 0 {
		t.Fatalf("refs=%+v err=%v", refs, err)
	}
	if _, err := ExtractRefs(json.RawMessage(`not json`)); err == nil {
		t.Fatal("want error for invalid JSON")
	}
}

func TestParseExpiry(t *testing.T) {
	if got, want := ParseExpiry("https://x/a.jpg?oe=6AE8D26D"), time.Date(2026, 11, 2, 18, 11, 57, 0, time.UTC); !got.Equal(want) {
		t.Errorf("ParseExpiry = %v, want %v", got, want)
	}
	for _, u := range []string{"https://x/a.jpg", "https://x/a.jpg?oe=zz", "::bad"} {
		if got := ParseExpiry(u); !got.IsZero() {
			t.Errorf("ParseExpiry(%q) = %v, want zero", u, got)
		}
	}
}

func TestStorageKey(t *testing.T) {
	if got, want := StorageKey("2179652559429754", "snapshot.images[0].resized_image_url", ".jpg"),
		"fbads/2179652559429754/snapshot_images_0_resized_image_url.jpg"; got != want {
		t.Errorf("StorageKey = %q, want %q", got, want)
	}
	if got := StorageKey("../x", "a", ".jpg"); strings.Contains(got, "..") {
		t.Errorf("StorageKey with traversal id = %q", got)
	}
}
