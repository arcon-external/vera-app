-- 026_company_uom: the units a quotation may carry are the company's own list.
--
-- Until now "Lot" was the only unit the backend knew by name: a line with
-- sub-items had to use it by a hardcoded comparison, and every other UoM was
-- whatever the editor's fixed catalog allowed through. That catalog -- the 14
-- values seeded below -- actually lives in the frontend, so nothing on the
-- server could validate "mile" or "each" against anything, or know what "m2"
-- should print as. This migration makes the list a company setting:
--
--   * value is what stores, signs and validates -- the stable key;
--   * label is what prints -- the issue's m2 -> m² case;
--   * sort_order is the display order: the store assigns it from the array
--     position on the wire, and the API never exposes it (the ordered array
--     IS the order contract, so a redundant number on the wire could drift).
--
-- The seeds are today's catalog verbatim with value = label, so every value a
-- quotation could already carry is already on the list, and every quotation
-- written before this migration keeps reading: stored revisions never read
-- this table at all -- they self-snapshot their own UoM column (016/017) --
-- and a value that later leaves the list still prints itself through the
-- fallback in label resolution (see backend/quotation.go's displayData).
--
-- canonical_uom on company_profile replaces the hardcoded "Lot": a package
-- parent is quoted in the canonical unit. The FK is DEFERRABLE INITIALLY
-- DEFERRED on purpose. UpdateCompanyUnits full-replaces by deleting every row
-- and re-inserting the new list inside one transaction, and a non-deferred FK
-- would fire at the end of the DELETE statement -- before the re-insert runs --
-- even though the canonical row is back by commit. Deferred, the database
-- still refuses any transaction that leaves the canonical unit absent: the
-- check happens at commit, where the new list is already in place, so the
-- DB-level existence guarantee is intact, just checked at the moment it can
-- see the whole transaction.
--
-- The FK is also why the canonical unit cannot be deleted out from under the
-- profile: removing it is refused by the database (and by the store's own
-- validation, which checks the new list still contains the current canonical
-- before it deletes anything).
CREATE TABLE {{schema}}.company_uom (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    value TEXT NOT NULL UNIQUE,
    label TEXT NOT NULL,
    sort_order INTEGER NOT NULL
);

INSERT INTO {{schema}}.company_uom (value, label, sort_order) VALUES
    ('Lot', 'Lot', 1),
    ('Pc', 'Pc', 2),
    ('EA', 'EA', 3),
    ('LS', 'LS', 4),
    ('set', 'set', 5),
    ('meter', 'meter', 6),
    ('m2', 'm2', 7),
    ('m3', 'm3', 8),
    ('Hour', 'Hour', 9),
    ('Day', 'Day', 10),
    ('Week', 'Week', 11),
    ('Month', 'Month', 12),
    ('Year', 'Year', 13),
    ('Quarter', 'Quarter', 14)
ON CONFLICT (value) DO NOTHING;

ALTER TABLE {{schema}}.company_profile
    ADD COLUMN IF NOT EXISTS canonical_uom TEXT NOT NULL DEFAULT 'Lot';

-- The seed above runs first, so the existing profile row's DEFAULT satisfies
-- the new FK at creation time. See the file header for why this constraint is
-- DEFERRABLE -- the one deliberate deviation from the house style of plain
-- REFERENCES, and it is the difference between a full-replace that works and
-- one that the database itself breaks.
ALTER TABLE {{schema}}.company_profile
    ADD CONSTRAINT company_profile_canonical_uom_fkey
    FOREIGN KEY (canonical_uom) REFERENCES {{schema}}.company_uom(value)
    DEFERRABLE INITIALLY DEFERRED;