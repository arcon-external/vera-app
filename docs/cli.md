# Vera from the command line (`vera`)

## What it is

`vera` is a single binary that drives the whole quotation system. It talks to
`veraserver` over HTTPS exactly as the desktop app does — same routes, same
`backend/apiclient`, same errors — and it also imports the application itself, so
it can do the two things a pure HTTP client cannot:

- **price a quotation** with the real pricing pipeline, and
- **render the PDF** with the real renderer.

Rendering is pure Go (`backend/render` uses gpdf), so this works on a machine
with no browser and no display. That is what makes it end to end: one command
prepares, allocates the quote number, stores a signed revision, renders the
document the customer receives, attaches it, and the result verifies through the
same public portal a customer would use.

It is built for two jobs:

1. **Testing the product.** Every feature has a command, so exercising something
   end to end is one line rather than a hand-written `curl` and a throwaway Go
   test.
2. **Scripting.** Every command takes `--json` and returns a documented exit
   code, so nothing has to parse prose.

## Where things run

```
  vera (CLI)  ──┐
                ├──►  veraserver (Cloud Run)  ──►  Neon Postgres
  Vera desktop ─┘

  vera also renders PDFs locally. The desktop does the same.
```

| Thing | Where it lives |
|---|---|
| Clients, quotations, revisions, users, company record | Neon Postgres, via veraserver |
| PDF rendering | **On the machine running `vera`** — pure Go, no browser |
| The signed revision | veraserver; signed at export with the server's Ed25519 key |
| Your session token | `~/.config/vera/session.json`, mode `0600` |

There is no local database in this picture. `VERA_POSTGRES_TEST_DSN` points at a
throwaway Postgres used only by the Go test suite, which drops the schema it
writes to; never point it at anything that matters.

The default server is production:

```
https://api.arconpentapersada.co.id
```

## Install

`vera` ships beside the desktop installers on every release, from the public
mirror. Pick your platform — staff cannot reach the GitHub release assets, so use
these:

| Your machine | Download |
|---|---|
| Windows (most PCs) | `Vera-CLI-v<version>-windows-amd64.exe` |
| Windows on ARM | `Vera-CLI-v<version>-windows-arm64.exe` |
| macOS, Apple Silicon (`uname -m` → `arm64`) | `Vera-CLI-v<version>-darwin-arm64` |
| macOS, Intel (`uname -m` → `x86_64`) | `Vera-CLI-v<version>-darwin-amd64` |
| Linux, x86-64 | `Vera-CLI-v<version>-linux-amd64` |
| Linux, ARM64 | `Vera-CLI-v<version>-linux-arm64` |

```
https://storage.googleapis.com/vera-downloads-508411/Vera-CLI-v<version>-<os>-<arch>
```

Each has a `.sha256` beside it. Verify before you run it:

```bash
curl -fsSLO https://storage.googleapis.com/vera-downloads-508411/Vera-CLI-v0.9.0-linux-amd64
curl -fsSLO https://storage.googleapis.com/vera-downloads-508411/Vera-CLI-v0.9.0-linux-amd64.sha256
sha256sum -c Vera-CLI-v0.9.0-linux-amd64.sha256
chmod +x Vera-CLI-v0.9.0-linux-amd64
./Vera-CLI-v0.9.0-linux-amd64 version
```

On macOS, `chmod +x` it and on first launch of an unsigned build, right-click →
Open instead of double-clicking.

From a checkout, build it yourself:

```bash
go build -o vera ./cmd/vera
```

The CLI links no cgo — rendering is pure Go — so it cross-compiles anywhere:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o vera ./cmd/vera
```

## Getting started

```bash
# 1. Sign in. The token is stored at ~/.config/vera/session.json (0600).
vera login --email you@arconpentapersada.co.id

# 2. Check who the server thinks you are.
vera whoami

# 3. See the client codes you can quote.
vera clients list

# 4. Write a quotation input file — this prints a worked example you can run.
vera schema > quotation.json

# 5. Check the arithmetic without filing anything.
vera quotations validate --input quotation.json

