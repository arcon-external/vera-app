-- 053_multiple_pos_per_quotation: one quotation may now produce many purchase
-- orders over time. The one-active-record rule is replaced by bought-state
-- allocations: 052 records exactly which quotation leaf each saved PO line
-- consumed, so the available quantity is derived instead of enforced by a
-- unique index.
--
-- The old unique index is dropped; the replacement is the lookup the selector
-- and the availability read use. Existing rows are untouched.

DROP INDEX IF EXISTS {{schema}}.purchase_orders_active_quotation_idx;

CREATE INDEX IF NOT EXISTS purchase_orders_source_lookup_idx
    ON {{schema}}.purchase_orders (org_id, quotation_id, source_revision, state)
    WHERE quotation_id IS NOT NULL;
