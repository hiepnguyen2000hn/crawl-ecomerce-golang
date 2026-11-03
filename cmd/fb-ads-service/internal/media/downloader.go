package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// ErrExpired means the source URL is gone for good (fbcdn answers 403 once
// the signature's `oe` expiry has passed). Retrying will not help.
var ErrExpired = errors.New("media: source url expired")

const userAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36"

// Downloader saves one media URL under Dir at its StorageKey.
type Downloader struct {
	HTTP     *http.Client
	Dir      string
	MaxBytes int64
}

type Result struct {
	StorageKey  string
	ContentType string
	Size        int64
}

func (d *Downloader) Fetch(ctx context.Context, r Row) (Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.URL, nil)
	if err != nil {
		return Result{}, fmt.Errorf("media: build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	client := d.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("media: get: %w", err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		return Result{}, fmt.Errorf("%w: http %d", ErrExpired, resp.StatusCode)
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		return Result{}, fmt.Errorf("media: http %d", resp.StatusCode)
	}

	contentType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if !strings.HasPrefix(contentType, "image/") {
		return Result{}, fmt.Errorf("media: unexpected content type %q", resp.Header.Get("Content-Type"))
	}

	key := StorageKey(r.AdArchiveID, r.JSONPath, extFor(contentType))
	dst := filepath.Join(d.Dir, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return Result{}, fmt.Errorf("media: mkdir: %w", err)
	}

	size, err := writeAtomic(dst, io.LimitReader(resp.Body, d.MaxBytes+1), d.MaxBytes)
	if err != nil {
		return Result{}, err
	}
	return Result{StorageKey: key, ContentType: contentType, Size: size}, nil
}

// writeAtomic streams src into dst via a temp file so a reader (nginx) never
// sees a half-written image, and nothing is left behind on failure.
func writeAtomic(dst string, src io.Reader, maxBytes int64) (int64, error) {
	// A unique temp name: the sweeper and a job message may fetch the same
	// row at the same time.
	f, err := os.CreateTemp(filepath.Dir(dst), "."+filepath.Base(dst)+".*.tmp")
	if err != nil {
		return 0, fmt.Errorf("media: create temp: %w", err)
	}
	tmp := f.Name()
	n, err := io.Copy(f, src)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > maxBytes {
		err = fmt.Errorf("media: body larger than %d bytes", maxBytes)
	}
	if err == nil {
		// OpenFile's mode is filtered by the umask; nginx must be able to read it.
		err = os.Chmod(tmp, 0o644)
	}
	if err == nil {
		err = os.Rename(tmp, dst)
	}
	if err != nil {
		os.Remove(tmp)
		return 0, fmt.Errorf("media: write %s: %w", dst, err)
	}
	return n, nil
}

func extFor(contentType string) string {
	switch contentType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		return ".img"
	}
}
