package apify

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// limitsStub serves the shape the real API returns. The field names are the
// non-obvious part (maxConcurrentActorJobs, not ...ActorRuns), so the stub
// mirrors a response captured from Apify rather than one invented here.
func limitsStub(t *testing.T, usage, limit float64, maxConc int, cycleEnds time.Time) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("token") == "" {
			t.Error("limits request carried no token")
		}
		fmt.Fprintf(w, `{"data":{
			"monthlyUsageCycle":{"startAt":"2026-10-06T00:00:00.000Z","endAt":%q},
			"limits":{"maxMonthlyUsageUsd":%v,"maxConcurrentActorJobs":%d},
			"current":{"monthlyUsageUsd":%v,"activeActorJobCount":2}}}`,
			cycleEnds.UTC().Format(time.RFC3339Nano), limit, maxConc, usage)
	}))
}

func newRegistry(t *testing.T, db *sql.DB, baseURL string) *Registry {
	t.Helper()
	r := NewRegistry(db, NewLimitsClient(baseURL))
	r.Logf = nil
	return r
}

func tokenRow(t *testing.T, db *sql.DB, idx int) TokenState {
	t.Helper()
	var s TokenState
	err := db.QueryRow(
		`SELECT token_index, max_concurrency, usage_usd, usage_limit_usd,
		        cycle_ends_at, active_jobs, checked_at, exhausted_until, last_error
		   FROM apify_tokens WHERE token_index = $1`, idx).
		Scan(&s.TokenIndex, &s.MaxConcurrency, &s.UsageUSD, &s.UsageLimitUSD,
			&s.CycleEndsAt, &s.ActiveJobs, &s.CheckedAt, &s.ExhaustedUntil, &s.LastError)
	if err != nil {
		t.Fatalf("read token %d: %v", idx, err)
	}
	return s
}

func TestRegistry_RefreshOneRecordsWhatApifyReports(t *testing.T) {
	db := testDB(t)
	cycleEnds := time.Now().Add(22 * 24 * time.Hour).Truncate(time.Second)
	srv := limitsStub(t, 0.37, 5, 25, cycleEnds)
	defer srv.Close()

	reg := newRegistry(t, db, srv.URL)
	if err := reg.Seed(context.Background(), []string{"tok-a"}, 5); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	if _, err := reg.RefreshOne(context.Background(), 0, "tok-a"); err != nil {
		t.Fatalf("RefreshOne: %v", err)
	}

	got := tokenRow(t, db, 0)
	if got.MaxConcurrency != 25 {
		t.Errorf("max_concurrency = %d, want 25 from the API -- a paid account's headroom is wasted if this stays hardcoded at 5", got.MaxConcurrency)
	}
	if got.UsageUSD == nil || *got.UsageUSD != 0.37 {
		t.Errorf("usage_usd = %v, want 0.37", got.UsageUSD)
	}
	if got.CycleEndsAt == nil || got.CycleEndsAt.Unix() != cycleEnds.Unix() {
		t.Errorf("cycle_ends_at = %v, want %v", got.CycleEndsAt, cycleEnds)
	}
	if got.ActiveJobs == nil || *got.ActiveJobs != 2 {
		t.Errorf("active_jobs = %v, want 2", got.ActiveJobs)
	}
	if got.CheckedAt == nil {
		t.Error("checked_at not set")
	}
}

// Parking ahead of the wall is the point of the registry: otherwise an
// account is only found to be spent by a job dying on it.
func TestRegistry_ParksBeforeTheAccountFails(t *testing.T) {
	db := testDB(t)
	cycleEnds := time.Now().Add(20 * 24 * time.Hour)
	srv := limitsStub(t, 4.9, 5, 5, cycleEnds) // 98% spent, not yet failing
	defer srv.Close()

	reg := newRegistry(t, db, srv.URL)
	if err := reg.Seed(context.Background(), []string{"tok-a"}, 5); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	if _, err := reg.RefreshOne(context.Background(), 0, "tok-a"); err != nil {
		t.Fatalf("RefreshOne: %v", err)
	}

	parked, _, err := reg.ApplyPolicy(context.Background())
	if err != nil {
		t.Fatalf("ApplyPolicy: %v", err)
	}
	if parked != 1 {
		t.Fatalf("parked = %d, want 1", parked)
	}

	got := tokenRow(t, db, 0)
	if !got.Parked() {
		t.Fatal("account not parked at 98% usage")
	}
	// The park must last until the billing cycle actually rolls over. A short
	// fixed park would let the pool retry a spent account hundreds of times
	// before it could possibly recover.
	if got.ExhaustedUntil.Sub(time.Now()) < 19*24*time.Hour {
		t.Errorf("parked only until %v, want the cycle end ~20 days out", got.ExhaustedUntil)
	}
}

func TestRegistry_LeavesHealthyAccountAlone(t *testing.T) {
	db := testDB(t)
	srv := limitsStub(t, 0.37, 5, 5, time.Now().Add(20*24*time.Hour)) // 7%
	defer srv.Close()

	reg := newRegistry(t, db, srv.URL)
	if err := reg.Seed(context.Background(), []string{"tok-a"}, 5); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	if _, err := reg.RefreshOne(context.Background(), 0, "tok-a"); err != nil {
		t.Fatalf("RefreshOne: %v", err)
	}
	parked, _, err := reg.ApplyPolicy(context.Background())
	if err != nil {
		t.Fatalf("ApplyPolicy: %v", err)
	}
	if parked != 0 {
		t.Errorf("parked %d healthy account(s), want 0", parked)
	}
}

