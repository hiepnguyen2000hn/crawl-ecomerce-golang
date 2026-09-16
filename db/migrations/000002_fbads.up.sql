CREATE TABLE fbads_raw (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id UUID NOT NULL REFERENCES jobs(id),
    ad_archive_id TEXT NOT NULL,
    page_id TEXT,
    page_name TEXT,
    is_active BOOLEAN,
    start_date TIMESTAMPTZ,
    end_date TIMESTAMPTZ,
    body TEXT,
    title TEXT,
    cta_text TEXT,
    cta_type TEXT,
    link_url TEXT,
    raw JSONB NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
