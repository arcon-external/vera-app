-- 020: a discount on the row, not only on the whole quotation.
--
-- DiscountPercent used to exist once, on the quotation. A line that needed
-- 15% off and a line that needed none either both got the discount, or the
-- drafter back-solved a unit price so the grand total came out right and the
-- line stopped showing the customer's real price. These columns give each row
-- its own, exactly as margin_percent already does.
--
-- Additive and defaulted, so every revision written before this column reads
-- back as what it printed: FALSE is "this row carried no discount", which is
-- true of every row in every existing revision. Stored revisions are never
-- rewritten.
--
-- The ceiling matches the whole-quotation discount (MaxDiscountPercent, 100):
-- 100% is a free line, which is a real thing to quote, and nothing above it
-- is a price rather than a discount.
--
-- discount_enabled is stored rather than inferred from percent > 0 so a row
-- explicitly set to 0% is distinguishable from one never touched. Neither
-- changes a price, but they print differently and the snapshot should keep
-- what was chosen.

-- list_unit_price is the marginalised price before the row's discount: the
-- figure the customer would otherwise have paid. It is stored rather than
-- recomputed on read for the same reason margin_percent sits beside
-- marginalized_unit_price -- a snapshot should say what it said, not what
-- today's arithmetic produces. discount_amount is deliberately NOT stored: it
-- is exactly list x qty - unit x qty, and persisting a derived figure twice is
-- how two copies start disagreeing.
ALTER TABLE {{schema}}.quotation_revision_items
    ADD COLUMN IF NOT EXISTS discount_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS discount_percent SMALLINT NOT NULL DEFAULT 0
        CHECK (discount_percent BETWEEN 0 AND 100),
    ADD COLUMN IF NOT EXISTS list_unit_price BIGINT NOT NULL DEFAULT 0
        CHECK (list_unit_price >= 0);

ALTER TABLE {{schema}}.quotation_revision_sub_items
    ADD COLUMN IF NOT EXISTS discount_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS discount_percent SMALLINT NOT NULL DEFAULT 0
        CHECK (discount_percent BETWEEN 0 AND 100),
    ADD COLUMN IF NOT EXISTS list_unit_price BIGINT NOT NULL DEFAULT 0
        CHECK (list_unit_price >= 0);

-- No index here on purpose. Nothing queries "the rows that carry a discount":
-- whether the customer document shows a discount column is a display decision
-- taken from rows already loaded for the revision, so an index would only add
-- a write to every export.