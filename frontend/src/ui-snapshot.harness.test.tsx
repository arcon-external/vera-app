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
import type { EditorItem } from "./components/ItemEditor";
import { ItemEditor } from "./components/ItemEditor";
import { CompanyGeneralPanel } from "./components/CompanyGeneralPanel";
import { CompanyPortalTab } from "./components/CompanyPortalTab";
import { CompanyQuotationTab } from "./components/CompanyQuotationTab";
import { CompanyTemplatePanel } from "./components/CompanyTemplatePanel";
import { CompanyUnitsPanel } from "./components/CompanyUnitsPanel";
import { LoginPage, LoginUpdateNotice } from "./components/LoginPage";
import { DocumentHomePage } from "./components/DocumentHomePage";
import { PurchaseOrderListView } from "./components/PurchaseOrderListPage";
import { POQuotationSelectorView } from "./components/POQuotationSelector";
import {
  PurchaseOrderDiscardPrompt,
  PurchaseOrderEditorView,
  PurchaseOrderLineRow,
} from "./components/PurchaseOrderEditor";
import { PurchaseOrderTotals } from "./components/PurchaseOrderTotals";
import { QuotationDetailHeader } from "./components/QuotationDetailPage";
import type { ArchivedQuotation } from "./archive";
import type { CurrencyInfo } from "./format";
import {
  initialPOSourceSelectorState,
  poSourceSelectorReducer,
} from "./poSelector";
import type { POSourceQuotation } from "./poSelector";
import {
  initialPOListFilters,
  initialPOListState,
} from "./poList";
import type { POListRowDTO, POListState } from "./poList";
import {
  calculatePurchaseOrder,
  initialPurchaseOrderEditorState,
} from "./purchaseOrder";
import type { PurchaseOrderDTO } from "./purchaseOrder";
import { PortalPreviewPanel } from "./components/PortalPreviewPanel";
import type { CompanyLabelEntry } from "./companyLabels";
import { DEFAULT_THEME, EMPTY_THEME } from "./company";
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

// The PO source selector fixture (issue #467): two available Quotations and
// one already used, so the screenshot proves the used row is greyed and marked
// without needing an interaction. The three states are the reducer's real
// output, not hand-written markup.
const PO_SELECTOR_ROWS: POSourceQuotation[] = [
  {
    id: 12,
    quoteNumber: "1502260012",
    subject: "Supply and installation of electrical systems",
    customerName: "PT. Infineon Technologies Batam",
    latestRevision: 2,
    issuedRevisionCount: 3,
    activePO: null,
  },
  {
    id: 11,
    quoteNumber: "1502260011",
    subject: "Mechanical works and pump replacement",
    customerName: "PT. H-Tech Oilfield Equipment, NOV XL Systems",
    latestRevision: 0,
    issuedRevisionCount: 1,
    activePO: { id: 20, state: "draft" },
  },
  {
    id: 10,
    quoteNumber: "1502260010",
    subject: "Cooling package",
    customerName: "PT. Datalec Global Solution",
    latestRevision: 1,
    issuedRevisionCount: 2,
    activePO: { id: 21, state: "saved" },
  },
];
const PO_SELECTOR_STEP_ONE = {
  ...initialPOSourceSelectorState,
  rows: PO_SELECTOR_ROWS,
  page: 1,
  pageSize: 15,
  total: 17,
  totalPages: 2,
};
const PO_SELECTOR_STEP_TWO = poSourceSelectorReducer(
  poSourceSelectorReducer(PO_SELECTOR_STEP_ONE, {
    type: "quotationSelected",
    quotation: PO_SELECTOR_ROWS[0],
  }),
  {
    type: "revisionsLoaded",
    quotationId: 12,
    revisions: [
      { revision: 2, issuedAt: "2026-10-20T10:00:00+07:00" },
      { revision: 1, issuedAt: "2026-10-18T10:00:00+07:00" },
      { revision: 0, issuedAt: "2026-10-15T10:00:00+07:00" },
    ],
  }
);
const PO_SELECTOR_STEP_THREE = poSourceSelectorReducer(
  poSourceSelectorReducer(PO_SELECTOR_STEP_TWO, {
    type: "revisionSelected",
    revision: 2,
  }),
  { type: "continue" }
);

