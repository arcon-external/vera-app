-- 050_po_business_category: the reporting category every purchase order
-- carries (issue #628).
--
-- The client reports purchase orders by business line (FM, FAB, SUPPLY_GEN,
-- OTHERS), and until now the category lived only in a hand-maintained
-- spreadsheet. It is a property of the purchase order, never of the quotation:
-- the same quotation can be ordered under different lines of business.
--
-- Empty is a real value only while a draft, and it is the honest default for
-- every record saved before this migration: they read Unassigned, and nothing
-- infers a category from the subject, the customer or the lines. The final
-- save refuses an empty category, so a new saved record always names one.
--
-- The correction snapshot carries the same column. A snapshot is the record as
-- it stood before the correction, so history keeps the category the corrected
-- version carried rather than only the current value. Existing snapshots read
-- empty for the same reason their live rows do.
ALTER TABLE {{schema}}.purchase_orders
    ADD COLUMN IF NOT EXISTS business_category TEXT NOT NULL DEFAULT ''
        CHECK (business_category IN ('', 'FM', 'FAB', 'SUPPLY_GEN', 'OTHERS'));

ALTER TABLE {{schema}}.purchase_order_corrections
    ADD COLUMN IF NOT EXISTS business_category TEXT NOT NULL DEFAULT ''
        CHECK (business_category IN ('', 'FM', 'FAB', 'SUPPLY_GEN', 'OTHERS'));
