-- Server-side export persists a revision with the rendered HTML snapshot
-- first; the desktop uploads locally rendered PDF bytes afterwards via the
-- document endpoint. The snapshot alone is sufficient to reproduce the
-- customer copy, so it satisfies the document guarantee on its own.
ALTER TABLE {{schema}}.quotation_documents
    DROP CONSTRAINT IF EXISTS quotation_documents_check;

ALTER TABLE {{schema}}.quotation_documents
    ADD CONSTRAINT quotation_documents_snapshot_check
    CHECK (html_snapshot <> '');
