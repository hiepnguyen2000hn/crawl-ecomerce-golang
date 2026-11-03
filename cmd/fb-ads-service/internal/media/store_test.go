package media

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Integration tests against a migrated database:
//
//	FBADS_MEDIA_TEST_DSN='postgres://crawl:crawl@127.0.0.1:5433/crawl?sslmode=disable' go test ./cmd/fb-ads-service/internal/media/
func testStore(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("FBADS_MEDIA_TEST_DSN")
	if dsn == "" {
		t.Skip("FBADS_MEDIA_TEST_DSN not set; skipping fbads_media integration tests")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`TRUNCATE fbads_media`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return &Store{DB: db}
}

// seedJob inserts a job and one fbads_raw row per ad ID, returning the job ID.
func seedJob(t *testing.T, s *Store, adIDs ...string) string {
	t.Helper()
	var jobID string
	if err := s.DB.QueryRow(`INSERT INTO jobs (type, status) VALUES ('fbads', 'crawled') RETURNING id`).Scan(&jobID); err != nil {
		t.Fatalf("insert job: %v", err)
	}
	for _, id := range adIDs {
		if _, err := s.DB.Exec(`INSERT INTO fbads_raw (job_id, ad_archive_id, raw) VALUES ($1, $2, '{}')`, jobID, id); err != nil {
			t.Fatalf("insert fbads_raw: %v", err)
		}
	}
	return jobID
}

func statusOf(t *testing.T, s *Store, adID, path string) (status, url string, attempts int) {
	t.Helper()
	if err := s.DB.QueryRow(`SELECT status, source_url, attempts FROM fbads_media WHERE ad_archive_id=$1 AND json_path=$2`, adID, path).
		Scan(&status, &url, &attempts); err != nil {
		t.Fatalf("select %s %s: %v", adID, path, err)
	}
	return
}

func TestUpsertPending_Idempotent(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	refs := []Ref{
		{JSONPath: "snapshot.images[0].resized_image_url", Kind: KindImage, URL: "https://x/a.jpg", ExpiresAt: time.Now().Add(time.Hour)},
		{JSONPath: "snapshot.videos[0].video_preview_image_url", Kind: KindVideoCover, URL: "https://x/b.jpg"},
	}
	for i := 0; i < 2; i++ {
		if err := s.UpsertPending(ctx, "ad-1", refs); err != nil {
			t.Fatalf("upsert #%d: %v", i, err)
		}
	}
	var n int
	if err := s.DB.QueryRow(`SELECT count(*) FROM fbads_media WHERE ad_archive_id='ad-1'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("rows = %d, want 2", n)
	}
}

func TestUpsertPending_RevivesFailedKeepsDone(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	a := Ref{JSONPath: "snapshot.images[0].resized_image_url", Kind: KindImage, URL: "https://x/a-old.jpg"}
	b := Ref{JSONPath: "snapshot.images[1].resized_image_url", Kind: KindImage, URL: "https://x/b-old.jpg"}
	if err := s.UpsertPending(ctx, "ad-2", []Ref{a, b}); err != nil {
		t.Fatal(err)
	}
	jobID := seedJob(t, s, "ad-2")
	rows, err := s.PendingForJob(ctx, jobID)
	if err != nil || len(rows) != 2 {
		t.Fatalf("PendingForJob = %+v, %v", rows, err)
	}
	for _, r := range rows {
		if r.JSONPath == a.JSONPath {
			if err := s.MarkDone(ctx, r.ID, "fbads/ad-2/a.jpg", "image/jpeg", 3); err != nil {
				t.Fatal(err)
			}
		} else if err := s.MarkFailed(ctx, r.ID, true, "403"); err != nil {
			t.Fatal(err)
		}
	}

	a.URL, b.URL = "https://x/a-new.jpg", "https://x/b-new.jpg"
	if err := s.UpsertPending(ctx, "ad-2", []Ref{a, b}); err != nil {
		t.Fatal(err)
	}
	if st, u, _ := statusOf(t, s, "ad-2", a.JSONPath); st != "done" || u != "https://x/a-old.jpg" {
		t.Errorf("done row changed: status=%s url=%s", st, u)
	}
	if st, u, att := statusOf(t, s, "ad-2", b.JSONPath); st != "pending" || u != "https://x/b-new.jpg" || att != 0 {
		t.Errorf("expired row not revived: status=%s url=%s attempts=%d", st, u, att)
	}
}

func TestPendingForJob_ScopedToJob(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ref := []Ref{{JSONPath: "snapshot.images[0].resized_image_url", Kind: KindImage, URL: "https://x/a.jpg"}}
	if err := s.UpsertPending(ctx, "ad-mine", ref); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertPending(ctx, "ad-other", ref); err != nil {
		t.Fatal(err)
	}
	jobID := seedJob(t, s, "ad-mine")
	seedJob(t, s, "ad-other")

	rows, err := s.PendingForJob(ctx, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].AdArchiveID != "ad-mine" || rows[0].URL != "https://x/a.jpg" || rows[0].Kind != KindImage {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestMarkFailed_CountsAttempts(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.UpsertPending(ctx, "ad-3", []Ref{{JSONPath: "p", Kind: KindImage, URL: "https://x/a.jpg"}}); err != nil {
		t.Fatal(err)
	}
	rows, err := s.PendingForJob(ctx, seedJob(t, s, "ad-3"))
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
	if err := s.MarkFailed(ctx, rows[0].ID, false, "timeout"); err != nil {
		t.Fatal(err)
	}
	if st, _, att := statusOf(t, s, "ad-3", "p"); st != "failed" || att != 1 {
		t.Errorf("status=%s attempts=%d, want failed/1", st, att)
	}
}

func TestRetryable_StalePendingAndFailedUnexpired(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	future, past := time.Now().Add(48*time.Hour), time.Now().Add(-time.Hour)
	refs := []Ref{
		{JSONPath: "fresh-pending", Kind: KindImage, URL: "u", ExpiresAt: future},
		{JSONPath: "stale-pending", Kind: KindImage, URL: "u", ExpiresAt: future},
		{JSONPath: "failed-ok", Kind: KindImage, URL: "u", ExpiresAt: future},
		{JSONPath: "failed-too-many", Kind: KindImage, URL: "u", ExpiresAt: future},
		{JSONPath: "failed-url-dead", Kind: KindImage, URL: "u", ExpiresAt: past},
		{JSONPath: "expired", Kind: KindImage, URL: "u", ExpiresAt: future},
		{JSONPath: "done", Kind: KindImage, URL: "u", ExpiresAt: future},
	}
	if err := s.UpsertPending(ctx, "ad-r", refs); err != nil {
		t.Fatal(err)
	}
	set := func(path, status string, attempts int, age time.Duration) {
		if _, err := s.DB.Exec(`UPDATE fbads_media SET status=$2, attempts=$3, updated_at=now()-$4::interval WHERE json_path=$1`,
			path, status, attempts, fmt.Sprintf("%d seconds", int(age.Seconds()))); err != nil {
			t.Fatal(err)
		}
	}
	set("stale-pending", "pending", 0, time.Hour)
	set("failed-ok", "failed", 1, time.Hour)
	set("failed-too-many", "failed", MaxAttempts, time.Hour)
	set("failed-url-dead", "failed", 1, time.Hour)
	set("expired", "expired", 1, time.Hour)
	set("done", "done", 1, time.Hour)

	rows, err := s.Retryable(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, r := range rows {
		got[r.JSONPath] = true
	}
	if len(rows) != 2 || !got["stale-pending"] || !got["failed-ok"] {
		t.Fatalf("retryable = %v, want stale-pending and failed-ok", got)
	}
}
