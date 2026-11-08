-- 044_po_reconciliation_repair_sessions: the one construction window a saved
-- record's reconciliation history may grow through (#502, ticket 06 / #518 B1).
--
-- 041 closed the live writer on a saved record: a read-only session refuses
-- every update, an item cannot be added under a read-only parent or a saved
-- record, and a hand-written UPDATE fails with 23514. That is the guarantee
-- #517 was accepted on and this migration keeps it byte for byte.
--
-- A source repair is the one operation that legitimately *adds* history: it
-- deactivates the previous generation and installs a new terminal one that
-- records what this repair compared and what it decided. 041's item guard makes
-- that impossible through the generic writer, and it must stay impossible:
-- flipping the PO back to draft, disabling a trigger, adding an
-- `allow_saved = true` flag or a session-level GUC would reopen the write path
-- #517 closed.
--
-- So this migration adds a *narrow construction window* and nothing else:
--
--   1. `purchase_order_reconciliation_sessions.repair_correction_id` links a
--      session to the actual repair that created it. One session per repair
--      (partial unique index); null for create/link sessions and for every
--      session written before this migration.
--   2. `purchase_order_source_repairs.reconciliation_seed_xid` is the repair's
--      own transaction id. It is added nullable **without** a default first and
--      only then given `DEFAULT pg_current_xact_id()`, so old repair rows stay
--      NULL and are never retro-linked by guesswork. A stamp can only be
--      satisfied by the transaction that inserted the source-repair row.
--   3. A shared predicate function decides whether a (PO, org, correction,
--      source pair, version, actor) tuple is a genuine repair seed: the saved
--      record's version is the correction's resulting version, the correction
--      is a `source_repair` with a reason, the source-repair satellite names
--      the same new pair, the actor is the correction's actor and is an active
--      admin, and the satellite row was stamped by *this* transaction.
--   4. The session INSERT guard requires that predicate for a session inserted
--      under a saved record or already read-only, and the item INSERT guard
--      requires it (plus actor equality and an already-terminal item) for items
--      added under a read-only session or a saved record.
--
-- Every other refusal is unchanged: item UPDATE/DELETE/retarget, session
-- UPDATE/DELETE, source-pointer clearing, and deactivation-only maintenance.
-- There is no exception for UPDATEs, no post-commit hole (the stamp stops
-- matching when the transaction ends), and no backfill.
--
-- The two foreign keys are deferrable and initially deferred so the existing
-- whole-PO cascade can delete both the correction chain and the session chain
-- in either trigger order, while a direct delete of a linked audit row that
-- outlives its session still fails.

-- 1. The repair link on a session.
ALTER TABLE {{schema}}.purchase_order_reconciliation_sessions
    ADD COLUMN IF NOT EXISTS repair_correction_id BIGINT NULL;

-- One installed session per actual repair. NULL is the normal case for every
-- create/link session, so the index is partial.
CREATE UNIQUE INDEX IF NOT EXISTS purchase_order_reconciliation_repair_idx
    ON {{schema}}.purchase_order_reconciliation_sessions (repair_correction_id)
    WHERE repair_correction_id IS NOT NULL;

-- 2. Composite identity for the deferred foreign key to the correction.
CREATE UNIQUE INDEX IF NOT EXISTS purchase_order_corrections_identity_idx
    ON {{schema}}.purchase_order_corrections (id, org_id, purchase_order_id);

-- 3. The repair transaction stamp. Nullable first so existing repair rows keep
-- NULL and no historical row is retro-linked to a transaction it never had.
ALTER TABLE {{schema}}.purchase_order_source_repairs
    ADD COLUMN IF NOT EXISTS reconciliation_seed_xid xid8 NULL;

ALTER TABLE {{schema}}.purchase_order_source_repairs
    ALTER COLUMN reconciliation_seed_xid SET DEFAULT pg_current_xact_id();

-- 4. Deferred foreign keys. MATCH SIMPLE means a NULL repair_correction_id
-- skips both, which is exactly the ordinary create/link session.
ALTER TABLE {{schema}}.purchase_order_reconciliation_sessions
    DROP CONSTRAINT IF EXISTS purchase_order_reconciliation_repair_correction_fk;
ALTER TABLE {{schema}}.purchase_order_reconciliation_sessions
    ADD CONSTRAINT purchase_order_reconciliation_repair_correction_fk
    FOREIGN KEY (repair_correction_id, org_id, purchase_order_id)
    REFERENCES {{schema}}.purchase_order_corrections (id, org_id, purchase_order_id)
    DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE {{schema}}.purchase_order_reconciliation_sessions
    DROP CONSTRAINT IF EXISTS purchase_order_reconciliation_repair_audit_fk;
