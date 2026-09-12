-- Repair migration for databases that applied 002_auth.sql before Google
-- sign-in support was removed (per director decision). Fresh installs never
-- need this: 002 already creates the password-only schema. Every statement
-- is idempotent.
ALTER TABLE {{schema}}.users DROP COLUMN IF EXISTS google_sub;

-- Drop the old either/or CHECK if it exists (auto-named by Postgres).
DO $$
DECLARE
    constraint_name TEXT;
BEGIN
    FOR constraint_name IN
        SELECT conname FROM pg_constraint
        WHERE conrelid = '{{schema}}.users'::regclass AND contype = 'c'
    LOOP
        IF pg_get_constraintdef((SELECT oid FROM pg_constraint WHERE conname = constraint_name AND conrelid = '{{schema}}.users'::regclass)) LIKE '%google_sub%' THEN
            EXECUTE 'ALTER TABLE {{schema}}.users DROP CONSTRAINT "' || REPLACE(constraint_name, '"', '""') || '"';
        END IF;
    END LOOP;
END
$$;

-- Email/password is now the only login method, so a hash is mandatory.
ALTER TABLE {{schema}}.users ALTER COLUMN password_hash SET NOT NULL;
