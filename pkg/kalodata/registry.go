// Package kalodata talks to the Kalodata REST API using session cookies from
// trial accounts, and manages those cookies in Postgres: each cookie lives 7
// days from first injection (hard death), and carries a per-day quota (search /
// detail). Daily quota exhaustion rotates to another cookie; it never kills one.
package kalodata

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	// ErrNoCookies means every cookie is dead or expired (7-day TTL). The job
	// cannot proceed until fresh cookies are injected.
	ErrNoCookies = errors.New("kalodata: no live cookies in pool")
	// ErrQuotaExhausted means live cookies exist but all are out of today's
	// quota for the requested op. The job should requeue and retry after the
	// daily reset.
	ErrQuotaExhausted = errors.New("kalodata: all cookies out of daily quota")
)

// ParseCookies splits the KALODATA_COOKIES env (one account's full Cookie header
// per line) into cookie strings, trimming blanks.
func ParseCookies(env string) []string {
	var out []string
	for _, line := range strings.Split(env, "\n") {
		if s := strings.TrimSpace(line); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// CookieHash is the stable identity of a cookie string.
func CookieHash(cookie string) string {
	sum := sha256.Sum256([]byte(cookie))
	return hex.EncodeToString(sum[:])
}

type CookieRegistry struct {
	db          *sql.DB
	ttl         time.Duration
	searchQuota int
	detailQuota int
}

func NewCookieRegistry(db *sql.DB, ttl time.Duration, searchQuota, detailQuota int) *CookieRegistry {
	return &CookieRegistry{db: db, ttl: ttl, searchQuota: searchQuota, detailQuota: detailQuota}
}

// SyncCookies upserts the env cookies. A new cookie starts its 7-day clock now;
// a re-synced cookie keeps its first_seen_at/expires_at (restarting the service
// must not reset the TTL) and is merely un-deadened in case it was retired.
func (r *CookieRegistry) SyncCookies(ctx context.Context, cookies []string) error {
	for _, c := range cookies {
		if _, err := r.db.ExecContext(ctx,
			`INSERT INTO kalodata_cookies (cookie_hash, cookie, expires_at)
			 VALUES ($1, $2, now() + make_interval(secs => $3))
			 ON CONFLICT (cookie_hash) DO UPDATE SET cookie = EXCLUDED.cookie, dead = false`,
			CookieHash(c), c, r.ttl.Seconds(),
		); err != nil {
			return fmt.Errorf("kalodata: sync cookie: %w", err)
		}
	}
	return nil
}

type Lease struct {
	ID     string
	Cookie string
}

// Lease atomically picks a live, in-quota cookie for op ("search" or "detail")
// and increments its daily counter. Counters are reset first for any row whose
// usage_date is not today. Returns ErrQuotaExhausted when live cookies exist but
// are all spent for the day, ErrNoCookies when none are live.
func (r *CookieRegistry) Lease(ctx context.Context, op string) (*Lease, error) {
	col, quota := "search_used", r.searchQuota
	if op == "detail" {
		col, quota = "detail_used", r.detailQuota
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Roll over any stale day before picking.
	if _, err := tx.ExecContext(ctx,
		`UPDATE kalodata_cookies
		 SET search_used = 0, detail_used = 0, usage_date = CURRENT_DATE
		 WHERE usage_date IS DISTINCT FROM CURRENT_DATE`,
	); err != nil {
		return nil, err
	}

	var l Lease
	err = tx.QueryRowContext(ctx,
		fmt.Sprintf(`UPDATE kalodata_cookies SET %[1]s = %[1]s + 1, last_used_at = now()
		 WHERE id = (
		   SELECT id FROM kalodata_cookies
		   WHERE dead = false AND expires_at > now() AND %[1]s < $1
		   ORDER BY random() LIMIT 1
		   FOR UPDATE SKIP LOCKED
		 )
		 RETURNING id, cookie`, col),
		quota,
	).Scan(&l.ID, &l.Cookie)
	if err == sql.ErrNoRows {
		var liveCount int
		if err := tx.QueryRowContext(ctx,
			`SELECT count(*) FROM kalodata_cookies WHERE dead = false AND expires_at > now()`,
		).Scan(&liveCount); err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		if liveCount == 0 {
			return nil, ErrNoCookies
		}
		return nil, ErrQuotaExhausted
	}
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &l, nil
}

// MarkDead retires a cookie permanently (7-day expiry handled by expires_at, this
// is for cookies the server rejected early, e.g. an early logout).
func (r *CookieRegistry) MarkDead(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE kalodata_cookies SET dead = true WHERE id = $1`, id)
	return err
}
