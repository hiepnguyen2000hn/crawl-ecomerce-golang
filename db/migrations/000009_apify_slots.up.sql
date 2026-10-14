-- Global concurrency accounting for Apify runs.
--
-- Every service that calls Apify (fb-ads, amazon, china1688, reddit) is a
-- separate process sharing one APIFY_API_TOKEN pool, so a per-process
-- semaphore cannot enforce the plan's per-account concurrency cap -- four
-- processes would each independently allow the full limit. These tables move
-- that accounting somewhere all of them can see.

CREATE TABLE apify_tokens (
    id              SMALLSERIAL PRIMARY KEY,
    -- Position in the comma-separated APIFY_API_TOKEN value, 0-based. The
    -- secret itself never lands in the database; it is resolved from env at
    -- call time by this index.
    token_index     SMALLINT NOT NULL UNIQUE CHECK (token_index >= 0),
    label           TEXT NOT NULL UNIQUE,
    max_concurrency SMALLINT NOT NULL DEFAULT 5 CHECK (max_concurrency > 0),
    -- Set when the token reports out-of-credits (402/403). Unlike a 429, that
    -- is not transient, and persisting it here is what stops every service
    -- from having to rediscover it with its own wasted request.
    exhausted_until TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Holds only live leases: releasing deletes the row. Keeps the table small
-- enough that counting live leases stays an index-only scan.
CREATE TABLE apify_leases (
    id         BIGSERIAL PRIMARY KEY,
    token_id   SMALLINT NOT NULL REFERENCES apify_tokens(id) ON DELETE CASCADE,
    actor_id   TEXT NOT NULL,
    job_id     TEXT,
    service    TEXT NOT NULL,
    acquired_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- A crashed worker must not hold a slot forever. Expired leases are
    -- ignored by the capacity query and reaped lazily on the next acquire,
    -- so no sweeper process is needed.
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX apify_leases_token_expires_idx ON apify_leases (token_id, expires_at);
