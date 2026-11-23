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
// states, and no pixel diff against a baseline. A case may crop its screenshot
// to a named heading so a below-the-fold state is visible; that shifts the
// viewport, never the rendered page.

import { mkdirSync, readFileSync, readdirSync, statSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { createRef, type ReactElement } from "react";
import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { backend, poreport } from "@backend/models";
import { DetailsPanel } from "./components/DetailsPanel";
import type { DetailsState } from "./components/DetailsPanel";
import { QuotationHero } from "./components/QuotationHero";
import { QuotationTutorialWorkspace } from "./featuresTutorial/QuotationTutorialWorkspace";
import type { EditorItem } from "./components/ItemEditor";
import { ItemEditor } from "./components/ItemEditor";
import { CompanyGeneralPanel } from "./components/CompanyGeneralPanel";
import { CompanyPortalTab } from "./components/CompanyPortalTab";
import { CompanyQuotationTab } from "./components/CompanyQuotationTab";
import { CompanyTemplatePanel } from "./components/CompanyTemplatePanel";
import { CompanyUnitsPanel } from "./components/CompanyUnitsPanel";
import { LoginPage, LoginUpdateNotice, loginUpdateNotice } from "./components/LoginPage";
import { SessionRenewBanner } from "./components/SessionRenewBanner";
import { DocumentHomePage } from "./components/DocumentHomePage";
import { AppShell } from "./components/AppShell";
import { AdminDashboard } from "./components/AdminDashboard";
import { PurchaseOrderReportsView } from "./components/PurchaseOrderReportsPage";
import { PurchaseOrderReportPickerView } from "./components/PurchaseOrderReportPicker";
import {
  normalizePurchaseOrderReport,
  type PurchaseOrderReportData,
  type PurchaseOrderReportMetrics,
} from "./poReport";
import {
  purchaseOrderReportFixture,
  purchaseOrderReportMaxFixture,
  purchaseOrderReportMonthFixture,
  purchaseOrderReportYearFixture,
  purchaseOrderReportYearWithRecordedFutureFixture,
} from "./poReportFixtures";
import {
  allMonthsOfYear,
  allYearForMonths,
  monthLabel,
  resolveCategoryReadout,
  resolveMonthAggregate,
  resolveMonthReadout,
  type PurchaseOrderMonthReadout,
  type ReportComparisonLayout,
  type ReportMonthLayout,
} from "./poReportComparison";
import { ToastProvider } from "./toast";
import { PurchaseOrderListView } from "./components/PurchaseOrderListPage";
import { ReconciliationPaneContent, ReconciliationSheetTrigger } from "./components/ReconciliationPane";
import { ReconciliationTotalsStrip } from "./components/ReconciliationTotalsStrip";
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
import { QuotationCloseDialog } from "./components/QuotationCloseDialog";
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
  initialPOListStateFor,
} from "./poList";
import type { POListRowDTO, POListState } from "./poList";
import {
  blankPurchaseOrderDraft,
  buildPurchaseOrderUpdate,
  calculatePurchaseOrder,
  initialPurchaseOrderEditorState,
} from "./purchaseOrder";
import type { PurchaseOrderHistoryEntry } from "./poCorrection";
import type { PurchaseOrderDTO } from "./purchaseOrder";
import type { PurchaseOrderDocumentStateDTO } from "./purchaseOrderDocument";
import { PortalPreviewPanel } from "./components/PortalPreviewPanel";
import { QuotationSettingsPage } from "./components/QuotationSettingsPage";
import { PurchaseOrderSettingsPage } from "./components/PurchaseOrderSettingsPage";
import {
  DEFAULT_PURCHASE_ORDER_SETTINGS,
  DEFAULT_QUOTATION_SETTINGS,
} from "./workspaceSettings";
import type { CompanyLabelEntry } from "./companyLabels";
import { DEFAULT_THEME, EMPTY_THEME } from "./company";
import { TAX_TYPE_OPTIONS, TAX_ZONE_OPTIONS } from "./tax";
import { UPDATE_NEWER, UPDATE_REQUIRED } from "./updateFixture";
import {
  addDaysIso,
  type ReportBucket,
  type ReportFilterState,
} from "./poReportPage";

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
  note: "",
};