ALTER TABLE {{schema}}.purchase_order_reconciliation_sessions
    ADD CONSTRAINT purchase_order_reconciliation_repair_audit_fk
    FOREIGN KEY (repair_correction_id)
    REFERENCES {{schema}}.purchase_order_source_repairs (correction_id)
    DEFERRABLE INITIALLY DEFERRED;

-- 5. The shared predicate. It answers one question: is this row being written
-- by the repair that is producing this exact saved version, in this exact
-- transaction? It is deliberately strict and returns FALSE (never NULL) so a
-- caller's `IF NOT ...` cannot be silently bypassed.
CREATE OR REPLACE FUNCTION {{schema}}.purchase_order_reconciliation_repair_seed_ok(
    p_po_id BIGINT,
    p_org_id UUID,
    p_correction_id BIGINT,
    p_quotation_id BIGINT,
    p_revision SMALLINT,
    p_revision_id BIGINT,
    p_po_version BIGINT,
    p_actor_id BIGINT
)
RETURNS BOOLEAN
LANGUAGE plpgsql
AS $reconciliation_repair_seed$
DECLARE
    ok BOOLEAN;
BEGIN
    -- A null link, actor or version can never be a genuine seed.
    IF p_correction_id IS NULL OR p_actor_id IS NULL OR p_revision_id IS NULL THEN
        RETURN FALSE;
    END IF;

    SELECT EXISTS (
        SELECT 1
        FROM {{schema}}.purchase_orders po
        JOIN {{schema}}.purchase_order_corrections c
          ON c.id = p_correction_id
         AND c.org_id = p_org_id
         AND c.purchase_order_id = p_po_id
        JOIN {{schema}}.purchase_order_source_repairs sr
          ON sr.correction_id = c.id
        JOIN {{schema}}.users u
          ON u.id = p_actor_id
        WHERE po.id = p_po_id
          AND po.org_id = p_org_id
          AND po.state = 'saved'
          AND c.kind = 'source_repair'
          AND c.reason IS NOT NULL AND btrim(c.reason) <> ''
          AND c.actor = p_actor_id
          AND u.role = 'admin' AND u.is_active
          -- One resulting version, one repair: the session records the version
          -- the repair actually produced.
          AND c.resulting_version = p_po_version
          AND po.version = p_po_version
          -- The repair's new pair is the record's current pair and the
          -- session's pair, and the pointer names that exact live revision.
          AND sr.new_quotation_id = p_quotation_id
          AND sr.new_source_revision = p_revision
          AND po.quotation_id = p_quotation_id
          AND po.source_revision = p_revision
          AND EXISTS (
              SELECT 1 FROM {{schema}}.quotation_revisions r
              WHERE r.id = p_revision_id
                AND r.quotation_id = p_quotation_id
                AND r.revision_number = p_revision
          )
          -- The construction window is this transaction and no other. The
          -- stamp is written by the source-repair INSERT above it; after commit
          -- it can never equal a later pg_current_xact_id().
          AND sr.reconciliation_seed_xid IS NOT NULL
          AND sr.reconciliation_seed_xid = pg_current_xact_id()
    ) INTO ok;

    IF NOT COALESCE(ok, FALSE) THEN
        RETURN FALSE;
    END IF;
    RETURN TRUE;
END;
$reconciliation_repair_seed$;

-- 6. Session guard. Everything 041 enforced stays; the INSERT branch gains the
-- repair-seed requirement for a saved/read-only parent, and the UPDATE branch
-- makes the repair link immutable.
CREATE OR REPLACE FUNCTION {{schema}}.purchase_order_reconciliation_session_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $reconciliation_guard$
DECLARE
    po_state TEXT;
    parent_saved BOOLEAN;
