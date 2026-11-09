-- 034_po_confirmation_numbers: the optional customer-system confirmation
-- reference for a saved Purchase Order (issue #474).
--
-- Staff may submit a PO to the customer's system under a Vera-issued reference.
-- The reference is optional: an unassigned record stores null and consumes no
-- sequence, so customers that do not require one never touch the counter.
--
-- # Number contract
--
-- Format: "23" + 2-digit customer code + 2-digit assignment year + sequence.
-- It is the quotation numbering contract with one substitution: the "23" prefix
-- replaces "15". The sequence starts at 0001, warns at 8000, widens to five
-- digits past 9999 instead of wrapping, and refuses at 99999. The counter lives
-- in sequence_counters under its own "po-confirmation:" namespace, so issuing a
-- Quotation never consumes a confirmation number and each January starts a
-- fresh customer cycle. The assignment year is the year in WIB, the company's
-- commercial date context (backend/store/company_date.go); the received date
-- does not decide it.
--
-- # Optional, immutable, never reused
--
-- All six columns are null together or set together. A confirmation_number may
-- only exist on a saved record. The trigger below refuses any change once a
-- number is present, so the store (and any future writer) cannot edit, clear or
-- regenerate it; allocation is the only null -> value transition, and the
-- store's idempotent assignment returns the existing value rather than writing
-- a second one.
--
-- Deleting a Purchase Order removes the row (#480 owns that path) but never
-- decrements the counter, so an issued number is never reissued. The two unique
-- indexes are backstops: one identifier per organization, and one sequence per
-- customer-year cycle, so a bug that bypassed the counter fails loudly.
ALTER TABLE {{schema}}.purchase_orders
    ADD COLUMN IF NOT EXISTS confirmation_number TEXT NULL,
    ADD COLUMN IF NOT EXISTS confirmation_customer_code SMALLINT NULL,
    ADD COLUMN IF NOT EXISTS confirmation_year SMALLINT NULL,
    ADD COLUMN IF NOT EXISTS confirmation_sequence BIGINT NULL,
    ADD COLUMN IF NOT EXISTS confirmation_assigned_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS confirmation_assigned_by BIGINT NULL
        REFERENCES {{schema}}.users(id);

ALTER TABLE {{schema}}.purchase_orders
    ADD CONSTRAINT purchase_orders_confirmation_shape_check CHECK (
        (
            confirmation_number IS NULL
            AND confirmation_customer_code IS NULL
            AND confirmation_year IS NULL
            AND confirmation_sequence IS NULL
            AND confirmation_assigned_at IS NULL
            AND confirmation_assigned_by IS NULL
        )
        OR (
            confirmation_number IS NOT NULL
            AND btrim(confirmation_number) <> ''
            AND confirmation_customer_code IS NOT NULL
            AND confirmation_year IS NOT NULL
            AND confirmation_sequence IS NOT NULL
            AND confirmation_assigned_at IS NOT NULL
            AND confirmation_assigned_by IS NOT NULL
        )
    );

-- The number is only meaningful on a finalized record.
ALTER TABLE {{schema}}.purchase_orders
    ADD CONSTRAINT purchase_orders_confirmation_saved_check CHECK (
        confirmation_number IS NULL OR state = 'saved'
    );

-- The format is part of the stored contract, not a rendering preference. Four
-- digits through 9999, five from 10000 to the 99999 bound.
ALTER TABLE {{schema}}.purchase_orders
    ADD CONSTRAINT purchase_orders_confirmation_format_check CHECK (
        confirmation_number IS NULL
        OR confirmation_number ~ '^23[0-9]{2}[0-9]{2}[0-9]{4,5}$'
    );

ALTER TABLE {{schema}}.purchase_orders
    ADD CONSTRAINT purchase_orders_confirmation_scope_check CHECK (
        confirmation_customer_code IS NULL
        OR (confirmation_customer_code BETWEEN 1 AND 99
            AND confirmation_year BETWEEN 0 AND 99
            AND confirmation_sequence BETWEEN 1 AND 99999)
    );

CREATE UNIQUE INDEX IF NOT EXISTS purchase_orders_confirmation_number_idx
    ON {{schema}}.purchase_orders (org_id, confirmation_number)
    WHERE confirmation_number IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS purchase_orders_confirmation_cycle_idx
    ON {{schema}}.purchase_orders (
        org_id, confirmation_customer_code, confirmation_year, confirmation_sequence)
    WHERE confirmation_number IS NOT NULL;

-- Immutability is enforced where every writer must pass: the row. The store
-- exposes no edit or clear operation, and this trigger makes that a property of
-- the data rather than a convention future code can forget.
CREATE OR REPLACE FUNCTION {{schema}}.purchase_orders_confirmation_immutable()
RETURNS trigger
LANGUAGE plpgsql
AS $confirmation_guard$
BEGIN
    IF OLD.confirmation_number IS NOT NULL AND (
        NEW.confirmation_number IS DISTINCT FROM OLD.confirmation_number
        OR NEW.confirmation_customer_code IS DISTINCT FROM OLD.confirmation_customer_code
        OR NEW.confirmation_year IS DISTINCT FROM OLD.confirmation_year
        OR NEW.confirmation_sequence IS DISTINCT FROM OLD.confirmation_sequence
        OR NEW.confirmation_assigned_at IS DISTINCT FROM OLD.confirmation_assigned_at
        OR NEW.confirmation_assigned_by IS DISTINCT FROM OLD.confirmation_assigned_by
    ) THEN
        RAISE EXCEPTION
            'purchase order % already has confirmation number %; an issued number is immutable',
            OLD.id, OLD.confirmation_number
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$confirmation_guard$;

DROP TRIGGER IF EXISTS purchase_orders_confirmation_immutable_trigger
    ON {{schema}}.purchase_orders;
CREATE TRIGGER purchase_orders_confirmation_immutable_trigger
    BEFORE UPDATE ON {{schema}}.purchase_orders
    FOR EACH ROW
    EXECUTE FUNCTION {{schema}}.purchase_orders_confirmation_immutable();
