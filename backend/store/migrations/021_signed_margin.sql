-- 021: a margin can be a loss.
--
-- Since 020 a row can carry its own discount, and a row discounted below what
-- we paid for it has a NEGATIVE margin amount. That is the honest figure, and
-- it is the one the editor shows. The column, though, still carried the
-- CHECK (margin_amount >= 0) written in 001 (and copied into 016 for
-- sub-items), so any quotation containing a discounted-below-cost line failed
-- to export at all:
--
--     new row for relation "quotation_revision_items" violates check
--     constraint "quotation_revision_items_margin_amount_check"
--
-- The constraint is now what it should have been from the start: not negative
-- beyond what a discount can produce. A margin amount is bounded below by
-- -unit_price (a line given away entirely), so both ends are pinned -- the
-- point of the constraint was to catch nonsense, and an unbounded BIGINT still
-- deserves that.
--
-- Dropping and re-adding is the only way to change a CHECK in Postgres. The
-- re-add scans existing rows: every stored revision satisfies it, because they
-- were written while the old constraint held.
--
-- Nothing here rewrites a stored revision. This only lets new ones be written.

ALTER TABLE {{schema}}.quotation_revision_items
    DROP CONSTRAINT IF EXISTS quotation_revision_items_margin_amount_check,
    ADD CONSTRAINT quotation_revision_items_margin_amount_check
        CHECK (margin_amount >= -unit_price);

-- Sub-items spell the two sides out: unit_cost is what we pay and unit_price is
-- the sell price, so the floor is -unit_cost here -- not -unit_price, which
-- would be a different (and wrong) number.
ALTER TABLE {{schema}}.quotation_revision_sub_items
    DROP CONSTRAINT IF EXISTS quotation_revision_sub_items_margin_amount_check,
    ADD CONSTRAINT quotation_revision_sub_items_margin_amount_check
        CHECK (margin_amount >= -unit_cost);