interface SnapshotCase {
  name: string;
  element: ReactElement;
  /**
   * Shifts the rendered page up so this heading sits at the top of the
   * screenshot. Used when the state under review is below the 900px fold; it
   * changes the screenshot's viewport position only, never the rendered page.
   */
  cropToHeading?: string;
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
    allBought: false,
    leafCount: 3,
    remainingLeafCount: 3,
    savedPoCount: 0,
    draftPoCount: 0,
    legacyHeaderDiscount: false,
    closed: false,
    closedAt: null,
    closedReason: "",
    validUntil: null,
    expiredUnclosed: false,
  },
  {
    id: 11,
    quoteNumber: "1502260011",
    subject: "Mechanical works and pump replacement",
    customerName: "PT. H-Tech Oilfield Equipment, NOV XL Systems",
    latestRevision: 0,
    issuedRevisionCount: 1,
    activePO: { id: 20, state: "draft" },
    allBought: false,
    leafCount: 3,
    remainingLeafCount: 2,
    savedPoCount: 1,
    draftPoCount: 1,
    legacyHeaderDiscount: false,
    closed: false,
    closedAt: null,
    closedReason: "",
    validUntil: null,
    expiredUnclosed: false,
  },
  {
    id: 10,
    quoteNumber: "1502260010",
    subject: "Cooling package",
    customerName: "PT. Datalec Global Solution",
    latestRevision: 1,
    issuedRevisionCount: 2,
    activePO: { id: 21, state: "saved" },
    allBought: true,
    leafCount: 2,
    remainingLeafCount: 0,
    savedPoCount: 2,
    draftPoCount: 0,
    legacyHeaderDiscount: false,
    closed: false,
    closedAt: null,
    closedReason: "",
    validUntil: null,
    expiredUnclosed: false,
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
// Issue #496: a source-less draft starts in the live company currency.
const PO_EDITOR_IDR: CurrencyInfo = {
  code: "IDR",
  exponent: 0,
  symbol: "Rp",
  position: "before",
  groupSep: ".",
  decimalSep: ",",
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
  BusinessCategory: "FAB",
  AdjustmentReason: "",
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
      GroupKey: "q:2",
      ParentDescription: "Mounting package",
      DescriptionLines: ["Cable tray", "Terminations"],
    },
    {
      QuotationItemNo: 2,
      QuotationSubNo: 2,
      LineNumber: 3,
      Description: "Cable termination and labelling",
      QuantityScaled: 10000,
      UoM: "Unit",
      UnitPriceMinor: 30000,
      DiscountEnabled: true,
      DiscountPercent: 5,
      GrossMinor: 30000,
      DiscountMinor: 1500,
      NetMinor: 28500,
      GroupKey: "q:2",
      ParentDescription: "Mounting package",
      DescriptionLines: ["Two technicians"],
    },
  ],
  ID: 7,
  QuotationID: 12,
  SourceRevision: 2,
  CustomerID: 2,
  PaymentTerms: "45 days",
  DiscountPercent: 0,
  PpnBps: 1100,
  SubtotalMinor: 351863,
  DiscountMinor: 0,
  PpnMinor: 38704,
  AdjustmentMinor: 0,
  GrandTotalMinor: 390567,
  PrintedSubtotalMinor: 325000,
  PrintedDiscountMinor: 0,
  PrintedPpnMinor: 35750,
  PrintedGrandTotalMinor: 360750,
};
const PO_EDITOR_INITIAL = initialPurchaseOrderEditorState(
  PO_EDITOR_ORDER,
  PO_EDITOR_USD
);
// #654: one aggregate line over the plain line and the split package. The
// header's derived total is the sum of all three child rows and its derived
// unit price divides that by the display quantity of 2; the children keep their
// depth and their own discounts. This is a presentation state, not a money
// change: the line values below the header are the same order as above.
const PO_AGGREGATE_ORDER: PurchaseOrderDTO = {
  ...PO_EDITOR_ORDER,
  Items: PO_EDITOR_ORDER.Items.map((item) => ({
    ...item,
    AggregateGroupID: 1,
  })),
  AggregateGroups: [
    {
      ID: 1,
      DisplayOrder: 1,
      Title: "Combined items",
      OrderedQuantityScaled: 20000,
      OrderedUoM: "Lot",
      ShowChildrenToCustomer: false,
      TotalMinor: 351863,
      DisplayUnitPriceMinor: 175932,
    },
  ],
};
const PO_AGGREGATE_STATE = initialPurchaseOrderEditorState(
  PO_AGGREGATE_ORDER,
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
    ID: 2,
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
    ID: 1,
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

// #518: the saved record's reconciliation read. The outer POVersion matches
// the record above (1) so the snapshot shows the settled current generation;
// the older generation exists so the Session history disclosure is visible.
const PO_SAVED_RECONCILIATION_SESSION: backend.POReconciliationSession = {
  ID: 21,
  PurchaseOrderID: 7,
  SourceQuotationID: 12,
  SourceRevision: 2,
  POVersion: 1,
  Active: true,
  ReadOnly: true,
  CreatedBy: 3,
  CreatedAt: "2026-10-21T09:00:00Z",
  UpdatedAt: "2026-10-21T09:05:00Z",
  LineIdentities: [],
  Items: [
    {
      Key: "header:paymentTerms",
      Field: "header:paymentTerms",
      Kind: "difference",
      LineKey: null,
      SourceLineRef: null,
      Label: "Payment terms",
      POValue: "30 days",
      SourceValue: "30 days",
      Status: "resolved",
      Resolution: "values_now_match",
      ActorID: 3,
      UpdatedAt: "2026-10-21T09:05:00Z",
      History: [],
    },
    {
      Key: "link:taxBasis",
      Field: "link:taxBasis",
      Kind: "tax_basis",
      LineKey: null,
      SourceLineRef: null,
      Label: "Tax basis",
      POValue: { TaxZone: "FTZ", TaxType: "PPN", ServiceCategory: "" },
      SourceValue: { TaxZone: "FTZ", TaxType: "PPN", ServiceCategory: "" },
      Status: "resolved",
      Resolution: "used_quotation",
      ActorID: 3,
      UpdatedAt: "2026-10-21T09:05:00Z",
      History: [],
    },
  ],
} as unknown as backend.POReconciliationSession;

const PO_SAVED_RECONCILIATION_READ: backend.POReconciliationSessions = {
  PurchaseOrderID: 7,
  POVersion: 1,
  Sessions: [
    {
      Generation: 1,
      SourceAvailable: true,
      RepairCorrectionID: null,
      Session: PO_SAVED_RECONCILIATION_SESSION,
    },
    {
      Generation: 0,
      SourceAvailable: true,
      RepairCorrectionID: null,
      Session: {
        ...PO_SAVED_RECONCILIATION_SESSION,
        ID: 14,
        Active: false,
        SourceRevision: 1,
        CreatedAt: "2026-10-20T09:00:00Z",
      },
    },
  ],
} as unknown as backend.POReconciliationSessions;

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
    BusinessCategory: "FAB",
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
    BusinessCategory: "SUPPLY_GEN",
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
    BusinessCategory: "FM",
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
    BusinessCategory: "",
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
const PO_DRAFT_ROWS = PO_LIST_ROWS.filter((row) => row.State === "draft");
const PO_SAVED_ROWS = PO_LIST_ROWS.filter((row) => row.State === "saved");
const PO_LIST_DRAFTS: POListState = {
  ...initialPOListStateFor("drafts"),
  rows: PO_DRAFT_ROWS,
  customers: PO_LIST_CUSTOMERS,
  page: 1,
  pageSize: 15,
  total: 8,
  totalPages: 1,
};
const PO_LIST_ARCHIVE: POListState = {
  ...initialPOListStateFor("archive"),
  rows: PO_SAVED_ROWS,
  customers: PO_LIST_CUSTOMERS,
  page: 1,
  pageSize: 15,
  total: 23,
  totalPages: 2,
};
// Issue #496: the three source states the archive copy must keep distinct. A
// never-linked draft has no quotation or Customer; a former row keeps its
// former source; a live revision 00 is a real link. The case exists so the
// copy is read at both widths, not only asserted in SSR.
const PO_LIST_SOURCE_MISSING_496: POListState = {
  ...initialPOListStateFor("drafts"),
  rows: [
    {
      UUID: "uuid-po-never",
      State: "draft",
      PONumber: "",
      QuoteNumber: "",
      CustomerName: "",
      ReceivedDate: "2026-10-22",
      Currency: "IDR",
      ID: 41,
      CustomerID: 0,
      SourceRevision: 0,
      SourceMissing: true,
      SourceState: "never",
    },
    {
      UUID: "uuid-po-former",
      State: "draft",
      PONumber: "PO-FORMER-496",
      QuoteNumber: "1502250007",
      CustomerName: "PT. Former Source",
      ReceivedDate: "2026-10-21",
      Currency: "IDR",
      ID: 40,
      CustomerID: 3,
      SourceRevision: 1,
      SourceMissing: true,
      SourceState: "former",
    },
    {
      UUID: "uuid-po-live0",
      State: "draft",
      PONumber: "PO-LIVE-REV00",
      QuoteNumber: "1502260008",
      CustomerName: "PT. Live Source",
      ReceivedDate: "2026-10-20",
      Currency: "IDR",
      ID: 39,
      CustomerID: 2,
      SourceRevision: 0,
      SourceMissing: false,
      SourceState: "live",
    },
  ],
  customers: [
    { ID: 2, Name: "PT. Live Source" },
    { ID: 3, Name: "PT. Former Source" },
  ],
  page: 1,
  pageSize: 15,
  total: 3,
  totalPages: 1,
};
const PO_LIST_NO_MATCH: POListState = {  ...initialPOListStateFor("archive"),
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


// Ticket 04: one unresolved header difference, one kept resolution and the
// link-time tax basis, as the pane shows them mid-reconciliation.
const PO_RECONCILIATION_SESSION = {
  ID: 3,
  PurchaseOrderID: 7,
  SourceQuotationID: 12,
  SourceRevision: 2,
  POVersion: 4,
  Active: true,
  ReadOnly: false,
  CreatedBy: 7,
  CreatedAt: "2026-10-26T10:00:00Z",
  UpdatedAt: "2026-10-28T00:00:05Z",
  LineIdentities: [],
  Items: [
    {
      Key: "header:paymentTerms",
      Field: "header:paymentTerms",
      Kind: "difference",
      LineKey: null,
      SourceLineRef: null,
      Label: "Payment terms",
      POValue: "50% advance, balance 30 days",
      SourceValue: "30 days from invoice",
      Status: "unresolved",
      Resolution: null,
      ActorID: 7,
      UpdatedAt: "2026-10-28T00:00:03Z",
      History: [],
    },
    {
      Key: "header:incoterms",
      Field: "header:incoterms",
      Kind: "difference",
      LineKey: null,
      SourceLineRef: null,
      Label: "Incoterms",
      POValue: "CIF",
      SourceValue: "CIF",
      Status: "resolved",
      Resolution: "kept_po",
      ActorID: 7,
      UpdatedAt: "2026-10-28T00:00:01Z",
      History: [],
    },
    {
      Key: "link:taxBasis",
      Field: "link:taxBasis",
      Kind: "tax_basis",
      LineKey: null,
      SourceLineRef: null,
      Label: "Tax basis",
      POValue: "FTZ · Goods",
      SourceValue: "FTZ · Goods",
      Status: "resolved",
      Resolution: "kept_po",
      ActorID: 7,
      UpdatedAt: "2026-10-28T00:00:00Z",
      History: [],
    },
  ],
};

const PO_RECONCILIATION_LINE_SESSION = {
  ...PO_RECONCILIATION_SESSION,
  LineIdentities: [
    { Key: "client:a", ID: null, Removed: false },
    { Key: "client:b", ID: null, Removed: false },
    { Key: "client:u", ID: null, Removed: false },
  ],
  Items: [
    {
      Key: "line:client:a:quantity",
      Field: "line:quantity",
      Kind: "difference",
      LineKey: "client:a",
      SourceLineRef: "1",
      Label: "Line 1 quantity",
      POValue: 30000,
      SourceValue: 20000,
      Status: "unresolved",
      Resolution: null,
      ActorID: 7,
      UpdatedAt: "2026-10-28T00:00:03Z",
      History: [],
    },
    {
      Key: "source:2:match",
      Field: "source:match",
      Kind: "missing_line",
      LineKey: null,
      SourceLineRef: "2",
      Label: "Missing line · Quotation 2",
      POValue: null,
      SourceValue: "Sprocket",
      Status: "unresolved",
      Resolution: null,
      ActorID: 7,
      UpdatedAt: "2026-10-28T00:00:03Z",
      History: [],
    },
    {
      Key: "line:client:b:match",
      Field: "line:match",
      Kind: "extra_line",
      LineKey: "client:b",
      SourceLineRef: null,
      Label: "Extra line · Line 2",
      POValue: null,
      SourceValue: null,
      Status: "unresolved",
      Resolution: null,
      ActorID: 7,
      UpdatedAt: "2026-10-28T00:00:03Z",
      History: [],
    },
    {
      Key: "line:client:u:match",
      Field: "line:match",
      Kind: "extra_line",
      LineKey: "client:u",
      SourceLineRef: null,
      Label: "Unknown reference · 99",
      POValue: "99",
      SourceValue: null,
      Status: "unresolved",
      Resolution: null,
      ActorID: 7,
      UpdatedAt: "2026-10-28T00:00:05Z",
      History: [
        {
          Kind: "extra_line",
          SourceLineRef: null,
          POValue: "99",
          SourceValue: null,
          Status: "unresolved",
          Resolution: "acknowledged_extra",
          ActorID: 7,
          At: "2026-10-28T00:00:05Z",
        },
      ],
    },
    {
      Key: "line:client:c:match",
      Field: "line:match",
      Kind: "duplicate_match",
      LineKey: "client:c",
      SourceLineRef: "1",
      Label: "Duplicate match · Quotation 1",
      POValue: "1",
      SourceValue: null,
      Status: "unresolved",
      Resolution: null,
      ActorID: 7,
      UpdatedAt: "2026-10-28T00:00:03Z",
      History: [],
    },
    {
      Key: "line:client:d:match",
      Field: "line:match",
      Kind: "duplicate_match",
      LineKey: "client:d",
      SourceLineRef: "1",
      Label: "Duplicate match · Quotation 1",
      POValue: "1",
      SourceValue: null,
      Status: "unresolved",
      Resolution: null,
      ActorID: 7,
      UpdatedAt: "2026-10-28T00:00:03Z",
      History: [],
    },
    {
      Key: "link:taxBasis",
      Field: "link:taxBasis",
      Kind: "tax_basis",
      LineKey: null,
      SourceLineRef: null,
      Label: "Tax basis",
      POValue: { TaxZone: "FTZ", TaxType: "Goods", ServiceCategory: "" },
      SourceValue: { TaxZone: "FTZ", TaxType: "Goods", ServiceCategory: "" },
      Status: "resolved",
      Resolution: "kept_po",
      ActorID: 7,
      UpdatedAt: "2026-10-28T00:00:00Z",
      History: [],
    },
  ],
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

// A second maximum-figure evidence case in the widest supported currency
// spelling: AED is a spaced Latin code (26 characters for a negative value),
// so the 360px one-column KPI cell is read against the longest accepted form,
// not only the familiar Rp.
const REPORTS_MAX_AED = purchaseOrderReportMaxFixture();
REPORTS_MAX_AED.filter = { ...REPORTS_MAX_AED.filter, currency: "AED" };

function cloneReport(report: PurchaseOrderReportData): PurchaseOrderReportData {
  return structuredClone(report);
}

function periodMetrics(
  period: PurchaseOrderReportData["periods"][number]
): PurchaseOrderReportMetrics {
  return {
    poCount: period.poCount,
    amountMinor: period.amountMinor,
    costedMinor: period.costedMinor,
    uncostedMinor: period.uncostedMinor,
    capitalMinor: period.capitalMinor,
    marginMinor: period.marginMinor,
    marginBps: period.marginBps,
    coverageBps: period.coverageBps,
  };
}

function zeroMonthMetrics() {
  return {
    poCount: 0,
    amountMinor: 0,
    costedMinor: 0,
    uncostedMinor: 0,
    capitalMinor: 0,
    marginMinor: 0,
    marginBps: null,
    coverageBps: null,
  };
}

// Issue #641: the static Reports evidence must include the real month browser,
// not the old fallback period table. These readouts are the same pure selection
// the live hook passes; no financial figure is recomputed here.
const REPORT_MONTH_ANNUAL = purchaseOrderReportYearWithRecordedFutureFixture();
const REPORT_MONTH_CURRENT_READOUT = resolveMonthReadout({
  overview: purchaseOrderReportFixture(),
  month: "2026-11",
  annualSource: REPORT_MONTH_ANNUAL,
  annualStatus: "ready",
  annualError: "",
});

// Issue #644: the full-page current Month/Year evidence now runs through the
// same Overview + independent month browser the live page renders. These
// readouts select the current month from the coherent #641 day/year fixtures;
// no figure is recomputed.
const REPORT_DAY_PAGE_READOUT = resolveMonthReadout({
  overview: purchaseOrderReportMonthFixture(),
  month: "2026-11",
  annualSource: null,
  annualStatus: "loading",
  annualError: "",
});
const REPORT_YEAR_PAGE_READOUT = resolveMonthReadout({
  overview: purchaseOrderReportYearFixture(),
  month: "2026-11",
  annualSource: null,
  annualStatus: "loading",
  annualError: "",
});

const REPORT_MONTH_PAST_ZERO = cloneReport(purchaseOrderReportYearFixture());
REPORT_MONTH_PAST_ZERO.periods[0] = {
  ...REPORT_MONTH_PAST_ZERO.periods[0],
  ...zeroMonthMetrics(),
};
const REPORT_MONTH_PAST_ZERO_READOUT = resolveMonthReadout({
  overview: REPORT_MONTH_PAST_ZERO,
  month: "2026-01",
  annualSource: null,
  annualStatus: "loading",
  annualError: "",
});

const REPORT_MONTH_FUTURE = purchaseOrderReportYearFixture();
const REPORT_MONTH_FUTURE_READOUT = resolveMonthReadout({
  overview: REPORT_MONTH_FUTURE,
  month: "2026-12",
  annualSource: null,
  annualStatus: "loading",
  annualError: "",
});

const REPORT_MONTH_FUTURE_UNAVAILABLE = cloneReport(
  purchaseOrderReportYearFixture()
);
REPORT_MONTH_FUTURE_UNAVAILABLE.currentProjection = {
  ...REPORT_MONTH_FUTURE_UNAVAILABLE.currentProjection,
  status: "unavailable",
  reason: "insufficient_days",
  buckets: [],
  currentMonth: undefined,
};
const REPORT_MONTH_FUTURE_UNAVAILABLE_READOUT = resolveMonthReadout({
  overview: REPORT_MONTH_FUTURE_UNAVAILABLE,
  month: "2026-12",
  annualSource: null,
  annualStatus: "loading",
  annualError: "",
});

const REPORT_MAX_CURRENT = purchaseOrderReportMaxFixture();
REPORT_MAX_CURRENT.monthSummary = {
  month: "2026-11",
  from: "2026-11-01",
  to: "2026-12-01",
  asOf: REPORT_MAX_CURRENT.asOf,
  mode: "month",
  actual: periodMetrics(REPORT_MAX_CURRENT.periods[10]),
  projection: {
    status: "unavailable",
    reason: "insufficient_months",
    method: "No current-year forecast in this fixture.",
  },
};
const REPORT_MAX_ANNUAL = cloneReport(REPORT_MAX_CURRENT);
REPORT_MAX_ANNUAL.filter = {
  from: "2026-01-01",
  to: "2027-01-01",
  bucket: "month",
  category: "all",
  currency: "IDR",
};
const REPORT_MAX_CURRENT_READOUT = resolveMonthReadout({
  overview: REPORT_MAX_CURRENT,
  month: "2026-11",
  annualSource: REPORT_MAX_ANNUAL,
  annualStatus: "ready",
  annualError: "",
});

const REPORT_MAX_AED_CURRENT = cloneReport(REPORTS_MAX_AED);
REPORT_MAX_AED_CURRENT.monthSummary = {
  month: "2026-11",
  from: "2026-11-01",
  to: "2026-12-01",
  asOf: REPORT_MAX_AED_CURRENT.asOf,
  mode: "month",
  actual: periodMetrics(REPORT_MAX_AED_CURRENT.periods[10]),
  projection: {
    status: "unavailable",
    reason: "insufficient_months",
    method: "No current-year forecast in this fixture.",
  },
};
const REPORT_MAX_AED_ANNUAL = cloneReport(REPORT_MAX_AED_CURRENT);
REPORT_MAX_AED_ANNUAL.filter = {
  from: "2026-01-01",
  to: "2027-01-01",
  bucket: "month",
  category: "all",
  currency: "AED",
};
const REPORT_MAX_AED_CURRENT_READOUT = resolveMonthReadout({
  overview: REPORT_MAX_AED_CURRENT,
  month: "2026-11",
  annualSource: REPORT_MAX_AED_ANNUAL,
  annualStatus: "ready",
  annualError: "",
});

function monthReportView(
  report: PurchaseOrderReportData,
  readout: PurchaseOrderMonthReadout,
  filters: ReportFilterState = reportChartFilters(report)
): ReactElement {
  return (
    <PurchaseOrderReportsView
      filters={filters}
      report={report}
      loading={false}
      stale={false}
      error=""
      validation=""
      catalogue={REPORT_CATALOGUE}
      exporting={false}
      canExport={false}
      onPresetChange={() => undefined}
      onPeriodChange={() => undefined}
      onFromChange={() => undefined}
      onToChange={() => undefined}
      onBucketChange={() => undefined}
      onCategoryChange={() => undefined}
      onCurrencyChange={() => undefined}
      onRetry={() => undefined}
      onExport={() => undefined}
      monthBrowser={{
        months: readout.month === "" ? [] : [readout.month],
        activeMonth: readout.month === "" ? null : readout.month,
        allYear: null,
        layout: "individual",
        notice: "",
        readouts: [readout],
        aggregate: null,
        triggerRef: createRef<HTMLButtonElement>(),
        onOpenPicker: () => undefined,
        onLayoutChange: () => undefined,
        onPrevious: () => undefined,
        onNext: () => undefined,
        onRetryMonth: () => undefined,
      }}
    />
  );
}

// Issue #631: the chart snapshot cases share the Reports view's real props and
// the same normalized server snapshot the screen consumes. The harness renders
// to a string and screenshots at 360 and 1280 only (there is no 390 viewport).
//
// What the static render cannot show: the chart's measured container width is a
// client effect. The SSR fallback is 960px, so the 360 screenshot shows the
// scroll container and the first columns rather than the measured 360 layout;
// the real browser measures and reflows. The 1280 read is the desktop chart.
// Hover, keyboard and entry motion remain hand-driven/DOM-tested.
const REPORT_CATALOGUE: CurrencyInfo[] = [
  {
    code: "IDR",
    exponent: 0,
    symbol: "Rp",
    position: "before",
    groupSep: ".",
    decimalSep: ",",
    symbolSpaced: false,
  },
  {
    code: "USD",
    exponent: 2,
    symbol: "$",
    position: "before",
    groupSep: ",",
    decimalSep: ".",
    symbolSpaced: false,
  },
];

function reportChartFilters(
  report: PurchaseOrderReportData,
  period: ReportFilterState["period"] = null
): ReportFilterState {
  return {
    preset: "custom",
    period,
    from: report.filter.from,
    to: addDaysIso(report.filter.to, -1),
    bucket: report.filter.bucket as ReportBucket,
    category: report.filter.category,
    currency: report.filter.currency,
  };
}

function reportChartView(
  report: PurchaseOrderReportData,
  period: ReportFilterState["period"] = null,
  monthReadout?: PurchaseOrderMonthReadout
): ReactElement {
  return (
    <PurchaseOrderReportsView
      filters={reportChartFilters(report, period)}
      report={report}
      loading={false}
      stale={false}
      error=""
      validation=""
      catalogue={REPORT_CATALOGUE}
      exporting={false}
      canExport={false}
      onPresetChange={() => undefined}
      onPeriodChange={() => undefined}
      onFromChange={() => undefined}
      onToChange={() => undefined}
      onBucketChange={() => undefined}
      onCategoryChange={() => undefined}
      onCurrencyChange={() => undefined}
      onRetry={() => undefined}
      onExport={() => undefined}
      monthReadout={monthReadout}
      onPreviousMonth={() => undefined}
      onNextMonth={() => undefined}
      onRetryMonth={() => undefined}
    />
  );
}

// Issue #642: static evidence for the live month comparison. The readouts and
// the aggregate are the same pure selection results the hook passes; every
// figure is read from a supplied server snapshot. These cases cannot show the
// live modal, focus restoration, paging or request counts — the DOM suites own
// those.
function monthComparisonSource(
  report: PurchaseOrderReportData,
  months: readonly string[],
  annualSource: PurchaseOrderReportData | null = null
): PurchaseOrderMonthReadout[] {
  return months.map((month) =>
    resolveMonthReadout({
      overview: report,
      month,
      annualSource,
      annualStatus: annualSource === null ? "loading" : "ready",
      annualError: "",
    })
  );
}

function zeroAnnualMonthSource(year: string): PurchaseOrderReportData {
  const source = cloneReport(purchaseOrderReportYearFixture());
  source.filter = {
    from: `${year}-01-01`,
    to: `${Number(year) + 1}-01-01`,
    bucket: "month",
    category: "all",
    currency: "IDR",
  };
  source.periods = allMonthsOfYear(year).map((month) => ({
    periodStart: `${month}-01`,
    partial: false,
    inProgress: false,
    ...zeroMonthMetrics(),
  }));
  source.totals = zeroMonthMetrics();
  source.categories = source.categories.map((category) => ({
    ...category,
    ...zeroMonthMetrics(),
  }));
  source.currentProjection = {
    ...source.currentProjection,
    mode: "none",
    status: "unavailable",
    reason: "not_current_period",
    buckets: [],
    currentMonth: undefined,
  };
  source.monthSummary = undefined;
  return source;
}

function monthComparisonView(
  report: PurchaseOrderReportData,
  months: readonly string[],
  layout: ReportMonthLayout,
  annualSource: PurchaseOrderReportData | null = null
): ReactElement {
  const allYear = allYearForMonths(months);
  return (
    <PurchaseOrderReportsView
      filters={reportChartFilters(report)}
      report={report}
      loading={false}
      stale={false}
      error=""
      validation=""
      catalogue={REPORT_CATALOGUE}
      exporting={false}
      canExport={false}
      onPresetChange={() => undefined}
      onPeriodChange={() => undefined}
      onFromChange={() => undefined}
      onToChange={() => undefined}
      onBucketChange={() => undefined}
      onCategoryChange={() => undefined}
      onCurrencyChange={() => undefined}
      onRetry={() => undefined}
      onExport={() => undefined}
      monthBrowser={{
        months,
        activeMonth: months[0] ?? null,
        allYear,
        layout,
        notice: "",
        readouts: monthComparisonSource(report, months, annualSource),
        aggregate:
          allYear === null
            ? null
            : resolveMonthAggregate({
                overview: report,
                year: allYear,
                annualSource: null,
                annualStatus: "loading",
                annualError: "",
              }),
        triggerRef: createRef<HTMLButtonElement>(),
        onOpenPicker: () => undefined,
        onLayoutChange: () => undefined,
        onPrevious: () => undefined,
        onNext: () => undefined,
        onRetryMonth: () => undefined,
      }}
    />
  );
}

// Issue #643: static evidence for the independent By category comparison. The
// readout is the same pure selection result the hook passes; every figure is
// read from a supplied all-category snapshot. These cases cannot show the live
// modal, focus restoration or request counts — the DOM suites own those.
function categoryComparisonView(
  overview: PurchaseOrderReportData,
  source: PurchaseOrderReportData,
  explicitCodes: readonly string[] | null,
  layout: ReportComparisonLayout
): ReactElement {
  const readout = resolveCategoryReadout({
    overview,
    source,
    status: "ready",
    error: "",
    explicitCodes,
  });
  const codes = readout.selected.map((item) => item.code);
  const allSelected =
    readout.supplied.length > 0 &&
    codes.length === readout.supplied.length &&
    readout.supplied.every((item) => codes.includes(item.code));
  return (
    <PurchaseOrderReportsView
      filters={reportChartFilters(overview)}
      report={overview}
      loading={false}
      stale={false}
      error=""
      validation=""
      catalogue={REPORT_CATALOGUE}
      exporting={false}
      canExport={false}
      onPresetChange={() => undefined}
      onPeriodChange={() => undefined}
      onFromChange={() => undefined}
      onToChange={() => undefined}
      onBucketChange={() => undefined}
      onCategoryChange={() => undefined}
      onCurrencyChange={() => undefined}
      onRetry={() => undefined}
      onExport={() => undefined}
      categoryBrowser={{
        codes,
        activeCode: codes[0] ?? null,
        allSelected,
        layout,
        readout,
        triggerRef: createRef<HTMLButtonElement>(),
        onOpenPicker: () => undefined,
        onLayoutChange: () => undefined,
        onPrevious: () => undefined,
        onNext: () => undefined,
        onRetry: () => undefined,
      }}
    />
  );
}

// The restricted Performance case: an FM snapshot with no purchase orders
// above a genuine all-category section whose source is the unrestricted
// fixture at the same range and currency.
const REPORT_CATEGORY_RESTRICTED = cloneReport(purchaseOrderReportFixture());
REPORT_CATEGORY_RESTRICTED.filter = {
  ...REPORT_CATEGORY_RESTRICTED.filter,
  category: "FM",
};
REPORT_CATEGORY_RESTRICTED.totals = zeroMonthMetrics();
REPORT_CATEGORY_RESTRICTED.periods = [];
REPORT_CATEGORY_RESTRICTED.categories = [];
REPORT_CATEGORY_RESTRICTED.categorySeries = [];
REPORT_CATEGORY_RESTRICTED.currentProjection = {
  ...REPORT_CATEGORY_RESTRICTED.currentProjection,
  mode: "none",
  status: "unavailable",
  reason: "not_current_period",
  buckets: [],
  currentMonth: undefined,
};
REPORT_CATEGORY_RESTRICTED.monthSummary = undefined;
const REPORT_CATEGORY_ALL = purchaseOrderReportFixture();
const REPORT_CATEGORY_FAB_READOUT = resolveCategoryReadout({
  overview: REPORT_CATEGORY_ALL,
  source: REPORT_CATEGORY_ALL,
  status: "ready",
  error: "",
  explicitCodes: ["FAB"],
});


function sharedReportSnapshot(): PurchaseOrderReportData {
  const raw = JSON.parse(
    readFileSync(
      path.join(
        REPO_ROOT,
        "backend",
        "render",
        "report",
        "testdata",
        "report_snapshot.json"
      ),
      "utf8"
    )
  ) as { report: poreport.Report };
  return normalizePurchaseOrderReport(raw.report);
}

// The shared fixture is read only when this case renders, never at module load:
// the harness is collected by every test run even when UI_SNAPSHOT is unset.
function SharedReportChartView(): ReactElement {
  return reportChartView(sharedReportSnapshot());
}

const CASES: SnapshotCase[] = [
  {
    // Ticket #536: the "Stay signed in" window, idle and with the last
    // operational failure shown. The 360 render is the informative one: the
    // sentence and the button have to wrap inside the narrowest column.
    name: "SessionRenewBanner",
    element: (
      <SessionRenewBanner
        minutesLeft={10}
        busy={false}
        error={null}
        onRenew={() => undefined}
      />
    ),
  },
  {
    name: "SessionRenewBannerError",
    element: (
      <SessionRenewBanner
        minutesLeft={1}
        busy={false}
        error="The session could not be renewed. Keep working; we will try again."
        onRenew={() => undefined}
      />
    ),
  },
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
    // member locked out by a rollout has to find the fix. The props come from
    // the shared fixture through loginUpdateNotice (#558), so the render is
    // the live v-prefixed shape — v0.12.1 running, v0.13.1 available, the
    // windows installer resolved by the Go side — and the 360 render shows the
    // copy wrapping beside the Download button inside the narrowest column it
    // ever sits in. Pure markup: no effect runs, which is the point.
    name: "LoginUpdateNotice",
    element: <LoginUpdateNotice {...loginUpdateNotice(UPDATE_NEWER)!} />,
  },
  {
    // The state the login page exists for (#436's follow-up): the build is
    // below the server's minimum supported version, so the notice takes
    // UpdateBanner's danger tone and its wording instead of the mild "is
    // available" copy. That sentence is the longer of the two, so the 360
    // render here is the more informative one.
    name: "LoginUpdateNoticeRequired",
    element: <LoginUpdateNotice {...loginUpdateNotice(UPDATE_REQUIRED)!} />,
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
    // The document home (issue #466, now the global workspace dashboard in
    // #491/#630): six destinations for an admin. Quotation, Purchase Order,
    // Reports and Admin are enabled; Delivery Order and Invoice render
    // disabled with their Coming later badges. Reports and Admin both use the
    // wide two-column pattern and stack at 360.
    //
    // What this case cannot show, stated so it is not assumed: the clicks
    // behind the enabled buttons, keyboard order, and focus visibility
    // stay hand-driven.
    name: "DocumentHome",
    element: (
      <DocumentHomePage
        isAdmin
        onOpenQuotation={() => undefined}
        onOpenPurchaseOrder={() => undefined}
        onOpenReports={() => undefined}
        onOpenAdmin={() => undefined}
      />
    ),
  },
  {
    // Issue #630: Home for a staff member. The Admin card is absent and the
    // Reports card is still present, so the screenshot proves both the gate
    // and Reports' independent availability.
    name: "DocumentHomeStaff",
    element: (
      <DocumentHomePage
        onOpenQuotation={() => undefined}
        onOpenPurchaseOrder={() => undefined}
        onOpenReports={() => undefined}
      />
    ),
  },
  {
    // Issue #491: the shell on Home — Vera + Business Suite, no workspace tabs,
    // no back arrow.
    name: "AppShellHome",
    element: (
      <AppShell page="home" onNavigate={() => undefined}>
        <p className="text-body text-ink-500">Workspace cards sit here.</p>
      </AppShell>
    ),
  },
  {
    // Issue #491: the Quotation workspace's three tabs beside the back arrow.
    name: "AppShellQuotation",
    element: (
      <AppShell page="create" onNavigate={() => undefined}>
        <p className="text-body text-ink-500">Quotation editor body.</p>
      </AppShell>
    ),
  },
  {
    // Issue #491: the PO workspace's tabs with Drafts active.
    name: "AppShellPurchaseOrder",
    element: (
      <AppShell page="po-drafts" onNavigate={() => undefined}>
        <p className="text-body text-ink-500">Draft purchase orders body.</p>
      </AppShell>
    ),
  },
  {
    // Issue #491: the Admin workspace's four tabs with Company active.
    name: "AppShellAdmin",
    element: (
      <AppShell page="admin-company" onNavigate={() => undefined}>
        <p className="text-body text-ink-500">Company settings body.</p>
      </AppShell>
    ),
  },
  {
    // Issue #630: Reports is its own global workspace. The shell names it,
    // offers Back to Home, and renders no PO tabs.
    name: "AppShellReports",
    element: (
      <AppShell page="reports" onNavigate={() => undefined}>
        <p className="text-body text-ink-500">Report filters sit here.</p>
      </AppShell>
    ),
  },
  {
    // Issue #630/#644: the assembled Reports page with a stable normalized
    // fixture. It is the presentational View, so renderToString shows the
    // loaded screen without effects. The Overview owns the controls, actual
    // KPIs, coverage and embedded chart; the category table and independent
    // month browser stay outside it. The detailed method/range prose is in the
    // closed Details region, so the default screenshot shows the compact
    // forecast status rather than repeated paragraphs.
    //
    // What this case cannot show: loading, error and empty states (covered by
    // the DOM suite), filter interactions, and the chart's measured width and
    // motion (covered by the DOM suite; the static chart uses its 960 fallback).
    name: "Reports",
    element: (
      <PurchaseOrderReportsView
        filters={{
          // The fixture is the "this year to today" snapshot on 19 Nov 2026,
          // so the controls name the same inclusive range the report covers.
          preset: "this-year",
          period: null,
          from: "2026-01-01",
          to: "2026-11-19",
          bucket: "month",
          category: "all",
          currency: "IDR",
        }}
        report={purchaseOrderReportFixture()}
        loading={false}
        stale={false}
        error=""
        validation=""
        catalogue={[
          {
            code: "IDR",
            exponent: 0,
            symbol: "Rp",
            position: "before",
            groupSep: ".",
            decimalSep: ",",
            symbolSpaced: false,
          },
          {
            code: "USD",
            exponent: 2,
            symbol: "$",
            position: "before",
            groupSep: ",",
            decimalSep: ".",
            symbolSpaced: false,
          },
        ]}
        exporting={false}
        canExport={false}
        onPresetChange={() => undefined}
        onPeriodChange={() => undefined}
        onFromChange={() => undefined}
        onToChange={() => undefined}
        onBucketChange={() => undefined}
        onCategoryChange={() => undefined}
        onCurrencyChange={() => undefined}
        onRetry={() => undefined}
        onExport={() => undefined}
        monthBrowser={{
          months: ["2026-11"],
          activeMonth: "2026-11",
          allYear: null,
          layout: "individual",
          notice: "",
          readouts: [REPORT_MONTH_CURRENT_READOUT],
          aggregate: null,
          triggerRef: createRef<HTMLButtonElement>(),
          onOpenPicker: () => undefined,
          onLayoutChange: () => undefined,
          onPrevious: () => undefined,
          onNext: () => undefined,
          onRetryMonth: () => undefined,
        }}
      />
    ),
  },
  {
    // Issue #641: a loaded past month with no orders is genuine zero evidence,
    // not a hidden section. The year KPI strip stays its own actual figures.
    name: "ReportsMonthPastZero",
    element: monthReportView(
      REPORT_MONTH_PAST_ZERO,
      REPORT_MONTH_PAST_ZERO_READOUT,
      reportChartFilters(REPORT_MONTH_PAST_ZERO, "year")
    ),
  },
  {
    // Issue #641: a future month inside the current year reads only the
    // current-year projection bucket. POs and Costed amount are not forecast.
    name: "ReportsMonthFutureAvailable",
    element: monthReportView(
      REPORT_MONTH_FUTURE,
      REPORT_MONTH_FUTURE_READOUT,
      reportChartFilters(REPORT_MONTH_FUTURE, "year")
    ),
  },
  {
    // Issue #641: an unavailable future forecast states its stable reason and
    // never invents zeros. The month table remains visible.
    name: "ReportsMonthFutureUnavailable",
    element: monthReportView(
      REPORT_MONTH_FUTURE_UNAVAILABLE,
      REPORT_MONTH_FUTURE_UNAVAILABLE_READOUT,
      reportChartFilters(REPORT_MONTH_FUTURE_UNAVAILABLE, "year")
    ),
  },
  {
    // Issue #642: three selected months, chronological, with the odd final card
    // spanning the last row above the lg breakpoint. The same vertical metric
    // table is used as the single-month table. Cropped to its heading so the
    // screenshot frames the section, not the report's top.
    name: "ReportsMonthComparisonGrid",
    cropToHeading: "By period",
    element: monthComparisonView(
      purchaseOrderReportYearFixture(),
      ["2026-01", "2026-06", "2026-11"],
      "individual"
    ),
  },
  {
    // Issue #642: December 2025 before January 2026, with each card naming its
    // own snapshot/as-of. The 2025 source is a loaded zero year, so the card is
    // genuine zero evidence rather than an unfetched guess.
    name: "ReportsMonthCrossYear",
    cropToHeading: "By period",
    element: monthComparisonView(
      purchaseOrderReportYearFixture(),
      ["2025-12", "2026-01"],
      "individual",
      zeroAnnualMonthSource("2025")
    ),
  },
  {
    // Issue #642: the default All presentation: one wide twelve-row table and
    // the annual server snapshot's Recorded total, with December's forecast as
    // a separate marked line below the table.
    name: "ReportsMonthAllAggregate",
    cropToHeading: "By period",
    element: monthComparisonView(
      purchaseOrderReportYearFixture(),
      allMonthsOfYear("2026"),
      "aggregate"
    ),
  },
  {
    // Issue #642: the remembered All/Individual display: the year's twelve
    // vertical cards in the two-column comparison grid.
    name: "ReportsMonthAllIndividual",
    cropToHeading: "By period",
    element: monthComparisonView(
      purchaseOrderReportYearFixture(),
      allMonthsOfYear("2026"),
      "individual"
    ),
  },
  {
    // Issue #642: the explicit choose-months state. It is not a zero-valued
    // financial report; the picker trigger stays available above it.
    name: "ReportsMonthEmpty",
    cropToHeading: "By period",
    element: monthComparisonView(
      purchaseOrderReportYearFixture(),
      [],
      "individual"
    ),
  },
  {
    // Issue #642: the real open picker surface (the same controller surface,
    // statically rendered) over the report. Item clicks and paging are DOM
    // behaviour and are covered by the page suite; this case is the 360/1280
    // visual read of labels, checks, count and the conspicuous X.
    name: "ReportsMonthPickerOpen",
    element: (
      <div>
        {monthComparisonView(
          purchaseOrderReportYearFixture(),
          ["2026-11"],
          "individual"
        )}
        <PurchaseOrderReportPickerView
          title="Choose months"
          items={allMonthsOfYear("2026").map((month) => ({
            key: month,
            label: monthLabel(month),
          }))}
          selectedKeys={["2026-11"]}
          allLabel="All months in 2026"
          allSelected={false}
          onSelectAll={() => undefined}
          onToggle={() => undefined}
          onClose={() => undefined}
          pageLabel="2026"
          onPreviousPage={() => undefined}
          onNextPage={() => undefined}
        />
      </div>
    ),
  },
  {
    // Issue #643: three independently selected Business categories: the
    // two-column grid's pair plus the full-width odd final card. Cropped to the
    // section heading so the screenshot frames By category, not the report top.
    name: "ReportsCategoryComparisonGrid",
    cropToHeading: "By category",
    element: categoryComparisonView(
      REPORT_CATEGORY_ALL,
      REPORT_CATEGORY_ALL,
      ["FM", "FAB", "SUPPLY_GEN"],
      "individual"
    ),
  },
  {
    // Issue #643: All categories in the remembered Individual display: five
    // vertical cards, two pairs and the full-width fifth card. The 360 render
    // is the narrow-window read of the one-column grid and exact money.
    name: "ReportsCategoryAllIndividual",
    cropToHeading: "By category",
    element: categoryComparisonView(
      REPORT_CATEGORY_ALL,
      REPORT_CATEGORY_ALL,
      null,
      "individual"
    ),
  },
  {
    // Issue #643: the real open category picker surface (the same controller
    // surface, statically rendered) over the report. Item clicks, background
    // updates and focus restoration are DOM behaviour in the page suite; this is
    // the 360/1280 visual read of supplied labels, the visible check and count,
    // and the conspicuous X.
    name: "ReportsCategoryPickerOpen",
    element: (
      <div>
        {categoryComparisonView(
          REPORT_CATEGORY_ALL,
          REPORT_CATEGORY_ALL,
          ["FAB"],
          "aggregate"
        )}
        <PurchaseOrderReportPickerView
          title="Choose categories"
          items={REPORT_CATEGORY_FAB_READOUT.supplied.map((item) => ({
            key: item.code,
            label: item.label,
          }))}
          selectedKeys={["FAB"]}
          allLabel="All categories"
          allSelected={false}
          onSelectAll={() => undefined}
          onToggle={() => undefined}
          onClose={() => undefined}
        />
      </div>
    ),
  },
  {
    // Issue #643: Performance is restricted to FM and has no purchase orders,
    // while By category still shows the genuine all-category universe. The scope
    // caption names the inherited range/currency/business date and the local All
    // selection, never FM as if it filtered this section.
    name: "ReportsCategoryRestrictedPerformance",
    cropToHeading: "By category",
    element: categoryComparisonView(
      REPORT_CATEGORY_RESTRICTED,
      REPORT_CATEGORY_ALL,
      null,
      "aggregate"
    ),
  },
  {
    // Issue #631/#644, normal actual data: the Overview cropped to its
    // Performance heading so the grouped money columns, zero baseline, margin %
    // line, compact legend and one forecast status are legible.
    name: "ReportsChartActual",
    cropToHeading: "Performance",
    element: reportChartView(purchaseOrderReportFixture()),
  },
  {
    // A historical shared snapshot with a real negative period (December). It
    // is not the current month or year, so it draws actuals only and the
    // compact forecast status states the no-ghost reason.
    name: "ReportsChartForecastNegative",
    cropToHeading: "Performance",
    element: <SharedReportChartView />,
  },
  {
    // Issue #638/#644: the full Reports screen in Month mode. The Overview
    // states "Current month to date", keeps one period toggle and draws the
    // daily ghost view; the independent month browser sits below it.
    name: "ReportsCurrentMonth",
    element: reportChartView(
      purchaseOrderReportMonthFixture(),
      "month",
      REPORT_DAY_PAGE_READOUT
    ),
  },
  {
    // Issue #638/#644: the full Reports screen in Year mode. The Overview
    // states "Current year to date" and the chart shows November's remainder
    // stack plus the December ghost month.
    name: "ReportsCurrentYear",
    element: reportChartView(
      purchaseOrderReportYearFixture(),
      "year",
      REPORT_YEAR_PAGE_READOUT
    ),
  },
  {
    // Issue #638/#644: the current calendar month by day. Days 20 to 30 are the
    // server's ghost buckets after the horizontal dashed divider; the one
    // compact forecast status is visible and the full basis/range prose is in
    // the closed Details region.
    name: "ReportsChartCurrentMonth",
    cropToHeading: "Performance",
    element: reportChartView(
      purchaseOrderReportMonthFixture(),
      "month",
      REPORT_DAY_PAGE_READOUT
    ),
  },
  {
    // Issue #638/#644: the current calendar year by month. November carries the
    // stacked projected remainder; December is the one ghost month. Both are
    // visibly outside the actual totals.
    name: "ReportsChartCurrentYear",
    cropToHeading: "Performance",
    element: reportChartView(
      purchaseOrderReportYearFixture(),
      "year",
      REPORT_YEAR_PAGE_READOUT
    ),
  },
  {
    // Issue #631, JavaScript's largest exact money range: the money axis must
    // keep a finite compact domain at this magnitude, and the negative margin
    // space must remain below zero.
    name: "ReportsChartMaxRange",
    cropToHeading: "Performance",
    element: reportChartView(purchaseOrderReportMaxFixture()),
  },
  {
    // Issue #630 review: the same assembled screen at JavaScript's largest
    // exact money figure. The 360 render is the evidence that the KPI strip
    // uses one column and keeps the complete `Rp9.007.199.254.740.991` inside
    // the card; 1280 is the five-column desktop read. The tables also carry
    // maximum figures so a wide value cannot silently truncate.
    name: "ReportsMaxAmounts",
    element: (
      <PurchaseOrderReportsView
        filters={{
          preset: "this-year",
          period: null,
          from: "2026-01-01",
          to: "2026-11-19",
          bucket: "month",
          category: "all",
          currency: "IDR",
        }}
        report={REPORT_MAX_CURRENT}
        loading={false}
        stale={false}
        error=""
        validation=""
        catalogue={[
          {
            code: "IDR",
            exponent: 0,
            symbol: "Rp",
            position: "before",
            groupSep: ".",
            decimalSep: ",",
            symbolSpaced: false,
          },
          {
            code: "USD",
            exponent: 2,
            symbol: "$",
            position: "before",
            groupSep: ",",
            decimalSep: ".",
            symbolSpaced: false,
          },
        ]}
        exporting={false}
        canExport={false}
        onPresetChange={() => undefined}
        onPeriodChange={() => undefined}
        onFromChange={() => undefined}
        onToChange={() => undefined}
        onBucketChange={() => undefined}
        onCategoryChange={() => undefined}
        onCurrencyChange={() => undefined}
        onRetry={() => undefined}
        onExport={() => undefined}
        monthReadout={REPORT_MAX_CURRENT_READOUT}
        onPreviousMonth={() => undefined}
        onNextMonth={() => undefined}
        onRetryMonth={() => undefined}
      />
    ),
  },
  {
    // The same maximum figures in the widest supported spelling (spaced AED).
    // The 360 render is the different-currency half of the KPI evidence.
    name: "ReportsMaxAed",
    element: (
      <PurchaseOrderReportsView
        filters={{
          preset: "custom",
          period: null,
          from: "2026-01-01",
          to: "2026-12-31",
          bucket: "month",
          category: "all",
          currency: "AED",
        }}
        report={REPORT_MAX_AED_CURRENT}
        loading={false}
        stale={false}
        error=""
        validation=""
        catalogue={[
          {
            code: "IDR",
            exponent: 0,
            symbol: "Rp",
            position: "before",
            groupSep: ".",
            decimalSep: ",",
            symbolSpaced: false,
          },
          {
            code: "AED",
            exponent: 2,
            symbol: "AED",
            position: "before",
            groupSep: ",",
            decimalSep: ".",
            symbolSpaced: true,
          },
        ]}
        exporting={false}
        canExport={false}
        onPresetChange={() => undefined}
        onPeriodChange={() => undefined}
        onFromChange={() => undefined}
        onToChange={() => undefined}
        onBucketChange={() => undefined}
        onCategoryChange={() => undefined}
        onCurrencyChange={() => undefined}
        onRetry={() => undefined}
        onExport={() => undefined}
        monthReadout={REPORT_MAX_AED_CURRENT_READOUT}
        onPreviousMonth={() => undefined}
        onNextMonth={() => undefined}
        onRetryMonth={() => undefined}
      />
    ),
  },
  {
    // Issue #494: the assembled Admin Company page, not the shell alone. The
    // primary bar is the only Clients/Team/Company/Backups navigation, and the
    // Company profile's own General/Units/Quotation/Portal bar sits in the body
    // where the duplicate main tablist used to be. The card's effects do not
    // run under renderToString, so the panel below the bar is its loading
    // state; that is enough for the placement question, because the bar renders
    // before the fetch resolves.
    //
    // What this case cannot show: the Company panels' loaded content, tab
    // switching, keyboard order and focus stay hand-driven.
    name: "AppShellAdminCompany",
    element: (
      <ToastProvider>
        <AppShell
          page="admin-company"
          settingsOrigin="admin"
          onNavigate={() => undefined}
        >
          <AdminDashboard tab="company" />
        </AppShell>
      </ToastProvider>
    ),
  },
  {
    // Ticket 03: the Drafts tab as a docket list. At ≥1024 a wrapping table
    // with no min-width and no horizontal scroll; below 1024 the same rows
    // stack as cards. The filters collapse behind a toggle with chips.
    //
    // What this case cannot show: typing, opening a row, the fetch effect,
    // focus order and the real breakpoint behavior stay hand-driven and are
    // read from the PNGs, not asserted here.
    name: "PurchaseOrderDrafts",
    element: (
      <PurchaseOrderListView
        mode="drafts"
        state={PO_LIST_DRAFTS}
        dispatch={() => undefined}
        onOpen={() => undefined}
        onAdd={() => undefined}
        onRetry={() => undefined}
      />
    ),
  },
  {
    // Issue #491: the Archive tab. The state is pinned to saved records, so the
    // State control is gone while search, dates, customers and source remain.
    name: "PurchaseOrderArchive",
    element: (
      <PurchaseOrderListView
        mode="archive"
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
        mode="archive"
        state={PO_LIST_NO_MATCH}
        dispatch={() => undefined}
        onOpen={() => undefined}
        onAdd={() => undefined}
        onRetry={() => undefined}
      />
    ),
  },
  {
    // Issue #496: never-linked, formerly-linked and a real revision 00 in one
    // list. The first row must read "No quotation linked" with no fabricated
    // Customer, the second keeps its former source, and the third keeps
    // "Revision 0". The Source missing badge is shared, because the filter and
    // the user-facing state are shared.
    name: "PurchaseOrderSourceMissing496",
    element: (
      <PurchaseOrderListView
        mode="drafts"
        state={PO_LIST_SOURCE_MISSING_496}
        dispatch={() => undefined}
        onOpen={() => undefined}
        onAdd={() => undefined}
        onRetry={() => undefined}
      />
    ),
  },
  {
    // Ticket 04: the two-pane editor composition: form column plus the
    // sticky rail at 1024 and above, the sheet trigger below. The pane
    // content is the same component in both places; only the layout wrapper
    // changes with the breakpoint.
    name: "PurchaseOrderEditorTwoPane",
    element: (
      <>
        <ReconciliationSheetTrigger
          unresolvedCount={1}
          sourceLabel="QT-2026-018 · revision 02"
          expanded={false}
          onOpen={() => undefined}
        />
        <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_320px] xl:grid-cols-[minmax(0,1fr)_360px]">
          <div className="flex min-w-0 flex-col gap-6">
            <div className="rounded-xl border border-rule bg-white p-5">
              <p className="text-small font-medium text-ink-900">Order details</p>
              <div className="mt-3 h-10 animate-pulse rounded-md bg-paper-muted" />
              <div className="mt-3 h-10 animate-pulse rounded-md bg-paper-muted" />
            </div>
          </div>
          <aside
            aria-label="Reconciliation"
            className="hidden min-h-0 lg:sticky lg:top-[88px] lg:block lg:max-h-[calc(100dvh-176px)] lg:flex-col"
          >
            <ReconciliationPaneContent
              idPrefix="harness-twopane"
              session={PO_RECONCILIATION_SESSION}
              status="ready"
              error={null}
              sourceLabel="QT-2026-018 · revision 02"
              sourceRevision={2}
              noSource={false}
              formerSource={false}
              readOnly={false}
              filter="all"
              onFilterChange={() => undefined}
              onShowAll={() => undefined}
              latestNotice={null}
              keepConfirmKey={null}
              onRequestKeep={() => undefined}
              onCancelKeep={() => undefined}
              onConfirmKeep={() => undefined}
              onUse={() => undefined}
              onRetry={() => undefined}
              actorNameFor={() => null}
            />
          </aside>
        </div>
      </>
    ),
  },
  {
    // Ticket 07: the reconciliation surfaces at both widths. At 360 the sheet
    // fills the viewport between the header and the action bar; at 1280 the
    // rail is the same pane. The editor's sheet state is internal, so this
    // case renders the settled sheet shell directly; the fixed action bar is
    // included so the bottom offset is visible.
    name: "PurchaseOrderReconciliationSurfaces",
    element: (
      <div className="flex min-h-[560px] flex-col">
        <div className="lg:hidden">
          <div
            role="region"
            aria-label="Reconciliation"
            className="animate-sheet-in fixed inset-x-0 bottom-0 top-[var(--shell-header-h,64px)] z-20 flex flex-col gap-3 overflow-y-auto border-t border-rule bg-paper px-5 pb-24 pt-4"
          >
            <ReconciliationPaneContent
              idPrefix="harness-sheet"
              session={PO_RECONCILIATION_SESSION}
              status="ready"
              error={null}
              sourceLabel="QT-2026-018 · revision 02"
              sourceRevision={2}
              noSource={false}
              formerSource={false}
              readOnly={false}
              filter="all"
              onFilterChange={() => undefined}
              onShowAll={() => undefined}
              latestNotice={null}
              keepConfirmKey={null}
              onRequestKeep={() => undefined}
              onCancelKeep={() => undefined}
              onConfirmKeep={() => undefined}
              onUse={() => undefined}
              onRetry={() => undefined}
              actorNameFor={() => null}
            />
          </div>
        </div>
        <div className="hidden lg:block">
          <aside
            aria-label="Reconciliation"
            className="animate-rail-in min-h-0 lg:sticky lg:top-[88px] lg:flex lg:max-h-[calc(100dvh-176px)] lg:flex-col"
          >
            <ReconciliationPaneContent
              idPrefix="harness-rail"
              session={PO_RECONCILIATION_SESSION}
              status="ready"
              error={null}
              sourceLabel="QT-2026-018 · revision 02"
              sourceRevision={2}
              noSource={false}
              formerSource={false}
              readOnly={false}
              filter="all"
              onFilterChange={() => undefined}
              onShowAll={() => undefined}
              latestNotice={null}
              keepConfirmKey={null}
              onRequestKeep={() => undefined}
              onCancelKeep={() => undefined}
              onConfirmKeep={() => undefined}
              onUse={() => undefined}
              onRetry={() => undefined}
              actorNameFor={() => null}
            />
          </aside>
        </div>
        <div className="fixed inset-x-0 bottom-0 z-30 border-t border-rule bg-paper/95 px-5 py-3">
          <div className="mx-auto flex max-w-[1560px] items-center justify-between gap-3">
            <span className="text-caption text-ink-500">Unsaved changes</span>
            <span className="flex items-center gap-2">
              <span className="inline-flex min-h-9 items-center rounded-lg border border-rule-strong bg-white px-3.5 text-small font-medium text-ink-700">
                Save draft
              </span>
              <span className="inline-flex min-h-9 items-center rounded-lg bg-accent-700 px-3.5 text-small font-medium text-white">
                Final Save
              </span>
            </span>
          </div>
        </div>
      </div>
    ),
  },
  {
    // Ticket 04: the live reconciliation pane with an unresolved difference,
    // a kept resolution and the link-time tax basis. Unresolved first, then
    // resolved; the count reads 1 to resolve. What this case cannot show is
    // committing, keep/use and the Final Save gate: those are interaction and
    // stay in the DOM suite and hand-driven review.
    name: "PurchaseOrderReconciliationPane",
    element: (
      <ReconciliationPaneContent
        idPrefix="harness"
        session={PO_RECONCILIATION_SESSION}
        status="ready"
        error={null}
        sourceLabel="QT-2026-018 · revision 02"
        sourceRevision={2}
        noSource={false}
        formerSource={false}
        readOnly={false}
        filter="all"
        onFilterChange={() => undefined}
        onShowAll={() => undefined}
        latestNotice={null}
        keepConfirmKey={null}
        onRequestKeep={() => undefined}
        onCancelKeep={() => undefined}
        onConfirmKeep={() => undefined}
        onUse={() => undefined}
        onRetry={() => undefined}
        actorNameFor={() => null}
      />
    ),
  },
  {
    // Ticket 05: the line set in the pane. A differing line field, a missing
    // line with its Add line action, an extra line, an unknown reference with
    // its recorded acknowledgement (still blocking), a blocking duplicate and
    // the link-time tax basis triple, plus the derived totals strip. What
    // this case cannot show is committing, rematch and the Final Save gate:
    // those are interaction and stay in the DOM suite and hand-driven review.
    name: "PurchaseOrderReconciliationLineSet",
    element: (
      <div className="flex min-h-0 flex-col gap-3">
      <ReconciliationPaneContent
        idPrefix="harness-lineset"
        session={PO_RECONCILIATION_LINE_SESSION}
        status="ready"
        error={null}
        sourceLabel="QT-2026-018 · revision 02"
        sourceRevision={2}
        noSource={false}
        formerSource={false}
        readOnly={false}
        filter="all"
        onFilterChange={() => undefined}
        onShowAll={() => undefined}
        latestNotice={null}
        keepConfirmKey={null}
        onRequestKeep={() => undefined}
        onCancelKeep={() => undefined}
        onConfirmKeep={() => undefined}
        onUse={() => undefined}
        onRetry={() => undefined}
        onAddLineFromSource={() => undefined}
        onAcknowledgeMissing={() => undefined}
        onAcknowledgeExtra={() => undefined}
        onFixMatch={() => undefined}
        currency={PO_EDITOR_IDR}
        actorNameFor={() => null}
        fillHeight={false}
        lineSummaries={{
          "client:a": { displayNumber: 1, description: "Widget" },
          "client:b": { displayNumber: 2, description: "Gadget" },
          "client:u": { displayNumber: 3, description: "Thingamajig" },
          "client:c": { displayNumber: 4, description: "Widget duplicate" },
          "client:d": { displayNumber: 5, description: "Sprocket" },
        }}
        sourceDescriptions={{ "1": "Widget", "2": "Sprocket" }}
      />
      <div className="rounded-lg border border-rule bg-white p-3">
        <ReconciliationTotalsStrip
          calculated={{
            subtotalMinor: 20000,
            discountMinor: 400,
            ppnMinor: 1960,
            grandTotalMinor: 21560,
          }}
          printed={{
            subtotalMinor: 20000,
            discountMinor: 0,
            ppnMinor: 2000,
            grandTotalMinor: 22000,
          }}
          reason="Whole-PO discount 2% vs 0%"
          currency={PO_EDITOR_IDR}
        />
      </div>
    </div>
    ),
  },
  {
    // Issue #623: the Quotation editor's hero, a new draft. The accent-800
    // surface, the DRAFT badge, the live customer total (PPN included) and the
    // four-item meta row come from the same computePricing path the Summary
    // rail uses. 360 is the informative width: the badge row, the total block
    // and the meta row must wrap instead of overflowing.
    //
    // What this case cannot show: live typing (the App DOM suite drives it)
    // and the hero's read-only nature, which the component suite pins.
    name: "QuotationHeroDraft",
    element: (
      <QuotationHero
        customerName="PT. Infineon Technologies Batam"
        details={{
          validity: 30,
          paymentTerm: 90,
          incoterm: "EXW",
          taxZone: "NonFTZ",
          taxType: "Goods",
          serviceCategory: "",
        }}
        currencyCode="IDR"
        currency={PO_EDITOR_IDR}
        customerGrand={1_110_000}
        ppnAmount={110_000}
        quoteNumber={null}
        nextRevision={null}
        draftSavedLabel={null}
        ready={false}
      />
    ),
  },
  {
    // Issue #623: editing an issued revision. The quotation number is the
    // title, the badge row names the source revision and the revision Export
    // will create, and the status line repeats both so they cannot be
    // confused.
    name: "QuotationHeroIssued",
    element: (
      <QuotationHero
        customerName="PT. Infineon Technologies Batam"
        details={{
          validity: 45,
          paymentTerm: 60,
          incoterm: "DAP",
          taxZone: "FTZ",
          taxType: "Goods",
          serviceCategory: "",
        }}
        currencyCode="IDR"
        currency={PO_EDITOR_IDR}
        customerGrand={0}
        ppnAmount={0}
        quoteNumber="1502260012"
        nextRevision={3}
        draftSavedLabel={null}
        ready={false}
      />
    ),
  },
  {
    // Issue #624: the Features Tutorial's practice Quotation workspace, hero
    // included, so the tour can be compared with the real editor at both
    // widths. The workspace caps its content at the editor's 1280px in the
    // shell, the notice spans it above the grid, and the hero is the left
    // column's first child above the practice details, lines and summary; the
    // practice quotation is always a draft.
    //
    // What this case cannot show: live typing (the tutorial DOM suite drives
    // it) and the guide overlay, which mounts only in the running app.
    name: "QuotationTutorialPractice",
    element: (
      <QuotationTutorialWorkspace
        currency={PO_EDITOR_IDR}
        canonicalUnit="Lot"
        uomOptions={[{ value: "Lot", label: "Lot" }]}
        incotermOptions={INCOTERM_OPTIONS}
        taxZoneOptions={TAX_ZONE_OPTIONS}
        taxTypeOptions={TAX_TYPE_OPTIONS}
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
    // #656: a closed quotation and an expired-unclosed one. Closed is disabled
    // with its own badge; expired-unclosed keeps its remaining quantity and is
    // selectable, so the two states must read differently at 360 and 1280.
    name: "POSelectorClosedAndExpired",
    element: (
      <POQuotationSelectorView
        state={{
          ...PO_SELECTOR_STEP_ONE,
          rows: [
            {
              ...PO_SELECTOR_ROWS[0],
              closed: true,
              closedAt: "2026-11-20T10:00:00+07:00",
              closedReason: "Customer confirmed the remaining quantity will not be ordered.",
            },
            {
              ...PO_SELECTOR_ROWS[1],
              activePO: null,
              expiredUnclosed: true,
              validUntil: "2026-10-01",
            },
            PO_SELECTOR_ROWS[2],
          ],
        }}
        dispatch={() => undefined}
        onClose={() => undefined}
        onConfirm={() => undefined}
      />
    ),
  },
  {
    // The manual PO draft editor (issue #468/#470, header from #580): the hero
    // names the customer's PO number with the quotation identity in the Source
    // quotation block; the Purchase order details card sits under it. Lines are
    // in the PO's own USD currency with per-row discount controls, and the split
    // line carries its own "Remove split line" control. At 360 this is the case
    // that says whether the five-field line grid reflows instead of overflowing.
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
    // #654: the aggregate presentation. The header card carries the derived
    // total and unit price, the display quantity/unit and the print toggle;
    // the three child rows below it are the same order as the normal editor,
    // with their split package and row discounts intact. At 360 this is the
    // case that says whether the header fields reflow instead of overflowing.
    name: "PurchaseOrderAggregateEditor",
    cropToHeading: "Line items",
    element: (
      <PurchaseOrderEditorView
        state={PO_AGGREGATE_STATE}
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
    // Inline quotation values: a differing Payment terms field is blue until
    // used, and the linked source lines sit in the same comparison shape the
    // editor's quotation ghosts consume. The line section is the crop so the
    // ghost under line 1 and the unmatched item are both visible.
    name: "PurchaseOrderInlineQuote",
    cropToHeading: "Line items",
    element: (
      <PurchaseOrderEditorView
        state={PO_EDITOR_INITIAL}
        dispatch={() => undefined}
        currency={PO_EDITOR_USD}
        uomOptions={PO_EDITOR_UOM_OPTIONS}
        incotermOptions={INCOTERM_OPTIONS}
        quoteSource={{ paymentTerms: "30 days", incoterms: "CIF" }}
        quoteLines={[
          {
            QuotationItemNo: 1,
            QuotationSubNo: null,
            LineLabel: "1",
            Description: "Supply and installation of electrical panels",
            UoM: "Lot",
            QuantityScaled: 25000,
            UnitPriceMinor: 130000,
            DiscountEnabled: true,
            DiscountPercent: 10,
          },
          {
            QuotationItemNo: 2,
            QuotationSubNo: 1,
            LineLabel: "2.1",
            ParentDescription: "Mounting package",
            Description: "Mounting and cable termination",
            DescriptionLines: ["Cable tray", "Terminations"],
            UoM: "Unit",
            QuantityScaled: 10000,
            UnitPriceMinor: 42000,
            DiscountEnabled: false,
            DiscountPercent: 0,
          },
          {
            QuotationItemNo: 2,
            QuotationSubNo: 2,
            LineLabel: "2.2",
            ParentDescription: "Mounting package",
            Description: "Cable termination and labelling",
            DescriptionLines: ["Two technicians"],
            UoM: "Unit",
            QuantityScaled: 10000,
            UnitPriceMinor: 31000,
            DiscountEnabled: true,
            DiscountPercent: 5,
          },
          {
            QuotationItemNo: 3,
            QuotationSubNo: null,
            LineLabel: "3",
            Description: "Spare parts kit",
            DescriptionLines: ["One-year consumables"],
            UoM: "Set",
            QuantityScaled: 10000,
            UnitPriceMinor: 8500,
            DiscountEnabled: false,
            DiscountPercent: 0,
          },
        ]}
        onSave={() => undefined}
        onCancel={() => undefined}
        onFinalSave={() => undefined}
        finalSaving={false}
      />
    ),
  },
  {
    // The two inline-valued header fields at their blue (available) state.
    name: "PurchaseOrderInlineFields",
    cropToHeading: "Purchase order details",
    element: (
      <PurchaseOrderEditorView
        state={PO_EDITOR_INITIAL}
        dispatch={() => undefined}
        currency={PO_EDITOR_USD}
        uomOptions={PO_EDITOR_UOM_OPTIONS}
        incotermOptions={INCOTERM_OPTIONS}
        quoteSource={{ paymentTerms: "30 days", incoterms: "CIF" }}
        onSave={() => undefined}
        onCancel={() => undefined}
        onFinalSave={() => undefined}
        finalSaving={false}
      />
    ),
  },
  {
    // #580: a linked draft that has had its last line removed. The hero keeps
    // the customer PO number, the Source quotation block keeps the quotation
    // identity, and the line section shows the empty state with Add line. The
    // zero-line draft is saveable; only Final Save refuses it. The empty state
    // sits below the 900px fold, so the viewport starts at the line section.
    name: "PurchaseOrderEmptyLines580",
    cropToHeading: "Line items",
    element: (
      <PurchaseOrderEditorView
        state={initialPurchaseOrderEditorState(
          { ...PO_EDITOR_ORDER, Items: [] },
          PO_EDITOR_USD
        )}
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
    // Issue #496: the PO-first blank editor. It has no source, Customer,
    // quote number, revision or printed totals, but it is the full manual-entry
    // form: Link a Quotation at the top, the editable source basis (currency
    // and tax triple) and the PO-owned header/lines. At 360 the link card,
    // basis grid and header wrap instead of overflowing, and the header must
    // never read "Draft 0" or "revision 0".
    name: "PurchaseOrderFirstDraft496",
    element: (
      <PurchaseOrderEditorView
        state={initialPurchaseOrderEditorState(
          blankPurchaseOrderDraft({
            currency: "IDR",
            taxZone: "NonFTZ",
            taxType: "Goods",
            serviceCategory: "",
            receivedDate: "2026-10-22",
            paymentTerms: "50% advance",
            incoterms: "EXW",
          }),
          PO_EDITOR_IDR
        )}
        dispatch={() => undefined}
        currency={PO_EDITOR_IDR}
        currencyOptions={[PO_EDITOR_IDR, PO_EDITOR_USD]}
        uomOptions={PO_EDITOR_UOM_OPTIONS}
        incotermOptions={INCOTERM_OPTIONS}
        onLinkQuotation={() => undefined}
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
    // The final-save difference review (issue #471, redesigned by #582): every
    // differing value as a two-sided diff with both choices unpressed, the
    // fixed currency statement, the summary strip, one omitted source line,
    // two extra PO lines (one with a suggestion), the corrected match picker
    // and the recalculated totals. At 360 the sides stack under `↓` instead of
    // overflowing.
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
    // #582: the same review after decisions. One value chose the quotation and
    // one kept the PO entry, so the selected side says "Will be used" and the
    // other is muted; the missing and extra lines are acknowledged, so their
    // rows show the ACKNOWLEDGED chip instead of the action.
    name: "PurchaseOrderDifferenceReviewDecided",
    element: (
      <PurchaseOrderDifferenceReviewView
        state={PO_REVIEW_STATE}
        source={PO_REVIEW_SOURCE}
        currency={PO_EDITOR_USD}
        quoteNumber="1502260012"
        documentState="none"
        decisions={{
          "header:paymentTerms": "quotation",
          "line:1:unitPrice": "po",
        }}
        acknowledgedMissing={["3"]}
        acknowledgedExtra={[3]}
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
    // #518: the confirmation repeats the review's line acknowledgements, so
    // the whole comparison is visible before the repair writes.
    name: "PurchaseOrderRepairAckSummary",
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
        acknowledgedMissing={["3", "5"]}
        acknowledgedExtra={[2]}
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
    // The saved record with an active customer PO PDF (#609 review): the PDF
    // attached badge, the uploader id beside size and date, and the compact
    // earlier-versions disclosure under the file.
    name: "PurchaseOrderSavedPdfAttached",
    element: (
      <PurchaseOrderSavedView
        order={{ ...PO_EDITOR_ORDER, State: "saved", PdfAbsentConfirmed: false }}
        currency={PO_EDITOR_USD}
        onBack={() => undefined}
        onCorrect={() => undefined}
        documentInitialState={PO_DOCUMENT_ATTACHED}
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
    // #518: the saved record's read-only reconciliation history. The rail
    // holds the current generation (context line, count, saved-only filters,
    // the resolved difference and the tax triple), the 360 trigger duplicates
    // its state, and the previous generation is disclosed — never the current
    // one. No Keep/Use/acknowledge control exists.
    name: "PurchaseOrderSavedReconciliation",
    element: (
      <PurchaseOrderSavedView
        order={{ ...PO_EDITOR_ORDER, State: "saved", PdfAbsentConfirmed: true }}
        currency={PO_EDITOR_USD}
        onBack={() => undefined}
        documentInitialState={PO_DOCUMENT_NONE}
        historyInitial={PO_HISTORY}
        reconciliationInitial={PO_SAVED_RECONCILIATION_READ}
        actorId={3}
        actorName="Budi Santoso"
      />
    ),
  },
  {
    // #518: the same record with an earlier generation selected — the
    // "Previous saved session" context, the Previous badge and Back to current
    // session. The harness selects it up front because SSR cannot click.
    name: "PurchaseOrderSavedReconciliationHistory",
    element: (
      <PurchaseOrderSavedView
        order={{ ...PO_EDITOR_ORDER, State: "saved", PdfAbsentConfirmed: true }}
        currency={PO_EDITOR_USD}
        onBack={() => undefined}
        documentInitialState={PO_DOCUMENT_NONE}
        historyInitial={PO_HISTORY}
        reconciliationInitial={PO_SAVED_RECONCILIATION_READ}
        reconciliationInitialSelection={0}
      />
    ),
  },
  {
    // #518: the read is still in flight. The loading sentence and skeleton,
    // no count and no filters, while the saved record itself stays readable.
    name: "PurchaseOrderSavedReconciliationLoading",
    element: (
      <PurchaseOrderSavedView
        order={{ ...PO_EDITOR_ORDER, State: "saved", PdfAbsentConfirmed: true }}
        currency={PO_EDITOR_USD}
        onBack={() => undefined}
        documentInitialState={PO_DOCUMENT_NONE}
        historyInitial={PO_HISTORY}
      />
    ),
  },
  {
    // #518: the read failed. The alert sentence and Try again, never the
    // no-session claim.
    name: "PurchaseOrderSavedReconciliationError",
    element: (
      <PurchaseOrderSavedView
        order={{ ...PO_EDITOR_ORDER, State: "saved", PdfAbsentConfirmed: true }}
        currency={PO_EDITOR_USD}
        onBack={() => undefined}
        documentInitialState={PO_DOCUMENT_NONE}
        historyInitial={PO_HISTORY}
        reconciliationInitialError="The connection was reset while reading the reconciliation history."
      />
    ),
  },
  {
    // #518: a saved PO that predates reconciliation sessions. The exact
    // no-session sentence, no count, no filters and no history disclosure.
    name: "PurchaseOrderSavedReconciliationNoSession",
    element: (
      <PurchaseOrderSavedView
        order={{ ...PO_EDITOR_ORDER, State: "saved", PdfAbsentConfirmed: true }}
        currency={PO_EDITOR_USD}
        onBack={() => undefined}
        documentInitialState={PO_DOCUMENT_NONE}
        historyInitial={PO_HISTORY}
        reconciliationInitial={{ PurchaseOrderID: 7, POVersion: 1, Sessions: [] }}
      />
    ),
  },
  {
    // #518: the saved session's source quotation was deleted. The notice names
    // the stored source and revision, and the retained items still render —
    // availability is context, not the no-session branch.
    name: "PurchaseOrderSavedReconciliationSourceDeleted",
    element: (
      <PurchaseOrderSavedView
        order={{ ...PO_EDITOR_ORDER, State: "saved", PdfAbsentConfirmed: true }}
        currency={PO_EDITOR_USD}
        onBack={() => undefined}
        documentInitialState={PO_DOCUMENT_NONE}
        historyInitial={PO_HISTORY}
        reconciliationInitial={{
          PurchaseOrderID: 7,
          POVersion: 1,
          Sessions: [
            {
              Generation: 2,
              SourceAvailable: false,
              RepairCorrectionID: null,
              Session: {
                ...PO_SAVED_RECONCILIATION_SESSION,
                ID: 22,
                SourceRevision: 1,
              },
            },
          ],
        }}
      />
    ),
  },
  {
    // #518: a saved session that recorded no differences. The empty-session
    // sentence and a 0 resolved count, with no filters and no history.
    name: "PurchaseOrderSavedReconciliationEmpty",
    element: (
      <PurchaseOrderSavedView
        order={{ ...PO_EDITOR_ORDER, State: "saved", PdfAbsentConfirmed: true }}
        currency={PO_EDITOR_USD}
        onBack={() => undefined}
        documentInitialState={PO_DOCUMENT_NONE}
        historyInitial={PO_HISTORY}
        reconciliationInitial={{
          PurchaseOrderID: 7,
          POVersion: 1,
          Sessions: [
            {
              Generation: 1,
              SourceAvailable: true,
              RepairCorrectionID: null,
              Session: { ...PO_SAVED_RECONCILIATION_SESSION, Items: [] },
            },
          ],
        }}
      />
    ),
  },
  {
    // The attachment panel with no PDF (#472): the exact empty-state sentence
    // and Attach. At 360 this says whether the action row wraps instead of
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
    // and the copy says the prior document versions are kept.
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
    // The explained calculation (issue #470): every stage from line totals to
    // the grand total, with PPN printed separately and the source document's
    // printed figures in their own group. The whole-PO discount and the signed
    // adjustment are retired (#573). Rendered alone because the editor page
    // pushes it below the fold.
    name: "PurchaseOrderTotals",
    element: (
      <PurchaseOrderTotals
        totals={PO_EDITOR_CALCULATION.totals}
        currency={PO_EDITOR_USD}
        ppnBps={PO_EDITOR_INITIAL.ppnBps}
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
        ppnBps={PO_EDITOR_INITIAL.ppnBps}
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
        ppnBps={PO_EDITOR_INITIAL.ppnBps}
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
    // Issue #499: the free Quotation. Exactly one PO action -- Create PO from
    // this Quotation -- sits beside Copy and Edit. At 360 this case says
    // whether the three actions wrap without overflowing.
    //
    // What this case cannot show: the click that creates the PO, the real
    // clipboard write, and the page below the header (which fetches its
    // revision in an effect) stay hand-driven.
    name: "QuotationDetailHeaderFree",
    element: (
      <QuotationDetailHeader
        quotation={DETAIL_QUOTATION}
        activeRevision={2}
        revisionOptions={[
          { value: 2, label: "Revision 02 (latest)" },
          { value: 1, label: "Revision 01" },
          { value: 0, label: "Revision 00" },
        ]}
        onRevisionChange={() => undefined}
        onBack={() => undefined}
        onEdit={() => undefined}
        purchaseOrder={null}
        purchaseOrderPending={false}
        purchaseOrderAllBought={false}
        onCreatePurchaseOrder={() => undefined}
        onOpenPurchaseOrder={() => undefined}
        onCopy={() => undefined}
      />
    ),
  },
  {
    // Issue #499: a draft occupies the Quotation, so the creation action is
    // replaced by Open PO draft. The revision picker still switches the
    // displayed revision; the occupancy answer does not change with it.
    name: "QuotationDetailHeaderOpenDraft",
    element: (
      <QuotationDetailHeader
        quotation={DETAIL_QUOTATION}
        activeRevision={2}
        revisionOptions={[
          { value: 2, label: "Revision 02 (latest)" },
          { value: 1, label: "Revision 01" },
          { value: 0, label: "Revision 00" },
        ]}
        onRevisionChange={() => undefined}
        onBack={() => undefined}
        onEdit={() => undefined}
        purchaseOrder={{ id: 7, state: "draft" }}
        purchaseOrderPending={false}
        purchaseOrderAllBought={false}
        onCreatePurchaseOrder={() => undefined}
        onOpenPurchaseOrder={() => undefined}
        onCopy={() => undefined}
      />
    ),
  },
  {
    // Issue #499: the finalized case. The same occupancy becomes View PO, so
    // the screenshot proves the three labels are distinct and that neither
    // occupied state offers another creation attempt.
    name: "QuotationDetailHeaderViewPO",
    element: (
      <QuotationDetailHeader
        quotation={DETAIL_QUOTATION}
        activeRevision={2}
        revisionOptions={[
          { value: 2, label: "Revision 02 (latest)" },
          { value: 1, label: "Revision 01" },
          { value: 0, label: "Revision 00" },
        ]}
        onRevisionChange={() => undefined}
        onBack={() => undefined}
        onEdit={() => undefined}
        purchaseOrder={{ id: 7, state: "saved" }}
        purchaseOrderPending={false}
        purchaseOrderAllBought={false}
        onCreatePurchaseOrder={() => undefined}
        onOpenPurchaseOrder={() => undefined}
        onCopy={() => undefined}
      />
    ),
  },
  {
    // #656: the closed header. Create PO is gone, the Closure block names who
    // closed it and why, and an admin sees the Reopen action beside Copy/Edit.
    // This case is the layout proof for the long reason sentence at 360.
    name: "QuotationDetailHeaderClosed",
    element: (
      <QuotationDetailHeader
        quotation={DETAIL_QUOTATION}
        activeRevision={2}
        revisionOptions={[
          { value: 2, label: "Revision 02 (latest)" },
          { value: 1, label: "Revision 01" },
          { value: 0, label: "Revision 00" },
        ]}
        onRevisionChange={() => undefined}
        onBack={() => undefined}
        onEdit={() => undefined}
        purchaseOrder={null}
        purchaseOrderPending={false}
        purchaseOrderAllBought={false}
        onCreatePurchaseOrder={() => undefined}
        onOpenPurchaseOrder={() => undefined}
        onCopy={() => undefined}
        closure={{
          closed: true,
          closedAt: "2026-11-20T10:00:00+07:00",
          closedByName: "Sari Admin",
          closedReason:
            "Customer confirmed the remaining 6 Pc will not be ordered on this quotation.",
          expiredUnclosed: false,
        }}
        isAdmin
        onCloseQuotation={() => undefined}
        onReopenQuotation={() => undefined}
      />
    ),
  },
  {
    // #656: the ask-for-a-reason dialog, empty and ready. The submit is the
    // disabled button until a reason is typed; the width and the 500-character
    // counter are what this case proves.
    name: "QuotationCloseDialogOpen",
    element: (
      <QuotationCloseDialog
        open
        isAdmin
        quotationNumber="1502260012"
        pending={false}
        error={null}
        onCancel={() => undefined}
        onConfirm={() => undefined}
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
  {
    // Issue #492: the Quotation defaults surface, at both widths. A
    // representative saved set (Services with a category, a default subject)
    // shows every control the page can render, including the conditional
    // service-category field.
    name: "QuotationSettings",
    element: (
      <QuotationSettingsPage
        settings={{
          ...DEFAULT_QUOTATION_SETTINGS,
          validityDays: 45,
          paymentTermDays: 30,
          taxZone: "NonFTZ",
          taxType: "Services",
          serviceCategory: "Construction",
          subject: "Supply and installation of electrical and mechanical systems",
        }}
        onChange={() => undefined}
        onClear={() => undefined}
      />
    ),
  },
  {
    // Issue #492: the Purchase Order defaults surface. The received date is
    // filled, so the date control reads as an entered value rather than an
    // empty box.
    name: "PurchaseOrderSettings",
    element: (
      <PurchaseOrderSettingsPage
        settings={{
          ...DEFAULT_PURCHASE_ORDER_SETTINGS,
          paymentTerms: "50% advance, balance 30 days after delivery",
          incoterms: "FOB",
          receivedDate: "2026-10-01",
          listPageSize: 25,
        }}
        onChange={() => undefined}
        onClear={() => undefined}
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
function buildPage(
  name: string,
  markup: string,
  cropToHeading?: string
): string {
  const css = newestBuiltCss();
  const shell = [
    '<div id="root">',
    '  <div class="flex min-h-full flex-col bg-paper">',
    '    <main class="flex-1">',
    '      <div class="mx-auto w-full max-w-[1560px] px-5 pt-7 pb-10 lg:px-8 lg:pt-9">',
    '        <div class="flex flex-col gap-6">',
    cropToHeading === undefined ? markup : `<div id="snapshot-crop">${markup}</div>`,
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
    `<title>${name}: UI snapshot</title>`,
    "<!--",
    "  Local UI snapshot written by scripts/ui-snapshot.sh. Not committed.",
    "  The app's font stack is -apple-system / BlinkMacSystemFont / Segoe UI /",
    "  system-ui, none of which exists inside the render container. Inter, the",
    "  face the PDF template already bundles, is inlined here (base64) as the",
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
    ...(cropToHeading === undefined
      ? []
      : [
          "<script>",
          'window.addEventListener("load", () => {',
          `  const heading = Array.from(document.querySelectorAll("h2")).find((node) => node.textContent === ${JSON.stringify(cropToHeading)});`,
          '  const crop = document.getElementById("snapshot-crop");',
          "  if (heading !== undefined && crop !== null) {",
          "    crop.style.marginTop = `-${Math.max(0, heading.getBoundingClientRect().top - 16)}px`;",
          "    window.dispatchEvent(new Event(\"resize\"));",
          "  }",
          "});",
          "</script>",
        ]),
    "<!-- The scroll-linked ghost reveal is a client effect; the static",
    "     snapshot shows its settled layout so the ghost row can be reviewed. -->",
    "<script>",
    'window.addEventListener("load", () => {',
    "  document.querySelectorAll('[data-quotation-ghost]').forEach((node) => {",
    "    node.style.opacity = '1';",
    "    node.style.transform = 'none';",
    "  });",
    "});",
    "</script>",
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
      const page = buildPage(
        snapshotCase.name,
        markup,
        snapshotCase.cropToHeading
      );

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
