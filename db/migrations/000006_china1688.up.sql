ALTER TABLE jobs DROP CONSTRAINT jobs_type_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_type_check CHECK (type IN ('fbads', 'trend', 'amazon', 'china1688'));

CREATE TABLE china1688_raw (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id UUID NOT NULL REFERENCES jobs(id),
    offer_id TEXT NOT NULL,
    title TEXT,
    price_min NUMERIC,
    price_max NUMERIC,
    currency TEXT,
    moq INTEGER,
    image_url TEXT,
    supplier_name TEXT,
    supplier_province TEXT,
    detail_url TEXT,
    raw JSONB NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
