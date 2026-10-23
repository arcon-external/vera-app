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

`ValidatePurchaseOrderPDF` in `backend/store/po_document.go` (with the parser in
`backend/store/pdf_structure.go`) enforces:

- **20 MiB bound** (`MaxPurchaseOrderDocumentBytes`), on the wire, again on the
  decoded bytes, and in the schema `CHECK`.
- a real `%PDF-` header and the `%%EOF` marker, allowing trailing whitespace and
  NUL padding.
- **a structural walk, not a substring scan.** The file is tokenized into
  objects, dictionaries and arrays. Every stream body is skipped from `stream`
  to its `endstream` using `/Length` when it is a direct integer or a reference
  to an integer object. A first pass harvests the file's top-level integer
  objects, so a `/Length` defined *later* than the stream — a forward
  reference, which real writers emit — is resolved before the structure pass.
  That harvest runs before any length is known, so it isolates a stream only
  at its first `endstream` marker and can miss a later integer object when
  content draws an `endstream`; the limits below state the consequence.
  When a `/Length` cannot be resolved, or the bytes at that offset are not
  followed by `endstream`, the scanner falls back to the first `endstream`
  marker and content past that point is scanned as structure, so exact
  isolation depends on a resolvable `/Length` that agrees with the file. The
  paired direct/forward tests pin the resolved case. The trailer and the `/Root` it names are
  read only from the dictionary `<startxref>` points at, never from a substring.
- the **last `startxref`** must be an integer offset inside the file and must
  point at either a classic `xref` table (followed by its `trailer`) or an
  object whose dictionary is `/Type /XRef` — compact `/Type/XRef` and spaced
  `/Type /XRef` are both accepted.
- the **`/Root` reference in that real trailer or xref-stream dictionary** must
  name a `/Type /Catalog` object that exists as a plain object. If the catalog
  is compressed into an `/ObjStm`, that object stream's header is decompressed
  to read `/N`, `/First` and the object-number pairs, and the catalog is
  accepted only when the index lists its object number (generation 0). The
  compressed catalog dictionary itself is not decompressed, so this is
  membership, not a catalog type check — a documented gap. Only
  `/Filter /FlateDecode`, its `/Fl` abbreviation, or a single-element
  `[/FlateDecode]` array (or `[/Fl]`) is supported for that header. An indirect
  `/Filter 12 0 R` is not resolved even when object 12 is `/FlateDecode`; it is
  refused with an error saying the `/Filter` is neither a name nor a list. An
  object stream that would be needed to resolve the catalog but uses any other
  filter, or cannot be decompressed, is a named rejection that names the
  filter; it is never a silent "catalog object is missing" and never a crash.
- **encryption** from the real trailer or xref-stream dictionary only. Both an
  indirect `/Encrypt N G R` and an inline `/Encrypt << ... >>` dictionary are
  detected, and name escapes are decoded (`/En#63rypt` is `/Encrypt`). An
  encrypted PDF is refused with an explicit request for an unlocked copy;
  passwords are never asked for, transported or stored.

Garbage, truncated, header-only, no-root, no-EOF, `startxref banana`, a forged
trailer with no xref section, and a `/Root` that names nothing are refused with
`po_document_invalid`. An encrypted document (including a real AES xref-stream
PDF) is refused with `po_document_encrypted`; an oversized file with
`po_document_too_large`; an object stream with an unsupported filter with
`po_document_invalid` naming that filter.

The real fixture files the structural tests pin live in `backend/testdata/pdf/`:
a MuPDF PDF with a classic literal trailer (accepted), and two PyMuPDF
AES-encrypted xref-stream PDFs, one with a literal `/Encrypt` and one with the
escaped `/En#63rypt` key (both refused as encrypted). The paired
direct/forward-`/Length` documents, whose content draws an `endstream` plus a
fake object header, and the unfiltered/Flate-compressed `/ObjStm` catalogs are
built by the tests in `backend/store/pdf_structure_test.go`, so those rules are
pinned without binary fixtures.

## What the structure check does not do

These are deliberate limits, stated so nobody reads more into a stored file
than the check actually proved. This is a bounded structural check, not a
conforming PDF parser: it reads the structure described above and nothing else.

