-- 040_drop_purchase_order_extractions: the extraction feature is deleted
-- (issue #504, spec #503).
--
-- 038_purchase_order_extractions created the table and stays in the repository
-- as applied history: a database that has run it must not be rewritten, and the
-- migration ledger must keep matching the files that were actually applied.
-- This forward migration is what removes the table, so an existing database and
-- a freshly migrated one end in the same state.
--
-- IF EXISTS keeps a development schema that never applied 038 (or was repaired
-- by hand) from aborting the migration.

DROP TABLE IF EXISTS {{schema}}.purchase_order_extractions;