# 6. File it and produce the customer's PDF.
vera quotations export --input quotation.json --out quotation.pdf
```

Step 6 prints the quote number and the verification link. Check the document the
way a customer would:

```bash
vera verify <short-id>
```

## Conventions

### Global flags

Accepted by **every** command, before or after the verb:

| Flag | Meaning |
|---|---|
| `--api URL` | Which server to talk to. Default: `VERA_API_URL`, then the address you signed in against, then production. |
| `--token TOKEN` | Session token. Default: `VERA_TOKEN`, then the stored session. |
| `--json` | Machine-readable output. |
| `--quiet` | Print nothing on success. Warnings still print. |
| `--yes` | Skip confirmation prompts. |
| `--insecure` | Allow credentials over plain `http` to a remote host. |
| `--timeout DURATION` | Per-request timeout (default `60s`). |

Both positions work:

```bash
vera --json clients list
vera clients list --json
```

### `--json`

Every command has it, and it is why the CLI is usable from a script: a test
should assert on a field, not on the spacing of a table. Field names come from
the API's own JSON, which the desktop already depends on, so there is one
vocabulary rather than two.

In JSON mode the human table is suppressed; notes go to **stderr**, so they never
contaminate what a script is reading. `--quiet` silences notes but never
warnings.

### Exit codes

| Code | Meaning |
|---|---|
| `0` | Success. |
| `1` | Something went wrong that fits nothing below. |
| `2` | The command line itself was wrong — bad flag, missing argument, unknown command. |
| `3` | Not signed in, or not allowed to do this. |
| `4` | The thing you named does not exist. |
| `5` | The server refused the input (validation). |
| `6` | The server or the network failed. |

These are part of the interface. A script branches on them instead of matching
English:

```bash
if ! out=$(vera quotations show "$id" --json 2>/dev/null); then
  case $? in
    4) echo "no such quotation" ;;
    3) echo "sign in first" ;;
    *) echo "unexpected failure" >&2 ;;
  esac