// An account whose cycle has rolled over must come back on its own, or
// capacity only ever shrinks.
func TestRegistry_ReleasesAccountWhoseCycleRolledOver(t *testing.T) {
	db := testDB(t)
	srv := limitsStub(t, 4.9, 5, 5, time.Now().Add(20*24*time.Hour))
	defer srv.Close()

	reg := newRegistry(t, db, srv.URL)
	if err := reg.Seed(context.Background(), []string{"tok-a"}, 5); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	if _, err := db.Exec(
		`UPDATE apify_tokens SET exhausted_until = now() - interval '1 minute',
		        usage_usd = 5, usage_limit_usd = 5 WHERE token_index = 0`); err != nil {
		t.Fatalf("expire park: %v", err)
	}

	_, released, err := reg.ApplyPolicy(context.Background())
	if err != nil {
		t.Fatalf("ApplyPolicy: %v", err)
	}
	if released != 1 {
		t.Fatalf("released = %d, want 1", released)
	}

	got := tokenRow(t, db, 0)
	if got.Parked() {
		t.Error("still parked after the cycle rolled over")
	}
	// Last cycle's usage figure must not survive, or the park rule re-fires
	// on a number that no longer describes reality.
	if got.UsageUSD != nil {
		t.Errorf("usage_usd = %v, want cleared so the stale number cannot re-park it", *got.UsageUSD)
	}
}

func TestRegistry_RecordsRefreshFailureWithoutLosingTheAccount(t *testing.T) {
	db := testDB(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	reg := newRegistry(t, db, srv.URL)
	if err := reg.Seed(context.Background(), []string{"tok-a"}, 5); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	if _, err := reg.RefreshOne(context.Background(), 0, "tok-a"); err == nil {
		t.Fatal("want an error from a failing limits endpoint")
	}

	got := tokenRow(t, db, 0)
	if got.LastError == nil {
		t.Error("last_error not recorded")
	}
	// checked_at must advance even on failure, or the sweep retries this one
	// account forever and never reaches the others.
	if got.CheckedAt == nil {
		t.Error("checked_at not advanced on failure; the sweep would loop on this account")
	}
	if got.Parked() {
		t.Error("a refresh failure must not park the account -- the API being down says nothing about its budget")
	}
}

// Re-seeding is how accounts are added; it must not wipe measured state.
func TestRegistry_SeedIsIdempotentAndKeepsMeasuredState(t *testing.T) {
	db := testDB(t)
	srv := limitsStub(t, 1.5, 5, 5, time.Now().Add(20*24*time.Hour))
	defer srv.Close()

	reg := newRegistry(t, db, srv.URL)
	ctx := context.Background()
	if err := reg.Seed(ctx, []string{"tok-a"}, 5); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	if _, err := reg.RefreshOne(ctx, 0, "tok-a"); err != nil {
		t.Fatalf("RefreshOne: %v", err)
	}

	if err := reg.Seed(ctx, []string{"tok-a", "tok-b"}, 5); err != nil {
		t.Fatalf("re-Seed: %v", err)
	}

	got := tokenRow(t, db, 0)
	if got.UsageUSD == nil || *got.UsageUSD != 1.5 {
		t.Errorf("usage_usd = %v after re-seed, want 1.5 preserved", got.UsageUSD)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM apify_tokens`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("%d accounts after re-seed, want 2", n)
	}
}

func TestRegistry_SummaryAnswersHowMuchCapacityIsLeft(t *testing.T) {
	db := testDB(t)
	reg := newRegistry(t, db, "")
	ctx := context.Background()
	if err := reg.Seed(ctx, []string{"a", "b", "c"}, 5); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	if _, err := db.Exec(`UPDATE apify_tokens SET usage_usd = 1, usage_limit_usd = 5`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`UPDATE apify_tokens SET exhausted_until = now() + interval '3 hours',
		        cycle_ends_at = now() + interval '3 hours' WHERE token_index = 2`); err != nil {
		t.Fatal(err)
	}

	s, err := reg.Summary(ctx)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if s.Accounts != 3 || s.Usable != 2 || s.Parked != 1 {
		t.Errorf("accounts=%d usable=%d parked=%d, want 3/2/1", s.Accounts, s.Usable, s.Parked)
	}
	if s.SlotsTotal != 10 {
		t.Errorf("slots_total = %d, want 10 (2 usable accounts x 5)", s.SlotsTotal)
	}
	if s.BudgetLeftUSD != 12 {
		t.Errorf("budget_left_usd = %v, want 12 (3 accounts x $4)", s.BudgetLeftUSD)
	}
	if s.ResetsNext24h != 1 {
		t.Errorf("resets_next_24h = %d, want 1", s.ResetsNext24h)
	}
}

// The slot pool must use the registry's secret, not a process-local env list:
// that is what makes 1000 accounts manageable without a 46KB env var.
func TestRegistry_SlotStoreUsesSecretFromTheRegistry(t *testing.T) {
	db := testDB(t)
	reg := newRegistry(t, db, "")
	if err := reg.Seed(context.Background(), []string{"secret-from-db"}, 5); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	// No tokens passed to the store at all -- the registry is the source.
	s := NewSlotStore(db, nil, "test", time.Minute)
	s.Logf = nil

	lease, err := s.TryAcquire(context.Background(), "actor", "job-1")
	if err != nil {
		t.Fatalf("TryAcquire with no env tokens: %v", err)
	}
	if lease.Token != "secret-from-db" {
		t.Errorf("token = %q, want the registry's secret", lease.Token)
	}
}
