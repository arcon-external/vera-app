-- 024_draft_currency: the currency a draft's amounts were written in, stored
-- per draft for the same reason revisions carry theirs (023): switching the
-- company currency later reinterprets stored integers, and the editor must
-- block a mismatched draft rather than silently relabel it.
--
-- Additive and defaulted, so every draft written before this column reads
-- back as IDR: that is what the company currency can only have been when no
-- other code existed. Stored drafts are never rewritten.
ALTER TABLE {{schema}}.quotation_drafts
    ADD COLUMN IF NOT EXISTS currency TEXT NOT NULL DEFAULT 'IDR';