fi
```

`vera verify` uses them to carry the verdict, so a script can tell *superseded*
from *forged* without reading a word: `0` valid, `4` no such code, `5` superseded
or mismatched, `6` the portal could not answer.

### Confirmations

Destructive commands ask. With `--yes` they do not. With **no terminal** — a
script or a pipe — they refuse unless `--yes` is given, because an unattended run
should have to say so explicitly.

`vera backup restore` requires `--yes` unconditionally: it is the one operation
that can take the company's records away, and a script pointed at production is
exactly how that happens by accident.

### Passwords

A password is never a flag: that would put it in the process list and the shell
history. Two forms are accepted:

- an interactive prompt with echo turned off, or
- `--password-stdin`, which reads it from a pipe.

```bash
printf '%s\n' "$VERA_PASSWORD" | vera login --email me@example.com --password-stdin
```

## The input file

`vera schema` prints a worked example and the field reference. This section is
the same thing in prose.

Money is always a **plain integer of rupiah** — `15000000`, never `"15.000.000"`
and never a float. Anything else invites a rounding bug into a pricing system.

```json
{
  "customerCode": 2,
  "subject": "Supply and installation of electrical and mechanical systems",
  "recipientName": "Dr. A. Wibowo",
  "incoterm": "DAP",
  "validityDays": 30,
  "paymentTermDays": 90,
  "leadTimeDays": 60,
  "taxZone": "FTZ",
  "taxType": "Goods",
  "items": [
    {
      "description": "Control panel replacement",
      "uom": "Lot",
      "qty": 2,
      "unitCost": 15000000,
      "marginEnabled": true,
      "marginPercent": 20
    },
    {
      "description": "Commissioning and handover",
      "uom": "LS",
      "qty": 1,
      "unitCost": 4000000,
      "marginEnabled": true,
      "marginPercent": 25,
      "discountEnabled": true,
      "discountPercent": 10
    },
    {
      "description": "Trauma system upgrade",
      "showSubPrices": true,
      "subItems": [
        {
          "description": "Panel beds",
          "uom": "Pc",
          "qty": 2,
          "unitCost": 8500000,
          "marginEnabled": true,
          "marginPercent": 20,
          "details": ["Powder-coated, with mounting hardware"]
        },
        {
          "description": "Monitor arms and cabling",
          "uom": "LS",
          "qty": 1,
          "unitCost": 1200000,
          "discountEnabled": true,
          "discountPercent": 10
        }
      ]
    }
  ]
}
```

### Fields

| Field | Notes |
|---|---|
| `customerCode` | **Required.** The two-digit client code. `vera clients list` shows them. |
| `subject` | **Required.** The subject line. |
| `recipientName` | **Required.** Who it is addressed to. |
| `incoterm` | `EXW` by default. |
| `validityDays` | `30` by default. |
| `paymentTermDays` | `90` by default. |
| `leadTimeDays` | `60` by default. |
| `taxZone` | `FTZ` (default) or `NonFTZ`. |
| `taxType` | `Goods` (default) or `Services`. |
| `serviceCategory` | Services only: `SupplyOnly`, `SupplyInstall`, `SupplyInstallConstruction`. Sending one with `Goods` is an error, so it is dropped rather than passed on. |
| `discountPercent` | Whole-quotation discount. Retired from the editor in favour of per-row discounts; still accepted so older payloads load. |
| `items[]` | At least one line. |

Per item:

| Field | Notes |
|---|---|
| `description` | **Required.** Prints on the customer's document. |
| `uom` | One the editor offers: `Lot`, `Pc`, `EA`, `LS`, `set`, `meter`, `m2`, `m3`, … |
| `qty` | Whole number, greater than zero. |
| `unitCost` | Plain rupiah. |
| `marginEnabled` / `marginPercent` | Adds to the line's own price. |
| `discountEnabled` / `discountPercent` | Comes off the marginalised price. |
| `showSubPrices` | `true` prints the line as a title with each part priced under it; `false` leaves the whole price on the line and prints the parts as unbilled scope. |
| `subItems[]` | Priced parts. A line with parts does not price itself: its total is their sum and it is always quantity 1. |
| `subItems[].details[]` | Scope bullets for the part, numbered `X.Y.Z` beneath it. |

`details` belongs to a **priced part**. A line with no `subItems` has nowhere to
put scope lines, so it is refused rather than printed with the text missing.

### Rules the server enforces

You do not have to re-check these — the CLI passes the server's own messages
through unchanged — but knowing them makes an input file predictable:

- **Margin first, then the row's discount.** The marginalised figure is the list
  price, and the discount comes off it, so the price printed always equals what
  staff saw in the editor.
- **A discount never reduces what we pay.** A row sold below cost shows a
  *negative* margin, and nett profit is signed.
- **The whole-quotation discount** applies to the summed subtotal, on top of the
  row discounts.
- **PPN** (11%, non-FTZ goods) is added on top of the price. The customer pays it;
  it is collected for the state and is never profit.
- **PPH22** (2% non-construction, 1.75% construction, services only, any zone) is
  internal. It is calculated once on the sell grand after discount, is never
  allocated per line, and is never printed.

### Reading the file

```
--input quotation.json     a file
--input -                  stdin, for piping from another tool
```

```bash
jq '.items[0].unitCost = 20000000' quotation.json | vera quotations validate --input -
```

A misspelled field is **refused**, not ignored: a silently dropped field on a
pricing document is a quotation for a number nobody chose.

---

# Commands

Run `vera` for the list, or `vera help <command>` for one.

## Sessions

### `vera login`

```bash
vera login --email you@example.com
vera login --email you@example.com --password-stdin
```

Signs in and stores the session. The stored session records **which server** it
was signed in against, and the token is only sent to that server — a token is
meaningful to one server, and sending it elsewhere produces a confusing "signed
out" rather than an obvious mistake.

If the account still has an invite pending, `login` says so; change the password
with `vera password change`.

### `vera logout`

```bash
vera logout
vera logout --everywhere
```

Forgets the local session, and asks the server to end it. `--everywhere` ends
every session for the account. If the server is unreachable the local file is
still removed — a token the server has already forgotten must not leave the next
command failing with a session nobody can clear.

### `vera whoami`

```bash
vera whoami
```

Asks the server, rather than reading the file, so it also proves the token still
works.

### `vera redeem`

```bash
vera redeem <invite-token>
printf '%s\n' "$NEW_PASSWORD" | vera redeem <invite-token> --password-stdin
```

Accepts an invite: sets the password for a new account. Invite tokens are issued
by `vera users add` and `vera users reset`, and expire.

### `vera password change`

```bash
vera password change
```

Prompts for the current password and the new one. Needs a terminal — there is no
flag for a password. If you cannot sign in at all, an admin issues a reset
instead: `vera users reset <id>`.

### `vera profile set`

```bash
vera profile set --display-name "New Name"
vera profile set --email new@example.com
```

A display name applies immediately. An email change is a change of identity, so
the server **queues** it for an admin rather than applying it — see
`vera account email-requests`.

### `vera account`

```bash
vera account delete-request     # ask an admin to delete your account
vera account requests           # your own deletion requests
vera account email-requests     # your own email-change requests
```

---

## Clients

Clients are database rows, never code: the editor's dropdown, quotation
resolution and the verification peek all read this directory. **Codes are
immutable** once created, because a code is printed inside every quote number
already issued to that client.

### `vera clients list`

```bash
vera clients list
vera clients list --json
```

### `vera clients add` (admin)

```bash
vera clients add --code 7 --name "PT Example" --address "Jl. Contoh 1, Batam"
```

`--code` is between 1 and 99 and unique. `--name` and `--address` are both
required — they print on the customer's document.

### `vera clients edit` (admin)

```bash
vera clients edit 3 --name "PT Example (renamed)"
vera clients edit 3 --address "Jl. Baru 2, Batam"
```

The code cannot be changed.

### `vera clients rm` (admin)

```bash
vera clients rm 3 --yes
```

A client with quotations cannot be removed.

---

## Quotations

The three kinds matter:

- **`validate` and `render`** work on this machine, using the application's own
  code, and touch the server only to read the client directory and the company
  profile. They are how you check a document without filing it.
- **`preview`** asks the server what *it* would produce, which is authoritative.
- **`export`** does the whole thing: number, signed revision, PDF, attach.

### `vera quotations validate`

```bash
vera quotations validate --input quotation.json
vera quotations validate --input quotation.json --json
```

Prices the input and prints the totals, filing nothing. The internal figures are
the operator's and are shown here on purpose; they never reach a customer.

```
The input prices cleanly.

