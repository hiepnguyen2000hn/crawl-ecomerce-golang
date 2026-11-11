package kalodata

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("KALODATA_TEST_DSN")
	if dsn == "" {
		t.Skip("KALODATA_TEST_DSN not set; skipping cookie-registry integration tests")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`TRUNCATE kalodata_cookies RESTART IDENTITY`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return db
}

func TestParseCookies(t *testing.T) {
	got := ParseCookies("a=1; b=2\n\n  c=3  \n")
	want := []string{"a=1; b=2", "c=3"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestSyncCookies_KeepsFirstSeenOnReSync(t *testing.T) {
	db := testDB(t)
	r := NewCookieRegistry(db, 168*time.Hour, 10, 10)
	ctx := context.Background()

	if err := r.SyncCookies(ctx, []string{"SESSION=x"}); err != nil {
		t.Fatal(err)
	}
	var first1 time.Time
	db.QueryRow(`SELECT first_seen_at FROM kalodata_cookies WHERE cookie_hash=$1`, CookieHash("SESSION=x")).Scan(&first1)

	if err := r.SyncCookies(ctx, []string{"SESSION=x"}); err != nil {
		t.Fatal(err)
	}
	var first2 time.Time
	var cnt int
	db.QueryRow(`SELECT count(*) FROM kalodata_cookies`).Scan(&cnt)
	db.QueryRow(`SELECT first_seen_at FROM kalodata_cookies WHERE cookie_hash=$1`, CookieHash("SESSION=x")).Scan(&first2)
	if cnt != 1 {
		t.Fatalf("want 1 row, got %d", cnt)
	}
	if !first1.Equal(first2) {
		t.Fatalf("first_seen_at reset: %v -> %v", first1, first2)
	}
}

func TestSyncCookies_DeadStaysDeadOnReSync(t *testing.T) {
	db := testDB(t)
	r := NewCookieRegistry(db, 168*time.Hour, 10, 10)
	ctx := context.Background()
	if err := r.SyncCookies(ctx, []string{"SESSION=x"}); err != nil {
		t.Fatal(err)
	}
	// server rejected it (401) -> marked dead
	db.Exec(`UPDATE kalodata_cookies SET dead = true WHERE cookie_hash = $1`, CookieHash("SESSION=x"))
	// restart re-syncs the same env cookie; it must NOT be revived
	if err := r.SyncCookies(ctx, []string{"SESSION=x"}); err != nil {
		t.Fatal(err)
	}
	var dead bool
	db.QueryRow(`SELECT dead FROM kalodata_cookies WHERE cookie_hash=$1`, CookieHash("SESSION=x")).Scan(&dead)
	if !dead {
		t.Fatal("re-sync revived a dead cookie; it must stay dead")
	}
}

func TestLease_RotatesOnQuota(t *testing.T) {
	db := testDB(t)
	r := NewCookieRegistry(db, 168*time.Hour, 1, 1) // search quota 1
	ctx := context.Background()
	if err := r.SyncCookies(ctx, []string{"c=A", "c=B"}); err != nil {
		t.Fatal(err)
	}

	l1, err := r.Lease(ctx, "search")
	if err != nil {
		t.Fatal(err)
	}
	l2, err := r.Lease(ctx, "search")
	if err != nil {
		t.Fatal(err)
	}
	if l1.ID == l2.ID {
		t.Fatal("expected rotation to a different cookie on quota")
	}
	if _, err := r.Lease(ctx, "search"); !errors.Is(err, ErrQuotaExhausted) {
		t.Fatalf("want ErrQuotaExhausted, got %v", err)
	}
	var dead int
	db.QueryRow(`SELECT count(*) FROM kalodata_cookies WHERE dead`).Scan(&dead)
	if dead != 0 {
		t.Fatal("quota exhaustion must not mark any cookie dead")
	}
}

func TestLease_SkipsDeadAndExpired(t *testing.T) {
	db := testDB(t)
	r := NewCookieRegistry(db, 168*time.Hour, 10, 10)
	ctx := context.Background()
	if err := r.SyncCookies(ctx, []string{"c=live"}); err != nil {
		t.Fatal(err)
	}
	// one expired, one dead
	db.Exec(`INSERT INTO kalodata_cookies (cookie_hash, cookie, expires_at) VALUES ('h-exp','c=exp', now()-interval '1 hour')`)
	db.Exec(`INSERT INTO kalodata_cookies (cookie_hash, cookie, expires_at, dead) VALUES ('h-dead','c=dead', now()+interval '1 hour', true)`)

	l, err := r.Lease(ctx, "search")
	if err != nil {
		t.Fatal(err)
	}
	if l.Cookie != "c=live" {
		t.Fatalf("leased wrong cookie: %q", l.Cookie)
	}
	if err := r.MarkDead(ctx, l.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Lease(ctx, "search"); !errors.Is(err, ErrNoCookies) {
		t.Fatalf("want ErrNoCookies, got %v", err)
	}
}

func TestLease_ResetsCountersOnNewDay(t *testing.T) {
	db := testDB(t)
	r := NewCookieRegistry(db, 168*time.Hour, 10, 10)
	ctx := context.Background()
	if err := r.SyncCookies(ctx, []string{"c=A"}); err != nil {
		t.Fatal(err)
	}
	// simulate yesterday fully used
	db.Exec(`UPDATE kalodata_cookies SET usage_date = CURRENT_DATE - 1, search_used = 10`)
	l, err := r.Lease(ctx, "search")
	if err != nil {
		t.Fatalf("expected reset+lease, got %v", err)
	}
	if l.Cookie != "c=A" {
		t.Fatal("wrong cookie")
	}
	var used int
	db.QueryRow(`SELECT search_used FROM kalodata_cookies`).Scan(&used)
	if used != 1 {
		t.Fatalf("want search_used=1 after reset+use, got %d", used)
	}
}
