-- 055_po_line_kind: the explicit distinction between a quotation-backed PO
-- line and an extra expense line (parent #648, #655).
--
-- A quotation line is backed by the quotation revision leaf it fulfils and, on
-- a new record, by exactly one allocation row. An extra line was never on the
-- quotation: it is a pass-through charge with no reference, no allocation and
-- zero margin, reported in its own Extra Expense bucket rather than as
-- uncosted revenue.
--
-- # The default is the compatibility contract
--
-- Every existing row becomes 'quotation', never 'extra'. A pre-052 null
-- reference line was a legacy or free line, not a deliberate expense; keeping
-- it quotation-kind preserves its uncosted report bucket and the bought-state
-- read's existing behaviour. Only a new explicit extra line is classified.
--
-- # The allocation guard is declarative
--
-- purchase_order_allocations.line_kind is pinned to 'quotation' by a CHECK, and
-- the composite foreign key proves the row it names was a quotation line at
-- commit. Raw SQL therefore cannot attach an allocation to an extra line, even
-- if it bypasses the store.
--
-- The key is DEFERRABLE INITIALLY DEFERRED because a correction may clear a
-- line's quotation reference and mark it extra in the same transaction: the
-- store updates the line first and then replaces the allocation set, and only
-- the committed state must agree. An immediate key would refuse that legitimate
-- transition, and a trigger would have to reimplement the same check.
ALTER TABLE {{schema}}.purchase_order_items
    ADD COLUMN IF NOT EXISTS line_kind TEXT NOT NULL DEFAULT 'quotation';

ALTER TABLE {{schema}}.purchase_order_items
    ADD CONSTRAINT purchase_order_items_line_kind_check
    CHECK (line_kind IN ('quotation', 'extra'));

-- An extra line has no quotation identity by definition. A reference would let
-- it masquerade as a bought quotation leaf while still declaring itself extra.
ALTER TABLE {{schema}}.purchase_order_items
    ADD CONSTRAINT purchase_order_items_extra_without_reference
    CHECK (
        line_kind <> 'extra'
        OR (quotation_item_no IS NULL AND quotation_sub_no IS NULL)
    );

-- A correction snapshot is evidence: it must keep the kind the record carried,
-- so a reopened history cannot turn an intentional extra into a legacy free
-- line. Existing snapshots default to 'quotation', exactly like their records.
ALTER TABLE {{schema}}.purchase_order_correction_items
    ADD COLUMN IF NOT EXISTS line_kind TEXT NOT NULL DEFAULT 'quotation';

ALTER TABLE {{schema}}.purchase_order_correction_items
    ADD CONSTRAINT purchase_order_correction_items_line_kind_check
    CHECK (line_kind IN ('quotation', 'extra'));

-- The allocation-side copy of the kind. The store writes nothing here, so the
-- column default is the only value: an allocation is always a quotation line.
ALTER TABLE {{schema}}.purchase_order_allocations
    ADD COLUMN IF NOT EXISTS line_kind TEXT NOT NULL DEFAULT 'quotation';

ALTER TABLE {{schema}}.purchase_order_allocations
    ADD CONSTRAINT po_allocations_line_kind_check CHECK (line_kind = 'quotation');

-- The composite key the guard references. The existing (id, purchase_order_id)
-- unique index stays: it still proves an allocation names a line of its own PO.
CREATE UNIQUE INDEX IF NOT EXISTS purchase_order_items_id_po_kind_uidx
    ON {{schema}}.purchase_order_items (id, purchase_order_id, line_kind);

ALTER TABLE {{schema}}.purchase_order_allocations
    ADD CONSTRAINT po_allocations_line_kind_fkey
    FOREIGN KEY (purchase_order_item_id, purchase_order_id, line_kind)
    REFERENCES {{schema}}.purchase_order_items (id, purchase_order_id, line_kind)
    ON DELETE CASCADE
    DEFERRABLE INITIALLY DEFERRED
    NOT VALID;

ALTER TABLE {{schema}}.purchase_order_allocations
    VALIDATE CONSTRAINT po_allocations_line_kind_fkey;