Cost subtotal                 52.200.000
Customer subtotal             62.100.000
Row discounts                 500.000
Whole-quotation discount      0
Grand total (customer pays)   62.100.000
Nett after tax                62.100.000
Nett profit                   9.900.000
```

### `vera quotations preview`

```bash
vera quotations preview --input quotation.json
```

The server's own `Prepare`, so this is what its renderer would see. Both views
are returned: `customer` (safe to show) and `internal` (never exported).

### `vera quotations render`

```bash
vera quotations render --input quotation.json --out quote.pdf
```

Renders the PDF locally. **Files nothing** and uses no quote number — the
document says `DRAFT` where the number would be. Use `export` to file it.

### `vera quotations export`

```bash
vera quotations export --input quotation.json --out quote.pdf
vera quotations export --input quotation.json --out quote.pdf --draft 12
```

The whole flow in one command:

1. ask the server for the next quote number for that client,
2. store a signed revision,
3. render the PDF here,
4. write it to `--out`,
5. attach it to the revision it belongs to.

It is the desktop's own export with `--out` in place of the save dialog, so a
scripted document cannot drift from a clicked one.

```
Quotation:     15070001
Revision:      00
Grand total:   62.100.000
PDF:           quotation.pdf
Verify:        https://api.arconpentapersada.co.id/v/Ab3xY9kQ
```

`--draft N` also consumes that draft; the server deletes it only *after* the
revision is stored, so a failure does not lose the work.

### `vera quotations next-number`

```bash
vera quotations next-number --client 7
```

Peeking does not consume it: the next export still gets that number.

### `vera quotations list`

```bash
vera quotations list
vera quotations list --limit 50
vera quotations list --customer "Example"
```

`--customer` matches the client name, quote number or subject, case-insensitively.

### `vera quotations show`

```bash
vera quotations show 12
vera quotations show 12 --revision 0
vera quotations show 12 --html > customer-document.html
```

`--html` prints the **stored snapshot** — the document exactly as issued, which
is how you see precisely what the customer saw. Revisions are immutable, so this
never changes.

### `vera quotations revisions`

```bash
vera quotations revisions 12
```

The first export is revision `00`; each edit adds the next. A stored revision is
never modified.

### `vera quotations attach`

```bash
vera quotations attach 12 0 --pdf rendered-elsewhere.pdf
```

For a PDF produced somewhere else. `export` attaches its own, so this is rarely
needed.

### `vera quotations rm` (admin)

```bash
vera quotations rm 12 --yes
```

Deletes the quotation and every revision of it.

---

## Drafts

Drafts are private to their owner — admins included — which is why they are their
own noun rather than part of the archive. A draft is the state of an unfinished
editor, saved unvalidated so half-typed work survives.

```bash
vera drafts list
vera drafts save --input quotation.json --label "Batam hospital"
vera drafts save --input quotation.json --draft 4      # overwrite
vera drafts show 4
vera drafts rm 4 --yes
```

---

## Verification

### `vera verify`

```bash
vera verify Ab3xY9kQ
vera verify Ab3xY9kQ --number 15070001
vera verify Ab3xY9kQ --amount 62100000
```

Checks a printed document against the **public portal**, with no session: this is
the same check a customer clicking the QR code performs, and it working without
credentials is the point of it.

Passing `--number` or `--amount` additionally asserts what is printed on the
document, so a tampered figure is caught.

| Verdict | Exit | Meaning |
|---|---|---|
| `valid` | 0 | The document matches the record. |
| `superseded` | 5 | Genuine, but a later revision replaced it. |
| `mismatch` | 5 | The printed figures or the code have been altered. |
| `notfound` | 4 | No document matches that code. |
| `unavailable` | 6 | The portal could not answer. |

If the page cannot be read, `vera` says so and exits non-zero — it never reports
an unreadable answer as valid.

---

## Company

The company record heads every document. Only the **name and address** print on
a quotation; phone, email and NPWP appear nowhere in the template.

```bash
vera company show
```

### `vera company set` (admin)

```bash
vera company set --name "PT Arcon Penta Persada" --address "Batam, Indonesia"
vera company set --phone "+62 778 000 000" --email "sales@example.com"
```

The endpoint replaces the record, so the CLI reads the current one and moves only
the fields you name — a partial body would blank the rest.

Header edits affect **future** quotations only. Archived snapshots keep theirs.

### `vera company theme set` (admin)

```bash
vera company theme set --deep "#244f39" --accent "#c9a227"
vera company theme set --soft "#f4f1e8" --ink "#1d2b23" --portal
```

Eight `#RRGGBB` colours: four for quotations, four for the portal, independently
settable. An empty one means the stylesheet's default. Colours are validated
before the request.

