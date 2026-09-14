ALTER TABLE {{schema}}.quotation_revisions
    ADD COLUMN IF NOT EXISTS tax_type TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS service_category TEXT NOT NULL DEFAULT '';

ALTER TABLE {{schema}}.quotation_revision_totals
    ADD COLUMN IF NOT EXISTS ppn_amount BIGINT NOT NULL DEFAULT 0 CHECK (ppn_amount >= 0),
    ADD COLUMN IF NOT EXISTS customer_grand_total BIGINT NOT NULL DEFAULT 0 CHECK (customer_grand_total >= 0);

-- Pre-change rows predate PPN-on-document: their customer grand equals the
-- sell grand they were exported with.
UPDATE {{schema}}.quotation_revision_totals
SET customer_grand_total = marginalized_grand_total;
