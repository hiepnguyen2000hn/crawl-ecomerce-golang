package apify

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"
)

// ErrNoCapacity means every token is at its concurrency limit or exhausted.
// Acquire retries on it internally; it only reaches callers from TryAcquire.
var ErrNoCapacity = errors.New("apify: no free concurrency slot")

// advisoryKey serializes slot acquisition across every process. Capacity is a
// read-then-insert, and without serialization two workers both reading "4 of 5
// used" would both insert and put 6 runs on an account that allows 5.
//
// A global lock is affordable here precisely because the critical section is
// tiny (one delete, one insert, sub-millisecond) while the work it guards --
// an Apify run -- lasts minutes. Contention is on the order of one lock
// acquisition per run, not per request.
const advisoryKey int64 = 0x4150494659534C54 // "APIFYSLT"

// Lease is one in-flight Apify run's claim on a token's concurrency budget.
// It must be released, including on the error path.
type Lease struct {
	ID         int64
	TokenID    int16
	TokenIndex int16
	// Token is the API token this run must use. The slot was counted against
	// this specific account, so using a different one would silently
	// over-subscribe that account.
	Token string
}

// SlotStore hands out per-token concurrency slots backed by Postgres.
//
// Every Apify-calling service is a separate process sharing one token pool,
// so this accounting cannot live in process memory -- four services each
// allowing 5 concurrent runs would put 20 on a 5-run account.
type SlotStore struct {
	db      *sql.DB
	tokens  []string
	service string
	ttl     time.Duration
	poll    time.Duration

	// Logf reports waits and exhaustion. Nil disables logging.
	Logf func(format string, args ...any)
}

// NewSlotStore builds a store over the tokens parsed from APIFY_API_TOKEN.
// ttl bounds how long a lease survives a crashed worker; set it above the
// actor run timeout so a slow-but-alive run never loses its slot.
func NewSlotStore(db *sql.DB, tokens []string, service string, ttl time.Duration) *SlotStore {
	if ttl <= 0 {
		ttl = 6 * time.Minute
	}
	return &SlotStore{
		db:      db,
		tokens:  tokens,
		service: service,
		ttl:     ttl,
		poll:    2 * time.Second,
		Logf:    log.Printf,
	}
}