### `vera company logo`

```bash
vera company logo set ./logo.png     # png, jpg, svg or webp; 512KB cap
vera company logo get --out logo.png
vera company logo clear --yes
```

The uploaded logo prints profile-first, with the built-in artwork as fallback.

---

## Users (admin)

### `vera users list`

```bash
vera users list
```

### `vera users add`

```bash
vera users add --email them@example.com --name "Their Name" --role staff
```

Creates the account **and prints an invite token**, because an account nobody can
sign into is not finished. Give the token to them; they accept it with
`vera redeem`.

### `vera users role`

```bash
vera users role 4 admin
```

The server refuses to remove the last active admin.

### `vera users active`

```bash
vera users active 4 --disable
vera users active 4 --enable
```

Disabling blocks sign-in and keeps the account, which is what you usually want.

### `vera users reset`

```bash
vera users reset 4
```

Issues a fresh invite token so the account can set its own password. **An admin
never chooses someone else's password** — that is why this issues a token rather
than setting one.

### `vera users email`

```bash
vera users email 4 --email new@example.com
```

The admin path: applies immediately, without the queue an email-change *request*
goes through.

### `vera users rm`

```bash
vera users rm 4 --yes
```

Permanently deletes the account. Prefer `--disable` when it is temporary.

---

## Requests (admin)

Two queues that need a decision. They exist because the two things a person can
ask about their own account — delete it, or move it to a new email — cannot take
effect on their own word: a lost laptop should not be able to delete a year of
quotations, and a stolen session should not be able to walk an account onto an
address its real owner does not control.

```bash
vera requests deletions list
vera requests deletions approve 3
vera requests deletions reject 3

vera requests emails list
vera requests emails approve 5
vera requests emails reject 5
```

Approving a deletion asks first and names the account, because "request 3" does
not carry enough meaning to be worth confirming.

---

## Backup (admin)

```bash
vera backup download --out backups/vera.dump
vera backup restore --file backups/vera.dump --yes
```

