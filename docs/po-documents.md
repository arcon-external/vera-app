# Purchase Order documents (#472)

A Purchase Order has **one optional customer PO PDF**. It is evidence for the
order, never a second PO and never a collection of supporting files. Adding,
replacing or removing it does not write a `purchase_orders` field; only the
final save writes the deliberate no-PDF confirmation.

## Schema and lifecycle

`purchase_order_documents` (migration 033) stores one row per version:

| Column | Meaning |
| --- | --- |
| `org_id` | The owning organization, required on every read and write |
| `purchase_order_id` | The one PO this document belongs to (`ON DELETE CASCADE`) |
| `version` | Monotonic per PO, starting at 1; never reused |
| `pdf_data` | The uploaded file, byte for byte |
| `sha256`, `size_bytes`, `content_type` | Integrity and response metadata |
| `original_filename` | Display name only; never a filesystem path |
| `uploaded_by`, `uploaded_at` | Attribution |
| `active` | Exactly one true row per PO |
| `replaced_at` | Set when a newer upload superseded this version |
| `removed_at`, `removed_by` | Set when staff deliberately removed this version |

The partial unique index `purchase_order_documents_active_idx` on
`(purchase_order_id) WHERE active` is the invariant: the database itself refuses
a second current document even if a code path forgets the pointer swap.
`UNIQUE (purchase_order_id, version)` is the second backstop.

Replaced and removed rows keep their bytes. Retrieval by version still serves
them, so the correction history can show the earlier evidence.

## Race and deletion-safety contract

1. Attach/replace and remove begin a transaction and lock the owning
   `purchase_orders` row (`SELECT ... FOR UPDATE` inside the caller's
   organization). Two concurrent replacements serialize: one gets version N,
   the next N+1, and exactly one row remains active.
2. The active pointer is moved and the next version inserted in one
   transaction. A failed upload leaves the previous current PDF untouched.
3. Every read joins `purchase_orders` and filters `po.org_id`; a document from
   another organization is indistinguishable from a missing one.
4. Removal marks the current row inactive with `removed_at`/`removed_by` and
   keeps the bytes as history. It does not touch the PO.
5. Deleting a PO cascades its document rows. The bytes live in Postgres, so no
   external file cleanup is part of that transaction. If document bytes ever
   move to object storage, cleanup must become a retryable record written in
   the same transaction, not a claim that cross-system deletion is atomic.

## Validation

`ValidatePurchaseOrderPDF` in `backend/store/po_document.go` enforces:

- **20 MiB bound** (`MaxPurchaseOrderDocumentBytes`), on the wire and again on
  the decoded bytes, plus the schema `CHECK`.
- a real `%PDF-` header, at least one PDF object, and `endobj`;
- a document catalog (`/Root`), a `startxref` pointer, and a trailer or xref
  stream;
- the `%%EOF` marker, allowing trailing whitespace;
- **encryption detection** from the trailer/xref-stream dictionary only, so a
  literal `/Encrypt` in page content does not reject a valid file. An encrypted
  PDF is refused with an explicit request for an unlocked copy; passwords are
  never asked for, transported or stored.

Garbage, truncated and oversized files are refused with named errors
(`po_document_invalid`, `po_document_encrypted`, `po_document_too_large`).

## HTTP API

| Method and path | Purpose |
| --- | --- |
| `POST /v1/purchase-orders/:id/document` | Upload/replace; multipart field `document`; 201 with metadata |
| `GET /v1/purchase-orders/:id/document` | `{ document: <metadata|null>, versions: [...] }` |
| `GET /v1/purchase-orders/:id/document/content` | Active PDF bytes (`application/pdf`, version/SHA headers) |
| `GET /v1/purchase-orders/:id/documents/:version/content` | One retained version's bytes |
| `DELETE /v1/purchase-orders/:id/document` | Body `{ "confirmed": true }`; removes the active document |

All routes require a session. A purchase order with no PDF answers the metadata
route with `document: null` and an empty `versions` list, not 404. An unknown
PO, or one outside the caller's organization, answers 404.

## Desktop bindings

```go
type PurchaseOrderDocumentUploadInput struct {
    PurchaseOrderID int64
    Data            []byte   // bytes read by the editor
    FilePath        string   // or a local path; Data wins when both are set
    Filename        string   // display name; defaults to the path's base
}

func (q *Quotation) AttachPurchaseOrderDocument(input PurchaseOrderDocumentUploadInput) (PurchaseOrderDocumentDTO, error)
func (q *Quotation) GetPurchaseOrderDocumentState(id int64) (PurchaseOrderDocumentStateDTO, error)
func (q *Quotation) GetActivePurchaseOrderDocumentContent(id int64) (PurchaseOrderDocumentContentDTO, error)
func (q *Quotation) GetPurchaseOrderDocumentVersionContent(id int64, version int32) (PurchaseOrderDocumentContentDTO, error)
func (q *Quotation) RemovePurchaseOrderDocument(id int64, confirmed bool) (PurchaseOrderDocumentDTO, error)
```

`PurchaseOrderDocumentDTO` carries id, UUID, PO id, version, SHA-256, size,
content type, original filename, uploader/time, active flag, and the
replaced/removed markers. The content DTO adds `Data []byte`.

## Final-save confirmation state machine

`documentState` is `none` or `attached`:

| Branch | First final save | Later behaviour |
| --- | --- | --- |
| `none` | Requires the no-PDF confirmation; records `pdf_absent_confirmed_at/by` | The saved record shows the persistent no-PDF state, with no repeated prompt |
| `attached` | No confirmation; both absence columns stay null even if the client sends the checkbox | The attached PDF stays with the record |
| Remove current PDF | Requires its own confirmation; removal metadata is retained | The next final save is back in the `none` branch |

Removing a PDF from an already saved record changes only the document table; it
does not rewrite the saved PO or its recorded absence fields.

## Out of scope

Extraction (#477) is not part of this slice. The editor shows **Extract and
compare** visibly disabled with a stated reason, rather than pretending the
action is available.