// SyncTokens registers any configured token that is not in apify_tokens yet,
// so adding an account is just appending to APIFY_API_TOKEN and restarting.
//
// Existing rows are left untouched. max_concurrency in particular is read
// from Apify per account by the token service -- updating it from config here
// would reset a paid account's real limit back to the free-plan guess every
// time a service restarts.
//
// Rows for tokens that disappeared are also left in place: their leases may
// still be live, and deleting would cascade those away and under-count a
// still-running account.
func (s *SlotStore) SyncTokens(ctx context.Context, maxConcurrency int) error {
	if len(s.tokens) == 0 {
		return errors.New("apify: no tokens configured")
	}
	if maxConcurrency <= 0 {
		maxConcurrency = 5
	}
	for i := range s.tokens {
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO apify_tokens (token_index, label, max_concurrency)
			 VALUES ($1, $2, $3)
			 ON CONFLICT (token_index) DO NOTHING`,
			i, fmt.Sprintf("acct-%d", i+1), maxConcurrency,
		); err != nil {
			return fmt.Errorf("apify: sync token %d: %w", i, err)
		}
	}
	return nil
}

const acquireSQL = `
WITH live AS (
    SELECT token_id, count(*) AS n
      FROM apify_leases
     GROUP BY token_id
),
-- Ordering, in priority: fewest runs in flight, then least budget spent, then
-- at random. The last two matter more than they look. With 10 jobs against
-- 1000 idle accounts every account ties on lease count, so a deterministic
-- tie-break hands every job to the same lowest-id accounts -- measured: 60 of
-- 1000 accounts touched, indexes 0..59, leaving 94% of the budget unreachable
-- until the first ten burn out.
pick AS (
    SELECT t.id, t.token_index, t.secret
      FROM apify_tokens t
      LEFT JOIN live l ON l.token_id = t.id
     WHERE (t.exhausted_until IS NULL OR t.exhausted_until <= now())
       AND COALESCE(l.n, 0) < t.max_concurrency
       AND ($4::int IS NULL OR t.token_index < $4)
     ORDER BY COALESCE(l.n, 0) ASC, t.usage_usd ASC NULLS FIRST, random()
     LIMIT 1
)
INSERT INTO apify_leases (token_id, actor_id, job_id, service, expires_at)
SELECT id, $1, NULLIF($2, ''), $3, now() + make_interval(secs => $5)
  FROM pick
RETURNING id, token_id,
          (SELECT token_index FROM pick),
          (SELECT secret FROM pick)`

// tokenCeiling bounds which accounts this process may use.
//
// When tokens come from the env var, a row with a higher index than that list
// has no secret this process can resolve and must be skipped. When the
// registry owns the secrets the process holds no list, so NULL lifts the
// bound entirely and every account is fair game.
func (s *SlotStore) tokenCeiling() sql.NullInt32 {
	if len(s.tokens) == 0 {
		return sql.NullInt32{}
	}
	return sql.NullInt32{Int32: int32(len(s.tokens)), Valid: true}
}

// TryAcquire takes a slot if one is free right now, else returns ErrNoCapacity.
func (s *SlotStore) TryAcquire(ctx context.Context, actorID, jobID string) (*Lease, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("apify: begin acquire: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, advisoryKey); err != nil {
		return nil, fmt.Errorf("apify: lock slots: %w", err)
	}

	// Reap inside the lock so a crashed worker's slot is reclaimed by whoever
	// needs it next, with no sweeper process to deploy or monitor.
	if _, err := tx.ExecContext(ctx, `DELETE FROM apify_leases WHERE expires_at <= now()`); err != nil {
		return nil, fmt.Errorf("apify: reap expired leases: %w", err)
	}

	var lease Lease
	var dbSecret sql.NullString
	err = tx.QueryRowContext(ctx, acquireSQL,
		actorID, jobID, s.service, s.tokenCeiling(), s.ttl.Seconds(),
	).Scan(&lease.ID, &lease.TokenID, &lease.TokenIndex, &dbSecret)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoCapacity
	}
	if err != nil {
		return nil, fmt.Errorf("apify: acquire slot: %w", err)
	}

	// The registry owns the secret. The env-seeded list is only a fallback
	// for rows written before the registry existed.
	switch {
	case dbSecret.Valid && dbSecret.String != "":
		lease.Token = dbSecret.String
	case int(lease.TokenIndex) < len(s.tokens):
		lease.Token = s.tokens[lease.TokenIndex]
	default:
		return nil, fmt.Errorf("apify: token_index %d has no secret in apify_tokens and no env fallback", lease.TokenIndex)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("apify: commit acquire: %w", err)
	}
	return &lease, nil
}

// Acquire blocks until a slot frees up or ctx ends.
//
// Callers must give it a context whose deadline covers queueing, not just the
// run -- a job waiting behind a full pool is healthy backpressure, not a
// failure, and should not be dead-lettered.
func (s *SlotStore) Acquire(ctx context.Context, actorID, jobID string) (*Lease, error) {
	waited := false
	for {
		lease, err := s.TryAcquire(ctx, actorID, jobID)
		if err == nil {
			if waited && s.Logf != nil {
				s.Logf("apify: got a slot on token %d after waiting", lease.TokenIndex+1)
			}
			return lease, nil
		}
		if !errors.Is(err, ErrNoCapacity) {
			return nil, err
		}

		if !waited && s.Logf != nil {
			s.Logf("apify: all %d token(s) at capacity, waiting for a slot", len(s.tokens))
			waited = true
		}

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("apify: waiting for a slot: %w", ctx.Err())
		case <-time.After(s.poll):
		}
	}
}

// Release frees the slot. Pass a context that is still live even when the run
// failed -- context.WithoutCancel on the job's context is the usual call --
// since a lease dropped on the error path stays booked until its TTL.
func (s *SlotStore) Release(ctx context.Context, l *Lease) error {
	if l == nil {
		return nil
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM apify_leases WHERE id = $1`, l.ID); err != nil {
		return fmt.Errorf("apify: release slot %d: %w", l.ID, err)
	}
	return nil
}

// MarkExhausted parks a token that reported out-of-credits so the other
// services stop picking it, instead of each burning a request to find out.
//
// Only for genuinely non-transient statuses (402/403). A 429 means "too many
// right now" and must not land here -- parking on 429 would retire a healthy
// account the moment it got busy.
func (s *SlotStore) MarkExhausted(ctx context.Context, tokenIndex int16, d time.Duration) error {
	if _, err := s.db.ExecContext(ctx,
		`UPDATE apify_tokens SET exhausted_until = now() + make_interval(secs => $2) WHERE token_index = $1`,
		tokenIndex, d.Seconds(),
	); err != nil {
		return fmt.Errorf("apify: mark token %d exhausted: %w", tokenIndex, err)
	}
	if s.Logf != nil {
		s.Logf("apify: token %d parked as exhausted for %s", tokenIndex+1, d)
	}
	return nil
}
