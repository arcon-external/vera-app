# Local mock data (`veraseed`)

`veraseed` fills **one explicitly confirmed local Postgres database** with six
fictional Quotations and eight fictional Purchase Orders for manual acceptance
testing (#500). It is a developer tool, not a product feature:

- it is a standalone command (`cmd/veraseed`, wrapped by
  `scripts/seed-local.sh`);
- it adds no HTTP route, Wails binding or frontend code;
- it never runs inside `veraserver`, and production has no way to invoke it;
- it writes no signed documents, no real customer data and no credentials.

Everything it creates is fictional. Subjects and customer names carry `[MOCK]` /
`(MOCK)`, customer PO numbers start with `MOCK-`, and every generated PDF says
`FICTIONAL MOCK ... - NOT A REAL ... DOCUMENT` on its first page.

## Safety contract

The seed refuses a target **before any migration or write**:

1. The resolved Postgres configuration (including the `PG*` fallback) must be
   exactly one local endpoint: `localhost`, a loopback address, or a local Unix
   socket. This is `store.RequireLocalDevelopmentTarget`, the same guard
   `scripts/dev-api.sh` uses.
2. `--target <database>` must name the database pgx resolved, exactly. A
   loopback host is not enough: a local tunnel or a restored copy can still be
   the wrong database, so the operator must confirm what will be filled.
3. Before migrating, the seed opens one connection and reads
   `current_database()`. If the server reports a different database than the
   confirmation, it refuses. (A container port-forward means the server-side
   address is not loopback even when the client connected to `127.0.0.1`; the
   database name is the check that survives that.)

Errors name the host or the two database names. They never print the DSN, its
password, or its query values. `veraseed` reads `VERA_POSTGRES_DSN` /
`VERA_POSTGRES_SCHEMA` (or `PGHOST`/`PGDATABASE`/`PGUSER`/…) exactly as the
server does, but it prints only `host/database (schema …)`.

The seed never contacts production, copies production rows, reads a signing key
or writes a real customer document. Do not point it at a restored production
backup: the confirmation step is the operator's promise that the database is a
throwaway local one.

## Prerequisites

- A local PostgreSQL 18 database (for example `vera_local`), with a local
  schema such as `vera`.
- A local `.env` (gitignored) carrying the local DSN and schema. Never copy the
  production DSN or credentials into it.
- At least one active **administrator** in that local database. The seed deletes
  its two synthetic source Quotations through the ordinary admin deletion
  lifecycle, so it needs an admin actor. Starting the local API once
  (`scripts/dev-api.sh`) bootstraps the first admin from
  `VERA_BOOTSTRAP_ADMIN_*` if the database has no users.
- Client codes **90–95** free, or already holding the same fictional `(MOCK)`
  name. The seed refuses to rename an existing client that occupies one of its
  codes; it never overwrites a real client's name.

```bash
# .env (local only; never commit)
VERA_API_URL=http://127.0.0.1:8081
VERA_POSTGRES_DSN=postgres://USER:PASSWORD@127.0.0.1:5432/vera_local?sslmode=disable
VERA_POSTGRES_SCHEMA=vera
```

## Run

```bash
# From the repository root; sources .env, then runs cmd/veraseed.
scripts/seed-local.sh --target vera_local

# Machine-readable report
scripts/seed-local.sh --target vera_local --json

# Explicit actor if there is more than one local admin
scripts/seed-local.sh --target vera_local --actor-email admin@example.invalid
```

`--target` is the database name, not a URL. A missing or mismatched value is a
refusal, not a prompt. The human report shows the resolved
`host/database (schema …)`, the mode (`created` or `existing`), the counts and
one line per fixture.

`veraseed` exits `0` on a completed run (created or existing), `1` on a refusal
or write failure, and `2` on bad flags. A report is printed before the error when
a partial run can be described.

## What a fresh run creates

Six **live** fictional Quotations (one is left available), eight fictional POs:

| Quotation fixture | Shapes it covers | Occupied by |
| --- | --- | --- |
| `ftz-goods-finalized-pdf` | FTZ goods, two revisions | `po-finalized-pdf` |
| `nonftz-goods-finalized-differences` | Non-FTZ goods with PPN, packaged sub-items, 5% whole-quotation discount | `po-finalized-differences-adjustment` |
| `services-nonconstruction-incomplete-draft` | Services, non-construction | `po-linked-incomplete-draft` |
| `services-construction-draft-differences` | Services, construction, 10% row discount | `po-linked-draft-differences` |
| `nonftz-subitems-no-pdf-correction` | Non-FTZ goods with PPN, sub-items, two revisions | `po-finalized-no-pdf-confirmation-correction` |
| `ftz-goods-available` | FTZ goods, two revisions, **available** for linking or repair | — |

| Purchase Order fixture | State |
| --- | --- |
| `po-linked-incomplete-draft` | linked draft, no customer PO number yet |
| `po-linked-draft-differences` | linked draft with a changed quantity and an extra line |
| `po-finalized-pdf` | finalized, matching the source revision, with a synthetic customer PO PDF |
| `po-finalized-differences-adjustment` | finalized with accepted differences and a reasoned positive adjustment |
| `po-finalized-no-pdf-confirmation-correction` | finalized without a PDF, with a confirmation number and one correction-history entry |
| `po-never-linked-mixed-pdf` | never-linked Source missing draft, manually entered, with a synthetic mixed text/scanned PDF |
| `po-former-linked-draft` | draft whose synthetic source quotation was deleted |
| `po-former-linked-finalized` | finalized record whose synthetic source quotation was deleted; ready for the admin source repair against the available Quotation |

Five live Quotations are occupied and one (`ftz-goods-available`) is free. The
two **synthetic source** Quotations (`synthetic-former-draft-source`,
`synthetic-former-finalized-source`) are created, linked to their POs and then
deleted through the admin deletion lifecycle. They are not counted among the six
live fixtures, but the deletion audits and their consumed quote numbers remain.

All PO states are produced through the ordinary store lifecycle: create from an
exact issued revision, draft update, document attach, difference review,
finalize, correction, and the admin deletion flow. No impossible stored state is
fabricated. Quotation revisions are unsigned by design — they are fixtures, not
issued documents, and the public verification portal is not part of this set.

## Synthetic documents and extraction limits

Quotation documents and customer PO documents are synthetic PDFs from
`backend/extract/pdfgen`. They contain known text and no customer content. The
mixed fixture has a text header page and an image-only "scanned" page with no
text layer. This exercises document handling and evidence display only: **no
extraction-accuracy claim** is made, and no extraction result is accepted for
the user. Authorized holdout measurement remains a separate quality gate.

## Re-running, edits and cleanup

The seed records a run marker before creating anything, then one marker per
fixture in a `local_seed_fixtures` table **in the confirmed schema**. The table
is created by `veraseed`, not by a migration; production never sees it unless
someone runs the seed there, which the target checks refuse.

On a second run:

- if any marker exists, nothing is created, overwritten or deleted;
- each fixture is reported `present`, `changed` (edited since seeding; for
  example a PO save, a new quotation revision, or a source repair) or `missing`;
- `po-former-*` records are not reported as changed merely because the seed's
  own source deletion moved their version — the marker is refreshed after that
  deletion;
- the two synthetic sources are reported `deleted`, as intended.

A partial run (a crash after the run marker, or a missing fixture) is reported as
missing and is **not** rebuilt automatically. To start over, drop only the local
schema/database that contains the fixtures, then seed again:

```sql
-- Local throwaway database only. Never run this against production.
DROP SCHEMA vera CASCADE;
```

There is deliberately no `--reset` or destructive rebuild flag. The seed's
ownership boundary is: the fixtures it marked, the marker table, the fictional
customers it created through the normal quotation path, and the deletion audits
for its two synthetic sources. It does not own or delete anything else.

## Tests

The package tests cover the manifest shape, pricing arithmetic, PDF validity, the
target refusal (including the no-schema-before-write proof), the fresh matrix,
rerun stability and edited-fixture preservation. The database tests need a
disposable PostgreSQL 18:

```bash
VERA_POSTGRES_TEST_DSN='postgres://vera:vera@127.0.0.1:55432/veratest?sslmode=disable' \
  go test ./backend/localseed -v

# Unit-only (no database)
go test ./backend/localseed
```

The tests drop and recreate schemas named `vera_seed_*` in that database. Point
the DSN only at a throwaway database.
