-- 045_recipient_company_snapshot: store the customer company name each
-- revision printed, instead of reconstructing it from the live customer row.
-- A customer rename after issue must not change an old revision's printed
-- document or break its verification code.
--
-- Old revisions keep the empty default rather than being backfilled: an empty
-- snapshot means "fall back to the live customer name", which is exactly how
-- those rows behaved before this column existed. Backfilling today's name
-- would silently rewrite what the verification code recomputes.
ALTER TABLE {{schema}}.quotation_revisions
    ADD COLUMN IF NOT EXISTS recipient_company TEXT NOT NULL DEFAULT '';
