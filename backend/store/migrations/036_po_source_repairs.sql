-- 036_po_source_repairs: an admin repair of a saved Purchase Order's source
-- link (issue #475).
--
-- #473 gave every saved record a monotonic version and an immutable snapshot
-- of the values a correction replaced. This migration extends that machinery
-- for the one write a staff correction deliberately cannot perform: changing
-- which issued Quotation revision the record follows.
--
-- The snapshot row stays the history spine. A repair writes one row in
-- purchase_order_corrections with kind = 'source_repair' (the values the record
-- carried before the repair), plus the two satellite rows below, which record
-- what that snapshot cannot: the source it moved *to*, and the explicit
-- difference choices the admin made against it. No new history table and no
-- change to 030-035: a reader that knows only 'correction' and
-- 'draft_overwrite' still reads every earlier snapshot.
--
-- Why the new source is stored here rather than derived from the live record:
-- a later repair moves the record again, while each history row must keep
-- saying which link it repaired. The old source is already on the snapshot
-- (quotation_id, source_revision, quote_number, customer_name); the new one is
-- copied beside it for the same reason, and so the history panel can render
-- "quotation A revision 2 -> quotation B revision 0" without joining anything
-- the record no longer holds.

-- Extend the kind vocabulary. The constraint is recreated by name because an
-- unnamed column CHECK is named purchase_order_corrections_kind_check by
-- Postgres; IF EXISTS keeps a hand-repaired development schema from aborting.
ALTER TABLE {{schema}}.purchase_order_corrections
    DROP CONSTRAINT IF EXISTS purchase_order_corrections_kind_check;
ALTER TABLE {{schema}}.purchase_order_corrections
    ADD CONSTRAINT purchase_order_corrections_kind_check
        CHECK (kind IN ('correction', 'draft_overwrite', 'source_repair'));

CREATE TABLE IF NOT EXISTS {{schema}}.purchase_order_source_repairs (
    correction_id BIGINT PRIMARY KEY
        REFERENCES {{schema}}.purchase_order_corrections(id) ON DELETE CASCADE,
    -- The link the record followed before the repair.
    previous_quotation_id BIGINT NOT NULL,
    previous_source_revision SMALLINT NOT NULL
        CHECK (previous_source_revision >= 0),
    previous_quote_number TEXT NOT NULL DEFAULT '',
    previous_customer_id BIGINT NOT NULL,
    previous_customer_name TEXT NOT NULL DEFAULT '',
    -- The link the repair stored. A repair must move the link: selecting the
    -- same quotation and revision is refused before this row is written.
    new_quotation_id BIGINT NOT NULL,
    new_source_revision SMALLINT NOT NULL CHECK (new_source_revision >= 0),
    new_quote_number TEXT NOT NULL DEFAULT '',
    new_customer_id BIGINT NOT NULL,
    new_customer_name TEXT NOT NULL DEFAULT '',
    CHECK (previous_quotation_id <> new_quotation_id
           OR previous_source_revision <> new_source_revision)
);

-- One row per difference the admin decided. The key is the same stable key
-- #471's comparison uses ("line:2:unitPrice", "header:paymentTerms"), so the
-- audit can be read against the review it came from. Values are not copied:
-- the snapshot holds the old side and the record held the new side at the
-- version this repair produced.
CREATE TABLE IF NOT EXISTS {{schema}}.purchase_order_source_repair_decisions (
    correction_id BIGINT NOT NULL
        REFERENCES {{schema}}.purchase_order_corrections(id) ON DELETE CASCADE,
    key TEXT NOT NULL CHECK (char_length(key) BETWEEN 1 AND 120),
    choice TEXT NOT NULL CHECK (choice IN ('po', 'quotation')),
    PRIMARY KEY (correction_id, key)
);

-- A repair record is evidence, exactly like the snapshot it hangs from. The
-- #473 guard raises on any UPDATE; the only allowed removal is the Purchase
-- Order's own deletion, through the foreign key's cascade.
DROP TRIGGER IF EXISTS purchase_order_source_repairs_immutable_trigger
    ON {{schema}}.purchase_order_source_repairs;
CREATE TRIGGER purchase_order_source_repairs_immutable_trigger
    BEFORE UPDATE ON {{schema}}.purchase_order_source_repairs
    FOR EACH ROW
    EXECUTE FUNCTION {{schema}}.purchase_order_corrections_immutable();

DROP TRIGGER IF EXISTS purchase_order_source_repair_decisions_immutable_trigger
    ON {{schema}}.purchase_order_source_repair_decisions;
CREATE TRIGGER purchase_order_source_repair_decisions_immutable_trigger
    BEFORE UPDATE ON {{schema}}.purchase_order_source_repair_decisions
    FOR EACH ROW
    EXECUTE FUNCTION {{schema}}.purchase_order_corrections_immutable();
