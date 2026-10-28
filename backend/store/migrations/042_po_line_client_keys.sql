-- 042_po_line_client_keys: stable line identity across draft saves
-- (#502, ticket 05).
--
-- 041 anticipated this: the session's LineIdentities map a stable line key to
-- its persisted row (`id:<row>` for a line that entered as persisted,
-- `client:<UUID>` for an editor line before its first save), and review items
-- key off that mapping instead of the display position. But
-- replacePurchaseOrderItems deleted and reinserted every row on each save, so
-- persisted ids churned and a reorder re-keyed every decision. 038 and 040
-- are untouched, and 041 is untouched: this migration only adds the durable
-- alias on the row itself.
--
-- # Why a column, not just the session mapping
--
-- The alias must survive where no session exists yet: an editor line created
-- on a never-linked (source-less) draft is saved before any session can
-- record it. Storing the editor alias on the row lets the later link seed the
-- session identities from the rows, and lets every later save resolve the
-- alias to its row without trusting client-supplied ids alone. Rows created
-- before this migration keep a NULL alias; their identity is `id:<row>`.
--
-- # Rules enforced here
--
-- A row carries at most one alias, and one alias names at most one row of its
-- purchase order (partial unique index). The alias namespace is the editor's
-- `client:` namespace; persisted rows are addressed by `id:<row>` and never
-- store that form here. The existing (purchase_order_id, line_number)
-- uniqueness is recreated DEFERRABLE so one transaction can swap two display
-- positions (a reorder) without tripping the index mid-flight; every line
-- write still holds the PO row lock, so the guarantee is unchanged, only the
-- check timing moves to commit.

ALTER TABLE {{schema}}.purchase_order_items
    ADD COLUMN IF NOT EXISTS client_key TEXT NULL
        CHECK (client_key IS NULL
               OR (char_length(client_key) BETWEEN 8 AND 120
                   AND client_key LIKE 'client:%'));

CREATE UNIQUE INDEX IF NOT EXISTS purchase_order_items_client_key_idx
    ON {{schema}}.purchase_order_items (purchase_order_id, client_key)
    WHERE client_key IS NOT NULL;

-- Recreate the display-position uniqueness as deferrable. The name keeps the
-- conventional Postgres naming so readers of migration 030 still find it.
ALTER TABLE {{schema}}.purchase_order_items
    DROP CONSTRAINT IF EXISTS purchase_order_items_purchase_order_id_line_number_key;
ALTER TABLE {{schema}}.purchase_order_items
    ADD CONSTRAINT purchase_order_items_purchase_order_id_line_number_key
        UNIQUE (purchase_order_id, line_number) DEFERRABLE INITIALLY DEFERRED;
