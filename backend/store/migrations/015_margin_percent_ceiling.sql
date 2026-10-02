-- 015_margin_percent_ceiling: widen the per-line profit margin ceiling.
--
-- The column is already SMALLINT and the Go side is already int16 downstream, so
-- only the CHECK constraint held the old limit. It is relaxed here rather than
-- in 001_initial.sql, which is append-only once shipped: an already-provisioned
-- database would never pick up an edit to it.
--
-- 1000% means selling at eleven times cost, which is ordinary for a low-cost
-- fitting on a service visit. The previous ceiling of 100% capped those at
-- double cost, which is not enough to build a quote around.
--
-- Discount percentage is a different column with its own ceiling and is not
-- touched here.
ALTER TABLE {{schema}}.quotation_revision_items
    DROP CONSTRAINT IF EXISTS quotation_revision_items_margin_percent_check;

ALTER TABLE {{schema}}.quotation_revision_items
    ADD CONSTRAINT quotation_revision_items_margin_percent_check
    CHECK (margin_percent BETWEEN 0 AND 1000);
