package apify

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"
)

// Registry maintains what the slot pool knows about each Apify account:
// its secret, its budget, when its billing cycle rolls over, and whether it
// is currently worth handing out.
//
// It is deliberately NOT on the crawl path. Services lease slots straight
// from Postgres; this only writes the state those leases are chosen by. If
// the registry stops running, crawling continues on data that goes stale --
// an account that just ran dry is noticed by the inline failure path instead
// of in advance, which is the behaviour from before this existed.
type Registry struct {
	db     *sql.DB
	limits *LimitsClient

	// ParkAbove is the usage fraction at which an account is parked without
	// waiting for it to fail. 0.95 keeps a margin for Apify settling
	// pay-per-result charges after the fact.
	ParkAbove float64

	Logf func(format string, args ...any)
}

func NewRegistry(db *sql.DB, limits *LimitsClient) *Registry {
	if limits == nil {
		limits = NewLimitsClient("")
	}
	return &Registry{db: db, limits: limits, ParkAbove: 0.95, Logf: log.Printf}
}

// Seed writes tokens from a config value into the registry, for first boot
// and for adding accounts. Existing rows keep their measured state: re-seeding
// must not wipe the usage figures the refresher collected.
func (r *Registry) Seed(ctx context.Context, tokens []string, defaultMaxConcurrency int) error {
	if len(tokens) == 0 {
		return errors.New("apify: no tokens to seed")
	}
	if defaultMaxConcurrency <= 0 {
		defaultMaxConcurrency = 5
	}
	for i, secret := range tokens {
		if _, err := r.db.ExecContext(ctx,
			`INSERT INTO apify_tokens (token_index, label, max_concurrency, secret)
			 VALUES ($1, $2, $3, $4)
			 ON CONFLICT (token_index) DO UPDATE SET secret = EXCLUDED.secret`,
			i, fmt.Sprintf("acct-%d", i+1), defaultMaxConcurrency, secret,
		); err != nil {
			return fmt.Errorf("apify: seed token %d: %w", i, err)
		}
	}
	return nil
}

// TokenState is one account as the registry sees it.
type TokenState struct {
	TokenIndex     int16
	Label          string
	MaxConcurrency int
	UsageUSD       *float64
	UsageLimitUSD  *float64
	CycleEndsAt    *time.Time
	ActiveJobs     *int
	CheckedAt      *time.Time
	ExhaustedUntil *time.Time
	LastError      *string
	LiveLeases     int
}

// Parked reports whether the pool is currently skipping this account.
func (s TokenState) Parked() bool {
	return s.ExhaustedUntil != nil && s.ExhaustedUntil.After(time.Now())
}

// RefreshOne asks Apify about a single account and records the answer.
//
// A failure here is recorded rather than returned as fatal: one unreachable
// account must not stop the sweep over the other 999.
func (r *Registry) RefreshOne(ctx context.Context, tokenIndex int16, secret string) (AccountLimits, error) {
	lim, err := r.limits.Fetch(ctx, secret)
	if err != nil {
		if _, dbErr := r.db.ExecContext(ctx,
			`UPDATE apify_tokens SET checked_at = now(), last_error = $2 WHERE token_index = $1`,
			tokenIndex, err.Error(),
		); dbErr != nil {
			return AccountLimits{}, fmt.Errorf("apify: record refresh failure for token %d: %w", tokenIndex, dbErr)
		}
		return AccountLimits{}, err
	}

	// max_concurrency comes from Apify rather than config: accounts on
	// different plans have different caps, and hardcoding 5 silently wastes
	// a paid account's headroom.
	if _, err := r.db.ExecContext(ctx,
		`UPDATE apify_tokens
		    SET usage_usd = $2, usage_limit_usd = $3, cycle_ends_at = $4,
		        active_jobs = $5, max_concurrency = $6,
		        checked_at = now(), last_error = NULL
		  WHERE token_index = $1`,
		tokenIndex, lim.MonthlyUsageUSD, lim.MaxMonthlyUsageUS, lim.CycleEndsAt,
		lim.ActiveJobCount, lim.MaxConcurrentJobs,
	); err != nil {
		return lim, fmt.Errorf("apify: record limits for token %d: %w", tokenIndex, err)
	}
	return lim, nil
}

