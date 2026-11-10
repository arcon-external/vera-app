# PO editor prototype

Two throwaway directions for the Purchase Order editor. Shared state, no persistence.

## Run

```bash
cd frontend/prototype/po-editor
python3 -m http.server 8099 --bind 0.0.0.0
```

Open on the tailnet:

- A, quotation language: `http://100.93.103.83:8099/?variant=a`
- B, original procurement sheet: `http://100.93.103.83:8099/?variant=b`

The bottom bar switches variants and opens the state drawer. Inputs, add/remove
line, and totals are live. The URL keeps the chosen variant.

## The two directions

- **A — Quotation language.** The quotation editor's two-column card layout:
  the same `fieldClass` inputs, section headings, line-item rows, and a 320px
  sticky summary rail. The PO's own fields and totals sit in that language.
- **B — Original.** A procurement sheet: a teal identity band, a supplier /
  ship-to block, a ruled line table with inline inputs, numbered sections, and
  an order-control rail with a status timeline and the grand total.

This is a prototype, not production. Fold the chosen direction into
`PurchaseOrderEditor.tsx` and delete this directory afterwards.
