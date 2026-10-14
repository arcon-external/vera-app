// ui-snapshot.harness.test.tsx — a local, evidence-only render harness.
//
// `npm --prefix frontend test` skips this file (UI_SNAPSHOT is unset): the
// suite stays green and the file stays visible-but-skipped. `scripts/ui-snapshot.sh`
// sets UI_SNAPSHOT=1, runs only this file, and screenshots every page it writes.
//
// This is not a test in the assertion sense and not a pixel diff. It renders a
// registered component with renderToString (the house pattern), wraps it in a
// shell copied from the app root, inlines the freshly built Tailwind CSS and the
// Inter face the PDF already bundles, and writes one self-contained HTML per
// case. The screenshot is the evidence; a person (or a vision-capable model)
// reads the PNG. Nothing here is committed: pages land in UI_SNAPSHOT_DIR.
//
// What it cannot see, stated here so it is not assumed: no effects, no
// interactions (keyboard, paste, focus), no real OS font, no animation end
// states, and no pixel diff against a baseline.

import { mkdirSync, readFileSync, readdirSync, statSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import type { ReactElement } from "react";
import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { DetailsPanel } from "./components/DetailsPanel";
import type { DetailsState } from "./components/DetailsPanel";
import { CompanyQuotationTab } from "./components/CompanyQuotationTab";
import { CompanyTemplatePanel } from "./components/CompanyTemplatePanel";
import { CompanyUnitsPanel } from "./components/CompanyUnitsPanel";
import { PortalPreviewPanel } from "./components/PortalPreviewPanel";
import type { CompanyLabelEntry } from "./companyLabels";
import { EMPTY_THEME } from "./company";
import { TAX_TYPE_OPTIONS, TAX_ZONE_OPTIONS } from "./tax";

// frontend/src -> frontend -> repo root. Resolved from this file, not cwd, so
// the harness behaves the same however it is invoked.
const SRC_DIR = path.dirname(fileURLToPath(import.meta.url));
const FRONTEND_DIR = path.resolve(SRC_DIR, "..");
const REPO_ROOT = path.resolve(FRONTEND_DIR, "..");

const INCOTERM_OPTIONS = [
  "EXW",
  "FCA",
  "FAS",
  "FOB",
  "CFR",
  "CIF",
  "CPT",
  "CIP",
  "DPU",
  "DAP",
  "DDP",
].map((value) => ({ value, label: value }));

// The pilot case: the lead-time group (#407) that has never been rendered. The
// figures are the issue's example, so the four segments read 2 / 10 / 4 / 5.
const DETAILS: DetailsState = {
  subject: "Supply and installation of electrical and mechanical systems",
  recipientName: "Budi Santoso",
  validity: 30,
  paymentTerm: 90,
  leadTimeYears: 2,
  leadTimeMonths: 10,
  leadTimeWeeks: 4,
  leadTimeDays: 5,
  discountPercent: 0,
  incoterm: "CIF",
  taxZone: "NonFTZ",
  taxType: "Goods",
  serviceCategory: "",
};

interface SnapshotCase {
  name: string;
  element: ReactElement;
}

// The served label set for the Template panel case: the 26 entries in document
// order, each carrying its built-in default. Two entries are served as a
// company would leave them after editing — one overridden, one cleared — so the
// snapshot shows the Reset button and the Built-in badge beside the untouched
// rows. This mirrors GET /v1/company/labels; the wording is the server's.
function label(
  Key: string,
  Default: string,
  MaxLength: number,
  Terms = false,
  Value = Default
): CompanyLabelEntry {
  return { Key, Value, Default, MaxLength, Terms };
}

const TEMPLATE_LABELS: CompanyLabelEntry[] = [
  label("docTitle", "QUOTATION", 24, false, "PROPOSAL"),
  label("recipientHeading", "Quotation to", 60),
  label("termsHeading", "Terms and Conditions", 60),
  label("metaQuotationNo", "Quotation No.", 20),
  label("metaIssueDate", "Issue Date", 20),
  label("metaValidUntil", "Valid Until", 20),
  label("metaRevision", "Revision", 20),
  label("subject", "Subject", 30),
  label("columnNo", "No.", 20),
  label("columnDescription", "Description", 20),
  label("columnQty", "Qty.", 20, false, ""),
  label("columnUoM", "UoM", 20),
  label("columnUnitPrice", "Unit Price", 20),
  label("columnAfterDisc", "After Disc.", 20),
  label("columnTotal", "Total", 20),
  label("totalSubTotal", "Sub Total", 60),
  label("totalDiscount", "Discount", 60),
  label("totalPpn", "PPN 11%", 60),
  label("totalGrandTotal", "Grand Total", 60),
  label(
    "closing",
    "We will be happy to supply information that you may need and trust " +
      "that you call on us to fill your order, which will receive our prompt " +
      "and careful attention.",
    300
  ),
  label("verifyScan", "Scan to verify this quotation", 60),
  label("verifyCode", "or enter code", 60),
  label("termsPayment", "1. Payment terms agreed for this quotation is {payment} days.", 300, true),
  label("termsValidity", "2. This quotation is valid for {validity} days.", 300, true),
  label("termsLeadTime", "3. Lead time is {leadtime}.", 300, true),
  label("termsIncoterm", "4. Incoterms: {incoterm}", 300, true),
];

const CASES: SnapshotCase[] = [
  {
    name: "DetailsPanel",
    element: (
      <DetailsPanel
        value={DETAILS}
        customerOptions={[{ value: 2, label: "PT Contoh Nusantara" }]}
        customerId={2}
        incotermOptions={INCOTERM_OPTIONS}
        taxZoneOptions={TAX_ZONE_OPTIONS}
        taxTypeOptions={TAX_TYPE_OPTIONS}
        onChange={() => undefined}
        onCustomerChange={() => undefined}
        onPaste={() => undefined}
      />
    ),
  },
  {
    // The Company settings Units tab (issue #337; the select restyle and the
    // row relayout of #429): the chevron-styled canonical-unit picker and the
    // served unit list with its captioned fields ("Value — stored" /
    // "Label — printed") and the footer row holding the badge and the row
    // controls. A representative slice of the seeded list — canonical first, a
    // short token (m2 whose label is the wider m²), and the widest seed
    // (Quarter) — so the snapshot reads the layout at both widths.
    name: "CompanyUnitsPanel",
    element: (
      <CompanyUnitsPanel
        units={[
          { value: "Lot", label: "Lot" },
          { value: "Pc", label: "Pc" },
          { value: "set", label: "set" },
          { value: "meter", label: "meter" },
          { value: "m2", label: "m²" },
          { value: "Hour", label: "Hour" },
          { value: "Quarter", label: "Quarter" },
        ]}
        canonical="Lot"
        onUnitsChange={() => undefined}
        onCanonicalChange={() => undefined}
        busy={false}
        error={null}
        onSaveUnits={() => undefined}
      />
    ),
  },
  {
    // The Company settings Template tab (issue #339): all 26 editable labels,
    // grouped, prefilled from the served shape. The fixture below mirrors what
    // GET /v1/company/labels serves — the wording is the server's, copied here
    // so the snapshot reads the real panel. Three row states are shown: an
    // override (docTitle prints PROPOSAL, so its Reset button appears), a
    // cleared field (columnQty is empty and marked Built-in), and untouched
    // rows. The colour card that shares this tab is the existing
    // CompanyThemePanel and needs no case of its own.
    name: "CompanyTemplatePanel",
    element: (
      <CompanyTemplatePanel
        labels={TEMPLATE_LABELS}
        saveState="saved"
        error={null}
        notice={null}
        onChange={() => undefined}
      />
    ),
  },
  {
    // The Company settings Portal tab (issue #340): the verification page
    // preview beside the four portal colours.
    //
    // What this case cannot show, stated so it is not assumed: the panel
    // fetches the real document from the server in an effect, and
    // renderToString runs no effects — so the screenshot is the panel's
    // pending state (header, intro, the five state buttons, the pending
    // rectangle, the fixed-wording note). The iframe the document lands in is
    // the same one ReviewPanel and QuotationDetailPage already use, and the
    // document itself is rendered by the server, not here. Seeing a real
    // verdict in the preview stays hand-driven.
    name: "PortalPreviewPanel",
    element: <PortalPreviewPanel theme={EMPTY_THEME} />,
  },
  {
    // The Company settings merged Quotation tab (issue #429): the real sample
    // PDF on top, the 26 editable labels below it, then the four quotation
    // colours and the one caption naming both save paths. The fixture reuses
    // the Template panel's served set above (one override, one cleared field)
    // so the preview and the wording snapshot the same draft, and the stacked
    // shape is the Portal tab's, verbatim.
    //
    // What this case cannot show, stated so it is not assumed: the panel
    // fetches the PDF bytes from the binding in an effect, and renderToString
    // runs no effects — so the screenshot is the preview's pending state
    // (header, intro, the pending rectangle, the sample note, the disabled
    // download) above the live wording and colour stacks. The iframe the
    // document lands in is a plain embed of those bytes, and the blur/Enter
    // re-render stays hand-driven.
    name: "CompanyQuotationTab",
    element: (
      <CompanyQuotationTab
        labels={TEMPLATE_LABELS}
        theme={EMPTY_THEME}
        refreshKey={0}
        labelsStatus="saved"
        labelsError={null}
        labelsNotice={null}
        onLabelChange={() => undefined}
        onThemeChange={() => undefined}
        onFieldBlur={() => undefined}
        onFieldEnter={() => undefined}
      />
    ),
  },
];

// newestBuiltCss reads the most recently written frontend/dist/assets/*.css.
// The script builds the frontend first, so this is the CSS for the code as it
// stands now, not a stale copy. Read lazily (inside the test): a fresh checkout
// has no dist/, and reading at module load would break the skipped suite.
function newestBuiltCss(): string {
  const assetsDir = path.join(FRONTEND_DIR, "dist", "assets");
  const files = readdirSync(assetsDir)
    .filter((file) => file.endsWith(".css"))
    .map((file) => path.join(assetsDir, file));
  if (files.length === 0) {
    throw new Error(`no built CSS in ${assetsDir}; run the frontend build first`);
  }
  files.sort((a, b) => statSync(b).mtimeMs - statSync(a).mtimeMs);
  return readFileSync(files[0], "utf8");
}

function fontFace(): string {
  const medium = readFileSync(
    path.join(REPO_ROOT, "backend", "assets", "inter-medium.ttf")
  ).toString("base64");
  const bold = readFileSync(
    path.join(REPO_ROOT, "backend", "assets", "inter-bold.ttf")
  ).toString("base64");
  return [
    `@font-face{font-family:"Inter";font-style:normal;font-weight:400;src:url(data:font/ttf;base64,${medium}) format("truetype");}`,
    `@font-face{font-family:"Inter";font-style:normal;font-weight:700;src:url(data:font/ttf;base64,${bold}) format("truetype");}`,
  ].join("\n");
}

// buildPage assembles one self-contained page: the built CSS, then the Inter
// face, then a body font-family that puts Inter first over the app's system
// stack. The last rule wins over Tailwind preflight's html font-family.
function buildPage(name: string, markup: string): string {
  const css = newestBuiltCss();
  const shell = [
    '<div id="root">',
    '  <div class="flex min-h-full flex-col bg-paper">',
    '    <main class="flex-1">',
    '      <div class="mx-auto w-full max-w-[1560px] px-5 pt-7 pb-10 lg:px-8 lg:pt-9">',
    '        <div class="flex flex-col gap-6">',
    `          ${markup}`,
    "        </div>",
    "      </div>",
    "    </main>",
    "  </div>",
    "</div>",
  ].join("\n");

  return [
    "<!DOCTYPE html>",
    '<html lang="en">',
    "<head>",
    '<meta charset="UTF-8" />',
    '<meta name="viewport" content="width=device-width, initial-scale=1.0" />',
    `<title>${name} — UI snapshot</title>`,
    "<!--",
    "  Local UI snapshot written by scripts/ui-snapshot.sh. Not committed.",
    "  The app's font stack is -apple-system / BlinkMacSystemFont / Segoe UI /",
    "  system-ui, none of which exists inside the render container. Inter — the",
    "  face the PDF template already bundles — is inlined here (base64) as the",
    "  closest available approximation: it is not the app's real rendering font,",
    "  only its stand-in. The CSS and the root shell classes are the app's own.",
    "-->",
    "<style>",
    css,
    fontFace(),
    'body{font-family:"Inter",-apple-system,BlinkMacSystemFont,"SF Pro Text","Segoe UI",system-ui,sans-serif;}',
    "</style>",
    "</head>",
    "<body>",
    shell,
    "</body>",
    "</html>",
    "",
  ].join("\n");
}

const ENABLED = process.env.UI_SNAPSHOT === "1";
const OUT_DIR = process.env.UI_SNAPSHOT_DIR ?? "/tmp/opencode/ui-snap";

describe.skipIf(!ENABLED)("UI snapshot harness", () => {
  for (const snapshotCase of CASES) {
    it(`writes ${snapshotCase.name}.html`, () => {
      const markup = renderToString(snapshotCase.element);
      const page = buildPage(snapshotCase.name, markup);

      mkdirSync(OUT_DIR, { recursive: true });
      const file = path.join(OUT_DIR, `${snapshotCase.name}.html`);
      writeFileSync(file, page, "utf8");

      // Cheap guards that the page is real: the markup landed and the styles
      // and font data are inlined (a page without them screenshots unstyled).
      expect(page).toContain(markup);
      expect(page).toContain("@font-face");
      expect(page).toContain("data:font/ttf;base64,");
      expect(page.length).toBeGreaterThan(100_000);
    });
  }
});
