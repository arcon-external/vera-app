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
import {
  DeletionChallengePanel,
  DeletionDialogView,
} from "./components/DeletionDialog";
import { PurchaseOrderDocumentPanelView } from "./components/PurchaseOrderDocumentPanel";
import { PurchaseOrderConflictPrompt } from "./components/PurchaseOrderConflictPrompt";
import { PurchaseOrderHistoryPanel } from "./components/PurchaseOrderHistoryPanel";
import { POQuotationSelectorView } from "./components/POQuotationSelector";
import {
  PurchaseOrderDiscardPrompt,
  PurchaseOrderEditorView,
  PurchaseOrderLineRow,
} from "./components/PurchaseOrderEditor";
import {
  PurchaseOrderDifferenceReviewView,
  PurchaseOrderFinalConfirmationView,
  PurchaseOrderSavedView,
} from "./components/PurchaseOrderFinalize";
import {
  evaluateReview,
  type PurchaseOrderReviewDTO,
  type PurchaseOrderSourceDTO,
} from "./poFinalize";
import { PurchaseOrderTotals } from "./components/PurchaseOrderTotals";
import {
  PurchaseOrderRepairConfirmationView,
  PurchaseOrderSourceChange,
} from "./components/PurchaseOrderRepair";
import { QuotationDetailHeader } from "./components/QuotationDetailPage";
import {
  deletionReducer,
  initialDeletionState,
  type DeletionPreviewDTO,
} from "./deletion";
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
  buildPurchaseOrderUpdate,
  calculatePurchaseOrder,
  initialPurchaseOrderEditorState,
} from "./purchaseOrder";
import type { PurchaseOrderHistoryEntry } from "./poCorrection";
import type { PurchaseOrderDTO } from "./purchaseOrder";
import type { PurchaseOrderDocumentStateDTO } from "./purchaseOrderDocument";
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
  Version: 1,
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
// The correction editor and its history (issue #473). The saved record is at
// version 4; the reason is required, the confirmation number is text rather
// than a control, and the two retained snapshots show a correction with its
// attachment reference and a recovered draft without one.
const PO_CORRECTING_STATE = initialPurchaseOrderEditorState(
  { ...PO_EDITOR_ORDER, State: "saved", Version: 4, ConfirmationNumber: "2302260001" },
  PO_EDITOR_USD
);
const PO_HISTORY: PurchaseOrderHistoryEntry[] = [
  {
    UUID: "corr-2",
    Kind: "correction",
    State: "saved",
    Reason: "The customer moved to a new warehouse",
    ActorName: "Budi Santoso",
    CreatedAt: "2026-10-21T10:15:00+07:00",
    QuoteNumber: "1502260044",
    CustomerName: "PT. Batam Electrical Works",
    SourceRevision: 1,
    Currency: "USD",
    TaxZone: "FTZ",
    TaxCategory: "Goods",
    BillingAddress: "Jl. Billing 12, Batam Center, Kota Batam",
    DeliveryAddress: "Batamindo Industrial Park, Lot 317, Mukakuning, Kota Batam",
    ShipToCode: "SHIP-INF-3",
    ContactEmail: "procurement@buyer.example",
    ContactPhone: "+62 778 555 0101",
    PONumber: "PO-INF-2026-091",
    PODate: "2026-10-02",
    ReceivedDate: "2026-10-03",
    RequestedDeliveryDate: "2026-11-20",
    PaymentTerms: "45 days",
    Incoterms: "CIF",
    AdjustmentReason: "Round to the printed total",
    PreviousVersion: 4,
    ResultingVersion: 5,
    DiscountPercent: 5,
    SubtotalMinor: 323363,
    DiscountMinor: 16168,
    PpnBps: 1100,
    PpnMinor: 33791,
    AdjustmentMinor: -250,
    GrandTotalMinor: 340736,
    Attachment: {
      Version: 2,
      SHA256: "d2f4a6c8e0b1".padEnd(64, "0"),
      Filename: "Customer PO 4500098123.pdf",
    },
    Items: [
      {
        LineNumber: 1,
        Description: "Supply and installation of electrical panels",
        QuantityScaled: 25000,
        UoM: "Lot",
        UnitPriceMinor: 125050,
        DiscountMinor: 31262,
        NetMinor: 281363,
      },
      {
        LineNumber: 2,
        Description: "Mounting and cable termination",
        QuantityScaled: 10000,
        UoM: "Unit",
        UnitPriceMinor: 42000,
        DiscountMinor: 0,
        NetMinor: 42000,
      },
    ],
  },
  {
    UUID: "corr-1",
    Kind: "draft_overwrite",
    State: "draft",
    Reason: "Draft overwritten after a concurrent save",
    ActorName: "Sari",
    CreatedAt: "2026-10-21T09:00:00+07:00",
    QuoteNumber: "1502260044",
    CustomerName: "PT. Batam Electrical Works",
    SourceRevision: 1,
    Currency: "USD",
    TaxZone: "FTZ",
    TaxCategory: "Goods",
    BillingAddress: "Jl. Billing 12, Batam Center, Kota Batam",
    DeliveryAddress: "Batamindo Industrial Park, Lot 317, Mukakuning, Kota Batam",
    ShipToCode: "SHIP-INF-3",
    ContactEmail: "procurement@buyer.example",
    ContactPhone: "+62 778 555 0101",
    PONumber: "PO-INF-2026-090",
    PODate: "",
    ReceivedDate: "2026-10-03",
    RequestedDeliveryDate: "",
    PaymentTerms: "30 days",
    Incoterms: "EXW",
    AdjustmentReason: "",
    PreviousVersion: 3,
    ResultingVersion: 4,
    DiscountPercent: 0,
    SubtotalMinor: 300000,
    DiscountMinor: 0,
    PpnBps: 0,
    PpnMinor: 0,
    AdjustmentMinor: 0,
    GrandTotalMinor: 300000,
    Attachment: null,
    Items: [
      {
        LineNumber: 1,
        Description: "Cooling package",
        QuantityScaled: 10000,
        UoM: "Lot",
        UnitPriceMinor: 300000,
        DiscountMinor: 0,
        NetMinor: 300000,
      },
    ],
  },
];

