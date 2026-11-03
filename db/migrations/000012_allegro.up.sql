-- Replace jobs_type_check constraint to include 'allegro' while preserving all existing job types
ALTER TABLE jobs DROP CONSTRAINT IF EXISTS jobs_type_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_type_check CHECK (
    type IN ('fbads', 'trend', 'amazon', 'china1688', 'reddit', 'reddit_api', 'allegro')
);

CREATE TABLE IF NOT EXISTS allegro_raw (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id UUID NOT NULL REFERENCES jobs(id),
    offer_id TEXT,
    product_id TEXT,
    title TEXT,
    brand TEXT,
    model TEXT,
    condition TEXT,
    seller TEXT,
    price NUMERIC,
    price_with_delivery NUMERIC,
    currency TEXT,
    rating NUMERIC,
    reviews_count INTEGER,
    bought_recently INTEGER,
    super_seller BOOLEAN,
    smart BOOLEAN,
    delivery TEXT,
    sponsored BOOLEAN,
    position INTEGER,
    page INTEGER,
    image_url TEXT,
    product_url TEXT,
    specs JSONB,
    raw JSONB NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS allegro_raw_offer_id_idx ON allegro_raw (offer_id);
CREATE INDEX IF NOT EXISTS allegro_raw_job_id_idx ON allegro_raw (job_id);
CREATE INDEX IF NOT EXISTS allegro_raw_brand_idx ON allegro_raw (brand);