// The manual PO draft editor (issue #468): the copied quotation lines, the
// editable header fields and the dirty-cancel state. Currency is the PO's own
// snapshot (USD here) rather than the live profile, so the money fields prove
// denomination follows the document.
const PO_EDITOR_USD: CurrencyInfo = {
  code: "USD",
  exponent: 2,
  symbol: "$",
  position: "before",
  groupSep: ",",
  decimalSep: ".",
  symbolSpaced: false,
};
const PO_EDITOR_ORDER: PurchaseOrderDTO = {
  PODate: "2026-10-01",
  RequestedDeliveryDate: "2026-11-20",
  UUID: "uuid-po-7",
  State: "draft",
  QuoteNumber: "1502260012",
  CustomerName: "PT. Infineon Technologies Batam",
  BillingAddress:
    "Batamindo Industrial Park, Lot 317, Mukakuning, Kota Batam, Kepulauan Riau, Indonesia",
  DeliveryAddress:
    "Batamindo Industrial Park, Lot 317, Mukakuning, Kota Batam, Kepulauan Riau, Indonesia",
  ShipToCode: "SHIP-317",
  ContactEmail: "buyer@infineon.example",
  ContactPhone: "+62 778 123 456",
  PONumber: "PO-INF-2026-091",
  ReceivedDate: "2026-10-02",
  Incoterms: "CIF",
  Currency: "USD",
  TaxZone: "NonFTZ",
  TaxCategory: "SupplyOnly",
  AdjustmentReason: "Round to the printed total",
  CreatedAt: "2026-10-02T10:00:00Z",
  UpdatedAt: "2026-10-02T10:00:00Z",
  Items: [
    {
      QuotationItemNo: 1,
      QuotationSubNo: null,
      LineNumber: 1,
      Description: "Supply and installation of electrical panels",
      QuantityScaled: 25000,
      UoM: "Lot",
      UnitPriceMinor: 125050,
      DiscountEnabled: true,
      DiscountPercent: 10,
      GrossMinor: 312625,
      DiscountMinor: 31262,
      NetMinor: 281363,
    },
    {
      QuotationItemNo: 2,
      QuotationSubNo: 1,
      LineNumber: 2,
      Description: "Mounting and cable termination",
      QuantityScaled: 10000,
      UoM: "Unit",
      UnitPriceMinor: 42000,
      DiscountEnabled: false,
      DiscountPercent: 0,
      GrossMinor: 42000,
      DiscountMinor: 0,
      NetMinor: 42000,
    },
  ],
  ID: 7,
  QuotationID: 12,
  SourceRevision: 2,
  CustomerID: 2,
  PaymentTerms: "45 days",
  DiscountPercent: 5,
  PpnBps: 1100,
  SubtotalMinor: 323363,
  DiscountMinor: 16168,
  PpnMinor: 33791,
  AdjustmentMinor: -250,
  GrandTotalMinor: 340736,
  PrintedSubtotalMinor: 325000,
  PrintedDiscountMinor: 0,
  PrintedPpnMinor: 35750,
  PrintedGrandTotalMinor: 360750,
};
const PO_EDITOR_INITIAL = initialPurchaseOrderEditorState(
  PO_EDITOR_ORDER,
  PO_EDITOR_USD
);
const PO_EDITOR_CALCULATION = calculatePurchaseOrder(
  PO_EDITOR_INITIAL,
  PO_EDITOR_USD
);
const PO_EDITOR_UOM_OPTIONS = [
  { value: "Lot", label: "Lot" },
  { value: "Unit", label: "Unit" },
  { value: "Pc", label: "Pc" },
];

