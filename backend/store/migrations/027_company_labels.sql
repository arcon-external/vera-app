-- 027_company_labels: the customer-visible wording of a quotation is company
-- data, not code.
--
-- Until now every label on the document -- the title, "Quotation to", the
-- meta labels, the seven column headers, the four totals labels, the closing
-- sentence, the two verification captions and the four terms sentences -- was
-- hard-coded twice: in template-fixed.html for the browser renderer and in the
-- native renderer's Go source. A company that wanted "PROPOSAL" instead of
-- "QUOTATION" needed a release.
--
-- The table holds overrides only, one row per label:
--
--   * key is the label's stable name, from the declaration in
--     backend/labels (a key that names a label is an editable label);
--   * value is what the company wants printed instead of the built-in
--     default.
--
-- Nothing is seeded. The defaults stay in Go -- backend/labels.DefaultLabels
-- -- and an absent or empty row means "print the built-in default", which is
-- why a cleared label can never print blank. One declaration, both renderers,
-- and the defaults are test-pinned against the strings the template carried
-- before this migration.
--
-- Archived revisions are untouched: they self-snapshot their rendered HTML
-- (004), so a wording change affects future exports only.
CREATE TABLE {{schema}}.company_labels (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
