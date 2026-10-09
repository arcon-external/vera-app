-- 018: the per-line sub-item display choice.
--
-- Whether an itemized line's sub-items print with their prices (state C) or
-- as unpriced scope rows under a full-value main row (state B) is a choice
-- made on the line. It shipped unpersisted in 016, which meant reopening an
-- archived quotation could silently flip the choice on the next revision;
-- the display belongs to the immutable snapshot like everything else.
--
-- Additive and defaulted, so every revision written before this column reads
-- as it always printed: FALSE is the state-B presentation those documents
-- were rendered with.

ALTER TABLE {{schema}}.quotation_revision_items
    ADD COLUMN IF NOT EXISTS show_sub_prices BOOLEAN NOT NULL DEFAULT FALSE;