const PO_EDITOR_UOM_OPTIONS = [
  { value: "Lot", label: "Lot" },
  { value: "Unit", label: "Unit" },
  { value: "Pc", label: "Pc" },
];

// The single optional customer PO PDF (issue #472), shown in both branches:
// empty, attached with one retained prior version, and the inline removal
// confirmation. The bytes are irrelevant to the panel; only the metadata is
// rendered.
const PO_DOCUMENT_NONE: PurchaseOrderDocumentStateDTO = {
  Document: null,
  Versions: [],
};
const PO_DOCUMENT_ATTACHED: PurchaseOrderDocumentStateDTO = {
  Document: {
    UUID: "doc-2",
    SHA256: "d2f4a6c8e0b1",
    ContentType: "application/pdf",
    OriginalFilename: "Customer PO 4500098123.pdf",
    UploadedAt: "2026-10-20T09:15:00Z",
    ID: 2,
    PurchaseOrderID: 7,
    Version: 2,
    SizeBytes: 1_536_000,
    UploadedBy: 4,
    Active: true,
  },
  Versions: [
    {
      UUID: "doc-2",
      SHA256: "d2f4a6c8e0b1",
      ContentType: "application/pdf",
      OriginalFilename: "Customer PO 4500098123.pdf",
      UploadedAt: "2026-10-20T09:15:00Z",
      ID: 2,
      PurchaseOrderID: 7,
      Version: 2,
      SizeBytes: 1_536_000,
      UploadedBy: 4,
      Active: true,
    },
    {
      UUID: "doc-1",
      SHA256: "a1b2c3d4e5f6",
      ContentType: "application/pdf",
      OriginalFilename: "Customer PO scanned.pdf",
      UploadedAt: "2026-10-18T14:40:00Z",
      ID: 1,
      PurchaseOrderID: 7,
      Version: 1,
      SizeBytes: 2_048_000,
      UploadedBy: 4,
      Active: false,
      ReplacedAt: "2026-10-20T09:15:00Z",
    },
  ],
};