BEGIN
    IF TG_OP = 'INSERT' THEN
        -- The source pair must name a real revision, even when the revision
        -- pointer is NULL (a nullable composite FK skips validation).
        IF NOT EXISTS (
            SELECT 1 FROM {{schema}}.quotation_revisions
            WHERE quotation_id = NEW.source_quotation_id
              AND revision_number = NEW.source_revision
        ) THEN
            RAISE EXCEPTION
                'source revision %/% does not exist', NEW.source_quotation_id, NEW.source_revision
                USING ERRCODE = '23514';
        END IF;
        -- 044: a session born read-only, or attached to a saved record, must be
        -- the terminal session a genuine repair is installing right now.
        SELECT state = 'saved' INTO parent_saved
        FROM {{schema}}.purchase_orders WHERE id = NEW.purchase_order_id;
        IF NEW.readonly OR COALESCE(parent_saved, FALSE) THEN
            IF NOT {{schema}}.purchase_order_reconciliation_repair_seed_ok(
                NEW.purchase_order_id, NEW.org_id, NEW.repair_correction_id,
                NEW.source_quotation_id, NEW.source_revision, NEW.source_revision_id,
                NEW.po_version, NEW.created_by) THEN
                RAISE EXCEPTION
                    'purchase order % is saved; reconciliation session % may only be installed by the source repair that produced version %',
                    NEW.purchase_order_id, NEW.id, NEW.po_version
                    USING ERRCODE = '23514';
            END IF;
            IF NOT NEW.active OR NOT NEW.readonly THEN
                RAISE EXCEPTION
                    'a repair session must be installed active and read-only'
                    USING ERRCODE = '23514';
            END IF;
        END IF;
        RETURN NEW;
    END IF;
    IF TG_OP = 'UPDATE'
       AND (NEW.source_quotation_id IS DISTINCT FROM OLD.source_quotation_id
            OR NEW.source_revision IS DISTINCT FROM OLD.source_revision)
       AND NOT EXISTS (
           SELECT 1 FROM {{schema}}.quotation_revisions
           WHERE quotation_id = NEW.source_quotation_id
             AND revision_number = NEW.source_revision
       ) THEN
        RAISE EXCEPTION
            'source revision %/% does not exist', NEW.source_quotation_id, NEW.source_revision
            USING ERRCODE = '23514';
    END IF;
    -- 044: the repair link is construction-time evidence and never moves, on
    -- any row, in any state.
    IF TG_OP = 'UPDATE'
       AND NEW.repair_correction_id IS DISTINCT FROM OLD.repair_correction_id THEN
        RAISE EXCEPTION
            'purchase order reconciliation session % is immutable; its repair link cannot be changed',
            OLD.id
            USING ERRCODE = '23514';
    END IF;
    IF TG_OP = 'DELETE' THEN
        -- Whole-chain PO deletion cascades here after the parent row is
        -- gone; that is the only delete path that may remove a session.
        SELECT state INTO po_state FROM {{schema}}.purchase_orders WHERE id = OLD.purchase_order_id;
        IF NOT FOUND THEN
            RETURN OLD;
        END IF;
        IF OLD.readonly THEN
            RAISE EXCEPTION
                'purchase order reconciliation session % is read-only; it cannot be deleted',
                OLD.id
                USING ERRCODE = '23514';
        END IF;
        IF po_state = 'saved' THEN
            RAISE EXCEPTION
                'purchase order % is saved; its reconciliation session % cannot be deleted',
                OLD.purchase_order_id, OLD.id
                USING ERRCODE = '23514';
        END IF;
        RETURN OLD;
    END IF;
    IF OLD.readonly THEN
        -- Two maintenance paths are allowed on a read-only session:
        --   * source deletion clears the revision pointer (FK maintenance);
        --   * the session is deactivated so a repair can install a new active
        --     generation. Content, attribution and po_version stay frozen.
        IF NEW.source_revision_id IS DISTINCT FROM OLD.source_revision_id
           AND NEW.source_revision_id IS NOT NULL THEN
            RAISE EXCEPTION
                'purchase order reconciliation session % is read-only; its source revision may only be cleared',
                OLD.id
                USING ERRCODE = '23514';
        END IF;
        IF NEW.org_id IS NOT DISTINCT FROM OLD.org_id
           AND NEW.purchase_order_id IS NOT DISTINCT FROM OLD.purchase_order_id
           AND NEW.source_quotation_id IS NOT DISTINCT FROM OLD.source_quotation_id
           AND NEW.source_revision IS NOT DISTINCT FROM OLD.source_revision
           AND NEW.generation IS NOT DISTINCT FROM OLD.generation
           AND NEW.readonly IS NOT DISTINCT FROM OLD.readonly
           AND NEW.po_version IS NOT DISTINCT FROM OLD.po_version
           AND NEW.line_identities IS NOT DISTINCT FROM OLD.line_identities
           AND NEW.created_by IS NOT DISTINCT FROM OLD.created_by
           AND NEW.created_at IS NOT DISTINCT FROM OLD.created_at
           AND (
                (OLD.source_revision_id IS NOT NULL
                 AND NEW.source_revision_id IS NULL
                 AND NEW.active IS NOT DISTINCT FROM OLD.active)
                OR (NEW.source_revision_id IS NOT DISTINCT FROM OLD.source_revision_id
                    AND OLD.active AND NOT NEW.active)
           ) THEN
            NEW.updated_at := NOW();
            RETURN NEW;
        END IF;
        RAISE EXCEPTION
            'purchase order reconciliation session % is read-only; record a new session instead',
            OLD.id
            USING ERRCODE = '23514';
    END IF;
    SELECT state INTO po_state FROM {{schema}}.purchase_orders WHERE id = OLD.purchase_order_id;
    IF FOUND AND po_state = 'saved' THEN
        -- Same allowances for sessions of saved records: clear the revision
        -- pointer only, or deactivate for a repair.
        IF NEW.source_revision_id IS DISTINCT FROM OLD.source_revision_id
           AND NEW.source_revision_id IS NOT NULL THEN
            RAISE EXCEPTION
                'purchase order % is saved; its reconciliation session % may only clear its source revision',
                OLD.purchase_order_id, OLD.id
                USING ERRCODE = '23514';
        END IF;
        IF NEW.org_id IS NOT DISTINCT FROM OLD.org_id
           AND NEW.purchase_order_id IS NOT DISTINCT FROM OLD.purchase_order_id
           AND NEW.source_quotation_id IS NOT DISTINCT FROM OLD.source_quotation_id
           AND NEW.source_revision IS NOT DISTINCT FROM OLD.source_revision
           AND NEW.generation IS NOT DISTINCT FROM OLD.generation
           AND NEW.readonly IS NOT DISTINCT FROM OLD.readonly
           AND NEW.po_version IS NOT DISTINCT FROM OLD.po_version
           AND NEW.line_identities IS NOT DISTINCT FROM OLD.line_identities
           AND NEW.created_by IS NOT DISTINCT FROM OLD.created_by
           AND NEW.created_at IS NOT DISTINCT FROM OLD.created_at
           AND (
                (OLD.source_revision_id IS NOT NULL
                 AND NEW.source_revision_id IS NULL
                 AND NEW.active IS NOT DISTINCT FROM OLD.active)
                OR (NEW.source_revision_id IS NOT DISTINCT FROM OLD.source_revision_id
                    AND OLD.active AND NOT NEW.active)
           ) THEN
            NEW.updated_at := NOW();
            RETURN NEW;
        END IF;
        RAISE EXCEPTION
            'purchase order % is saved; its reconciliation session % is immutable',
            OLD.purchase_order_id, OLD.id
            USING ERRCODE = '23514';
    END IF;
    NEW.updated_at := NOW();
    RETURN NEW;
