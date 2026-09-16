-- db/migrations/000003_products.up.sql
CREATE TABLE products (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id UUID NOT NULL REFERENCES jobs(id),
    ad_id UUID REFERENCES fbads_raw(id),
    url TEXT NOT NULL,
    product_name TEXT,
    price NUMERIC,
    currency TEXT,
    sku TEXT,
    raw JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
