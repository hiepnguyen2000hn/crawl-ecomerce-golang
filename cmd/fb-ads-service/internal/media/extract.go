// Package media keeps local copies of the images a Facebook ad points at.
//
// The Apify actor returns fbcdn URLs that are signed and expire a few days
// after the crawl (the `oe` query parameter, unix seconds in hex). Once they
// expire the creative is gone, so after each crawl we download the SD image
// of every ad -- and the cover frame of video ads -- to local disk. The raw
// Apify JSON in fbads_raw is never rewritten; fbads_media tracks the copies.
package media

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Kind string

const (
	KindImage      Kind = "image"
	KindVideoCover Kind = "video_cover"
)

// Ref is one media URL found in an ad's snapshot. JSONPath is stable across
// re-crawls of the same ad (the URL is not: it is re-signed every time), so
// it is what identifies the media together with the ad archive ID.
type Ref struct {
	JSONPath  string
	Kind      Kind
	URL       string
	ExpiresAt time.Time // zero when the URL carries no parseable expiry
}

type snapshotItem struct {
	ResizedImageURL      *string `json:"resized_image_url"`
	OriginalImageURL     *string `json:"original_image_url"`
	VideoPreviewImageURL *string `json:"video_preview_image_url"`
}

type adSnapshot struct {
	Snapshot struct {
		Images      []snapshotItem `json:"images"`
		ExtraImages []snapshotItem `json:"extra_images"`
		Cards       []snapshotItem `json:"cards"`
		Videos      []snapshotItem `json:"videos"`
		ExtraVideos []snapshotItem `json:"extra_videos"`
	} `json:"snapshot"`
}

// ExtractRefs returns the SD image and video-cover URLs of one Apify ad item.
// Video files, HD/original variants (except as a fallback when the resized
// image is missing), watermarked copies and page/profile pictures are
// deliberately skipped.
func ExtractRefs(raw json.RawMessage) ([]Ref, error) {
	var ad adSnapshot
	if err := json.Unmarshal(raw, &ad); err != nil {
		return nil, fmt.Errorf("media: decode ad snapshot: %w", err)
	}

	var refs []Ref
	add := func(path string, kind Kind, u string) {
		refs = append(refs, Ref{JSONPath: path, Kind: kind, URL: u, ExpiresAt: ParseExpiry(u)})
	}
	groups := []struct {
		name  string
		items []snapshotItem
	}{
		{"images", ad.Snapshot.Images},
		{"extra_images", ad.Snapshot.ExtraImages},
		{"cards", ad.Snapshot.Cards},
		{"videos", ad.Snapshot.Videos},
		{"extra_videos", ad.Snapshot.ExtraVideos},
	}
	for _, g := range groups {
		for i, it := range g.items {
			prefix := fmt.Sprintf("snapshot.%s[%d].", g.name, i)
			switch {
			case nonEmpty(it.ResizedImageURL):
				add(prefix+"resized_image_url", KindImage, *it.ResizedImageURL)
			case nonEmpty(it.OriginalImageURL):
				add(prefix+"original_image_url", KindImage, *it.OriginalImageURL)
			}
			if nonEmpty(it.VideoPreviewImageURL) {
				add(prefix+"video_preview_image_url", KindVideoCover, *it.VideoPreviewImageURL)
			}
		}
	}
	return refs, nil
}

func nonEmpty(s *string) bool { return s != nil && strings.TrimSpace(*s) != "" }

// ParseExpiry reads fbcdn's `oe` parameter (unix seconds, hex). It returns
// the zero time when the parameter is missing or malformed.
func ParseExpiry(rawURL string) time.Time {
	u, err := url.Parse(rawURL)
	if err != nil {
		return time.Time{}
	}
	oe := u.Query().Get("oe")
	if oe == "" {
		return time.Time{}
	}
	sec, err := strconv.ParseInt(oe, 16, 64)
	if err != nil || sec <= 0 {
		return time.Time{}
	}
	return time.Unix(sec, 0).UTC()
}

var (
	unsafeIDChars = regexp.MustCompile(`[^0-9A-Za-z_-]`)
	pathSeparator = regexp.MustCompile(`[.\[\]]+`)
	repeatedUnder = regexp.MustCompile(`_+`)
)

// StorageKey is the file's path relative to the media root, e.g.
// fbads/2179652559429754/snapshot_images_0_resized_image_url.jpg. The ad ID
// comes from scraped data, so it is reduced to a safe charset before it
// becomes a directory name.
func StorageKey(adArchiveID, jsonPath, ext string) string {
	id := unsafeIDChars.ReplaceAllString(adArchiveID, "_")
	if id == "" {
		id = "_"
	}
	slug := repeatedUnder.ReplaceAllString(pathSeparator.ReplaceAllString(jsonPath, "_"), "_")
	slug = unsafeIDChars.ReplaceAllString(strings.Trim(slug, "_"), "_")
	return "fbads/" + id + "/" + slug + ext
}
