-- 037_po_deletion: permanent deletion of one record or its document chain
-- (issue #480).
--
-- # What deletion must leave behind
--
-- A deleted record is gone: its rows, its lines, its attachments and its
-- recoverable correction versions are removed together (the existing foreign
-- keys cascade), and a minimal audit row keeps who deleted what, when, why and
-- which source it followed. A Quotation's confirmation counters and sequence
-- counters are never touched: an issued number is consumed forever.
--
-- # The source link becomes releasable
--
-- Deleting a Quotation does not delete a surviving Purchase Order. The PO keeps
-- its own readable snapshot and gains a minimal former-source reference, so the
-- archive can say "Source missing" and a later admin repair (#475/#481) knows
-- which quotation and revision the record used to follow. quotation_id is
-- therefore nullable, and the row is exactly one of two shapes:
--
--   live link     quotation_id IS NOT NULL and every former_* column NULL
--   source missing quotation_id IS NULL and every former_* column set
--
-- The former reference is a copy, not a join: the quotation row is deleted, so
-- there is nothing left to join and nothing left that could be resurrected.
-- Keeping the former quotation id is deliberate even though it no longer
-- references a row: it is what the deletion audit and the PO history quote.
--
-- # Why the uniqueness index becomes partial
--
-- Postgres already treats NULLs as distinct in a unique index, so several
-- source-missing POs could coexist without it. The partial predicate states the
-- rule anyway ("one active record per live Quotation") and keeps the index from
-- carrying an entry for rows that occupy nothing. Both a source-missing PO and a
-- deleted one can no longer block creating a new PO for a re-issued quotation.
--
-- # Challenges
--
-- Whole-chain deletion needs a second, typed confirmation: ten random uppercase
-- letters excluding I, L and O, accepted case-insensitively. The challenge is a
-- server-side row, not a client promise: it stores the exact chain fingerprint it
-- was issued against, who asked, when it expires and whether it was consumed.
-- Deletion re-reads the chain inside its transaction and compares fingerprints:
-- any change refuses the write and the admin must renew the challenge. Letters
-- are stored as a SHA-256 of their uppercase form, never as plaintext -- they are
-- deliberate-action friction, not an authentication factor, and there is no
-- reason to keep them readable. Retention is bounded rather than left to a
-- background job: every path that can observe an expired challenge (preview,
-- challenge issuance, confirmation) removes it, and a successful deletion
-- removes every challenge for any record in the chain it touched in the same
-- transaction. A challenge row therefore never outlives the deletion it names,
-- and never outlives its expiry by more than the next deletion operation.
--
-- # Audit
--
-- One row per deleted document (a quotation and a PO each get their own), with
-- the target's UUID and readable number because the target row no longer exists.
-- target_id is kept as a plain bigint with no foreign key for the same reason.
-- Backups expire on the operator's retention schedule; the audit is the only
-- record Vera itself keeps.

ALTER TABLE {{schema}}.purchase_orders
    ALTER COLUMN quotation_id DROP NOT NULL;

ALTER TABLE {{schema}}.purchase_orders
    ADD COLUMN IF NOT EXISTS former_quotation_id BIGINT NULL,
    ADD COLUMN IF NOT EXISTS former_quotation_uuid UUID NULL,
    ADD COLUMN IF NOT EXISTS former_quote_number TEXT NULL,
    ADD COLUMN IF NOT EXISTS former_source_revision SMALLINT NULL
        CHECK (former_source_revision IS NULL OR former_source_revision >= 0);

ALTER TABLE {{schema}}.purchase_orders
    ADD CONSTRAINT purchase_orders_source_shape_check CHECK (
        (
            quotation_id IS NOT NULL
            AND former_quotation_id IS NULL
            AND former_quotation_uuid IS NULL
            AND former_quote_number IS NULL
            AND former_source_revision IS NULL
        )
        OR
        (
            quotation_id IS NULL
            AND former_quotation_id IS NOT NULL
            AND former_quotation_uuid IS NOT NULL
            AND former_quote_number IS NOT NULL
            AND btrim(former_quote_number) <> ''
            AND former_source_revision IS NOT NULL
        )
    );

DROP INDEX IF EXISTS {{schema}}.purchase_orders_active_quotation_idx;
CREATE UNIQUE INDEX IF NOT EXISTS purchase_orders_active_quotation_idx
    ON {{schema}}.purchase_orders (quotation_id)
    WHERE quotation_id IS NOT NULL;

-- The source-missing archive filter (#469) reads this state per organization.
CREATE INDEX IF NOT EXISTS purchase_orders_source_missing_idx
    ON {{schema}}.purchase_orders (org_id, id)
    WHERE quotation_id IS NULL;

CREATE TABLE IF NOT EXISTS {{schema}}.deletion_challenges (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES {{schema}}.organizations(id),
    target_type TEXT NOT NULL
        CHECK (target_type IN ('quotation', 'purchase_order')),
    target_id BIGINT NOT NULL CHECK (target_id > 0),
    -- Chain deletion is the only scope that asks for the ten-letter challenge.
    scope TEXT NOT NULL CHECK (scope = 'chain'),
    -- The canonical chain fingerprint the challenge was issued against. A
    -- deletion whose current fingerprint differs is refused, never applied.
    fingerprint TEXT NOT NULL
        CHECK (char_length(fingerprint) BETWEEN 1 AND 4000),
    -- SHA-256 (lower-case hex) of the uppercase letters. Case-insensitive
    -- acceptance is the uppercase form on both sides.
    letters_sha256 TEXT NOT NULL CHECK (letters_sha256 ~ '^[0-9a-f]{64}$'),
    created_by BIGINT NOT NULL REFERENCES {{schema}}.users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- A short life and a delete-on-use rule keep this table from becoming a
    -- second history: once the deletion commits, the audit is the only record.
    expires_at TIMESTAMPTZ NOT NULL,
    CHECK (expires_at > created_at)
);

CREATE INDEX IF NOT EXISTS deletion_challenges_target_idx
    ON {{schema}}.deletion_challenges (
        org_id, target_type, target_id, created_at DESC);

CREATE TABLE IF NOT EXISTS {{schema}}.deletion_audits (
    id BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    org_id UUID NOT NULL REFERENCES {{schema}}.organizations(id),
    -- The deleted document's own type, id and UUID. No foreign key: the row it
    -- names is gone, and a cascade would erase the audit with it.
    target_type TEXT NOT NULL
        CHECK (target_type IN ('quotation', 'purchase_order')),
    target_id BIGINT NOT NULL CHECK (target_id > 0),
    target_uuid UUID NOT NULL,
    -- The readable identifier the admin typed: a quotation number, or the PO's
    -- customer number / source quotation number for a draft.
    document_number TEXT NOT NULL DEFAULT ''
        CHECK (char_length(document_number) <= 200),
    -- The source the deleted record followed, when it had one: a PO's quotation
    -- number. Empty for a quotation.
    source_reference TEXT NOT NULL DEFAULT ''
        CHECK (char_length(source_reference) <= 200),
    scope TEXT NOT NULL CHECK (scope IN ('record', 'chain')),
    reason TEXT NOT NULL
        CHECK (char_length(reason) BETWEEN 1 AND 500 AND btrim(reason) <> ''),
    actor BIGINT NOT NULL REFERENCES {{schema}}.users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS deletion_audits_org_idx
    ON {{schema}}.deletion_audits (org_id, created_at DESC);