// The PO archive (issue #469): a mixed page of drafts and a saved record, one
// row without a customer PO number, the customer multi-select and the 31-row
// pagination. The Source missing badge is deliberately absent: no record can
// carry it until deletion ships, and a fixture that shows it would imply the
// feature exists.
const PO_LIST_ROWS: POListRowDTO[] = [
  {
    UUID: "uuid-po-9",
    State: "draft",
    PONumber: "PO-INF-2026-091",
    QuoteNumber: "1502260012",
    CustomerName: "PT. Infineon Technologies Batam",
    ReceivedDate: "2026-10-02",
    Currency: "USD",
    ID: 9,
    CustomerID: 2,
    SourceRevision: 2,
    SourceMissing: false,
  },
  {
    UUID: "uuid-po-8",
    State: "saved",
    PONumber: "",
    QuoteNumber: "1503260004",
    CustomerName: "PT. H-Tech Oilfield Equipment, NOV XL Systems",
    ReceivedDate: "2026-09-30",
    Currency: "IDR",
    ID: 8,
    CustomerID: 3,
    SourceRevision: 0,
    SourceMissing: false,
  },
  {
    UUID: "uuid-po-7",
    State: "draft",
    PONumber: "PO-DAT-2026-14",
    QuoteNumber: "1504260007",
    CustomerName: "PT. Datalec Global Solution",
    ReceivedDate: "2026-09-24",
    Currency: "IDR",
    ID: 7,
    CustomerID: 4,
    SourceRevision: 1,
    SourceMissing: false,
  },
];
const PO_LIST_CUSTOMERS = [
  { ID: 2, Name: "PT. Infineon Technologies Batam" },
  { ID: 3, Name: "PT. H-Tech Oilfield Equipment, NOV XL Systems" },
  { ID: 4, Name: "PT. Datalec Global Solution" },
];
const PO_LIST_ARCHIVE: POListState = {
  ...initialPOListState,
  rows: PO_LIST_ROWS,
  customers: PO_LIST_CUSTOMERS,
  page: 1,
  pageSize: 15,
  total: 31,
  totalPages: 3,
};
const PO_LIST_NO_MATCH: POListState = {
  ...initialPOListState,
  filters: {
    ...initialPOListFilters,
    search: "datalec",
    customerIds: [2],
    receivedFrom: "2026-10-01",
    state: "saved",
  },
  customers: PO_LIST_CUSTOMERS,
  total: 0,
  totalPages: 1,
};


// The issued-Quotation detail header (#467 wave 2): the second PO entry path
// beside Edit, with the revision picker present because this quotation has
// three issued revisions. Only the header is rendered; the page fetches its
// revision in an effect, which this harness never runs.
const DETAIL_QUOTATION = {
  QuoteNumber: "1502260012",
  RevisionNumber: 2,
  AvailableRevisions: [2, 1, 0],
  CustomerName: "PT. Infineon Technologies Batam",
  Subject: "Supply and installation of electrical systems",
} as ArchivedQuotation;

