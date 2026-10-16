DROP INDEX IF EXISTS apify_tokens_checked_at_idx;

ALTER TABLE apify_tokens
    DROP COLUMN IF EXISTS secret,
    DROP COLUMN IF EXISTS usage_usd,
    DROP COLUMN IF EXISTS usage_limit_usd,
    DROP COLUMN IF EXISTS cycle_ends_at,
    DROP COLUMN IF EXISTS active_jobs,
    DROP COLUMN IF EXISTS checked_at,
    DROP COLUMN IF EXISTS last_error;
