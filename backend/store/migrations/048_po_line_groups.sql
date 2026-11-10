-- 048_po_line_groups: split-line structure for the PO editor (#573).
--
-- The PO stores one row per priced line because reconciliation, totals and
-- the final gate all key off that flat shape. The quotation editor, however,
-- presents a split item as one parent row with sub-items beneath it, and each
-- sub-item may carry its own description lines ("sub-sub-items"). This
-- migration adds the minimum the PO needs to present and round-trip that
-- structure without changing the flat pricing shape:
--
--   group_key          '' for a plain line; the same value on every row of one
--                      split line. Source groups use `q:<itemNo>` (the source
--                      item number is stable for the life of the draft), extra
--                      groups use `g:<uuid>`.
--   parent_description the split line's title, copied to every row of the
--                      group so a read never needs a join to a parent row that
--                      does not exist.
--   description_lines  a JSON array of text lines: description bullets on a
--                      plain line, or sub-sub description lines on a sub-item.
--                      Empty for a row that has none.
--
-- A group has no parent row. The parent is a presentation of the rows that
-- share its group_key; pricing still sums the flat rows, so the existing
-- calculation and reconciliation are untouched. The correction history keeps
-- the same three fields so a retained snapshot reads back exactly as the
-- record did.

ALTER TABLE {{schema}}.purchase_order_items
    ADD COLUMN IF NOT EXISTS group_key TEXT NOT NULL DEFAULT ''
        CHECK (char_length(group_key) <= 120),
    ADD COLUMN IF NOT EXISTS parent_description TEXT NOT NULL DEFAULT ''
        CHECK (char_length(parent_description) <= 500),
    ADD COLUMN IF NOT EXISTS description_lines JSONB NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(description_lines) = 'array');

-- A split group is read as one unit; the index keeps that read cheap without
-- constraining how many rows share a key.
CREATE INDEX IF NOT EXISTS purchase_order_items_group_idx
    ON {{schema}}.purchase_order_items (purchase_order_id, group_key)
    WHERE group_key <> '';

ALTER TABLE {{schema}}.purchase_order_correction_items
    ADD COLUMN IF NOT EXISTS group_key TEXT NOT NULL DEFAULT ''
        CHECK (char_length(group_key) <= 120),
    ADD COLUMN IF NOT EXISTS parent_description TEXT NOT NULL DEFAULT ''
        CHECK (char_length(parent_description) <= 500),
    ADD COLUMN IF NOT EXISTS description_lines JSONB NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(description_lines) = 'array');

-- A pre-045 draft already stores one row per priced part with the item and
-- sub numbers. Those rows are one split line; grouping them here means a
-- reopened draft shows the structure it already had instead of a flat list
-- until the next save. The parent title is unknown for these rows, so the
-- editor falls back to "Quotation item N" rather than inventing one.
UPDATE {{schema}}.purchase_order_items
SET group_key = 'q:' || quotation_item_no
WHERE quotation_sub_no IS NOT NULL
  AND quotation_item_no IS NOT NULL
  AND group_key = '';
