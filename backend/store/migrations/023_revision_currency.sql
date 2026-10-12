-- 023_revision_currency: the currency a revision was issued in, stored per
-- revision beside the issuer name (012) for the same reason: renaming the
-- company -- or changing its currency -- later must not rewrite history or
-- break old signatures.
--
-- Additive and defaulted, so every revision written before this column reads
-- back as IDR: SignableFromEvidence normalizes the same way, and their
-- canonical bytes -- and their signatures -- stay byte-identical. Stored
-- revisions are never rewritten.
ALTER TABLE {{schema}}.quotation_revisions
    ADD COLUMN IF NOT EXISTS currency TEXT NOT NULL DEFAULT 'IDR';
