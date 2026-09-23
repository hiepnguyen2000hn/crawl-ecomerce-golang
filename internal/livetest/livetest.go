//go:build live

// Package livetest holds helpers shared by the `live`-tagged tests that
// crawl real sources (SerpAPI, Apify, crawl4ai, OpenRouter). Those tests
// spend real quota, so everything here — and every test using it — only
// compiles with `go test -tags live`.
//
// Environment shared by all live tests:
//
//	SERPAPI_API_KEY, APIFY_API_TOKEN, OPENROUTER_API_KEY
//	                 read from the environment, falling back to <repo>/.env
//	LIVE_OUT_DIR     where fetched data is saved (default <repo>/exports/live)
//	LIVE_PG_DSN      enables the end-to-end tests, e.g.
//	                 postgres://crawl:crawl@localhost:5433/crawl?sslmode=disable
//	LIVE_AMQP_URL    enables the end-to-end tests, e.g.
//	                 amqp://guest:guest@localhost:5673/
package livetest

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hiepnv/crawl-ecomerce-golang/pkg/db"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Exchange is deliberately not the production "crawl" exchange: nothing
// is bound to it, so completion events published by a worker under test
// do not wake up downstream services and spend their quota.
const Exchange = "crawl_live_test"

// RepoRoot walks up from the test's working directory (its package dir) to
// the directory holding go.mod.
func RepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above test directory")
		}
		dir = parent
	}
}

// Secret returns the named credential from the environment or the repo's
// .env file, and skips the test when it is set in neither.
func Secret(t *testing.T, name string) string {
	t.Helper()
	if v := os.Getenv(name); v != "" {
		return v
	}
	if f, err := os.Open(filepath.Join(RepoRoot(t), ".env")); err == nil {
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			if v, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), name+"="); ok {
				if v = strings.Trim(v, `"' `); v != "" {
					return v
				}
			}
		}
	}
	t.Skipf("%s not set and not found in .env — skipping live test", name)
	return ""
}

// EnvOr returns the named env var, or def when it is unset. An env var set
// to "" is returned as "" (e.g. an empty geo meaning worldwide).
func EnvOr(name, def string) string {
	if v, ok := os.LookupEnv(name); ok {
		return v
	}
	return def
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// Save writes data under LIVE_OUT_DIR as <name>_<timestamp>.<ext> so the
// crawled data can be inspected or diffed against a previous run.
func Save(t *testing.T, name, ext string, data []byte) string {
	t.Helper()
	dir := EnvOr("LIVE_OUT_DIR", filepath.Join(RepoRoot(t), "exports", "live"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create out dir: %v", err)
	}
	name = strings.Trim(unsafeName.ReplaceAllString(name, "-"), "-")
	if len(name) > 80 {
		name = name[:80]
	}
	path := filepath.Join(dir, fmt.Sprintf("%s_%s.%s", name, time.Now().UTC().Format("20060102T150405Z"), ext))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	t.Logf("saved %s", path)
	return path
}

// SaveJSON is Save for a value marshalled as indented JSON.
func SaveJSON(t *testing.T, name string, v any) string {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal %s: %v", name, err)
	}
	return Save(t, name, "json", data)
}

// Stack returns the Postgres DSN and AMQP URL of a running docker compose
// stack, skipping the test when either is not configured.
func Stack(t *testing.T) (dsn, amqpURL string) {
	t.Helper()
	dsn, amqpURL = os.Getenv("LIVE_PG_DSN"), os.Getenv("LIVE_AMQP_URL")
	if dsn == "" || amqpURL == "" {
		t.Skip("LIVE_PG_DSN / LIVE_AMQP_URL not set — skipping end-to-end live test")
	}
	return dsn, amqpURL
}

// PostgresDSN is Stack for tests that only need the database.
func PostgresDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("LIVE_PG_DSN")
	if dsn == "" {
		t.Skip("LIVE_PG_DSN not set — skipping end-to-end live test")
	}
	return dsn
}

// DB connects to Postgres and closes the connection when the test ends.
func DB(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	conn, err := db.Connect(dsn)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// CaptureEvent binds a throwaway queue to routingKey on Exchange and
// returns a func that waits up to timeout for the next message body.
func CaptureEvent(t *testing.T, amqpURL, routingKey string) func(timeout time.Duration) []byte {
	t.Helper()
	conn, err := amqp.Dial(amqpURL)
	if err != nil {
		t.Fatalf("dial rabbitmq: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("open channel: %v", err)
	}
	if err := ch.ExchangeDeclare(Exchange, "topic", true, false, false, false, nil); err != nil {
		t.Fatalf("declare exchange: %v", err)
	}
	q, err := ch.QueueDeclare("", false, true, true, false, nil)
	if err != nil {
		t.Fatalf("declare capture queue: %v", err)
	}
	if err := ch.QueueBind(q.Name, routingKey, Exchange, false, nil); err != nil {
		t.Fatalf("bind capture queue: %v", err)
	}
	return func(timeout time.Duration) []byte {
		t.Helper()
		deadline := time.Now().Add(timeout)
		for {
			d, ok, err := ch.Get(q.Name, true)
			if err != nil {
				t.Fatalf("get %s event: %v", routingKey, err)
			}
			if ok {
				return d.Body
			}
			if time.Now().After(deadline) {
				t.Fatalf("no %s event received within %s", routingKey, timeout)
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}
