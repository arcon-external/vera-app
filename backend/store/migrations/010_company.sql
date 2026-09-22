-- 010_company: singleton company profile for the admin-only Company
-- section of Settings. One row (id = 1), seeded with the current
-- hard-coded identity. logo_path is a stub: logo bytes are object-storage
-- work (same class as PDF blobs) and wire up in a follow-up.
CREATE TABLE IF NOT EXISTS {{schema}}.company_profile (
    id SMALLINT PRIMARY KEY CHECK (id = 1),
    legal_name TEXT NOT NULL DEFAULT '',
    address TEXT NOT NULL DEFAULT '',
    phone TEXT NOT NULL DEFAULT '',
    email TEXT NOT NULL DEFAULT '',
    tax_id TEXT NOT NULL DEFAULT '',
    logo_path TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO {{schema}}.company_profile (id, legal_name)
VALUES (1, 'PT Arcon Penta Persada')
ON CONFLICT (id) DO NOTHING;
