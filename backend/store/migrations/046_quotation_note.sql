-- 046_quotation_note: one optional customer-facing note per quotation,
-- snapshotted on each immutable revision. The note is not stored on the
-- mutable quotation and not on line items: the printed document, its
-- snapshot and its signature belong to one revision.
--
-- Old revisions keep the empty default rather than being backfilled: an empty
-- note emits nothing in the canonical payload, so every printed verification
-- code keeps verifying. 1000 characters is the same limit Prepare and the
-- editor enforce.
ALTER TABLE {{schema}}.quotation_revisions
    ADD COLUMN IF NOT EXISTS note TEXT NOT NULL DEFAULT '';

ALTER TABLE {{schema}}.quotation_revisions
    ADD CONSTRAINT quotation_revisions_note_length
    CHECK (char_length(note) <= 1000);