`download` fetches the most recent database dump from the server. It is a
Postgres **custom-format** dump, not a `.sql` file.

`restore` replaces the server's database. It refuses without `--yes`, checks the
`PGDMP` magic locally rather than uploading 300MB to be told, and the server
takes a safety snapshot before it starts. pg_restore warnings are reported
rather than swallowed.

---

## The installation

```bash
vera version        # this build's version
vera updates check  # what the server says the current release is
vera changelog      # the release notes the server serves
```

`updates check` uses the same public endpoint the desktop's update banner reads,
so it needs no session.

---

## Local settings

```bash
vera config show                              # settings, session, and where they point
vera config set --default-api https://staging.example.com
vera config set --json-output
vera config clear
```

Two files, and the split is deliberate:

| File | Holds | Mode |
|---|---|---|
| `~/.config/vera/config.json` | the default API address and output preferences | `0644` |
| `~/.config/vera/session.json` | the bearer token and who it belongs to | `0600` |

One is shareable; the other is a credential. `vera config show` never prints the
token — it is the command people paste into a bug report.

The desktop stores its token in the OS keyring instead. That is right for a
windowed app on a laptop and wrong here: a headless server has no keyring, and
neither does a CI job. A file with `0600` is the honest equivalent, and
`VERA_TOKEN` overrides it entirely for scripts that should not write one at all.

---

# Recipes

## Drive the whole system end to end

```bash
set -euo pipefail
API=https://api.arconpentapersada.co.id

printf '%s\n' "$VERA_PASSWORD" | vera login --email "$VERA_EMAIL" --password-stdin --api "$API"

vera clients list --json | jq -r '.clients[] | "\(.code)\t\(.name)"'

vera quotations validate --input quotation.json
out=$(vera quotations export --input quotation.json --out /tmp/q.pdf --json)
short=$(jq -r .shortId <<<"$out")
number=$(jq -r .quoteNumber <<<"$out")

vera verify "$short" --number "$number"
vera quotations list --customer "$number"
```

## Assert a document from a test or a CI job

```bash
# VERA_TOKEN avoids writing a session file at all.
export VERA_TOKEN=...
export VERA_API_URL=https://api.arconpentapersada.co.id

vera quotations export --input quotation.json --out q.pdf --json \
  | jq -e '.grandTotal == "62.100.000"'
```

## Find out why an export failed

```bash
vera quotations validate --input quotation.json     # local pricing, no filing
vera quotations preview  --input quotation.json     # what the server would do
vera --timeout 120s quotations export --input quotation.json --out q.pdf
```

A validation message names the line and the field, so it points straight at the
input file.

## Check the boundary between customer and internal output

```bash
vera quotations show 12 --html > customer.html
grep -i -e nett -e pph22 -e profit customer.html   # must print nothing
```

---

# Troubleshooting

**`not signed in; run 'vera login'`**
There is no token: not in `--token`, not in `VERA_TOKEN`, not in the session
file. `vera whoami` proves whether a token you *do* have still works.

**A token that works but reports signed out**
The token belongs to a different server than the one being asked. A session
records its server on purpose. Check `vera config show` for the address in use,
and `--api` for what you actually meant.

**`plain http on a remote host`**
The CLI refuses to send a password or token over unencrypted HTTP to anything but
loopback. Use `https`, or `--insecure` if the network is genuinely trusted.

**`the server has no backups yet`**
Nothing has been taken yet. That is a state, not a fault.

**A command is slow**
`--timeout` bounds each request. A cold Cloud Run instance adds a few seconds to
the first call; min-instances is 0.

**The PDF looks different from the desktop's**
The CLI loads the live company profile for the letterhead and brand colours. If
the profile could not be read it warns on stderr and uses the built-in artwork.

**Where is my session?**
`vera config show` prints the path. Deleting it signs you out.

# Security notes

- A token is a credential: `session.json` is `0600`, and `vera config show`
  never prints it.
- Passwords are never flags. Use the prompt or `--password-stdin`.
- Plain HTTP to a remote host is refused unless `--insecure`.
- Credentials are never printed, logged, or included in error messages.
- Tests never point at production: the Go suite's database is a throwaway
  schema that the setup drops, and it is reached only through
  `VERA_POSTGRES_TEST_DSN`.