END;
$reconciliation_guard$;

DROP TRIGGER IF EXISTS purchase_order_reconciliation_sessions_guard_trigger
    ON {{schema}}.purchase_order_reconciliation_sessions;
CREATE TRIGGER purchase_order_reconciliation_sessions_guard_trigger
    BEFORE INSERT OR UPDATE OR DELETE ON {{schema}}.purchase_order_reconciliation_sessions
    FOR EACH ROW
    EXECUTE FUNCTION {{schema}}.purchase_order_reconciliation_session_guard();

-- 7. Item guard. UPDATE/DELETE/retarget refusals stay exactly as 041 wrote
-- them. The INSERT branch keeps its two refusals but opens them for a genuine
-- repair seed, and only for an item that is already terminal and written by the
-- session's own actor.
CREATE OR REPLACE FUNCTION {{schema}}.purchase_order_reconciliation_item_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $reconciliation_item_guard$
DECLARE
    session_readonly BOOLEAN;
    session_found BOOLEAN;
    session_created_by BIGINT;
    session_repair_id BIGINT;
    session_quotation_id BIGINT;
    session_revision SMALLINT;
    session_revision_id BIGINT;
    session_po_version BIGINT;
    po_state TEXT;
    po_found BOOLEAN;
    seed_ok BOOLEAN;
