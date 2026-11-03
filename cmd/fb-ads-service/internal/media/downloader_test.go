package media

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func serve(t *testing.T, status int, contentType, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" || strings.HasPrefix(r.Header.Get("User-Agent"), "Go-http-client") {
			t.Errorf("downloader sent default User-Agent %q", r.Header.Get("User-Agent"))
		}
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func testRow(url string) Row {
	return Row{ID: "r1", AdArchiveID: "2179652559429754", JSONPath: "snapshot.images[0].resized_image_url", Kind: KindImage, URL: url}
}

// listFiles returns every regular file under dir, relative to it.
func listFiles(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(dir, p)
			out = append(out, rel)
		}
		return nil
	})
	return out
}

func TestDownloader_SavesJPEG(t *testing.T) {
	srv := serve(t, 200, "image/jpeg", "abc")
	dir := t.TempDir()
	d := &Downloader{HTTP: srv.Client(), Dir: dir, MaxBytes: 1 << 20}

	res, err := d.Fetch(context.Background(), testRow(srv.URL+"/a.jpg?oe=6AE8D26D"))
	if err != nil {
		t.Fatal(err)
	}
	want := "fbads/2179652559429754/snapshot_images_0_resized_image_url.jpg"
	if res.StorageKey != want || res.ContentType != "image/jpeg" || res.Size != 3 {
		t.Fatalf("result = %+v", res)
	}
	path := filepath.Join(dir, want)
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "abc" {
		t.Fatalf("file = %q, %v", b, err)
	}
	st, _ := os.Stat(path)
	if st.Mode().Perm() != 0o644 {
		t.Errorf("mode = %v, want 0644", st.Mode().Perm())
	}
	if files := listFiles(t, dir); len(files) != 1 {
		t.Errorf("files = %v, want only the image", files)
	}
}

func TestDownloader_PNGExtension(t *testing.T) {
	srv := serve(t, 200, "image/png; charset=binary", "png")
	d := &Downloader{HTTP: srv.Client(), Dir: t.TempDir(), MaxBytes: 1 << 20}
	res, err := d.Fetch(context.Background(), testRow(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(res.StorageKey, ".png") || res.ContentType != "image/png" {
		t.Fatalf("result = %+v", res)
	}
}

func TestDownloader_403MarksExpired(t *testing.T) {
	for _, code := range []int{403, 404, 410} {
		srv := serve(t, code, "text/plain", "URL signature expired")
		d := &Downloader{HTTP: srv.Client(), Dir: t.TempDir(), MaxBytes: 1 << 20}
		if _, err := d.Fetch(context.Background(), testRow(srv.URL)); !errors.Is(err, ErrExpired) {
			t.Errorf("status %d: err = %v, want ErrExpired", code, err)
		}
	}
}

func TestDownloader_ServerErrorIsRetryable(t *testing.T) {
	srv := serve(t, 503, "text/plain", "busy")
	d := &Downloader{HTTP: srv.Client(), Dir: t.TempDir(), MaxBytes: 1 << 20}
	_, err := d.Fetch(context.Background(), testRow(srv.URL))
	if err == nil || errors.Is(err, ErrExpired) {
		t.Fatalf("err = %v, want non-expired error", err)
	}
}

func TestDownloader_RejectsNonImage(t *testing.T) {
	srv := serve(t, 200, "text/html", "<html>login</html>")
	dir := t.TempDir()
	d := &Downloader{HTTP: srv.Client(), Dir: dir, MaxBytes: 1 << 20}
	if _, err := d.Fetch(context.Background(), testRow(srv.URL)); err == nil {
		t.Fatal("want error for text/html")
	}
	if files := listFiles(t, dir); len(files) != 0 {
		t.Errorf("files written: %v", files)
	}
}

func TestDownloader_RejectsOversize(t *testing.T) {
	srv := serve(t, 200, "image/jpeg", "01234567890")
	dir := t.TempDir()
	d := &Downloader{HTTP: srv.Client(), Dir: dir, MaxBytes: 10}
	if _, err := d.Fetch(context.Background(), testRow(srv.URL)); err == nil {
		t.Fatal("want error for 11-byte body with MaxBytes 10")
	}
	if files := listFiles(t, dir); len(files) != 0 {
		t.Errorf("files left behind: %v", files)
	}
}

func TestDownloader_ConcurrentSameRow(t *testing.T) {
	// The sweeper and a job message can fetch the same row at once; both
	// must succeed and leave exactly one intact file.
	body := strings.Repeat("x", 1<<20)
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write([]byte(body[:len(body)/2]))
		w.(http.Flusher).Flush()
		<-release // both requests are now mid-write
		w.Write([]byte(body[len(body)/2:]))
	}))
	defer srv.Close()
	dir := t.TempDir()
	d := &Downloader{HTTP: srv.Client(), Dir: dir, MaxBytes: 2 << 20}

	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { _, err := d.Fetch(context.Background(), testRow(srv.URL)); errs <- err }()
	}
	time.Sleep(200 * time.Millisecond)
	close(release)
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Errorf("fetch %d: %v", i, err)
		}
	}
	files := listFiles(t, dir)
	if len(files) != 1 {
		t.Fatalf("files = %v, want exactly one", files)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, files[0])); len(b) != len(body) {
		t.Errorf("file size = %d, want %d", len(b), len(body))
	}
}
