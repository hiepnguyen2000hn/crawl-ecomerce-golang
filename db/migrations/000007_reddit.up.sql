ALTER TABLE jobs DROP CONSTRAINT jobs_type_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_type_check CHECK (type IN ('fbads', 'trend', 'amazon', 'china1688', 'reddit'));

CREATE TABLE reddit_posts_raw (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id UUID NOT NULL REFERENCES jobs(id),
    post_id TEXT NOT NULL,
    title TEXT,
    url TEXT,
    community_name TEXT,
    upvotes INTEGER,
    num_comments INTEGER,
    created_at_reddit TIMESTAMPTZ,
    raw JSONB NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE reddit_comments_raw (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id UUID NOT NULL REFERENCES jobs(id),
    post_id TEXT NOT NULL,
    comment_id TEXT NOT NULL,
    username TEXT,
    body TEXT,
    upvotes INTEGER,
    created_at_reddit TIMESTAMPTZ,
    raw JSONB NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_reddit_comments_raw_job_post ON reddit_comments_raw (job_id, post_id);
