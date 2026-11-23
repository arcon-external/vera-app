-- Purchase-order allocation cutover (#658): destructive-rollback precheck.
--
-- Migrations are forward-only: `backend/store/migrations.go` applies each file
-- once and has no down path, so a `_down` file cannot ship -- it would be
-- replayed as a new forward migration. The supported rollback is a SERVER
-- rollback (the newer schema is additive and an older server ignores the new
-- columns) plus, per purchase order, another audited correction. A destructive
-- schema rollback is a last resort that needs a new, hand-written forward
-- migration.
--
-- That last resort cannot simply restore migration 053's state, because 053
-- dropped the one-purchase-order-per-quotation unique index. Before that index
-- can exist again, every quotation must have at most one stored purchase order.
-- This query is the guard: it returns one row per quotation that does not.
--
-- Usage (the runbook's command):
--
--   psql "$VERA_POSTGRES_DSN" -v schema=vera \
--     -f docs/agents/allocation-rollback-precheck.sql
--
-- Every returned row is a refusal: resolve it (delete the duplicate draft, or
-- correct/retire the extra record) and run the query again. Zero rows is the
-- only answer that permits a destructive rollback. Drafts count as live: a
-- draft reserves the quotation just as a saved record does, and the unique
-- index does not distinguish them.
SELECT q.id AS quotation_id,
       q.quote_number,
       COUNT(po.id) AS live_purchase_orders
FROM :"schema".quotations q
JOIN :"schema".purchase_orders po ON po.quotation_id = q.id
GROUP BY q.id, q.quote_number
HAVING COUNT(po.id) > 1
ORDER BY q.id;
