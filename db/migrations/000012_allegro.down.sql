DROP TABLE IF EXISTS allegro_raw;

ALTER TABLE jobs DROP CONSTRAINT IF EXISTS jobs_type_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_type_check CHECK (
    type IN ('fbads', 'trend', 'amazon', 'china1688', 'reddit', 'reddit_api')
);
