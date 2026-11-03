-- Local copies of the (expiring) fbcdn images an ad points at. One row per
-- (ad, position in the snapshot); fbads_raw.raw keeps the original URLs.
CREATE TABLE fbads_media (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ad_archive_id TEXT NOT NULL,
    json_path TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('image', 'video_cover')),
    source_url TEXT NOT NULL,
    expires_at TIMESTAMPTZ,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'done', 'failed', 'expired')),
    storage_key TEXT,
    content_type TEXT,
    size_bytes BIGINT,
    attempts INT NOT NULL DEFAULT 0,
    error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (ad_archive_id, json_path)
);

CREATE INDEX fbads_media_pending_idx ON fbads_media (ad_archive_id) WHERE status = 'pending';

-- PendingForJob looks ads up by job.
CREATE INDEX IF NOT EXISTS fbads_raw_job_id_idx ON fbads_raw (job_id);