- **Incremental updates are not followed.** Only the last `startxref` and the
  trailer or xref stream it points at are read; `/Prev` chains are ignored. A
  latest trailer that omits `/Root` is refused, even when an earlier trailer in
  the same file carries one.
- **Classic xref entry offsets are not validated.** Object lookup is
  scan-based: an entry whose offset is wrong does not hide an object, and an
  entry that points into content does not create one. What is checked is that
  the table parses and that the last `startxref` offset lands on the table (or
  on the `/Type /XRef` object). A file with a stale entry is still accepted.
- **The first-pass harvest can be stopped by stream content, so a readable
  file can be refused.** The harvest runs before any `/Length` is known, so it
  isolates a stream only at its first `endstream` marker. Content that itself
  draws an `endstream`, followed by bytes that do not parse as structure, ends
  that stream early in the harvest pass, and an object after it — including
  the integer object a forward `/Length` names — can then be missed. The
  structure pass resolves `/Length` only from the harvested set, so it cannot
  recover the missing length and falls back to that same marker; the file is
  then refused as `po_document_invalid` even though a reader would open it.
  The paired direct/forward test pins the case where the harvest succeeds, not
  this one. This is the cost of a bounded single-pass harvest, not a security
  hole: `/Root` and `/Encrypt` are still read only from the dictionary
  `startxref` points at, in the trailer or xref stream, never from stream
  content directly.
- **A compressed catalog is checked for membership only.** The `/ObjStm`
  header is decoded (see above), but the catalog dictionary's own bytes are
  not, so its `/Type /Catalog` is not re-read from inside the object stream.
- **An indirect `/Filter` is not resolved.** The object stream's filter must be
  a name (`/FlateDecode`, `/Fl`) or a single-element array of names
  (`[/FlateDecode]`, `[/Fl]`). A reference such as `/Filter 12 0 R` is refused
  even when object 12 is `/FlateDecode`, with an error that says the `/Filter`
  is neither a name nor a list. A longer filter chain is refused by name
  rather than guessed at.
- **Rendering is not checked.** Page trees, fonts, images and whether a reader
  would display the file are out of scope; the question is whether the bytes
  are a document rather than a bag of tokens that resembles one.

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
    DataBase64      string // the file the editor read, base64-encoded
    Filename        string // display name
}

func (q *Quotation) AttachPurchaseOrderDocument(input PurchaseOrderDocumentUploadInput) (PurchaseOrderDocumentDTO, error)
func (q *Quotation) GetPurchaseOrderDocumentState(id int64) (PurchaseOrderDocumentStateDTO, error)
func (q *Quotation) GetActivePurchaseOrderDocumentContent(id int64) (PurchaseOrderDocumentContentDTO, error)
func (q *Quotation) GetPurchaseOrderDocumentVersionContent(id int64, version int32) (PurchaseOrderDocumentContentDTO, error)
func (q *Quotation) RemovePurchaseOrderDocument(id int64, confirmed bool) (PurchaseOrderDocumentDTO, error)
```

`PurchaseOrderDocumentDTO` carries id, UUID, PO id, version, SHA-256, size,
content type, original filename, uploader/time, active flag, and the
replaced/removed markers. The content DTO adds `DataBase64 string`.

**The base64 representation is explicit, in both directions.** Wails serializes
a Go `[]byte` as base64 text, so an earlier `Data []byte` silently crossed the
boundary as a string while the generated TypeScript declared `number[]`; the
panel then built a zero-length array and previewed an empty PDF. Both DTOs now
say `DataBase64`, the generated `frontend/wailsjs/go/models.ts` declares
`DataBase64: string`, and `frontend/src/purchaseOrderDocument.ts` owns the only
encoder/decoder (`bytesToBase64` / `base64ToBytes`). The Go wrapper bounds the
decoded upload at 20 MiB before decoding; the desktop editor always uses its
file picker and reads the bytes itself — there is no path variant, because no
client supplied one. `TestGeneratedModelsExposeTheBase64DocumentFields` (root
package) fails if the committed generated models drift; a Go JSON test pins the
wire value, and the matching TypeScript test decodes the same base64 literal to
the exact `%PDF-1.7` bytes.

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
