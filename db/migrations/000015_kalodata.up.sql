-- Add 'kalodata' job type while preserving all existing types.
ALTER TABLE jobs DROP CONSTRAINT IF EXISTS jobs_type_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_type_check CHECK (
    type IN ('fbads', 'trend', 'amazon', 'china1688', 'reddit', 'reddit_api', 'allegro', 'linkedin', 'kalodata')
);

-- Account/cookie registry: 7-day TTL (hard death) + per-day quota (10 search /
-- 10 detail, reset daily). Each account carries a built Cookie header
-- (SESSION=...; deviceId=...), its own device_id and residential proxy. Daily
-- quota exhaustion rotates to another account; only TTL expiry or auth rejection
-- sets dead = true.
CREATE TABLE IF NOT EXISTS kalodata_cookies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    cookie_hash TEXT UNIQUE NOT NULL,
    cookie TEXT NOT NULL,
    device_id TEXT,
    proxy TEXT,
    label TEXT,
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    dead BOOLEAN NOT NULL DEFAULT false,
    usage_date DATE,
    search_used INTEGER NOT NULL DEFAULT 0,
    detail_used INTEGER NOT NULL DEFAULT 0,
    last_used_at TIMESTAMPTZ
);

-- Kalodata returns revenue/price/commission as formatted display strings
-- ("฿2.57m", "฿18.77k", "1%"); gmv is the numeric GMV. Columns match the actual
-- API field types (verified live); raw keeps the full object.
CREATE TABLE IF NOT EXISTS kalodata_products (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id UUID NOT NULL REFERENCES jobs(id),
    country TEXT,
    product_id TEXT,
    title TEXT,
    revenue TEXT,
    gmv NUMERIC,
    sales INTEGER,
    rating NUMERIC,
    unit_price TEXT,
    commission_rate TEXT,
    thumbnail_url TEXT,
    product_url TEXT,
    raw JSONB NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS kalodata_products_job_id_idx ON kalodata_products (job_id);

CREATE TABLE IF NOT EXISTS kalodata_videos (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id UUID NOT NULL REFERENCES jobs(id),
    country TEXT,
    video_id TEXT,
    handle TEXT,
    title TEXT,
    views TEXT,
    revenue TEXT,
    sales INTEGER,
    duration TEXT,
    thumbnail_url TEXT,
    tiktok_url TEXT,
    detail JSONB,
    raw JSONB NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS kalodata_videos_job_id_idx ON kalodata_videos (job_id);
