-- 032_po_finalization: turning a reviewed draft into a saved record (issue #471).
--
-- 030 stored the entered draft and 031 the explained calculation. This
-- migration adds the minimum a saved Purchase Order must carry, the per-customer
-- uniqueness the final save enforces, and the deliberate no-PDF confirmation.
--
-- Nothing here rewrites a draft's values, and nothing here touches a Quotation
-- revision: the PO is a snapshot that becomes the future downstream source, and
-- the issued Quotation it was checked against stays immutable.
--
-- # Customer PO number policy
--
-- po_number is the customer's own number, free text while drafting. On final
-- save the store trims surrounding whitespace (Go strings.TrimSpace, the same
-- set the editor and the rest of this feature use) and stores the trimmed text
-- in po_number. po_number_key holds the comparison key: the trimmed number
-- lower-cased with Go strings.ToLower. Two numbers that differ only by case or
-- surrounding whitespace are the same order for one customer, which is what
-- the partial unique index below enforces among saved rows.
--
-- Drafts never write the key and are never checked: a draft may repeat a number
-- while staff prepare it, and two people may compete for one order. Clearing the
-- key on a draft update is what keeps that freedom from weakening the final-save
-- rule.
--
-- # Why a partial index, not a plain unique constraint
--
-- Uniqueness is per customer and only among saved, non-empty numbers. The
-- predicate expresses exactly that, so drafts and future soft-deleted rows
-- (#480) do not collide with live saved records by accident.

ALTER TABLE {{schema}}.purchase_orders
    ADD COLUMN IF NOT EXISTS po_number_key TEXT NULL,
    ADD COLUMN IF NOT EXISTS saved_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS saved_by BIGINT NULL REFERENCES {{schema}}.users(id),
    -- The confirmation that this save is deliberately without the customer's
    -- PDF. #472 adds attachments; when a PDF is attached this pair stays null
    -- and no confirmation is asked. Storing who and when makes the missing
    -- evidence an attributable decision rather than a silent omission.
    ADD COLUMN IF NOT EXISTS pdf_absent_confirmed_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS pdf_absent_confirmed_by BIGINT NULL REFERENCES {{schema}}.users(id);

-- The final-save decision is a one-way transition: a saved row must name its
-- number, its actor and its time. Drafts keep all three unset.
--
-- No saved rows can exist before this migration: nothing set state = 'saved'
-- in the unreleased sprint branch. The guard names the disorder instead of the
-- constraint failing with a bare row value, if a development schema disagrees.
DO $saved_guard$
BEGIN
    IF EXISTS (
        SELECT 1 FROM {{schema}}.purchase_orders
        WHERE state = 'saved'
          AND (po_number IS NULL OR btrim(po_number) = ''
               OR po_number_key IS NULL OR po_number_key = ''
               OR saved_at IS NULL OR saved_by IS NULL)
    ) THEN
        RAISE EXCEPTION
            'migration 032 found a saved purchase order without a customer number or actor; it cannot be backfilled';
    END IF;
END
$saved_guard$;

ALTER TABLE {{schema}}.purchase_orders
    ADD CONSTRAINT purchase_orders_saved_identity_check CHECK (
        state <> 'saved'
        OR (
            po_number IS NOT NULL
            AND btrim(po_number) <> ''
            AND po_number_key IS NOT NULL
            AND po_number_key <> ''
            AND saved_at IS NOT NULL
            AND saved_by IS NOT NULL
        )
    );

-- Case- and surrounding-whitespace-insensitive uniqueness per customer, only
-- among saved records whose key is present. The index is the concurrency
-- guard: two simultaneous final saves race in Postgres, one wins, and the
-- loser is refused with a named error rather than silently stored twice.
CREATE UNIQUE INDEX IF NOT EXISTS purchase_orders_saved_number_idx
    ON {{schema}}.purchase_orders (org_id, customer_id, po_number_key)
    WHERE state = 'saved' AND po_number_key IS NOT NULL;