// The line-item editor fixture (issue #430): the proof surface for the UoM
// select's background chevron. One plain line (its own UoM select) plus one
// line whose panel is open with a sub-item, so both UoMSelect call sites — the
// line row and the sub-item row — are on the page.
const EDITOR_ITEMS: EditorItem[] = [
  {
    key: 0,
    description: "Widget",
    bullets: [],
    qty: "1",
    uom: "Pc",
    unitPrice: "1000",
    marginEnabled: false,
    marginPercent: "0",
    discountEnabled: false,
    discountPercent: "0",
    subItems: [],
    showSubPrices: false,
  },
  {
    key: 1,
    description: "Installation",
    bullets: [],
    qty: "1",
    uom: "Lot",
    unitPrice: "0",
    marginEnabled: false,
    marginPercent: "0",
    discountEnabled: false,
    discountPercent: "0",
    showSubPrices: false,
    subItems: [
      {
        id: 1,
        description: "Cable tray",
        qty: "10",
        uom: "m",
        unitCost: "5000",
        marginEnabled: false,
        marginPercent: "0",
        discountEnabled: false,
        discountPercent: "0",
        bullets: [],
      },
    ],
  },
];

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
    // The Company settings General tab (#432): the two edited fields, the
    // currency picker and the logo card, with the tab's own save row under the
    // profile card. The snapshot carries the chip's "Saved" state, so the row
    // reads as a row rather than an error; the Unsaved changes state is pinned
    // in CompanyGeneralPanel.test.tsx.
    name: "CompanyGeneralPanel",
    element: (
      <CompanyGeneralPanel
        form={{ legalName: "PT Arcon Penta Persada", address: "Jl. Merdeka 1, Jakarta" }}
        onFieldChange={() => undefined}
        logoPreview={null}
        hasLogo={false}
        logoBusy={false}
        logoMeta={null}
        onPickLogo={() => undefined}
        onRemoveLogo={() => undefined}
        currency="IDR"
        catalogue={[
          { code: "IDR", exponent: 0, symbol: "Rp", position: "before", groupSep: ".", decimalSep: ",", symbolSpaced: false },
          { code: "USD", exponent: 2, symbol: "$", position: "before", groupSep: ",", decimalSep: ".", symbolSpaced: false },
        ]}
        onCurrencyChange={() => undefined}
        status="saved"
        busy={false}
        error={null}
        onSave={() => undefined}
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
    //
    // The save row at the bottom is the shared shape all four Company tabs now
    // render (#432): the button, the live chip and the standing hint.
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
        status="saved"
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
    // The Company settings Portal tab (#432): the verification page preview
    // above the four portal colours, with the tab's own save row and the frozen
    // sentence about the verdict colours that stay fixed.
    name: "CompanyPortalTab",
    element: (
      <CompanyPortalTab
        theme={DEFAULT_THEME}
        onChange={() => undefined}
        status="saved"
        busy={false}
        error={null}
        onSave={() => undefined}
      />
    ),
  },
  {
    // The Company settings merged Quotation tab (issues #429/#432): the real
    // sample document on top, then the four quotation colours, the 26 editable
    // labels, the tab's own save row and the two standing notes. The fixture
    // reuses the Template panel's served set above (one override, one cleared
    // field) so the preview and the wording snapshot the same draft, and the
    // stacked shape is the Portal tab's, verbatim.
    //
    // What this case cannot show, stated so it is not assumed: the panel
    // fetches the document from the binding in an effect, and renderToString
    // runs no effects — so the screenshot is the preview's pending state
    // (header, intro, the pending rectangle, the sample notes, the disabled
    // download) above the live colour, wording and save stacks. The document
    // itself is rendered by the engine, not here, and the blur/Enter
    // re-render stays hand-driven.
    name: "CompanyQuotationTab",
    element: (
      <CompanyQuotationTab
        labels={TEMPLATE_LABELS}
        theme={EMPTY_THEME}
        refreshKey={0}
        wordingStatus="saved"
        tabStatus="saved"
        labelsError={null}
        labelsNotice={null}
        themeError={null}
        invalid={null}
        busy={false}
        onLabelChange={() => undefined}
        onThemeChange={() => undefined}
        onSave={() => undefined}
        onFieldBlur={() => undefined}
        onFieldEnter={() => undefined}
      />
    ),
  },
  {
    // The sign-in page's update notice (issue #436): the only surface a staff
    // member locked out by a rollout has to find the fix. Fixed props in the
    // shape CheckForUpdates answers — 0.12.1 running, 0.13.1 available, the
    // windows installer resolved by the Go side — so the 360 render shows the
    // copy wrapping beside the Download button inside the narrowest column it
    // ever sits in. Pure markup: no effect runs, which is the point.
    name: "LoginUpdateNotice",
    element: (
      <LoginUpdateNotice
        current="0.12.1"
        latest="0.13.1"
        downloadURL="https://vera-downloads-508411.storage.googleapis.com/Vera-v0.13.1-windows-amd64-installer.exe"
        notesURL="https://api.arconpentapersada.co.id/v1/app/changelog"
        required={false}
      />
    ),
  },
  {
    // The state the login page exists for (#436's follow-up): the build is
    // below the server's minimum supported version, so the notice takes
    // UpdateBanner's danger tone and its wording instead of the mild "is
    // available" copy. That sentence is the longer of the two, so the 360
    // render here is the more informative one.
    name: "LoginUpdateNoticeRequired",
    element: (
      <LoginUpdateNotice
        current="0.12.1"
        latest="0.13.1"
        downloadURL="https://vera-downloads-508411.storage.googleapis.com/Vera-v0.13.1-windows-amd64-installer.exe"
        notesURL="https://api.arconpentapersada.co.id/v1/app/changelog"
        required
      />
    ),
  },
  {
    // The assembled sign-in page (issue #434): the logo, the form, and the
    // keep-me-signed-in checkbox in its default unchecked state, so the
    // whole surface finally has render evidence at both widths — the
    // checkbox sits between the password field and the submit button, and
    // its helper copy is the longest sentence in the narrow 360 column.
    //
    // What this case cannot show, stated so it is not assumed: the update
    // notice never appears here, because the mount effect that resolves it
    // runs in the browser only and renderToString runs no effects — this is
    // the page as a staff member sees it when the update check is silent
    // (up to date, or unreachable). The notice itself is the two cases
    // above. Checking the box, typing, and submitting stay hand-driven.
    name: "LoginPage",
    element: <LoginPage onSignedIn={() => undefined} />,
  },
  {
    // The document home (issue #466): the four destinations staff land on
    // after sign-in. Quotation and Purchase Order are enabled; Delivery Order
    // and Invoice render disabled with their Coming later badges, so the
    // screenshot shows unavailable features are not misleading.
    //
    // What this case cannot show, stated so it is not assumed: the clicks
    // behind the two enabled buttons, keyboard order, and focus visibility
    // stay hand-driven.
    name: "DocumentHome",
    element: (
      <DocumentHomePage
        onOpenQuotation={() => undefined}
        onOpenPurchaseOrder={() => undefined}
      />
    ),
  },
  {
    // The Purchase Order archive (issue #469): search, the customer
    // multi-select, the inclusive received range, the stored Draft/Saved
    // state, the disabled Source missing filter with its honest note, a mixed
    // page of rows and the server pager. At 360 the table scrolls inside its
    // card, like the Quotation archive; the filter grid stacks.
    //
    // What this case cannot show: typing, opening a row, the fetch effect,
    // focus order and the real scrollbar stay hand-driven.
    name: "PurchaseOrderArchive",
    element: (
      <PurchaseOrderListView
        state={PO_LIST_ARCHIVE}
        dispatch={() => undefined}
        onOpen={() => undefined}
        onAdd={() => undefined}
        onRetry={() => undefined}
      />
    ),
  },
  {
    // The filtered empty state (issue #469): four active filter dimensions,
    // no match, and the enabled Reset filters action. It is the case that
    // proves an empty page is not the unfiltered "No purchase orders yet"
    // message.
    name: "PurchaseOrderArchiveNoMatch",
    element: (
      <PurchaseOrderListView
        state={PO_LIST_NO_MATCH}
        dispatch={() => undefined}
        onOpen={() => undefined}
        onAdd={() => undefined}
        onRetry={() => undefined}
      />
    ),
  },
  {
    // The PO source selector, step 1 (issue #467): search, the checked
    // filter, pagination, and a used Quotation greyed with its reason. The
    // list scrolls inside the dialog rather than growing the page.
    //
    // What this case cannot show: typing, paging, selection, focus order and
    // the browser's real scrollbar stay hand-driven.
    name: "POSelectorStepOne",
    element: (
      <POQuotationSelectorView
        state={PO_SELECTOR_STEP_ONE}
        dispatch={() => undefined}
        onClose={() => undefined}
        onConfirm={() => undefined}
        onOpenDraft={() => undefined}
      />
    ),
  },
  {
    // Step 2 (issue #467): three issued revisions, newest first, with the
    // Continue action disabled until one is chosen.
    name: "POSelectorStepTwo",
    element: (
      <POQuotationSelectorView
        state={PO_SELECTOR_STEP_TWO}
        dispatch={() => undefined}
        onClose={() => undefined}
        onConfirm={() => undefined}
        onOpenDraft={() => undefined}
      />
    ),
  },
  {
    // Step 3 (issue #467): the exact pair, with Confirm enabled because the
    // reducer produced a complete selection.
    name: "POSelectorStepThree",
    element: (
      <POQuotationSelectorView
        state={PO_SELECTOR_STEP_THREE}
        dispatch={() => undefined}
        onClose={() => undefined}
        onConfirm={() => undefined}
        onOpenDraft={() => undefined}
      />
    ),
  },
  {
    // The manual PO draft editor (issue #468/#470): header snapshot, copied
    // lines in the PO's own USD currency, each line's own discount controls,
    // and the calculation inputs. At 360 this is the case that says whether
    // the five-field line grid reflows instead of overflowing.
    //
    // The totals panel sits below the 900px viewport, so it has its own case
    // (PurchaseOrderTotals) rather than being cropped out of this one.
    //
    // What this case cannot show: typing, Select behaviour, saving, and the
    // click that opens the discard confirmation stay hand-driven.
    name: "PurchaseOrderEditor",
    element: (
      <PurchaseOrderEditorView
        state={PO_EDITOR_INITIAL}
        dispatch={() => undefined}
        currency={PO_EDITOR_USD}
        uomOptions={PO_EDITOR_UOM_OPTIONS}
        incotermOptions={INCOTERM_OPTIONS}
        onSave={() => undefined}
        onCancel={() => undefined}
      />
    ),
  },
  {
    // One PO line with its row discount (issue #470). The editor page pushes
    // the lines below the fold at 360, so this is the case that shows whether
    // the discount controls and the line total reflow at the narrow width.
    name: "PurchaseOrderLineDiscount",
    element: (
      <div className="grid gap-3">
        <PurchaseOrderLineRow
          line={PO_EDITOR_INITIAL.lines[0]}
          index={0}
          currency={PO_EDITOR_USD}
          uomOptions={PO_EDITOR_UOM_OPTIONS}
          calculation={PO_EDITOR_CALCULATION.lines[0] ?? null}
          dispatch={() => undefined}
        />
        <PurchaseOrderLineRow
          line={PO_EDITOR_INITIAL.lines[1]}
          index={1}
          currency={PO_EDITOR_USD}
          uomOptions={PO_EDITOR_UOM_OPTIONS}
          calculation={PO_EDITOR_CALCULATION.lines[1] ?? null}
          dispatch={() => undefined}
        />
      </div>
    ),
  },
  {
    // The explained calculation (issue #470): every stage from line totals to
    // the grand total, the PPN printed separately, the signed adjustment with
    // its reason, and the source document's printed figures in their own
    // group. Rendered alone because the editor page pushes it below the fold.
    name: "PurchaseOrderTotals",
    element: (
      <PurchaseOrderTotals
        totals={PO_EDITOR_CALCULATION.totals}
        currency={PO_EDITOR_USD}
        discountPercent={5}
        ppnBps={PO_EDITOR_INITIAL.ppnBps}
        adjustmentReason={PO_EDITOR_INITIAL.header.adjustmentReason}
        printedTotals={PO_EDITOR_INITIAL.printedTotals}
        sourceRevision={2}
      />
    ),
  },
  {
    // The same panel while an input cannot be parsed (issue #470 wave 2): an
    // explicit pending state, never a column of fabricated zeros. The printed
    // group stays visible because it is source evidence, not our arithmetic.
    name: "PurchaseOrderTotalsPending",
    element: (
      <PurchaseOrderTotals
        totals={null}
        currency={PO_EDITOR_USD}
        discountPercent={null}
        ppnBps={PO_EDITOR_INITIAL.ppnBps}
        adjustmentReason=""
        printedTotals={PO_EDITOR_INITIAL.printedTotals}
        sourceRevision={2}
      />
    ),
  },
  {
    // The one discard dialog (issue #468, wave 3): Cancel and a guarded tab
    // click open this same modal, so both widths must keep both choices
    // readable and the scrim must cover the page behind it.
    name: "PurchaseOrderTabDiscard",
    element: (
      <div className="min-h-[420px] bg-paper p-4">
        <p className="text-small text-ink-500">
          The purchase order editor sits behind the prompt.
        </p>
        <PurchaseOrderDiscardPrompt
          onKeepEditing={() => undefined}
          onDiscard={() => undefined}
        />
      </div>
    ),
  },
  {
    // The issued-Quotation detail header (#467 wave 2): the title, revision
    // picker, the new "Add Purchase Order from a Quotation" action and Edit.
    // At 360 it is the case that says whether the two actions wrap without
    // overflowing.
    //
    // What this case cannot show: the click that opens the shared selector,
    // the focus restore on close, and the page below the header (which fetches
    // its revision in an effect) stay hand-driven.
    name: "QuotationDetailHeader",
    element: (
      <QuotationDetailHeader
        quotation={DETAIL_QUOTATION}
        activeRevision={2}
        revisionOptions={[
          { value: 2, label: "Revision 02 — latest" },
          { value: 1, label: "Revision 01" },
          { value: 0, label: "Revision 00" },
        ]}
        onRevisionChange={() => undefined}
        onBack={() => undefined}
        onEdit={() => undefined}
        onAddPurchaseOrder={() => undefined}
      />
    ),
  },
  {
    // The line-item editor (issue #430): the proof surface for the UoM
    // select's background chevron. The fixture above renders one plain line
    // and one line with an open sub-item panel, so both UoMSelect call sites
    // are on the page and the screenshot shows whether the arrow paints —
    // the defect was a data-URI SVG drawn without its xmlns, which Chrome
    // paints as nothing at all, silently.
    //
    // What this case cannot show, stated so it is not assumed: grid
    // navigation, paste handling and every other interaction stay
    // hand-driven — this is the initial render with a representative
    // fixture and the served unit list resolved.
    name: "ItemEditor",
    element: (
      <ItemEditor
        items={EDITOR_ITEMS}
        uomOptions={[
          { value: "Lot", label: "Lot" },
          { value: "Pc", label: "Pc" },
          { value: "m", label: "m" },
          { value: "m2", label: "m²" },
        ]}
        onAddItem={() => undefined}
        onRemoveItem={() => undefined}
        onChange={() => undefined}
        onAddBullet={() => undefined}
        onRemoveBullet={() => undefined}
        onBulletChange={() => undefined}
        onEnableSubItems={() => undefined}
        onDisableSubItems={() => undefined}
        onAddSubRow={() => undefined}
        onRemoveSubRow={() => undefined}
        onPatchSubRow={() => undefined}
        onMoveSubRow={() => undefined}
        onPasteSubRows={() => undefined}
        onAddSubBullet={() => undefined}
        onRemoveSubBullet={() => undefined}
        onSubBulletChange={() => undefined}
        onPasteItems={() => undefined}
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
