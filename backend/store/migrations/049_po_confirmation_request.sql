-- 049_po_confirmation_request: remember whether a draft asked for a
-- confirmation number (issue #581).
--
-- The confirmation number itself is allocated only by the final save (#474).
-- This column carries the editor's draft request so Save draft and reopen keep
-- the choice: it allocates nothing, is not part of any signed payload, and a
-- record that already has a number is unaffected.
--
-- Existing rows read false, which is the same safe default a new draft gets.
-- No backfill is needed: no earlier record expressed the request.
ALTER TABLE {{schema}}.purchase_orders
    ADD COLUMN IF NOT EXISTS confirmation_requested BOOLEAN NOT NULL DEFAULT FALSE;
