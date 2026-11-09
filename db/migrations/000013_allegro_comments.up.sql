-- Product comments (opinie) scraped per Allegro product, linked to allegro_raw rows.
CREATE TABLE IF NOT EXISTS allegro_comments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    allegro_raw_id UUID NOT NULL REFERENCES allegro_raw(id) ON DELETE CASCADE,
    job_id UUID NOT NULL REFERENCES jobs(id),
    offer_id TEXT,
    product_id TEXT,
    author TEXT,
    rating NUMERIC,
    content TEXT,
    review_date TEXT,
    confirmed_purchase BOOLEAN,
    helpful_count INTEGER,
    pros TEXT,
    cons TEXT,
    position INTEGER,
    raw JSONB NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS allegro_comments_raw_id_idx ON allegro_comments (allegro_raw_id);
CREATE INDEX IF NOT EXISTS allegro_comments_job_id_idx ON allegro_comments (job_id);