// The final-save review (issue #471): the exact source revision beside the
// entered PO. Two differing values (payment terms and the first line's price),
// one source line the PO omits, and one extra PO line, so the screenshot shows
// both choices unpressed, the missing/extra review, and the printed-total
// check with its unexplained remainder.
const PO_REVIEW_SOURCE: PurchaseOrderSourceDTO = {
  QuotationID: 12,
  RevisionNumber: 2,
  QuoteNumber: "1502260012",
  CustomerID: 2,
  CustomerName: "PT. Infineon Technologies Batam",
  Currency: "USD",
  PaymentTerms: "60 days from delivery",
  Incoterms: "FOB",
  DiscountPercent: 5,
  Lines: [
    {
      QuotationItemNo: 1,
      QuotationSubNo: null,
      LineLabel: "1",
      Description: "Supply and installation of electrical panels",
      UoM: "Lot",
      QuantityScaled: 25000,
      UnitPriceMinor: 118000,
      DiscountEnabled: true,
      DiscountPercent: 10,
    },
    {
      QuotationItemNo: 2,
      QuotationSubNo: 1,
      LineLabel: "2.1",
      Description: "Mounting and cable termination",
      UoM: "Unit",
      QuantityScaled: 10000,
      UnitPriceMinor: 42000,
      DiscountEnabled: false,
      DiscountPercent: 0,
    },
    {
      QuotationItemNo: 3,
      QuotationSubNo: null,
      LineLabel: "3",
      Description: "Electrical commissioning report",
      UoM: "Lot",
      QuantityScaled: 10000,
      UnitPriceMinor: 0,
      DiscountEnabled: false,
      DiscountPercent: 0,
    },
  ],
  PrintedSubtotalMinor: 339690,
  PrintedDiscountMinor: 16984,
  PrintedPpnMinor: 35500,
  PrintedGrandTotalMinor: 358205,
};
const PO_REVIEW_ORDER: PurchaseOrderDTO = {
  ...PO_EDITOR_ORDER,
  PaymentTerms: "45 days",
  Items: [
    PO_EDITOR_ORDER.Items[0],
    PO_EDITOR_ORDER.Items[1],
    {
      QuotationItemNo: null,
      QuotationSubNo: null,
      LineNumber: 3,
      Description: "Freight and insurance",
      QuantityScaled: 10000,
      UoM: "Lot",
      UnitPriceMinor: 12500,
      DiscountEnabled: false,
      DiscountPercent: 0,
      GrossMinor: 12500,
      DiscountMinor: 0,
      NetMinor: 12500,
    },
  ],
};
const PO_REVIEW_STATE = initialPurchaseOrderEditorState(
  PO_REVIEW_ORDER,
  PO_EDITOR_USD
);
const PO_REVIEW_DTO: PurchaseOrderReviewDTO = {
  Order: PO_REVIEW_ORDER,
  Source: PO_REVIEW_SOURCE,
  DocumentState: "none",
};
const PO_REVIEW_EVALUATION = evaluateReview(
  PO_REVIEW_STATE,
  PO_REVIEW_SOURCE,
  PO_EDITOR_USD,
  {},
  [],
  []
);

// The source repair (issue #475) compares the same record against a genuinely
// different link, so the old → new pair in the evidence is not two identical
// quotations: revision 2 becomes revision 0 of another issued quotation.
const PO_REPAIR_SOURCE: PurchaseOrderSourceDTO = {
  ...PO_REVIEW_SOURCE,
  QuotationID: 19,
  QuoteNumber: "1502260051",
  RevisionNumber: 0,
};
const PO_REPAIR_REVIEW_DTO: PurchaseOrderReviewDTO = {
  Order: {
    ...PO_REVIEW_ORDER,
    State: "saved",
    PdfAbsentConfirmed: true,
    ConfirmationNumber: "2302260001",
  },
  Source: PO_REPAIR_SOURCE,
  DocumentState: "none",
};
const PO_REPAIR_EVALUATION = evaluateReview(
  PO_REVIEW_STATE,
  PO_REPAIR_SOURCE,
  PO_EDITOR_USD,
  {},
  [],
  []
);

