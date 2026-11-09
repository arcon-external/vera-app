-- 029_organizations: thin tenancy boundary for the PO sprint (issue #465).
-- The fleet is single-tenant today (company_profile singleton, no
-- organization columns). New PO tables carry an organization foreign key, so
-- the boundary is established here: one organizations row seeded from the
-- current company profile, and nothing else. No org_id is added to existing
-- tables, no RLS, no memberships.
CREATE TABLE IF NOT EXISTS {{schema}}.organizations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL CHECK (name <> ''),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    suspended_at TIMESTAMPTZ NULL
);

-- Seed exactly one organization. The name follows the live company profile
-- (so an upgrade keeps the customized identity), falling back to the
-- long-standing default when the profile row is absent or blank (fresh
-- database). The NOT EXISTS guard makes the seed idempotent: replaying this
-- migration never adds a second row.
INSERT INTO {{schema}}.organizations (name)
SELECT COALESCE(NULLIF(TRIM(p.legal_name), ''), 'PT Arcon Penta Persada')
FROM {{schema}}.company_profile AS p
WHERE p.id = 1
  AND NOT EXISTS (SELECT 1 FROM {{schema}}.organizations)
UNION ALL
SELECT 'PT Arcon Penta Persada'
WHERE NOT EXISTS (SELECT 1 FROM {{schema}}.organizations)
  AND NOT EXISTS (SELECT 1 FROM {{schema}}.company_profile WHERE id = 1);
