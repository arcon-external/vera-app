-- 031_po_calculations: the explained customer-facing calculation (issue #470).
--
-- 030 stored the entered Purchase Order; this migration adds the arithmetic
-- those entries produce. A PO is a customer document, so only customer-facing
-- figures live here: row and whole-PO discounts, the PPN that will be printed
-- on the order, a signed adjustment with its reason, and the calculated
-- totals. Internal cost, margin, PPH22 and profit are deliberately absent --
-- they must never become a PO comparison or a PO price.
--
-- The calculated figures are stored beside their inputs rather than recomputed
-- on every read, so a stored PO keeps saying what it said even if the
-- calculator is later corrected. The rounding stages are decided in Go
-- (backend/store/po_calc.go) and pinned by tests:
--
--   1. line gross    = round half away from zero(qty_scaled * unit_price / 10^4)
--   2. row discount  = truncate(gross * row_percent / 100)
--   3. subtotal      = sum of row nets
--   4. PO discount   = truncate(subtotal * po_percent / 100)
--   5. PPN           = truncate(after * ppn_bps / 10000), customer-facing only
--   6. adjustment    = signed minor units, reason required
--   7. grand total   = after + PPN + adjustment, never negative
--
-- Money stays BIGINT minor units. No float and no NUMERIC money column enters
-- the schema; NUMERIC appears only inside the one-off backfill below, where it
-- is the exact intermediate the Go calculator also uses.

ALTER TABLE {{schema}}.purchase_orders
    ADD COLUMN IF NOT EXISTS discount_percent SMALLINT NOT NULL DEFAULT 0
        CHECK (discount_percent BETWEEN 0 AND 100),
    -- Sum of line nets, after each row's own discount. Never includes PPN.
    ADD COLUMN IF NOT EXISTS subtotal_minor BIGINT NOT NULL DEFAULT 0
        CHECK (subtotal_minor >= 0),
    -- The whole-PO discount amount, truncated to a whole minor unit.
    ADD COLUMN IF NOT EXISTS discount_minor BIGINT NOT NULL DEFAULT 0
        CHECK (discount_minor >= 0),
    -- The rate is a snapshot: a later legislative change must not rewrite an
    -- entered PO. 0 for FTZ and for services, 1100 for Non-FTZ goods. PPH22
    -- (services, internal) never appears here.
    ADD COLUMN IF NOT EXISTS ppn_bps SMALLINT NOT NULL DEFAULT 0
        CHECK (ppn_bps BETWEEN 0 AND 10000),
    ADD COLUMN IF NOT EXISTS ppn_minor BIGINT NOT NULL DEFAULT 0
        CHECK (ppn_minor >= 0),
    -- Signed on purpose: a positive adjustment raises the printed total, a
    -- negative one lowers it. It never changes a row price, a discount or a
    -- tax entry -- that is what the correction slice (#473) is for.
    ADD COLUMN IF NOT EXISTS adjustment_minor BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS adjustment_reason TEXT NOT NULL DEFAULT ''
        CHECK (char_length(adjustment_reason) <= 200),
    ADD COLUMN IF NOT EXISTS grand_total_minor BIGINT NOT NULL DEFAULT 0
        CHECK (grand_total_minor >= 0),
    -- The source revision's printed totals, copied once at creation. They are
    -- document evidence: Vera's calculation must never silently replace them
    -- (parent story 45). The net after discount is stored implicitly as
    -- subtotal_minor - discount_minor, exactly as the quotation does.
    ADD COLUMN IF NOT EXISTS printed_subtotal_minor BIGINT NOT NULL DEFAULT 0
        CHECK (printed_subtotal_minor >= 0),
    ADD COLUMN IF NOT EXISTS printed_discount_minor BIGINT NOT NULL DEFAULT 0
        CHECK (printed_discount_minor >= 0),
    ADD COLUMN IF NOT EXISTS printed_ppn_minor BIGINT NOT NULL DEFAULT 0
        CHECK (printed_ppn_minor >= 0),
    ADD COLUMN IF NOT EXISTS printed_grand_total_minor BIGINT NOT NULL DEFAULT 0
        CHECK (printed_grand_total_minor >= 0);

ALTER TABLE {{schema}}.purchase_order_items
    -- discount_enabled is stored rather than inferred from percent > 0, so a
    -- row explicitly set to 0% stays distinguishable from one never touched --
    -- the same rule the quotation rows follow.
    ADD COLUMN IF NOT EXISTS discount_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS discount_percent SMALLINT NOT NULL DEFAULT 0
        CHECK (discount_percent BETWEEN 0 AND 100),
    ADD COLUMN IF NOT EXISTS gross_minor BIGINT NOT NULL DEFAULT 0
        CHECK (gross_minor >= 0),
    ADD COLUMN IF NOT EXISTS discount_minor BIGINT NOT NULL DEFAULT 0
        CHECK (discount_minor >= 0),
    ADD COLUMN IF NOT EXISTS net_minor BIGINT NOT NULL DEFAULT 0
        CHECK (net_minor >= 0);

ALTER TABLE {{schema}}.purchase_order_items
    ADD CONSTRAINT purchase_order_items_net_formula_check
        CHECK (net_minor = gross_minor - discount_minor),
    ADD CONSTRAINT purchase_order_items_discount_bound_check
        CHECK (discount_minor <= gross_minor);

-- Every amount this feature stores is bounded to 2^53-1, JavaScript's
-- Number.MAX_SAFE_INTEGER. The columns are BIGINT, but each figure travels to
-- the desktop as a JSON number, which cannot name every minor unit above this
-- bound: 27_021_597_764_222_973 reads back as ...972. The Go calculator and the
-- editor refuse the same range; these constraints are the backstop for a
-- hand-written statement or a future caller that forgets.
--
-- The bound covers both the PO's own money and the printed source totals it
-- copies as evidence. The adjustment is signed, so it is bounded on both
-- sides; every other amount is nonnegative.
ALTER TABLE {{schema}}.purchase_orders
    ADD CONSTRAINT purchase_orders_exact_range_check CHECK (
        subtotal_minor BETWEEN 0 AND 9007199254740991
        AND discount_minor BETWEEN 0 AND 9007199254740991
        AND ppn_minor BETWEEN 0 AND 9007199254740991
        AND adjustment_minor BETWEEN -9007199254740991 AND 9007199254740991
        AND grand_total_minor BETWEEN 0 AND 9007199254740991
        AND printed_subtotal_minor BETWEEN 0 AND 9007199254740991
        AND printed_discount_minor BETWEEN 0 AND 9007199254740991
        AND printed_ppn_minor BETWEEN 0 AND 9007199254740991
        AND printed_grand_total_minor BETWEEN 0 AND 9007199254740991
    );

ALTER TABLE {{schema}}.purchase_order_items
    ADD CONSTRAINT purchase_order_items_exact_range_check CHECK (
        quantity_scaled BETWEEN 1 AND 9007199254740991
        AND unit_price_minor BETWEEN 0 AND 9007199254740991
        AND gross_minor BETWEEN 0 AND 9007199254740991
        AND discount_minor BETWEEN 0 AND 9007199254740991
        AND net_minor BETWEEN 0 AND 9007199254740991
    );

-- Drafts created before this migration have no discounts and no adjustment, so
-- their calculation follows from the entered quantity and price alone. The
-- printed snapshot comes from the source revision they were created from.
--
-- The printed figures are evidence, not arithmetic: if a pre-031 draft's source
-- revision has no totals row, there is nothing to copy, and writing 0 would
-- fabricate a document total the customer never saw. This slice is unreleased,
-- so that can only be a broken development schema; the migration aborts and
-- names the disorder rather than inventing a figure. A fresh creation refuses
-- the same source with po_source_incomplete instead.
DO $backfill_guard$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM {{schema}}.purchase_orders po
        LEFT JOIN {{schema}}.quotation_revisions r
          ON r.quotation_id = po.quotation_id
         AND r.revision_number = po.source_revision
        LEFT JOIN {{schema}}.quotation_revision_totals t
          ON t.revision_id = r.id
        WHERE r.id IS NULL OR t.revision_id IS NULL
    ) THEN
        RAISE EXCEPTION
            'migration 031 cannot backfill printed totals: a purchase order has no source revision totals';
    END IF;
END
$backfill_guard$;

-- Gross and net are written in one statement: the item net check is already in
-- place, so a two-step gross-then-net update would violate it in between.
UPDATE {{schema}}.purchase_order_items
SET gross_minor = round(quantity_scaled::numeric * unit_price_minor / 10000)::bigint,
    discount_minor = 0,
    net_minor = round(quantity_scaled::numeric * unit_price_minor / 10000)::bigint;

UPDATE {{schema}}.purchase_orders po
SET subtotal_minor = COALESCE(lines.subtotal, 0),
    discount_minor = 0,
    adjustment_minor = 0,
    adjustment_reason = ''
FROM (
    SELECT purchase_order_id, SUM(net_minor::numeric) AS subtotal
    FROM {{schema}}.purchase_order_items
    GROUP BY purchase_order_id
) AS lines
WHERE lines.purchase_order_id = po.id;

-- Both tax models: a current revision carries tax_type/service_category, an
-- older one only the legacy category. Reading both keeps a pre-031 draft's PPN
-- right whichever model issued its Quotation. The join is its own statement
-- because a JOIN in an UPDATE's FROM list cannot see the target table.
UPDATE {{schema}}.purchase_orders po
SET ppn_bps = CASE
        WHEN po.tax_zone <> 'NonFTZ' THEN 0
        WHEN r.tax_type = 'Goods' THEN 1100
        WHEN COALESCE(r.tax_type, '') = '' AND po.tax_category = 'SupplyOnly' THEN 1100
        ELSE 0
    END
FROM {{schema}}.quotation_revisions r
WHERE r.quotation_id = po.quotation_id AND r.revision_number = po.source_revision;

UPDATE {{schema}}.purchase_orders po
SET subtotal_minor = 0
WHERE NOT EXISTS (
    SELECT 1 FROM {{schema}}.purchase_order_items i
    WHERE i.purchase_order_id = po.id
);

UPDATE {{schema}}.purchase_orders po
SET ppn_minor = trunc(po.subtotal_minor::numeric * po.ppn_bps / 10000)::bigint,
    printed_subtotal_minor = t.marginalized_subtotal,
    printed_discount_minor = t.marginalized_discount,
    printed_ppn_minor = t.ppn_amount,
    printed_grand_total_minor = t.customer_grand_total
FROM {{schema}}.quotation_revisions r
JOIN {{schema}}.quotation_revision_totals t ON t.revision_id = r.id
WHERE r.quotation_id = po.quotation_id AND r.revision_number = po.source_revision;

UPDATE {{schema}}.purchase_orders
SET grand_total_minor = subtotal_minor - discount_minor + ppn_minor + adjustment_minor;

-- The two totals invariants the calculation guarantees, enforced by the schema
-- as well so a partial or hand-written update cannot leave a PO whose printed
-- arithmetic disagrees with its own components.
ALTER TABLE {{schema}}.purchase_orders
    ADD CONSTRAINT purchase_orders_discount_bound_check
        CHECK (discount_minor <= subtotal_minor),
    ADD CONSTRAINT purchase_orders_grand_formula_check
        CHECK (grand_total_minor = subtotal_minor - discount_minor + ppn_minor + adjustment_minor),
    ADD CONSTRAINT purchase_orders_adjustment_explained_check
        CHECK (
            adjustment_minor = 0
            -- The same set Go's strings.TrimSpace removes and the editor's
            -- trimAdjustmentReason mirrors: ASCII whitespace plus NEL, NBSP,
            -- U+1680, U+2000-U+200A, U+2028, U+2029, U+202F, U+205F and U+3000.
            -- U+FEFF (a BOM) is deliberately content, as it is for Go.
            OR adjustment_reason !~ '^[\t\n\v\f\r \u0085\u00A0\u1680\u2000-\u200A\u2028\u2029\u202F\u205F\u3000]+$'
        );

-- No new index: every read is by purchase_order_items.purchase_order_id, which
-- the existing UNIQUE (purchase_order_id, line_number) already serves, and no
-- query filters by a discount or total.
