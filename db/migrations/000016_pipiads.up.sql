-- Add 'pipiads' job type while preserving all existing types.
ALTER TABLE jobs DROP CONSTRAINT IF EXISTS jobs_type_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_type_check CHECK (
    type IN ('fbads', 'trend', 'amazon', 'china1688', 'reddit', 'reddit_api', 'allegro', 'linkedin', 'kalodata', 'pipiads')
);

-- Account registry for PiPiAds: tracks session tokens, device_id, proxy, and credits quota.
-- Each trial account starts with 500 extra ad credits (total: 500).
-- When credits are exhausted or auth fails, dead=true or rotated.
CREATE TABLE IF NOT EXISTS pipiads_accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email TEXT UNIQUE NOT NULL,
    password TEXT NOT NULL,
    access_token TEXT,
    device_id TEXT,
    proxy TEXT,
    total_credits INTEGER NOT NULL DEFAULT 500,
    used_credits INTEGER NOT NULL DEFAULT 0,
    dead BOOLEAN NOT NULL DEFAULT false,
    last_used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- PiPiAds E-commerce Ads search results
CREATE TABLE IF NOT EXISTS pipiads_ads (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id UUID NOT NULL REFERENCES jobs(id),
    video_id TEXT,
    ad_id TEXT,
    keyword TEXT,
    title TEXT,
    description TEXT,
    app_name TEXT,
    advertiser_name TEXT,
    platform INTEGER,
    play_count BIGINT DEFAULT 0,
    digg_count BIGINT DEFAULT 0,
    comment_count BIGINT DEFAULT 0,
    share_count BIGINT DEFAULT 0,
    cover_url TEXT,
    video_url TEXT,
    landing_page_url TEXT,
    country TEXT,
    first_seen_at BIGINT,
    last_seen_at BIGINT,
    raw JSONB NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS pipiads_ads_job_id_idx ON pipiads_ads (job_id);
CREATE INDEX IF NOT EXISTS pipiads_ads_keyword_idx ON pipiads_ads (keyword);
CREATE INDEX IF NOT EXISTS pipiads_ads_video_id_idx ON pipiads_ads (video_id);
