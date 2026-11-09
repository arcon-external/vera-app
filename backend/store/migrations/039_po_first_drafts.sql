-- 039_po_first_drafts: a Purchase Order can start before its source exists
-- (issue #496).
--
-- # Three live row shapes, never two
--
-- 037 made a deleted source expressible: quotation_id is released and the row
-- keeps a complete former_* reference. PO-first entry needs a third shape that
-- 037 refused: a draft that was never linked at all. The CHECK becomes a
-- superset, so every existing linked and former row validates unchanged:
--
--   linked          quotation_id IS NOT NULL, every former_* NULL
--   former          quotation_id IS NULL, every former_* set
--   never linked    quotation_id IS NULL, every former_* NULL, state = draft
--
-- A never-linked row is deliberately draft-only. Finalize and review refuse it
-- at the server boundary, and this CHECK is the schema backstop: a hand-written
-- UPDATE cannot turn it into a finalized record with no authoritative customer.
--
-- # Honest NULLs, not invented identities
--
-- customer_id and source_revision become nullable. A never-linked draft has no
-- Customer, and inventing a placeholder customer (or a zero that an old reader
-- cannot distinguish from a real revision 00) would be a fabricated identity.
-- The wire keeps its numeric fields for older desktops and adds availability
-- states in the API layer; the database stores the truth.
--
-- # The full tax triple
--
-- A source-less draft starts from the user's Quotation tax defaults and stays
-- editable until a quotation is linked. tax_zone and the legacy tax_category
-- were not enough to persist that basis: a Services draft needs its service
-- category, and a current revision needs its tax type. The two columns below
-- are added rather than overloading tax_category, and existing linked rows are
-- backfilled from the exact revision they froze so their reopened basis is
-- unchanged.
--
-- # Correction snapshots
--
-- A never-linked draft overwrite keeps an immutable snapshot. Its prior source
-- and customer identities are SQL NULL: 0 would be a fabricated reference in
-- the audit trail, and revision 00 is a real linked revision. Only the 035
-- snapshot columns are relaxed here. 036's repair-audit constraints (previous_*
-- NOT NULL and the must-move CHECK) belong to #497's link path and are not
-- touched.

ALTER TABLE {{schema}}.purchase_orders
    ALTER COLUMN customer_id DROP NOT NULL;

ALTER TABLE {{schema}}.purchase_orders
    ALTER COLUMN source_revision DROP NOT NULL;

ALTER TABLE {{schema}}.purchase_orders
    ADD COLUMN IF NOT EXISTS tax_type TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS service_category TEXT NOT NULL DEFAULT '';

-- Preserve the frozen basis of every row that still has its exact source. The
-- revision is the authority; a legacy revision with an empty tax_type keeps the
-- legacy tax_category fallback the Go resolver already understands.
--
-- 035's BEFORE UPDATE trigger bumps `version` on every write. This backfill is
-- not a user write: without disabling the trigger it would move every existing
-- record's optimistic-concurrency version (and disagree with its retained
-- correction snapshots). The trigger is restored in the same transaction, so a
-- failed migration rolls both the data and the trigger state back together.
ALTER TABLE {{schema}}.purchase_orders
    DISABLE TRIGGER purchase_orders_bump_version_trigger;
UPDATE {{schema}}.purchase_orders po
SET tax_type = r.tax_type,
    service_category = r.service_category
FROM {{schema}}.quotation_revisions r
WHERE r.quotation_id = po.quotation_id
  AND r.revision_number = po.source_revision
  AND po.tax_type = ''
  AND po.service_category = '';
ALTER TABLE {{schema}}.purchase_orders
    ENABLE TRIGGER purchase_orders_bump_version_trigger;

-- Replace the two-shape CHECK with the three-shape superset. Dropping by name
-- is safe because 037 created it explicitly; IF EXISTS keeps a hand-repaired
-- development schema from aborting.
ALTER TABLE {{schema}}.purchase_orders
    DROP CONSTRAINT IF EXISTS purchase_orders_source_shape_check;
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
        OR
        (
            quotation_id IS NULL
            AND former_quotation_id IS NULL
            AND former_quotation_uuid IS NULL
            AND former_quote_number IS NULL
            AND former_source_revision IS NULL
            AND state = 'draft'
        )
    );

-- A never-linked overwrite snapshot keeps NULL identities. The 035 immutability
-- trigger refuses UPDATEs on snapshots, so this is a constraint relaxation
-- only; no historical row is rewritten.
ALTER TABLE {{schema}}.purchase_order_corrections
    ALTER COLUMN quotation_id DROP NOT NULL;
ALTER TABLE {{schema}}.purchase_order_corrections
    ALTER COLUMN source_revision DROP NOT NULL;
ALTER TABLE {{schema}}.purchase_order_corrections
    ALTER COLUMN customer_id DROP NOT NULL;
