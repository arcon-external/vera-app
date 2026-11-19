-- 051_po_report_index: the purchase-order report's population index (#629).
--
-- Every report query starts from the caller's organization, the half-open
-- received-date range, and the saved state: drafts are not orders, and the
-- report never leaves the tenant. The existing indexes cover identity and the
-- archive's filters but not this one, so a growing archive would scan saved POs
-- across every organization to answer a single report.
--
-- The predicate is part of the index because drafts are a large, permanently
-- excluded population: they are edited far more often than they are saved, and
-- including them would make every entry-point save maintain index entries the
-- report can never read. Currency and business category stay out of the index
-- on purpose: they are low-cardinality filters applied after the range, and
-- adding them would multiply index variants without narrowing this scan.
CREATE INDEX IF NOT EXISTS purchase_orders_report_idx
    ON {{schema}}.purchase_orders (org_id, received_date)
    WHERE state = 'saved';