// The PO archive (issue #469, #481): a mixed page of drafts and saved records,
// one row without a customer PO number, one source-missing row whose former
// quotation is named beside its own readable values, the independent Source
// filter, the customer multi-select and the 31-row pagination.
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
  {
    UUID: "uuid-po-6",
    State: "saved",
    PONumber: "PO-HTE-2026-08",
    QuoteNumber: "1505260011",
    CustomerName: "PT. H-Tech Oilfield Equipment, NOV XL Systems",
    ReceivedDate: "2026-09-18",
    Currency: "IDR",
    ID: 6,
    CustomerID: 3,
    SourceRevision: 2,
    SourceMissing: true,
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


// #480: the permanent-deletion dialog. Three states are evidence: a single
// record with the surviving document listed, a whole chain with the issued
// ten-letter challenge, and a chain that moved between the challenge and the
// confirmation. The dialog is an overlay in the app; here it is rendered inline
// because the harness screenshots a page, not a modal interaction.
const DELETION_PREVIEW: DeletionPreviewDTO = {
  TargetType: "quotation",
  TargetID: 12,
  TargetUUID: "4d0f8f0e-1d43-4e3c-9f9a-5c0e0b7a1f12",
  DocumentNumber: "1502260012",
  DocumentNumberLabel: "Quotation number",
  QuoteNumber: "1502260012",
  PONumber: "",
  CustomerName: "PT. Infineon Technologies Batam",
  State: "issued",
  SourceMissing: false,
  Version: 0,
  Fingerprint: "v1;target=quotation;quotation_id=12;revisions=0,1",
  Scopes: [
    {
      Scope: "record",
      Available: true,
      UnavailableReason: "",
      RequiresChallenge: false,
      Removes: [
        {
          Type: "quotation",
          ID: 12,
          UUID: "4d0f8f0e-1d43-4e3c-9f9a-5c0e0b7a1f12",
          DocumentNumber: "1502260012",
          SourceReference: "",
          Label: "Quotation 1502260012",
          State: "issued",
          Version: 0,
          Revisions: [0, 1],
          DocumentVersions: 2,
          HistoryEntries: 0,
        },
      ],
      Preserves: [
        {
          Type: "purchase_order",
          ID: 34,
          UUID: "9a6f7e2c-51c8-4f18-9a3a-1d0f2b6c4e77",
          DocumentNumber: "PO-INF-2026-091",
          SourceReference: "1502260012",
          Label: "Purchase order PO-INF-2026-091",
          State: "saved",
          Version: 7,
          Revisions: [],
          DocumentVersions: 1,
          HistoryEntries: 3,
        },
      ],
    },
    {
      Scope: "chain",
      Available: true,
      UnavailableReason: "",
      RequiresChallenge: true,
      Removes: [
        {
          Type: "quotation",
          ID: 12,
          UUID: "4d0f8f0e-1d43-4e3c-9f9a-5c0e0b7a1f12",
          DocumentNumber: "1502260012",
          SourceReference: "",
          Label: "Quotation 1502260012",
          State: "issued",
          Version: 0,
          Revisions: [0, 1],
          DocumentVersions: 2,
          HistoryEntries: 0,
        },
        {
          Type: "purchase_order",
          ID: 34,
          UUID: "9a6f7e2c-51c8-4f18-9a3a-1d0f2b6c4e77",
          DocumentNumber: "PO-INF-2026-091",
          SourceReference: "1502260012",
          Label: "Purchase order PO-INF-2026-091",
          State: "saved",
          Version: 7,
          Revisions: [],
          DocumentVersions: 1,
          HistoryEntries: 3,
        },
      ],
      Preserves: [],
    },
  ],
};

function deletionState(scope: "record" | "chain") {
  return deletionReducer(
    deletionReducer(initialDeletionState, {
      type: "previewLoaded",
      preview: DELETION_PREVIEW,
    }),
    { type: "scopeChanged", scope }
  );
}

const DELETION_CHAIN_READY = deletionReducer(deletionState("chain"), {
  type: "challengeLoaded",
  challenge: {
    ChallengeID: "challenge-480",
    Letters: "ABCDEFGHJK",
    ExpiresAt: "2026-10-21T10:15:00Z",
    Fingerprint: DELETION_PREVIEW.Fingerprint,
  },
});

const DELETION_CHANGED = deletionReducer(DELETION_CHAIN_READY, {
  type: "chainChanged",
  changed: {
    Preview: { ...DELETION_PREVIEW, Fingerprint: "v1;target=quotation;quotation_id=12;revisions=0,1,2" },
    Changes: [
      {
        Field: "purchase_order_34_version",
        Label: "Purchase order 34 version",
        Before: "7",
        After: "8",
      },
      {
        Field: "revisions",
        Label: "Quotation revisions",
        Before: "0,1",
        After: "0,1,2",
      },
    ],
  },
});

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
    // The selector in repair mode (issue #475): the same three steps with the
    // repair wording. The server has already exempted the repaired record's
    // own Quotation, so the remaining occupied rows are still disabled and no
    // "Open draft" shortcut is offered.
    name: "POSelectorRepairMode",
    element: (
      <POQuotationSelectorView
        state={{
          ...PO_SELECTOR_STEP_ONE,
          rows: [
            { ...PO_SELECTOR_ROWS[0], activePO: null },
            PO_SELECTOR_ROWS[1],
            PO_SELECTOR_ROWS[2],
          ],
        }}
        dispatch={() => undefined}
        mode="repair"
        onClose={() => undefined}
        onConfirm={() => undefined}
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
        onFinalSave={() => undefined}
        finalSaving={false}
      />
    ),
  },
  {
    // The correction editor (issue #473): the required reason, the saved
    // record's version, and the protected confirmation number shown as text.
    // At 360 this says whether the reason card and the protected note wrap.
    name: "PurchaseOrderCorrectionEditor",
    element: (
      <PurchaseOrderEditorView
        state={PO_CORRECTING_STATE}
        dispatch={() => undefined}
        currency={PO_EDITOR_USD}
        uomOptions={PO_EDITOR_UOM_OPTIONS}
        incotermOptions={INCOTERM_OPTIONS}
        onSave={() => undefined}
        onCancel={() => undefined}
        onFinalSave={() => undefined}
        finalSaving={false}
        mode="correction"
        correctionReason="The customer moved to a new warehouse"
        onCorrectionReasonChange={() => undefined}
        confirmationNumber="2302260001"
      />
    ),
  },
  {
    // The version warning (issue #473): the two choices when a draft really is
    // still a draft -- the current version, the writer's name and time, and
    // Overwrite as the primary action.
    name: "PurchaseOrderVersionConflict",
    element: (
      <div className="min-h-[420px] bg-paper p-4">
        <p className="text-small text-ink-500">
          The correction editor sits behind the warning.
        </p>
        <PurchaseOrderConflictPrompt
          conflict={{
            CurrentState: "draft",
            CurrentVersion: 5,
            UpdatedBy: 9,
            UpdatedByName: "Budi Santoso",
            UpdatedAt: "2026-10-21T10:15:00+07:00",
          }}
          overwriting={false}
          onCancel={() => undefined}
          onCorrectSaved={() => undefined}
          onOverwrite={() => undefined}
        />
      </div>
    ),
  },
  {
    // The saved-record transition (issue #473 wave 2): a draft write met a
    // record that is already saved, so the dialog offers no Overwrite. The
    // correction form, with its required reason, is the one safe way forward.
    name: "PurchaseOrderSavedRecordTransition",
    element: (
      <div className="min-h-[420px] bg-paper p-4">
        <p className="text-small text-ink-500">
          The draft editor keeps its working copy behind the warning.
        </p>
        <PurchaseOrderConflictPrompt
          conflict={{
            CurrentState: "saved",
            CurrentVersion: 5,
            UpdatedBy: 9,
            UpdatedByName: "Budi Santoso",
            UpdatedAt: "2026-10-21T10:15:00+07:00",
          }}
          overwriting={false}
          savedTransition
          onCancel={() => undefined}
          onCorrectSaved={() => undefined}
          onOverwrite={() => undefined}
        />
      </div>
    ),
  },
  {
    // The correction history (issue #473): a saved correction with its
    // attachment reference and a recovered draft, each with reason, actor,
    // time, versions, replaced lines and totals.
    name: "PurchaseOrderHistory",
    element: (
      <PurchaseOrderHistoryPanel
        entries={PO_HISTORY}
        currency={PO_EDITOR_USD}
        expandPreviousValues
      />
    ),
  },
  {
    // The final-save difference review (issue #471): every differing value
    // with both choices unpressed, the fixed currency statement, one omitted
    // source line, two extra PO lines (one with a suggestion), the corrected
    // match picker and the recalculated totals. At 360 this is the case that
    // says whether the two-choice row stacks instead of overflowing.
    name: "PurchaseOrderDifferenceReview",
    element: (
      <PurchaseOrderDifferenceReviewView
        state={PO_REVIEW_STATE}
        source={PO_REVIEW_SOURCE}
        currency={PO_EDITOR_USD}
        quoteNumber="1502260012"
        documentState="none"
        decisions={{}}
        acknowledgedMissing={[]}
        acknowledgedExtra={[]}
        onDecision={() => undefined}
        onToggleMissing={() => undefined}
        onToggleExtra={() => undefined}
        onMatchChange={() => undefined}
        onBack={() => undefined}
        onContinue={() => undefined}
      />
    ),
  },
  {
    // The repair difference review (issue #475): the same #471 choices against
    // the candidate revision, no choice preselected, and the repair wording
    // instead of the final-save one.
    name: "PurchaseOrderRepairReview",
    element: (
      <PurchaseOrderDifferenceReviewView
        state={PO_REVIEW_STATE}
        source={PO_REPAIR_SOURCE}
        currency={PO_EDITOR_USD}
        quoteNumber={PO_REPAIR_SOURCE.QuoteNumber}
        mode="repair"
        documentState="none"
        decisions={{}}
        acknowledgedMissing={[]}
        acknowledgedExtra={[]}
        onDecision={() => undefined}
        onToggleMissing={() => undefined}
        onToggleExtra={() => undefined}
        onMatchChange={() => undefined}
        onBack={() => undefined}
        onContinue={() => undefined}
      />
    ),
  },
  {
    // The repair confirmation (issue #475): old link → new link, the kept
    // confirmation number, the choices this repair stores and the required
    // reason. At 360 this says whether the link pair and the reason field
    // stack instead of overflowing.
    name: "PurchaseOrderRepairConfirmation",
    element: (
      <PurchaseOrderRepairConfirmationView
        review={PO_REPAIR_REVIEW_DTO}
        evaluation={PO_REPAIR_EVALUATION}
        currency={PO_EDITOR_USD}
        reason="The order followed the wrong quotation revision"
        onReasonChange={() => undefined}
        onBack={() => undefined}
        onSave={() => undefined}
        saving={false}
        error={null}
      />
    ),
  },
  {
    // The required final confirmation when no customer PDF exists (#471).
    // The checkbox is unchecked and Final save is disabled: the deliberate
    // acknowledgement must be given, not assumed.
    name: "PurchaseOrderFinalConfirmation",
    element: (
      <PurchaseOrderFinalConfirmationView
        review={PO_REVIEW_DTO}
        evaluation={PO_REVIEW_EVALUATION}
        payload={buildPurchaseOrderUpdate(PO_REVIEW_EVALUATION.applied, PO_EDITOR_USD).input!}
        currency={PO_EDITOR_USD}
        documentConfirmed={false}
        onToggleDocumentConfirmed={() => undefined}
        assignConfirmation={false}
        onToggleAssignConfirmation={() => undefined}
        onBack={() => undefined}
        onFinalSave={() => undefined}
        saving={false}
        error={null}
      />
    ),
  },
  {
    // A saved record (issue #471/#472): read-only PO fields and totals, the
    // printed source figures beside them, the recorded no-PDF confirmation and
    // the attachment panel's empty state. The document metadata is supplied so
    // the SSR snapshot shows the settled state rather than a loading line.
    //
    // #474: this record has no confirmation number, so the panel shows "Not
    // assigned" and the later Assign action.
    name: "PurchaseOrderSaved",
    element: (
      <PurchaseOrderSavedView
        order={{ ...PO_EDITOR_ORDER, State: "saved", PdfAbsentConfirmed: true }}
        currency={PO_EDITOR_USD}
        onBack={() => undefined}
        onCorrect={() => undefined}
        documentInitialState={PO_DOCUMENT_NONE}
        historyInitial={PO_HISTORY}
      />
    ),
  },
  {
    // The saved record with the admin source-repair action (issue #475), beside
    // Correct. The action is absent from the staff case above.
    name: "PurchaseOrderSavedRepairAction",
    element: (
      <PurchaseOrderSavedView
        order={{ ...PO_EDITOR_ORDER, State: "saved", PdfAbsentConfirmed: true }}
        currency={PO_EDITOR_USD}
        onBack={() => undefined}
        onCorrect={() => undefined}
        canRepairSource
        onRepairSource={() => undefined}
        documentInitialState={PO_DOCUMENT_NONE}
        historyInitial={PO_HISTORY}
      />
    ),
  },
  {
    // A saved record whose source Quotation was deleted (issue #481): the
    // independent Source missing badge and banner, the former quotation named
    // beside the record's own readable values, the no-new-downstream rule, and
    // the admin-only Link Quotation action. Correct is absent because the
    // server refuses a correction while the source is missing.
    name: "PurchaseOrderSavedSourceMissing",
    element: (
      <PurchaseOrderSavedView
        order={{
          ...PO_EDITOR_ORDER,
          State: "saved",
          PdfAbsentConfirmed: true,
          SourceMissing: true,
          ConfirmationNumber: "2302268001",
          ConfirmationAssignedAt: "2026-10-21T09:00:00Z",
        }}
        currency={PO_EDITOR_USD}
        onBack={() => undefined}
        onCorrect={() => undefined}
        canRepairSource
        onRepairSource={() => undefined}
        documentInitialState={PO_DOCUMENT_NONE}
        historyInitial={PO_HISTORY}
      />
    ),
  },
  {
    // A saved record whose confirmation number is issued (#474): the number is
    // read-only beside Copy, with the manual-submission explanation and no
    // Submitted claim. The warning banner is shown because the stored sequence
    // crossed 8000.
    name: "PurchaseOrderSavedConfirmationAssigned",
    element: (
      <PurchaseOrderSavedView
        order={{
          ...PO_EDITOR_ORDER,
          State: "saved",
          PdfAbsentConfirmed: true,
          ConfirmationNumber: "2302268000",
          ConfirmationAssignedAt: "2026-10-21T09:00:00Z",
          ConfirmationWarn: true,
        }}
        currency={PO_EDITOR_USD}
        onBack={() => undefined}
        documentInitialState={PO_DOCUMENT_NONE}
        historyInitial={PO_HISTORY}
      />
    ),
  },
  {
    // The attachment panel with no PDF (#472): the exact empty-state sentence,
    // Attach, and the visibly disabled Extract and compare action with its
    // reason. At 360 this says whether the action row wraps instead of
    // overflowing.
    name: "PurchaseOrderDocumentNone",
    element: (
      <PurchaseOrderDocumentPanelView
        state={PO_DOCUMENT_NONE}
        onChooseFile={() => undefined}
        onView={() => undefined}
        onRemove={() => undefined}
        onConfirmRemove={() => undefined}
        onCancelRemove={() => undefined}
      />
    ),
  },
  {
    // The attachment panel with version 2 active and version 1 retained
    // (#472): filename, size, "Version 2 of 2", View, Replace and Remove.
    name: "PurchaseOrderDocumentAttached",
    element: (
      <PurchaseOrderDocumentPanelView
        state={PO_DOCUMENT_ATTACHED}
        onChooseFile={() => undefined}
        onView={() => undefined}
        onRemove={() => undefined}
        onConfirmRemove={() => undefined}
        onCancelRemove={() => undefined}
      />
    ),
  },
  {
    // Removal's own confirmation (#472): no second dialog exists elsewhere,
    // and the copy says the prior versions remain in correction history.
    name: "PurchaseOrderDocumentRemovalConfirmation",
    element: (
      <PurchaseOrderDocumentPanelView
        state={PO_DOCUMENT_ATTACHED}
        pendingRemoval
        onChooseFile={() => undefined}
        onView={() => undefined}
        onRemove={() => undefined}
        onConfirmRemove={() => undefined}
        onCancelRemove={() => undefined}
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
    // The same panel when the display-only aggregate gross cannot be named
    // exactly (issue #470 wave 3). Each line and the stored subtotal/grand are
    // exact; only the sum of line grosses is above 2^53-1. This case proves the
    // long status text fits at 360 without clipping the money column.
    name: "PurchaseOrderTotalsExactRange",
    element: (
      <PurchaseOrderTotals
        totals={{
          ...PO_EDITOR_CALCULATION.totals!,
          grossMinor: null,
          rowDiscountMinor: null,
        }}
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
    // #480: single-record scope. The connected purchase order is listed as
    // surviving, because deleting the quotation releases its link rather than
    // deleting it.
    name: "DeletionDialogRecord",
    element: (
      <DeletionDialogView
        state={deletionState("record")}
        targetLabel="quotation 1502260012"
        onClose={() => undefined}
        onScope={() => undefined}
        onReason={() => undefined}
        onTypedNumber={() => undefined}
        onChallengeInput={() => undefined}
        onRequestChallenge={() => undefined}
        onChangeReviewed={() => undefined}
        onConfirm={() => undefined}
      />
    ),
  },
  {
    // #480: whole-chain scope with the issued challenge. The ten letters are
    // the second confirmation; this is the state where the delete button is
    // still disabled until the admin types them.
    name: "DeletionDialogChain",
    element: (
      <DeletionDialogView
        state={DELETION_CHAIN_READY}
        targetLabel="quotation 1502260012"
        onClose={() => undefined}
        onScope={() => undefined}
        onReason={() => undefined}
        onTypedNumber={() => undefined}
        onChallengeInput={() => undefined}
        onRequestChallenge={() => undefined}
        onChangeReviewed={() => undefined}
        onConfirm={() => undefined}
      />
    ),
  },
  {
    // #480: the challenge card on its own. In the 900px dialog screenshot the
    // card sits below the fold of the dialog's own scroll container, so this
    // case is the evidence that the issued letters, the expiry and the input
    // render legibly at both widths.
    name: "DeletionDialogChallengePanel",
    element: (
      <DeletionChallengePanel
        state={DELETION_CHAIN_READY}
        challengeError="Those letters do not match the challenge."
        onChallengeInput={() => undefined}
        onRequestChallenge={() => undefined}
      />
    ),
  },
  {
    // #480: the changed-chain stop. Nothing was deleted; the modal names what
    // moved and asks for the confirmation to be renewed.
    name: "DeletionDialogChanged",
    element: (
      <DeletionDialogView
        state={DELETION_CHANGED}
        targetLabel="quotation 1502260012"
        onClose={() => undefined}
        onScope={() => undefined}
        onReason={() => undefined}
        onTypedNumber={() => undefined}
        onChallengeInput={() => undefined}
        onRequestChallenge={() => undefined}
        onChangeReviewed={() => undefined}
        onConfirm={() => undefined}
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
