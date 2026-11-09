-- 043_po_line_alias_ownership: durable per-PO editor-alias ownership
-- (#502, ticket 05 round 2, K-S5).
--
-- The session's LineIdentities record one first-seen decision key per row,
-- so an adopted row-carried alias never becomes a second identity. But when
-- that row is later removed, the alias disappears with it: the retained
-- `id:<row>` identity cannot refuse a later unrelated row reusing the alias.
-- A session-only fix cannot retain the lost address without changing the
-- frozen session shape, and the live-row column alone disappears on deletion.
-- This migration adds the minimal durable bookkeeping: which client alias
-- belongs to which original row of which purchase order, surviving row
-- deletion, alias replacement, session generations and source deletion.
--
-- # Shape
--
-- Four columns, no surrogate ID, no timestamps, no retired flag. The primary
-- key is (purchase_order_id, client_key): one owner per alias within one PO,
-- while the same alias on another PO is independent. owner_line_id is the
-- original purchase_order_items.id as a scalar, retained after deletion; it
-- deliberately has NO foreign key to the live line (that would prevent
-- removal or destroy the evidence). The composite parent FK enforces
-- tenant/parent consistency through 041's existing unique index. NULL owner
-- is a permanent reservation for a retained client-first identity whose row
-- owner is unrecoverable at migration time: never a free alias, never
-- rebound later. Physical PO deletion cascades the claims; nothing else
-- releases them.
--
-- # Backfill
--
-- Live 042 row aliases plus every retained client-key session identity
-- (all sessions and generations, removed or not) are aggregated per
-- (PO, alias). Exactly one positive owner wins (null snapshots do not
-- override a recorded binding); unbound-only aliases become NULL
-- reservations; competing positive owners, malformed claims or foreign-PO
-- owners abort the migration with a diagnostic. Adopted aliases already
-- erased from both live rows and retained identities are not recoverable
-- and are not invented. Backfill touches no PO rows, versions, sessions,
-- identities, items, history or tax data.

CREATE TABLE IF NOT EXISTS {{schema}}.purchase_order_line_aliases (
    org_id UUID NOT NULL,
    purchase_order_id BIGINT NOT NULL,
    client_key TEXT NOT NULL
        CHECK (char_length(client_key) BETWEEN 8 AND 120
               AND client_key LIKE 'client:%'),
    owner_line_id BIGINT NULL
        CHECK (owner_line_id IS NULL OR owner_line_id > 0),
    PRIMARY KEY (purchase_order_id, client_key),
    FOREIGN KEY (purchase_order_id, org_id)
        REFERENCES {{schema}}.purchase_orders (id, org_id)
        ON DELETE CASCADE
);

-- Stabilize live aliases and identities against writes during backfill.
-- Old writers must be stopped at cutover: a binary ignoring this table
-- cannot provide the forward ownership guarantee.
DO $alias_backfill$
DECLARE
    sess RECORD;
    elem JSONB;
    alias_key TEXT;
    id_elem JSONB;
    id_numeric NUMERIC;
    owner_count INTEGER;
    conflict RECORD;
BEGIN
    LOCK TABLE {{schema}}.purchase_orders,
               {{schema}}.purchase_order_items,
               {{schema}}.purchase_order_reconciliation_sessions
        IN SHARE ROW EXCLUSIVE MODE;

    CREATE TEMPORARY TABLE alias_evidence (
        org_id UUID NOT NULL,
        purchase_order_id BIGINT NOT NULL,
        client_key TEXT NOT NULL,
        owner_line_id BIGINT NULL
    ) ON COMMIT DROP;

    -- Source 1: every live 042 row alias with its own row as owner.
    INSERT INTO alias_evidence (org_id, purchase_order_id, client_key, owner_line_id)
    SELECT po.org_id, i.purchase_order_id, i.client_key, i.id
    FROM {{schema}}.purchase_order_items i
    JOIN {{schema}}.purchase_orders po ON po.id = i.purchase_order_id
    WHERE i.client_key IS NOT NULL;

    -- Source 2: every retained client-key session identity, across all
    -- sessions and generations, removed or not. Read canonical persisted
    -- Key/ID members only; display order, descriptions and id: strings are
    -- never aliases.
    FOR sess IN
        SELECT s.purchase_order_id AS po_id, po.org_id AS org_id,
               s.line_identities AS identities
        FROM {{schema}}.purchase_order_reconciliation_sessions s
        JOIN {{schema}}.purchase_orders po ON po.id = s.purchase_order_id
    LOOP
        FOR elem IN
            SELECT * FROM jsonb_array_elements(sess.identities)
        LOOP
            alias_key := elem->>'Key';
            IF alias_key IS NULL OR alias_key NOT LIKE 'client:%' THEN
                CONTINUE;
            END IF;
            IF char_length(alias_key) NOT BETWEEN 8 AND 120 THEN
                RAISE EXCEPTION
                    'purchase order % has malformed retained alias (not recoverable as ownership): %',
                    sess.po_id, left(alias_key, 120)
                    USING ERRCODE = '23514';
            END IF;
            id_elem := elem->'ID';
            IF id_elem IS NULL OR id_elem = 'null'::jsonb THEN
                INSERT INTO alias_evidence (org_id, purchase_order_id, client_key, owner_line_id)
                VALUES (sess.org_id, sess.po_id, alias_key, NULL);
            ELSIF jsonb_typeof(id_elem) = 'number' THEN
                BEGIN
                    id_numeric := (id_elem)::text::numeric;
                EXCEPTION WHEN OTHERS THEN
                    RAISE EXCEPTION
                        'purchase order % has malformed retained alias owner for % (not recoverable as ownership)',
                        sess.po_id, left(alias_key, 120)
                        USING ERRCODE = '23514';
                END;
                IF trunc(id_numeric) <> id_numeric OR id_numeric < 1 OR id_numeric > 9223372036854775807 THEN
                    RAISE EXCEPTION
                        'purchase order % has malformed retained alias owner for % (not recoverable as ownership)',
                        sess.po_id, left(alias_key, 120)
                        USING ERRCODE = '23514';
                END IF;
                INSERT INTO alias_evidence (org_id, purchase_order_id, client_key, owner_line_id)
                VALUES (sess.org_id, sess.po_id, alias_key, id_numeric::bigint);
            ELSE
                RAISE EXCEPTION
                    'purchase order % has malformed retained alias owner for % (not recoverable as ownership)',
                    sess.po_id, left(alias_key, 120)
                    USING ERRCODE = '23514';
            END IF;
        END LOOP;
    END LOOP;

    -- A positive owner recorded under a different PO than the claim is
    -- inconsistent backfill evidence, never a cross-PO binding.
    FOR conflict IN
        SELECT e.purchase_order_id AS po_id, e.client_key AS alias_key,
               e.owner_line_id AS owner_id
        FROM alias_evidence e
        WHERE e.owner_line_id IS NOT NULL
          AND EXISTS (
              SELECT 1 FROM {{schema}}.purchase_order_items i
              WHERE i.id = e.owner_line_id AND i.purchase_order_id <> e.purchase_order_id
          )
    LOOP
        RAISE EXCEPTION
            'purchase order % claims alias % owned by row % of another purchase order (not recoverable as ownership)',
            conflict.po_id, left(conflict.alias_key, 120), conflict.owner_id
            USING ERRCODE = '23514';
    END LOOP;

    -- Competing positive owners for one alias abort with a diagnostic. One
    -- positive binding wins over unbound snapshots; unbound-only aliases
    -- become NULL reservations below.
    FOR conflict IN
        SELECT purchase_order_id AS po_id, client_key AS alias_key,
               array_agg(DISTINCT owner_line_id) AS owners
        FROM alias_evidence
        WHERE owner_line_id IS NOT NULL
        GROUP BY purchase_order_id, client_key
        HAVING COUNT(DISTINCT owner_line_id) > 1
    LOOP
        RAISE EXCEPTION
            'purchase order % has competing owners % for alias % (resolve with evidence, not automatic cleanup)',
            conflict.po_id, conflict.owners, left(conflict.alias_key, 120)
            USING ERRCODE = '23514';
    END LOOP;

    INSERT INTO {{schema}}.purchase_order_line_aliases
        (org_id, purchase_order_id, client_key, owner_line_id)
    SELECT DISTINCT e.org_id, e.purchase_order_id, e.client_key, e.owner_line_id
    FROM alias_evidence e
    WHERE e.owner_line_id IS NOT NULL;

    INSERT INTO {{schema}}.purchase_order_line_aliases
        (org_id, purchase_order_id, client_key, owner_line_id)
    SELECT DISTINCT e.org_id, e.purchase_order_id, e.client_key, NULL::bigint
    FROM alias_evidence e
    WHERE e.owner_line_id IS NULL
      AND NOT EXISTS (
          SELECT 1 FROM {{schema}}.purchase_order_line_aliases c
          WHERE c.purchase_order_id = e.purchase_order_id
            AND c.client_key = e.client_key
      );
END
$alias_backfill$;

-- Claim ownership is append-only. UPDATE is always refused; DELETE is
-- refused while the owning PO still exists and allowed only for the PO's
-- own physical cascade (parent already gone), following 041's guard
-- pattern. Inserts and repeated INSERT ... ON CONFLICT DO NOTHING are
-- allowed; the store compares the stored owner afterward and never uses
-- ON CONFLICT DO UPDATE to change ownership.
CREATE OR REPLACE FUNCTION {{schema}}.purchase_order_line_alias_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $alias_guard$
DECLARE
    parent_exists BOOLEAN;
BEGIN
    IF TG_OP = 'UPDATE' THEN
        RAISE EXCEPTION
            'purchase order line alias ownership is append-only'
            USING ERRCODE = '23514';
    END IF;
    SELECT EXISTS (
        SELECT 1 FROM {{schema}}.purchase_orders WHERE id = OLD.purchase_order_id
    ) INTO parent_exists;
    IF parent_exists THEN
        RAISE EXCEPTION
            'purchase order line alias % of purchase order % is retained and cannot be deleted',
            left(OLD.client_key, 120), OLD.purchase_order_id
            USING ERRCODE = '23514';
    END IF;
    RETURN OLD;
END;
$alias_guard$;

DROP TRIGGER IF EXISTS purchase_order_line_alias_guard_trigger
    ON {{schema}}.purchase_order_line_aliases;
CREATE TRIGGER purchase_order_line_alias_guard_trigger
    BEFORE UPDATE OR DELETE ON {{schema}}.purchase_order_line_aliases
    FOR EACH ROW
    EXECUTE FUNCTION {{schema}}.purchase_order_line_alias_guard();
