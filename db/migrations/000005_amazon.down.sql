DROP TABLE amazon_raw;

ALTER TABLE jobs DROP CONSTRAINT jobs_type_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_type_check CHECK (type IN ('fbads', 'trend'));
