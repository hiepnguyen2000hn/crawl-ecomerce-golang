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
	"encoding/json"
	"errors"
	"fmt"
	"os"
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

// Account is one Kalodata trial account as stored in kalodata_full_info.json:
// its session token, device id and residential proxy. The Cookie header sent to
// Kalodata is "SESSION=<session_cookie>; deviceId=<device_id>" (verified live).
type Account struct {
	SessionCookie string `json:"session_cookie"`
	DeviceID      string `json:"device_id"`
	Proxy         string `json:"proxy"`
}

func (a Account) cookieHeader() string {
	h := "SESSION=" + a.SessionCookie
	if a.DeviceID != "" {
		h += "; deviceId=" + a.DeviceID
	}
	return h
}

// LoadAccounts reads kalodata_full_info.json (array of accounts) and returns the
// ones with a session cookie.
func LoadAccounts(path string) ([]Account, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var accs []Account
	if err := json.Unmarshal(data, &accs); err != nil {
		return nil, fmt.Errorf("kalodata: parse accounts file: %w", err)
	}
	out := make([]Account, 0, len(accs))
	for _, a := range accs {
		if a.SessionCookie != "" {
			out = append(out, a)
		}
	}
	return out, nil
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

// SyncCookies registers env cookies. A new cookie (new hash) starts its 7-day
// clock now. A re-synced cookie (same value → same hash) is left untouched: its
// first_seen_at/expires_at (TTL) are preserved across restarts, and crucially a
// cookie already MarkDead-ed by an auth rejection stays dead rather than being
// revived every restart. To revive an account, inject a *fresh* cookie value
// (new hash → new live row).
func (r *CookieRegistry) SyncCookies(ctx context.Context, cookies []string) error {
	for _, c := range cookies {
		if _, err := r.db.ExecContext(ctx,
			`INSERT INTO kalodata_cookies (cookie_hash, cookie, expires_at)
			 VALUES ($1, $2, now() + make_interval(secs => $3))
			 ON CONFLICT (cookie_hash) DO NOTHING`,
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
	Proxy  string
}

// SyncAccounts registers accounts from kalodata_full_info.json. Identity is the
// session token hash, so a re-login (new session) is a new live row with a fresh
// 7-day clock; a re-synced session keeps its clock and dead flag (see note on
// SyncCookies).
func (r *CookieRegistry) SyncAccounts(ctx context.Context, accounts []Account) error {
	for _, a := range accounts {
		if _, err := r.db.ExecContext(ctx,
			`INSERT INTO kalodata_cookies (cookie_hash, cookie, device_id, proxy, expires_at)
			 VALUES ($1, $2, $3, $4, now() + make_interval(secs => $5))
			 ON CONFLICT (cookie_hash) DO NOTHING`,
			CookieHash(a.SessionCookie), a.cookieHeader(), a.DeviceID, a.Proxy, r.ttl.Seconds(),
		); err != nil {
			return fmt.Errorf("kalodata: sync account: %w", err)
		}
	}
	return nil
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
	var proxy sql.NullString
	err = tx.QueryRowContext(ctx,
		fmt.Sprintf(`UPDATE kalodata_cookies SET %[1]s = %[1]s + 1, last_used_at = now()
		 WHERE id = (
		   SELECT id FROM kalodata_cookies
		   WHERE dead = false AND expires_at > now() AND %[1]s < $1
		   ORDER BY random() LIMIT 1
		   FOR UPDATE SKIP LOCKED
		 )
		 RETURNING id, cookie, proxy`, col),
		quota,
	).Scan(&l.ID, &l.Cookie, &proxy)
	l.Proxy = proxy.String
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