BEGIN
    IF TG_OP = 'INSERT' THEN
        SELECT s.readonly, s.created_by, s.repair_correction_id,
               s.source_quotation_id, s.source_revision, s.source_revision_id,
               s.po_version
          INTO session_readonly, session_created_by, session_repair_id,
               session_quotation_id, session_revision, session_revision_id,
               session_po_version
        FROM {{schema}}.purchase_order_reconciliation_sessions s
        WHERE s.id = NEW.session_id AND s.org_id = NEW.org_id;
        session_found := FOUND;
        SELECT state INTO po_state FROM {{schema}}.purchase_orders WHERE id = NEW.purchase_order_id;
        po_found := FOUND;
        IF (session_found AND session_readonly) OR (po_found AND po_state = 'saved') THEN
            seed_ok := FALSE;
            IF session_found THEN
                seed_ok := {{schema}}.purchase_order_reconciliation_repair_seed_ok(
                    NEW.purchase_order_id, NEW.org_id, session_repair_id,
                    session_quotation_id, session_revision, session_revision_id,
                    session_po_version, session_created_by);
            END IF;
            IF NOT COALESCE(seed_ok, FALSE) THEN
                IF session_found AND session_readonly THEN
                    RAISE EXCEPTION
                        'purchase order reconciliation session % is read-only; it cannot gain items',
                        NEW.session_id
                        USING ERRCODE = '23514';
                END IF;
                RAISE EXCEPTION
                    'purchase order % is saved; its reconciliation session cannot gain items',
                    NEW.purchase_order_id
                    USING ERRCODE = '23514';
            END IF;
            -- The seed authorises construction, not authorship or editing.
            IF NEW.actor_id IS DISTINCT FROM session_created_by THEN
                RAISE EXCEPTION
                    'a repair session may only be seeded by the actor who performed the repair'
                    USING ERRCODE = '23514';
            END IF;
            IF NEW.status <> 'resolved' OR NEW.resolution IS NULL THEN
                RAISE EXCEPTION
                    'a repair session may only be seeded with resolved items'
                    USING ERRCODE = '23514';
            END IF;
        END IF;
        RETURN NEW;
    END IF;
    IF TG_OP = 'DELETE' THEN
        SELECT s.readonly INTO session_readonly
        FROM {{schema}}.purchase_order_reconciliation_sessions s WHERE s.id = OLD.session_id;
        session_found := FOUND;
        SELECT state INTO po_state FROM {{schema}}.purchase_orders WHERE id = OLD.purchase_order_id;
        po_found := FOUND;
        -- Cascade from a deleted PO or session arrives after its parent is
        -- gone; that is the only delete path that may remove an item.
        IF NOT po_found THEN
            RETURN OLD;
        END IF;
        IF session_found AND session_readonly THEN
            RAISE EXCEPTION
                'purchase order reconciliation item % is read-only; it cannot be deleted',
                OLD.id
                USING ERRCODE = '23514';
        END IF;
        IF po_state = 'saved' THEN
            RAISE EXCEPTION
                'purchase order % is saved; its reconciliation item % cannot be deleted',
                OLD.purchase_order_id, OLD.id
                USING ERRCODE = '23514';
        END IF;
        RETURN OLD;
    END IF;
    -- An UPDATE can retarget the row at another session/PO. The destination
    -- must be mutable even when the old parent was.
    SELECT s.readonly INTO session_readonly
    FROM {{schema}}.purchase_order_reconciliation_sessions s WHERE s.id = NEW.session_id;
    IF FOUND AND session_readonly THEN
        RAISE EXCEPTION
            'purchase order reconciliation session % is read-only; it cannot gain items',
            NEW.session_id
            USING ERRCODE = '23514';
    END IF;
    SELECT state INTO po_state FROM {{schema}}.purchase_orders WHERE id = NEW.purchase_order_id;
    IF FOUND AND po_state = 'saved' THEN
        RAISE EXCEPTION
            'purchase order % is saved; its reconciliation session cannot gain items',
            NEW.purchase_order_id
            USING ERRCODE = '23514';
    END IF;
    SELECT s.readonly INTO session_readonly
    FROM {{schema}}.purchase_order_reconciliation_sessions s WHERE s.id = OLD.session_id;
    IF FOUND AND session_readonly THEN
        RAISE EXCEPTION
            'purchase order reconciliation item % is read-only; record a new decision instead',
            OLD.id
            USING ERRCODE = '23514';
    END IF;
    SELECT state INTO po_state FROM {{schema}}.purchase_orders WHERE id = OLD.purchase_order_id;
    IF FOUND AND po_state = 'saved' THEN
        RAISE EXCEPTION
            'purchase order % is saved; its reconciliation item % is immutable',
            OLD.purchase_order_id, OLD.id
            USING ERRCODE = '23514';
    END IF;
    NEW.updated_at := NOW();
    RETURN NEW;
END;
$reconciliation_item_guard$;

DROP TRIGGER IF EXISTS purchase_order_reconciliation_items_guard_trigger
    ON {{schema}}.purchase_order_reconciliation_items;
CREATE TRIGGER purchase_order_reconciliation_items_guard_trigger
    BEFORE INSERT OR UPDATE OR DELETE ON {{schema}}.purchase_order_reconciliation_items
    FOR EACH ROW
    EXECUTE FUNCTION {{schema}}.purchase_order_reconciliation_item_guard();