// ApplyPolicy parks accounts that are nearly spent and releases ones whose
// billing cycle has rolled over.
//
// Parking ahead of the wall is the point: without it an account is only found
// to be spent by a job failing on it, and that job is the cost of the
// discovery.
func (r *Registry) ApplyPolicy(ctx context.Context) (parked, released int, err error) {
	// Park until the cycle actually ends. Apify's cycle is a rolling 30 days
	// from the account's own start date, so this is typically days or weeks
	// away -- the fixed short park it replaces would retry a spent account
	// hundreds of times before it could possibly recover.
	res, err := r.db.ExecContext(ctx,
		`UPDATE apify_tokens
		    SET exhausted_until = cycle_ends_at
		  WHERE usage_limit_usd > 0
		    AND usage_usd >= usage_limit_usd * $1
		    AND cycle_ends_at > now()
		    AND (exhausted_until IS NULL OR exhausted_until < cycle_ends_at)`,
		r.ParkAbove)
	if err != nil {
		return 0, 0, fmt.Errorf("apify: park spent accounts: %w", err)
	}
	n, _ := res.RowsAffected()
	parked = int(n)

	// An account whose park has expired is usable again. Its usage figure is
	// stale until the next refresh, so clear it too rather than let the park
	// rule immediately re-fire on last cycle's number.
	res, err = r.db.ExecContext(ctx,
		`UPDATE apify_tokens
		    SET exhausted_until = NULL, usage_usd = NULL, checked_at = NULL
		  WHERE exhausted_until IS NOT NULL AND exhausted_until <= now()`)
	if err != nil {
		return parked, 0, fmt.Errorf("apify: release recovered accounts: %w", err)
	}
	n, _ = res.RowsAffected()
	released = int(n)

	if r.Logf != nil && (parked > 0 || released > 0) {
		r.Logf("apify registry: parked %d spent account(s), released %d recovered", parked, released)
	}
	return parked, released, nil
}

// StaleTokens returns the accounts most overdue for a refresh, never-checked
// ones first, with their secrets.
func (r *Registry) StaleTokens(ctx context.Context, olderThan time.Duration, limit int) ([]TokenState, []string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT token_index, label, secret
		   FROM apify_tokens
		  WHERE secret IS NOT NULL
		    AND (checked_at IS NULL OR checked_at < now() - make_interval(secs => $1))
		  ORDER BY checked_at NULLS FIRST
		  LIMIT $2`,
		olderThan.Seconds(), limit)
	if err != nil {
		return nil, nil, fmt.Errorf("apify: query stale tokens: %w", err)
	}
	defer rows.Close()

	var states []TokenState
	var secrets []string
	for rows.Next() {
		var s TokenState
		var secret string
		if err := rows.Scan(&s.TokenIndex, &s.Label, &secret); err != nil {
			return nil, nil, fmt.Errorf("apify: scan stale token: %w", err)
		}
		states = append(states, s)
		secrets = append(secrets, secret)
	}
	return states, secrets, rows.Err()
}

// Summary is the operational view: enough to answer "how much crawling can we
// still do, and when does capacity come back".
type Summary struct {
	Accounts      int        `json:"accounts"`
	Usable        int        `json:"usable"`
	Parked        int        `json:"parked"`
	NeverChecked  int        `json:"never_checked"`
	SlotsTotal    int        `json:"slots_total"`
	SlotsInUse    int        `json:"slots_in_use"`
	BudgetLeftUSD float64    `json:"budget_left_usd"`
	NextResetAt   *time.Time `json:"next_reset_at"`
	ResetsNext24h int        `json:"resets_next_24h"`
}

func (r *Registry) Summary(ctx context.Context) (Summary, error) {
	var s Summary
	err := r.db.QueryRowContext(ctx, `
		SELECT
		  count(*),
		  count(*) FILTER (WHERE exhausted_until IS NULL OR exhausted_until <= now()),
		  count(*) FILTER (WHERE exhausted_until > now()),
		  count(*) FILTER (WHERE checked_at IS NULL),
		  COALESCE(sum(max_concurrency) FILTER (WHERE exhausted_until IS NULL OR exhausted_until <= now()), 0),
		  COALESCE(sum(GREATEST(usage_limit_usd - usage_usd, 0)), 0),
		  min(cycle_ends_at) FILTER (WHERE exhausted_until > now()),
		  count(*) FILTER (WHERE exhausted_until > now() AND exhausted_until < now() + interval '24 hours')
		FROM apify_tokens`).
		Scan(&s.Accounts, &s.Usable, &s.Parked, &s.NeverChecked,
			&s.SlotsTotal, &s.BudgetLeftUSD, &s.NextResetAt, &s.ResetsNext24h)
	if err != nil {
		return s, fmt.Errorf("apify: summary: %w", err)
	}
	if err := r.db.QueryRowContext(ctx,
		`SELECT count(*) FROM apify_leases WHERE expires_at > now()`).Scan(&s.SlotsInUse); err != nil {
		return s, fmt.Errorf("apify: summary leases: %w", err)
	}
	return s, nil
}
