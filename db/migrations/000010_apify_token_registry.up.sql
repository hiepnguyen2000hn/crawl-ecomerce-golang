-- Turn apify_tokens from a thing seeded by an env var into the registry the
-- token service maintains.
--
-- At 1000 accounts the comma-separated APIFY_API_TOKEN value is ~46KB. It
-- works, but you cannot rotate one account, cannot see which one is spent,
-- and cannot read it in a log. The secret moves here; the env var becomes a
-- seed for first boot.

ALTER TABLE apify_tokens
    -- The API token itself. Plaintext, like the .env file it replaces --
    -- anyone who can read this table can use these accounts. Restrict DB
    -- access accordingly, and treat a dump of this table as a credential
    -- leak requiring rotation of every row.
    ADD COLUMN secret TEXT,

    -- Budget, straight from GET /users/me/limits. Knowing an account is at
    -- 95% is what lets it be parked before a job dies on it, instead of
    -- after.
    ADD COLUMN usage_usd       NUMERIC(10, 4),
    ADD COLUMN usage_limit_usd NUMERIC(10, 4),

    -- Apify's billing cycle is a rolling 30 days anchored to the account's
    -- own start date, NOT the calendar month: an account can read
    -- "2026-10-06 -> 2026-11-05". This is the only correct park-until value;
    -- parking a spent account for a fixed 15 minutes retries it ~2900 times
    -- before it can possibly recover.
    ADD COLUMN cycle_ends_at TIMESTAMPTZ,

    -- Apify's own count of what is running on this account. Compared against
    -- live leases it catches drift in either direction.
    ADD COLUMN active_jobs INT,

    ADD COLUMN checked_at TIMESTAMPTZ,
    ADD COLUMN last_error TEXT;

-- Drives the refresher's work queue: least-recently-checked first, nulls
-- (never checked) ahead of everything.
CREATE INDEX apify_tokens_checked_at_idx ON apify_tokens (checked_at NULLS FIRST);
