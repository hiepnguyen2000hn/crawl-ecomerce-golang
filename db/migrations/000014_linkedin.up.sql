-- Add 'linkedin' job type while preserving all existing types.
ALTER TABLE jobs DROP CONSTRAINT IF EXISTS jobs_type_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_type_check CHECK (
    type IN ('fbads', 'trend', 'amazon', 'china1688', 'reddit', 'reddit_api', 'allegro', 'linkedin')
);

CREATE TABLE IF NOT EXISTS linkedin_posts_raw (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id UUID NOT NULL REFERENCES jobs(id),
    post_id TEXT NOT NULL,
    url TEXT,
    content TEXT,
    author_name TEXT,
    author_url TEXT,
    posted_at TIMESTAMPTZ,
    likes INTEGER,
    comments_count INTEGER,
    shares INTEGER,
    raw JSONB NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS linkedin_posts_raw_job_id_idx ON linkedin_posts_raw (job_id);
