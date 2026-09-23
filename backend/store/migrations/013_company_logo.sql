-- 013_company_logo: store the uploaded logo artwork on the singleton row.
-- A ~100KB image in one row is nothing; object storage for a single image
-- would be over-engineering. Empty bytes mean "use the baked asset".
ALTER TABLE {{schema}}.company_profile
    ADD COLUMN IF NOT EXISTS logo_bytes BYTEA;

ALTER TABLE {{schema}}.company_profile
    ADD COLUMN IF NOT EXISTS logo_mime TEXT NOT NULL DEFAULT '';
