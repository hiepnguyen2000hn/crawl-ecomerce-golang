ALTER TABLE jobs DROP CONSTRAINT jobs_type_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_type_check CHECK (type IN ('fbads', 'trend', 'amazon'));

CREATE TABLE amazon_raw (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id UUID NOT NULL REFERENCES jobs(id),
    asin TEXT NOT NULL,
    title TEXT,
    price NUMERIC,
    currency TEXT,
    rating NUMERIC,
    reviews_count INTEGER,
    brand TEXT,
    image_url TEXT,
    product_url TEXT,
    raw JSONB NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
