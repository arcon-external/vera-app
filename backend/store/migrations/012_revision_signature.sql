-- 012_revision_signature: cryptographic verification support.
--
-- Revisions gain a short public identifier plus the Ed25519 signature that
-- covers their canonical field set. The handle identifies one *revision*
-- (two revisions of a quotation are different documents): first 14 hex
-- characters of the quotation UUID plus the revision number in 2 hex digits,
-- so codes are 16 characters, non-enumerable, and never collide across
-- revisions. Old rows backfill and self-heal on first verification.
--
-- signing_keys is a public-key registry: rotations add a row, retired keys
-- stay so old printed codes keep verifying under their original key id.
-- Private keys never live here — they arrive through the environment.
ALTER TABLE {{schema}}.quotation_revisions
    ADD COLUMN IF NOT EXISTS short_id TEXT;

ALTER TABLE {{schema}}.quotation_revisions
    ADD COLUMN IF NOT EXISTS signature TEXT NOT NULL DEFAULT '';

ALTER TABLE {{schema}}.quotation_revisions
    ADD COLUMN IF NOT EXISTS sig_key_id TEXT NOT NULL DEFAULT '';

ALTER TABLE {{schema}}.quotation_revisions
    ADD COLUMN IF NOT EXISTS canonical_sha256 TEXT NOT NULL DEFAULT '';

-- The issuer name is part of the signed field set and must reflect who
-- issued the document at the time, not the live company profile: renaming
-- the company later must not invalidate historical signatures.
ALTER TABLE {{schema}}.quotation_revisions
    ADD COLUMN IF NOT EXISTS issuer_name TEXT NOT NULL DEFAULT '';

UPDATE {{schema}}.quotation_revisions r
SET short_id = LOWER(LEFT(REPLACE(q.uuid::text, '-', ''), 14))
    || LPAD(TO_HEX(r.revision_number::INTEGER), 2, '0')
FROM {{schema}}.quotations q
WHERE r.quotation_id = q.id AND r.short_id IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS quotation_revisions_short_id_idx
    ON {{schema}}.quotation_revisions (short_id);

CREATE TABLE IF NOT EXISTS {{schema}}.signing_keys (
    key_id TEXT PRIMARY KEY,
    public_key TEXT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    retired_at TIMESTAMPTZ
);
