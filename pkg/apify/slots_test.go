package apify

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// These are integration tests: the whole point of SlotStore is the Postgres
// behaviour (advisory lock, SKIP-the-race, expiry reaping), so mocking the
// database would test nothing worth testing.
//
//	docker compose up -d postgres
//	APIFY_SLOTS_TEST_DSN='postgres://crawl:crawl@127.0.0.1:5433/crawl?sslmode=disable' go test ./pkg/apify/
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("APIFY_SLOTS_TEST_DSN")
	if dsn == "" {
		t.Skip("APIFY_SLOTS_TEST_DSN not set; skipping SlotStore integration tests")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`TRUNCATE apify_leases, apify_tokens RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("truncate (did migration 000009 run?): %v", err)
	}
	return db
}

func newStore(t *testing.T, db *sql.DB, tokens []string, maxConc int) *SlotStore {
	t.Helper()
	s := NewSlotStore(db, tokens, "test", time.Minute)
	s.Logf = nil
	s.poll = 20 * time.Millisecond
	if err := s.SyncTokens(context.Background(), maxConc); err != nil {
		t.Fatalf("SyncTokens: %v", err)
	}
	return s
}

func liveLeases(t *testing.T, db *sql.DB, tokenIndex int) int {
	t.Helper()
	var n int
	if err := db.QueryRow(
		`SELECT count(*) FROM apify_leases l JOIN apify_tokens t ON t.id = l.token_id
		  WHERE t.token_index = $1 AND l.expires_at > now()`, tokenIndex).Scan(&n); err != nil {
		t.Fatalf("count leases: %v", err)
	}
	return n
}

// The headline requirement: 2 accounts x 5 concurrent = 10 parallel runs, and
// not one more, even when every worker asks at the same instant.
func TestAcquire_CapIsGlobalUnderConcurrency(t *testing.T) {
	db := testDB(t)
	s := newStore(t, db, []string{"tok-a", "tok-b"}, 5)
	ctx := context.Background()

	const attempts = 40
	var granted, refused atomic.Int64
	var wg sync.WaitGroup
	start := make(chan struct{})

	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start // maximise the overlap on the read-then-insert window
			lease, err := s.TryAcquire(ctx, "actor", "")
			switch {
			case err == nil:
				granted.Add(1)
				_ = lease
			case errors.Is(err, ErrNoCapacity):
				refused.Add(1)
			default:
				t.Errorf("TryAcquire: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()

	if got := granted.Load(); got != 10 {
		t.Errorf("granted = %d, want exactly 10 (2 tokens x 5)", got)
	}
	if got := refused.Load(); got != attempts-10 {
		t.Errorf("refused = %d, want %d", got, attempts-10)
	}
	for idx := 0; idx < 2; idx++ {
		if n := liveLeases(t, db, idx); n != 5 {
			t.Errorf("token %d holds %d leases, want 5", idx, n)
		}
	}
}

// Sticky rotation piles every parallel run onto one token; the allocator must
// spread them instead, or the second account is dead weight.
func TestAcquire_SpreadsLeastLoadedFirst(t *testing.T) {
	db := testDB(t)
	s := newStore(t, db, []string{"tok-a", "tok-b"}, 5)
	ctx := context.Background()

	for i := 0; i < 4; i++ {
		if _, err := s.TryAcquire(ctx, "actor", ""); err != nil {
			t.Fatalf("acquire %d: %v", i, err)
		}
	}
	a, b := liveLeases(t, db, 0), liveLeases(t, db, 1)
	if a != 2 || b != 2 {
		t.Errorf("leases split %d/%d, want 2/2 (least-loaded first)", a, b)
	}
}

func TestRelease_FreesTheSlot(t *testing.T) {
	db := testDB(t)
	s := newStore(t, db, []string{"tok-a"}, 1)
	ctx := context.Background()

	lease, err := s.TryAcquire(ctx, "actor", "job-1")
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if _, err := s.TryAcquire(ctx, "actor", "job-2"); !errors.Is(err, ErrNoCapacity) {
		t.Fatalf("second acquire err = %v, want ErrNoCapacity", err)
	}
	if err := s.Release(ctx, lease); err != nil {
		t.Fatalf("release: %v", err)
	}
	if _, err := s.TryAcquire(ctx, "actor", "job-3"); err != nil {
		t.Errorf("acquire after release: %v", err)
	}
}

// A worker that dies mid-run must not hold its slot forever.
func TestAcquire_ReclaimsExpiredLease(t *testing.T) {
	db := testDB(t)
	s := newStore(t, db, []string{"tok-a"}, 1)
	ctx := context.Background()

	if _, err := s.TryAcquire(ctx, "actor", "crashed"); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	// Simulate the holder's process dying: nobody will ever call Release.
	if _, err := db.Exec(`UPDATE apify_leases SET expires_at = now() - interval '1 second'`); err != nil {
		t.Fatalf("expire lease: %v", err)
	}
	if _, err := s.TryAcquire(ctx, "actor", "next"); err != nil {
		t.Errorf("acquire after expiry: %v, want the slot reclaimed", err)
	}
}

func TestAcquire_SkipsExhaustedToken(t *testing.T) {
	db := testDB(t)
	s := newStore(t, db, []string{"tok-a", "tok-b"}, 5)
	ctx := context.Background()

	if err := s.MarkExhausted(ctx, 0, time.Hour); err != nil {
		t.Fatalf("MarkExhausted: %v", err)
	}
	for i := 0; i < 5; i++ {
		lease, err := s.TryAcquire(ctx, "actor", "")
		if err != nil {
			t.Fatalf("acquire %d: %v", i, err)
		}
		if lease.TokenIndex != 1 {
			t.Fatalf("acquire %d landed on token_index %d, want 1 (0 is exhausted)", i, lease.TokenIndex)
		}
	}
	if _, err := s.TryAcquire(ctx, "actor", ""); !errors.Is(err, ErrNoCapacity) {
		t.Errorf("err = %v, want ErrNoCapacity once the only live token is full", err)
	}
}

// Acquire blocks rather than failing, so a job behind a full pool queues
// instead of being dead-lettered.
func TestAcquire_BlocksThenSucceedsOnRelease(t *testing.T) {
	db := testDB(t)
	s := newStore(t, db, []string{"tok-a"}, 1)
	ctx := context.Background()

	held, err := s.TryAcquire(ctx, "actor", "holder")
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}

	done := make(chan *Lease, 1)
	go func() {
		l, err := s.Acquire(ctx, "actor", "waiter")
		if err != nil {
			t.Errorf("Acquire: %v", err)
			done <- nil
			return
		}
		done <- l
	}()

	select {
	case <-done:
		t.Fatal("Acquire returned while the only slot was still held")
	case <-time.After(100 * time.Millisecond):
	}

	if err := s.Release(ctx, held); err != nil {
		t.Fatalf("release: %v", err)
	}
	select {
	case l := <-done:
		if l == nil {
			t.Fatal("waiter failed")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Acquire did not pick up the freed slot")
	}
}

func TestAcquire_RespectsContextDeadline(t *testing.T) {
	db := testDB(t)
	s := newStore(t, db, []string{"tok-a"}, 1)

	if _, err := s.TryAcquire(context.Background(), "actor", "holder"); err != nil {
		t.Fatalf("acquire: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	if _, err := s.Acquire(ctx, "actor", "waiter"); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want DeadlineExceeded", err)
	}
}

func TestSyncTokens_AddingAnAccountRaisesCapacity(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	one := newStore(t, db, []string{"tok-a"}, 5)
	for i := 0; i < 5; i++ {
		if _, err := one.TryAcquire(ctx, "actor", ""); err != nil {
			t.Fatalf("acquire %d: %v", i, err)
		}
	}
	if _, err := one.TryAcquire(ctx, "actor", ""); !errors.Is(err, ErrNoCapacity) {
		t.Fatalf("want ErrNoCapacity at 5/5")
	}

	// Appending a token to APIFY_API_TOKEN and restarting is the whole
	// scale-out story; nothing else should need to change.
	two := NewSlotStore(db, []string{"tok-a", "tok-b"}, "test", time.Minute)
	two.Logf = nil
	if err := two.SyncTokens(ctx, 5); err != nil {
		t.Fatalf("SyncTokens: %v", err)
	}
	lease, err := two.TryAcquire(ctx, "actor", "")
	if err != nil {
		t.Fatalf("acquire after adding a token: %v", err)
	}
	if lease.TokenIndex != 1 {
		t.Errorf("landed on token_index %d, want 1", lease.TokenIndex)
	}
}

// The justification for putting this in Postgres instead of a mutex: each
// service is its own process with its own SlotStore, and the cap has to hold
// across all of them. A per-process semaphore passes every other test in this
// file and still lets four services run 4x the limit in production.
func TestAcquire_CapHoldsAcrossSeparateStores(t *testing.T) {
	db := testDB(t)
	tokens := []string{"tok-a", "tok-b"}

	// Four stores standing in for fb-ads, amazon, china1688 and reddit --
	// separate instances, sharing nothing but the database.
	services := []string{"fb-ads", "amazon", "china1688", "reddit"}
	stores := make([]*SlotStore, len(services))
	for i, name := range services {
		stores[i] = newStore(t, db, tokens, 5)
		stores[i].service = name
	}

	var granted atomic.Int64
	var wg sync.WaitGroup
	start := make(chan struct{})
	for _, s := range stores {
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(s *SlotStore) {
				defer wg.Done()
				<-start
				if _, err := s.TryAcquire(context.Background(), "actor", ""); err == nil {
					granted.Add(1)
				} else if !errors.Is(err, ErrNoCapacity) {
					t.Errorf("TryAcquire: %v", err)
				}
			}(s)
		}
	}
	close(start)
	wg.Wait()

	if got := granted.Load(); got != 10 {
		t.Errorf("granted = %d across 4 services, want 10 -- the cap is global, not per-process", got)
	}
	for idx := range tokens {
		if n := liveLeases(t, db, idx); n != 5 {
			t.Errorf("token %d holds %d leases, want 5", idx, n)
		}
	}
}
