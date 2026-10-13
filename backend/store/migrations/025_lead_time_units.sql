-- 025_lead_time_units: a lead time is four numbers, not one.
--
-- "60 days" was the only shape a quotation could state, so a nine-month
-- delivery had to be back-solved into a figure nobody would have written, and
-- there was no way to print "2 Years, 10 Months, 4 Weeks, and 5 Days". The
-- editor now collects years, months, weeks and days, and the revision stores
-- what was entered -- there is no normalisation anywhere, because "4 Weeks" is
-- what staff typed.
--
-- Additive and defaulted, so every revision written before this migration
-- reads back as the days figure it always had: 0 for the three new columns,
-- and SignableRevision emits each of the three only when it is non-zero, which
-- keeps every stored revision's canonical bytes -- and its signature --
-- exactly what they were. lead_time_days itself stays unconditional in the
-- signed payload for the same reason. Stored revisions are never rewritten.
--
-- The three new columns are SMALLINT like their neighbour (the store scans
-- SMALLINT into int16), bounded to the uint8 the input model allows: 0 means
-- "not stated" in that unit, and nothing above 255 is a duration anybody
-- quotes.
--
-- lead_time_days loses its > 0 check. It came from 001, where a lead time was
-- required and could only be days; a quotation with no lead time at all -- or
-- one stated purely in months -- stores 0 there now, and the old check would
-- have refused the export with a constraint violation instead of printing a
-- document. Dropping and re-adding is the only way to change a CHECK in
-- Postgres; the re-add scans existing rows and they all satisfy it.
ALTER TABLE {{schema}}.quotation_revisions
    ADD COLUMN IF NOT EXISTS lead_time_years SMALLINT NOT NULL DEFAULT 0
        CHECK (lead_time_years BETWEEN 0 AND 255),
    ADD COLUMN IF NOT EXISTS lead_time_months SMALLINT NOT NULL DEFAULT 0
        CHECK (lead_time_months BETWEEN 0 AND 255),
    ADD COLUMN IF NOT EXISTS lead_time_weeks SMALLINT NOT NULL DEFAULT 0
        CHECK (lead_time_weeks BETWEEN 0 AND 255);

ALTER TABLE {{schema}}.quotation_revisions
    DROP CONSTRAINT IF EXISTS quotation_revisions_lead_time_days_check,
    ADD CONSTRAINT quotation_revisions_lead_time_days_check
        CHECK (lead_time_days BETWEEN 0 AND 255);

-- No index on purpose: nothing queries "the revisions that state a lead time
-- in weeks". The columns are read as part of the revision that is already
-- being loaded, so an index would only add a write to every export.
