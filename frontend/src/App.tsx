import { useState, useCallback, useRef, useEffect, useMemo } from "react";
import {
  Prepare,
  Export,
  GetCustomerPreview,
  GetInternalBreakdown,
  GetQuotationPreviewHTML,
  ListCustomers,
  ListDrafts,
  GetDraft,
  SaveDraft,
  SetDraftContext,
  ClearDraftContext,
  SetRevisionContext,
  ClearRevisionContext,
  ListPOSourceQuotations,
  ListPOSourceRevisions,
  ValidatePOSource,
  CreatePurchaseOrderDraft,
  GetPurchaseOrder,
} from "@backend/backend/Quotation";
import { backend } from "@backend/models";
import { AppShell } from "./components/AppShell";
import { ArchivePage } from "./components/ArchivePage";
import { DraftsPage } from "./components/DraftsPage";
import {
  countDraftItems,
  parseDraftPayload,
  relativeSavedAt,
  DEFAULT_CURRENCY_CODE,
  currencyEditBlockMessage,
  normalizeCurrencyCode,
} from "./drafts";
import type { DraftSummaryDTO } from "./drafts";
import { QuotationDetailPage } from "./components/QuotationDetailPage";
import { DetailsPanel } from "./components/DetailsPanel";
import type { DetailsState } from "./components/DetailsPanel";
import { parseLeadTimeSegment } from "./components/DetailsPanel";
import { ItemEditor, newSubItem } from "./components/ItemEditor";
import type { EditorItem, EditorSubItem } from "./components/ItemEditor";
import type { CompanyUnitOption } from "./companyUnits";
import { ReviewPanel } from "./components/ReviewPanel";
import { SummaryRail } from "./components/SummaryRail";
import { Button, Card, SectionHeading, StatusBanner } from "./components/ui";
import { MAX_MARGIN_PERCENT, parseMoneyInput } from "./format";
import type { CurrencyInfo } from "./format";
import type { CustomerPreview, InternalBreakdown } from "./preview";
import { ArchivedQuotation, resolveLiveEntry } from "./archive";
import {
  TAX_TYPE_OPTIONS,
  TAX_ZONE_OPTIONS,
  legacyServiceCategory,
  legacyTaxType,
  normalizeServiceCategory,
  normalizeTaxType,
  normalizeTaxZone,
} from "./tax";
import { LoginPage } from "./components/LoginPage";
import type { SignedInUser } from "./components/LoginPage";
import { DocumentHomePage } from "./components/DocumentHomePage";
import { PurchaseOrderListPage } from "./components/PurchaseOrderListPage";
import { POQuotationSelector } from "./components/POQuotationSelector";
import type {
  POSourcePage,
  POSourceQuery,
  POSourceRevision,
  POSourceSelection,
  POSourceValidation,
} from "./poSelector";
import {
  PurchaseOrderDiscardPrompt,
  PurchaseOrderEditorPage,
} from "./components/PurchaseOrderEditor";
import { PurchaseOrderSavedView } from "./components/PurchaseOrderFinalize";
import { isSavedPurchaseOrder } from "./poFinalize";
import { usePurchaseOrderLeaveGuard, type AppPage } from "./poLeaveGuard";
import type { PurchaseOrderDTO } from "./purchaseOrder";
import { UpdateBanner, useUpdateState } from "./components/UpdateBanner";
import { UsersPage } from "./components/UsersPage";
import { ClientsPage } from "./components/ClientsPage";
import { EXPORT_DIR_KEY, SettingsPage } from "./components/SettingsPage";
import { CurrentUser, GetCompany, Logout, RestoreSession } from "@backend/backend/AuthAPI";
import { GetCompanyUnits, GetCurrencyCatalogue } from "@backend/backend/AdminAPI";
import { loadBindings, matchShortcut } from "./shortcuts";
import { ShortcutsHelp } from "./components/ShortcutsHelp";
import { WhatsNewModal, useWhatsNew } from "./components/WhatsNew";
import { useToast } from "./toast";

interface StatusState {
  type: "idle" | "loading" | "error";
  message: string;
}

const DEFAULT_COMPANY_NAME = "PT Arcon Penta Persada";

function pickCustomerId(options: { value: number }[], current: number): number {
  if (options.some((option) => option.value === current)) return current;
  if (options.some((option) => option.value === 2)) return 2;
  return options[0]?.value ?? 0;
}

// The staff unit catalog is served, no longer hardcoded here: App fetches it
// once with GetCompanyUnits() (see loadUnits below) and passes it to the
// editor as options, the same pattern the currency catalogue already uses
// (issue #336). Two copies of a list drift, and a hardcoded copy here would
// keep offering a unit a company has removed. Legacy values stored before the
// catalog (e.g. "Unit", "PC", "Meter") still display through the
// "(not recognised)" rule in UoMSelect. The canonical unit the defaults fall
// back to is served too, on the company profile. See issue #337.

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

const DEFAULT_ITEM: EditorItem = {
  key: 0,
  description: "",
  bullets: [],
  qty: "1",
  // Empty here on purpose: the served canonical unit is applied per-company
  // by newItem below. A hardcoded "Lot" would force a unit a company may have
  // removed or replaced as canonical (issue #337).
  uom: "",
  unitPrice: "",
  marginEnabled: false,
  marginPercent: "0",
  discountEnabled: false,
  discountPercent: "0",
  subItems: [],
  showSubPrices: false,
};

const DEFAULT_DETAILS: DetailsState = {
  subject: "",
  recipientName: "",
  validity: 30,
  paymentTerm: 90,
  leadTimeYears: 0,
  leadTimeMonths: 0,
  leadTimeWeeks: 0,
  leadTimeDays: 0,
  // No quotation-wide discount is offered any more: discounts are entered per
  // line. The field stays in the payload so older drafts and revisions keep
  // whatever value they were saved with.
  discountPercent: 0,
  incoterm: "EXW",
  taxZone: "FTZ",
  taxType: "Goods",
  serviceCategory: "",
};

/**
 * Builds one empty line whose unit of measure is the company's canonical
 * unit. Exported for its test: the new-line default must be the served
 * canonical, not a hardcoded "Lot" (#337) — a company may have removed that
 * unit or set a different canonical one, and a line defaulting to a unit the
 * server will refuse fails Prepare. Empty only in the pre-load window; the
 * fill-empty effect below sets it once the served list arrives.
 */
export function newItem(key: number, uom: string): EditorItem {
  return { ...DEFAULT_ITEM, key, uom };
}

function splitRows(text: string): string[] {
  const result: string[] = [];
  let current = "";
  let inQuotes = false;

  for (let i = 0; i < text.length; i++) {
    const ch = text[i];
    if (ch === '"') {
      inQuotes = !inQuotes;
    } else if (ch === "\n" && !inQuotes) {
      result.push(current);
      current = "";
    } else {
      current += ch;
    }
  }
  if (current) result.push(current);
  return result;
}

function parseRow(row: string): string[] {
  const cols: string[] = [];
  let current = "";
  let inQuotes = false;

  for (let i = 0; i < row.length; i++) {
    const ch = row[i];
    if (ch === '"') {
      inQuotes = !inQuotes;
    } else if (ch === "\t" && !inQuotes) {
      cols.push(current);
      current = "";
    } else {
      current += ch;
    }
  }
  cols.push(current);
  return cols;
}

// The IDR catalogue entry, transcribed field-for-field from
// backend/currency/currency.go. This is the offline fallback only: the live
// entry always comes from GetCurrencyCatalogue() when the binding answers,
// so a catalogue change on the Go side reaches every path except a browser
// with no Wails runtime. A single-entry fallback is not a catalogue mirror
// (the forbidden second list of exponents); it is loadCompany's keep-default
// rule for the one code every stored integer was written in.
const IDR_FALLBACK_ENTRY: CurrencyInfo = {
  code: "IDR",
  exponent: 0,
  symbol: "Rp",
  position: "before",
  groupSep: ".",
  decimalSep: ",",
  symbolSpaced: false,
};

// The one message the shortcut guards show when the live entry is unknown
// (#390). Shortcuts bypass rendering, so the same refusal has to exist where
// the keyboard can reach.
const CURRENCY_UNKNOWN_TOAST =
  "The currency list could not be loaded, so amounts cannot be shown or used. " +
  "Choose Try again to reload it.";

/**
 * Stands in for every money surface while the live profile currency cannot be
 * resolved (#390): a card naming the profile code, with a manual retry.
 *
 * A figure carrying the wrong denomination is worse than no figure. The
 * resolution used to fall back to the IDR entry, so a USD profile with a
 * failed catalogue read printed Rp over integers stored as USD minor units --
 * off by a factor of 100 in the editor and in every pasted cell. There is no
 * rate to convert with, so the honest answer is to withhold the figures and
 * name the reason.
 */
function CurrencyUnknownCard({
  code,
  onRetry,
}: {
  code: string;
  onRetry: () => void;
}) {
  return (
    <Card className="overflow-hidden">
      <div className="flex flex-col items-center gap-3 px-6 py-16 text-center">
        <span className="flex h-11 w-11 items-center justify-center rounded-lg border border-rule bg-paper-subtle text-ink-400">
          <svg width="18" height="18" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.4">
            <path d="M8 1.333l5.333 10H2.667z" />
            <path d="M8 6v2.667M8 10.667h.007" />
          </svg>
        </span>
        <p className="text-lead font-medium text-ink-900">Amounts are hidden</p>
        <p className="max-w-md text-body text-ink-500">
          The company currency is {code}, but the currency list could not be
          loaded, so Vera cannot tell which denomination these figures are in.
          Nothing is shown rather than the wrong symbol. Saved work is not
          affected.
        </p>
        <Button variant="primary" onClick={onRetry} className="mt-1">
          Try again
        </Button>
      </div>
    </Card>
  );
}

/**
 * Parses the pasted lead-time cell (column 4 of a details paste) into the
 * days segment. Pasted rows predate per-unit lead times and carry one
 * days-only figure; it restores into days exactly as old drafts do, and an
 * empty or unreadable cell reads as unstated (0), not as the old 60-day
 * default. Digits-only through the same segment parser the inputs use.
 */
export function parsePastedLeadTimeDays(cell: string | undefined): number {
  return parseLeadTimeSegment(cell ?? "");
}

/**
 * Parses one pasted money cell into the stored minor-unit digit string,
 * using the live profile currency's own separators and exponent.
 *
 * Empty stays empty so untouched cells never read as zero. A successful
 * parse stores minor units, so a USD "$1,500" stores 150000 rather than the
 * 1500 ($15.00, 100x off) the old strip-non-digits path stored. Anything
 * else stores "": the field renders empty instead of a silently wrong
 * number. In particular a genuine zero still stores "0", while garbage
 * ("abc") and a figure from another convention (IDR grouping under a USD
 * profile) are refused rather than converted, because there is no rate to
 * convert with. Never stores a wrong number for a non-empty paste.
 */
export function parsePasteMoneyCell(entry: CurrencyInfo, cell: string | undefined): string {
  const text = (cell ?? "").trim();
  if (text === "") return "";
  const minor = parseMoneyInput(entry, text);
  if (minor !== 0) return String(minor);
  let rest = text;
  if (entry.symbol !== "") rest = rest.split(entry.symbol).join("");
  rest = rest.replace(/\s/g, "");
  if (entry.groupSep !== "") rest = rest.split(entry.groupSep).join("");
  if (entry.decimalSep !== "") rest = rest.split(entry.decimalSep).join("");
  if (rest.startsWith("-") || rest.startsWith("+")) rest = rest.slice(1);
  if (/^0+$/.test(rest)) return "0";
  return "";
}

// normalizeUoM turns a pasted cell into a unit of measure, or into the
// canonical unit when it is not one.
//
// A pasted row is positional, and parseRow splits on tabs while honouring quotes,
// so a description cell containing an unquoted tab arrives as two columns and the
// tail of the description is read as the UoM. Quantity, price and margin come
// from later columns and land correctly, which is what made the row look nearly
// right while the UoM carried a sentence.
//
// Anything that is not a UoM becomes the canonical unit rather than being
// stored: the UoM control is a <select> bound to this value, so it cannot
// display a value the served list does not carry and would show a unit while
// the state held the text. That mismatch is what let the value reach a
// customer quotation unnoticed, so a value that cannot be displayed must not
// be stored either. The fallback is the company's canonical unit, not a
// hardcoded "Lot" (#337), so a paste under a company that never used "Lot"
// still lands on a unit the served list accepts.
//
// The match is case-insensitive and returns the served spelling, so "lot"
// and "LOT" both become the list's spelling of that unit.
//
// Pasted rows rarely carry the canonical spelling: plurals ("hours"), short
// forms ("hr", "mtr") and area/volume symbols ("m²", "sqm") all appear in real
// spreadsheets. The alias table folds them onto the served list rather than
// letting a real unit fall back to the canonical -- which would silently change
// what the line means.
//
// The alias spellings (the keys) are fixed; the targets are resolved against
// the served list, so a fold naming a value the company removed falls through
// to the exact match / canonical fallback instead of storing a unit off the
// list.
const UOM_ALIASES: Record<string, string> = {
  lots: "Lot",
  pc: "Pc",
  pcs: "Pc",
  piece: "Pc",
  pieces: "Pc",
  unit: "Pc",
  units: "Pc",
  ls: "LS",
  lump: "LS",
  set: "set",
  sets: "set",
  m: "meter",
  mtr: "meter",
  mtrs: "meter",
  meters: "meter",
  metre: "meter",
  metres: "meter",
  m2: "m2",
  "m²": "m2",
  sqm: "m2",
  sqmeters: "m2",
  sqmeter: "m2",
  m3: "m3",
  "m³": "m3",
  cbm: "m3",
  cum: "m3",
  hr: "Hour",
  hrs: "Hour",
  hours: "Hour",
  days: "Day",
  weeks: "Week",
  months: "Month",
  years: "Year",
  quarters: "Quarter",
};

/**
 * Folds one pasted UoM cell onto the served unit list, or onto the canonical
 * unit when it is not a unit the company uses. Exported for its test: the
 * fallback must be the served canonical, never a hardcoded "Lot" (#337), and
 * an alias target must resolve against the served list so a removed value
 * never gets stored.
 */
export function normalizeUoM(
  raw: string,
  options: readonly { value: string; label: string }[],
  canonical: string
): string {
  const candidate = raw.trim();
  if (candidate === "") return canonical;

  const key = candidate.toLowerCase();

  const aliased = UOM_ALIASES[key];
  if (aliased) {
    // Resolve the alias target against the served list, so a fold naming a
    // value the company removed falls through rather than storing it.
    const resolved = options.find(
      (option) => option.value.toLowerCase() === aliased.toLowerCase()
    );
    if (resolved) return resolved.value;
  }

  const match = options.find((option) => key === option.value.toLowerCase());
  return match ? match.value : canonical;
}

function parseSubItems(text: string): { description: string; subItems: string[] } {
  const lines = text.split("\n").filter((line) => line.trim());
  const mainParts: string[] = [];
  const bullets: string[] = [];

  for (const line of lines) {
    const trimmed = line.trimStart();
    if (trimmed.startsWith("- ") || trimmed.startsWith("* ")) {
      bullets.push(trimmed.slice(2).trim());
    } else {
      mainParts.push(trimmed);
    }
  }

  return { description: mainParts.join(" "), subItems: bullets };
}

function buildDescription(description: string, bullets: string[]): string {
  const parts = [description];
  for (const line of bullets) {
    parts.push("- " + line);
  }
  return parts.join("\n");
}

function getErrorMessage(error: unknown, fallback: string): string {
  if (typeof error === "string" && error.trim()) return error;
  if (error instanceof Error && error.message) return error.message;
  if (typeof error === "object" && error !== null && "message" in error) {
    const message = (error as { message?: unknown }).message;
    if (typeof message === "string" && message.trim()) return message;
  }
  return fallback;
}

function App() {
  // The document home (#466) is the landing page after sign-in and session
  // restore: "home" holds the four destinations, "po" the honest Purchase
  // Order placeholder until the entry workspace lands. The Quotation pages
  // ("create", "drafts", "archive", "detail") keep their existing meanings.
  const [page, setPage] = useState<
    "home" | "po" | "create" | "drafts" | "archive" | "detail" | "users" | "clients" | "settings"
  >("home");
  // #468 wave 2: the PO editor owns its reducer state, so a tab that unmounts
  // it loses unsaved work. The editor lifts only its dirty flag; this gate
  // asks through the same discard panel before any navigation leaves it.
  const [selectedQuotation, setSelectedQuotation] = useState<{
    id: number;
    revision: number;
  } | null>(null);
  // The PO source selector (#467) is shared by both PO entry paths.
  const [poSelectorOpen, setPOSelectorOpen] = useState(false);
  // The draft the PO workspace is editing (#468). Null means the PO page is
  // the archive (#469): the selector's two paths create or reopen a draft and
  // set this.
  const [poDraft, setPODraft] = useState<PurchaseOrderDTO | null>(null);
  // #473: a saved record's Correct action opens the same editor in correction
  // mode, which requires a reason and keeps the replaced values.
  const [poCorrecting, setPOCorrecting] = useState(false);
  // Leaving the PO page clears the edited draft, so returning to the PO surface
  // shows the archive rather than a stale editor. The leave guard owns the
  // dirty-check before this runs.
  const navigatePage = useCallback((next: AppPage) => {
    if (next !== "po") {
      setPODraft(null);
      setPOCorrecting(false);
    }
    setPage(next);
  }, []);
  const poLeave = usePurchaseOrderLeaveGuard(page, navigatePage);
  const [editingQuote, setEditingQuote] = useState<{
    number: string;
    revision: number;
  } | null>(null);

  const nextKey = useRef(1);
  const nextSubId = useRef(1);
  // "" until the served list arrives; the fill-empty effect below sets the
  // canonical unit on this first line once GetCompanyUnits() answers.
  const [items, setItems] = useState<EditorItem[]>([newItem(0, "")]);
  const [details, setDetails] = useState<DetailsState>(DEFAULT_DETAILS);
  const [customerId, setCustomerId] = useState(2);
  const [customerOptions, setCustomerOptions] = useState<{ value: number; label: string }[]>([]);
  const [status, setStatus] = useState<StatusState>({ type: "idle", message: "" });
  const [confirmingNew, setConfirmingNew] = useState(false);
  const newConfirmTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [customerPreview, setCustomerPreview] = useState<CustomerPreview | null>(null);
  const [breakdown, setBreakdown] = useState<InternalBreakdown | null>(null);
  const [quotationHTML, setQuotationHTML] = useState<string | null>(null);
  // The draft the editor is currently working from, 0 when this is not a
  // resumed draft. Saved drafts are overwritten rather than duplicated, and
  // the id travels with the export so the server can consume it.
  const [activeDraftId, setActiveDraftId] = useState(0);
  const [draftSavedAt, setDraftSavedAt] = useState<string | null>(null);
  const [savingDraft, setSavingDraft] = useState(false);
  const [draftCount, setDraftCount] = useState(0);
  const { pushToast } = useToast();
  // #469: a row opens the exact stored draft by id through the #468 route.
  // The list already knows the id; the server is the authority on the record,
  // so it is fetched rather than passed in from the row.
  const openPurchaseOrder = useCallback(
    async (id: number) => {
      try {
        const draft = await GetPurchaseOrder(id);
        poLeave.setDirty(false);
        setPOCorrecting(false);
        setPODraft(draft);
        setPage("po");
      } catch (error) {
        pushToast(
          "error",
          getErrorMessage(error, "The purchase order could not be opened.")
        );
      }
    },
    [poLeave, pushToast]
  );
  const [user, setUser] = useState<SignedInUser | null>(null);
  const [checkingSession, setCheckingSession] = useState(true);
  const [companyName, setCompanyName] = useState(DEFAULT_COMPANY_NAME);
  // The live company currency: the denomination fresh input is written in,
  // and what a stored draft or revision is compared against before it may be
  // opened for editing.
  const [companyCurrency, setCompanyCurrency] = useState(DEFAULT_CURRENCY_CODE);
  // The shipped catalogue for resolving the code above to a full entry. Null
  // until the binding answers (or when it throws in plain-browser dev); every
  // consumer below reads liveEntry, never this, so the fallback stays in one
  // place.
  const [currencyCatalogue, setCurrencyCatalogue] = useState<CurrencyInfo[] | null>(null);
  // The company's served unit list, in display order, as the editor dropdown
  // sees it (issue #337). Null until GetCompanyUnits() answers (or when it
  // throws / the device is offline): the UoM selects render a disabled pending
  // state rather than a stale or invented list, and an empty array is never
  // treated as "no units" — a company always has at least one.
  const [storedUoMs, setStoredUoMs] = useState<CompanyUnitOption[] | null>(null);
  // True when that read failed, so the pending state can say so honestly (and
  // offer a retry) instead of loading forever.
  const [unitsError, setUnitsError] = useState(false);
  // The canonical unit from the company profile: the value a new line,
  // sub-item, split-line parent and unrecognized paste fall back to. The
  // server seeds it to "Lot" and a company may change it to any unit on the
  // served list, so nothing here hardcodes a unit.
  const [canonicalUoM, setCanonicalUoM] = useState("");

  const signedIn = user !== null;

  // Release check runs once per launch after sign-in; the banner covers
  // every page while Settings owns the manual re-check.
  const { state: updateState, dismiss: dismissUpdate } = useUpdateState(signedIn);
  const { notes: whatsNew, dismiss: dismissWhatsNew } = useWhatsNew(signedIn);

  // Company identity tracks the stored profile; the baked-in default shows
  // while loading or when the server is unreachable — never blank.
  const loadCompany = useCallback(async () => {
    try {
      const profile = (await GetCompany()) as backend.CompanyProfile;
      if (profile?.LegalName?.trim()) setCompanyName(profile.LegalName.trim());
      setCompanyCurrency(normalizeCurrencyCode(profile?.Currency));
      // The served canonical unit. Trimmed: the server stores it byte for byte
      // against a unit value (issue #337). Empty only in the pre-load window.
      setCanonicalUoM((profile?.CanonicalUoM ?? "").trim());
    } catch {
      // Offline or plain-browser dev: keep the default.
    }
  }, []);

  useEffect(() => {
    if (signedIn) void loadCompany();
    else {
      setCompanyName(DEFAULT_COMPANY_NAME);
      setCompanyCurrency(DEFAULT_CURRENCY_CODE);
      setCanonicalUoM("");
    }
  }, [signedIn, loadCompany]);

  // The catalogue is static, so one fetch per launch is enough. It rides its
  // own try/catch rather than loadCompany's: a catalogue failure must not
  // clear the company name, and the gates below keep every consumer from
  // rendering a figure while the entry is unknown. The failure path keeps the
  // previous value -- a loaded catalogue is never nulled by a later failure
  // (#390) -- and there is no automatic retry: the binding reads compiled-in
  // memory with no error channel, so there is no transient to retry away. The
  // card's Try again calls this by hand.
  const loadCatalogue = useCallback(async () => {
    try {
      const catalogue = (await GetCurrencyCatalogue()) as CurrencyInfo[];
      setCurrencyCatalogue(catalogue);
    } catch {
      setCurrencyCatalogue((prev) => prev);
    }
  }, []);

  useEffect(() => {
    if (!signedIn) {
      setCurrencyCatalogue(null);
      return;
    }
    let active = true;
    void loadCatalogue().then(() => {
      // A sign-out mid-flight must not leave a catalogue behind on the login
      // page, the same race the inline fetch always covered.
      if (!active) setCurrencyCatalogue(null);
    });
    return () => {
      active = false;
    };
  }, [signedIn, loadCatalogue]);

  // The served unit list (#337). Read once per sign-in, like the catalogue:
  // the list is company data the editor dropdown, the canonical default and
  // the paste fallback all draw from. It rides its own try/catch so a units
  // failure never clears the currency, and it keeps a loaded list on a later
  // failure rather than nulling it (the same keep-previous rule the catalogue
  // uses). Unlike the catalogue it does not gate money: a failed read leaves
  // the selects disabled pending with a retry (see unitsError), because a
  // stale unit is a label, not a denomination.
  const loadUnits = useCallback(async () => {
    try {
      const units = (await GetCompanyUnits()) as { Value: string; Label: string }[];
      setStoredUoMs((units ?? []).map((unit) => ({ value: unit.Value, label: unit.Label })));
      setUnitsError(false);
    } catch {
      setUnitsError(true);
    }
  }, []);

  useEffect(() => {
    if (!signedIn) {
      setStoredUoMs(null);
      setUnitsError(false);
      return;
    }
    void loadUnits();
  }, [signedIn, loadUnits]);

  // The live profile currency, resolved once from the catalogue by the
  // normalized profile code. Null means the entry cannot be known -- the
  // catalogue is absent and the profile is not IDR -- and every money surface
  // below gates on it (#390). The IDR fallback is the one honest default:
  // every pre-currency integer was written in IDR. Never `?? IDR_FALLBACK_ENTRY`
  // on a non-IDR code: that printed Rp over USD minor units.
  const liveEntry: CurrencyInfo | null = useMemo(
    () => resolveLiveEntry(currencyCatalogue, companyCurrency, IDR_FALLBACK_ENTRY),
    [currencyCatalogue, companyCurrency]
  );

  // The PO's own currency, resolved from its snapshot rather than the live
  // profile (#390's rule applied to the new surface): an IDR default over a
  // USD PO would be wrong by a factor of 100. Null gates the editor's money
  // fields until the catalogue can name the denomination.
  const poEntry = useMemo(
    () =>
      poDraft === null
        ? null
        : resolveLiveEntry(currencyCatalogue, poDraft.Currency, IDR_FALLBACK_ENTRY),
    [currencyCatalogue, poDraft]
  );

  // The resolved canonical unit every default falls back to: the profile's
  // canonical when it is on the served list, otherwise the first served unit —
  // never a value off the list, and never a hardcoded default (#337). Empty
  // only before either read lands; the profile value rides the loading window
  // so a line added right after sign-in still names the company's unit.
  const canonicalUnit = useMemo(() => {
    if (storedUoMs === null) return canonicalUoM;
    if (canonicalUoM !== "" && storedUoMs.some((unit) => unit.value === canonicalUoM)) {
      return canonicalUoM;
    }
    return storedUoMs[0]?.value ?? "";
  }, [canonicalUoM, storedUoMs]);

  // A line created before the served list arrived carries an empty unit (the
  // pre-load default). When the list lands, fill every empty unit with the
  // canonical so the dropdown reads a real unit rather than "(not recognised)".
  // Only empty units are touched: a staff-entered or restored value is never
  // overwritten.
  useEffect(() => {
    if (storedUoMs === null) return;
    const fallback = canonicalUnit;
    if (fallback === "") return;
    setItems((prev) => {
      const empty = prev.some(
        (item) => item.uom === "" || item.subItems.some((sub) => sub.uom === "")
      );
      if (!empty) return prev;
      return prev.map((item) => ({
        ...item,
        uom: item.uom === "" ? fallback : item.uom,
        subItems: item.subItems.map((sub) =>
          sub.uom === "" ? { ...sub, uom: fallback } : sub
        ),
      }));
    });
  }, [storedUoMs, canonicalUnit]);

  // Restore the keychain session on startup; otherwise show the login page.
  // The try/catch also covers plain-browser dev, where the Wails runtime
  // (window.go) does not exist and the binding throws synchronously.
  useEffect(() => {
    try {
      RestoreSession()
        .then((restored) => setUser(restored as SignedInUser))
        .catch(() => setUser(null))
        .finally(() => setCheckingSession(false));
    } catch {
      setUser(null);
      setCheckingSession(false);
    }
  }, []);

  const clearPreview = useCallback(() => {
    setCustomerPreview(null);
    setBreakdown(null);
    setQuotationHTML(null);
  }, []);

  const resetForm = useCallback(() => {
    nextKey.current = 1;
    nextSubId.current = 1;
    setItems([newItem(0, canonicalUnit)]);
    setDetails(DEFAULT_DETAILS);
    setCustomerId((current) => pickCustomerId(customerOptions, current));
    setEditingQuote(null);
    // The editor is no longer a resumed draft, so the next export must not try
    // to consume a draft that has nothing to do with this quotation.
    setActiveDraftId(0);
    setDraftSavedAt(null);
    void ClearDraftContext().catch(() => undefined);
    clearPreview();
    setStatus({ type: "idle", message: "" });
  }, [clearPreview, customerOptions, canonicalUnit]);

  // Server-owned client directory: the editor dropdown reads here, and new
  // clients appear without a rebuild. Prepare validates server-side.
  const loadCustomers = useCallback(async () => {
    try {
      const rows = (await ListCustomers()) as backend.ManagedClient[];
      const options = rows.map((row) => ({ value: row.Code, label: row.Name }));
      setCustomerOptions(options);
      setCustomerId((current) => pickCustomerId(options, current));
    } catch {
      // Keep previous options; the server rejects unknown codes on Prepare.
    }
  }, []);

  useEffect(() => {
    if (signedIn) void loadCustomers();
  }, [signedIn, loadCustomers]);


  const handleSignOut = useCallback(async () => {
    try {
      await Logout();
    } catch {
      // Session may already be invalid; sign out locally regardless.
    } finally {
      setUser(null);
      resetForm();
      setPage("home");
    }
  }, [resetForm]);

  // Revalidate the session while signed in so server-side revocation
  // (deactivation, approved deletion, password change) signs the device out
  // within about a minute. Network errors are ignored: an offline laptop
  // waking up must not be signed out by a failed poll.
  useEffect(() => {
    if (!signedIn) return;
    const timer = window.setInterval(() => {
      CurrentUser()
        .then((restored) => setUser(restored as SignedInUser))
        .catch((err: unknown) => {
          const message = typeof err === "string" ? err : "";
          if (message.startsWith("reach the quotation server")) return;
          setUser(null);
          resetForm();
          setPage("home");
          pushToast(
            "error",
            "You've been signed out. Contact your administrator if this was unexpected."
          );
        });
    }, 60000);
    return () => window.clearInterval(timer);
  }, [signedIn, resetForm, pushToast]);

  const handleNewQuotation = useCallback(() => {
    void ClearRevisionContext().catch(() => undefined);
    if (newConfirmTimer.current) clearTimeout(newConfirmTimer.current);
    setConfirmingNew(false);
    resetForm();
    setPage("create");
  }, [resetForm]);

  // A draft count for the nav badge. Best-effort: a failure here must never
  // block the page the user asked for.
  const refreshDraftCount = useCallback(async () => {
    try {
      const rows = (await ListDrafts()) as DraftSummaryDTO[];
      setDraftCount(rows?.length ?? 0);
    } catch {
      /* the badge is decoration; leave the last known count alone */
    }
  }, []);

  // Once on sign-in, to give the badge a number. Re-run only when the Drafts
  // tab is opened or the editor hands a draft over: refreshing on every page
  // change would be a round trip the user cannot see the point of.
  useEffect(() => {
    if (!signedIn) {
      setDraftCount(0);
      return;
    }
    void refreshDraftCount();
  }, [signedIn, refreshDraftCount]);

  useEffect(() => {
    if (signedIn && page === "drafts") void refreshDraftCount();
  }, [signedIn, page, refreshDraftCount]);

  const handleSaveDraft = useCallback(async () => {
    setSavingDraft(true);
    try {
      const customerName =
        customerOptions.find((option) => option.value === customerId)?.label ?? "";
      const id = await SaveDraft(
        new backend.SavedDraft({
          Label: details.subject.trim(),
          Subject: details.subject.trim(),
          CustomerName: customerName,
          Payload: JSON.stringify({ items, details, customerId }),
          ID: activeDraftId,
          ItemCount: countDraftItems(items),
          CustomerID: customerId,
        })
      );
      setActiveDraftId(id);
      setDraftSavedAt(new Date().toISOString());
      void SetDraftContext(id).catch(() => undefined);
      setDraftCount((count) => (activeDraftId === 0 ? count + 1 : count));
      pushToast(
        "success",
        activeDraftId === 0
          ? "Draft saved. Find it under Drafts whenever you need it."
          : "Draft updated."
      );
    } catch (error: unknown) {
      pushToast(
        "error",
        getErrorMessage(error, "We couldn't save this draft. Try again.")
      );
    } finally {
      setSavingDraft(false);
    }
  }, [items, details, customerId, customerOptions, activeDraftId, pushToast]);

  const handleOpenDraft = useCallback(
    async (id: number) => {
      try {
        const draft = (await GetDraft(id)) as backend.DraftDTO;
        // A draft is integers in the currency it was saved in. Opening it
        // under a different company currency would silently reinterpret
        // every amount, so the edit is blocked before the editor is touched.
        const blocked = currencyEditBlockMessage(draft.Currency, companyCurrency, {
          kind: "draft",
          name: draft.Label || draft.Subject,
        });
        if (blocked) {
          pushToast("error", blocked);
          return;
        }
        const restored = parseDraftPayload(draft.Payload);
        if (!restored) {
          pushToast(
            "error",
            "This draft couldn't be read, so nothing was overwritten. Delete it and start again."
          );
          return;
        }
        void ClearRevisionContext().catch(() => undefined);
        // Reissue the row keys from the restored list so the editor's React
        // identity never collides with keys left over from another quotation.
        // Sub-item ids only have to be unique within their own line, but the
        // counter that hands out new ones is global -- so it has to start past
        // every id already in use, not past the number of lines.
        nextKey.current = restored.items.length;
        nextSubId.current =
          restored.items.reduce(
            (highest, item) =>
              Math.max(
                highest,
                ...item.subItems.map((sub) => sub.id)
              ),
            0
          ) + 1;
        setItems(restored.items);
        setDetails(restored.details);
        setCustomerId(
          restored.customerId ||
            draft.CustomerID ||
            pickCustomerId(customerOptions, customerId)
        );
        setEditingQuote(null);
        setActiveDraftId(draft.ID ?? id);
        await SetDraftContext(id).catch(() => undefined);
        setDraftSavedAt(draft.UpdatedAt ?? null);
        clearPreview();
        setStatus({ type: "idle", message: "" });
        setPage("create");
        pushToast("info", "Draft opened. Save again to update it.");
      } catch (error: unknown) {
        // Logged as well as shown: a swallowed reason here once hid a shape
        // mismatch between the binding and the parser for a whole debugging
        // session. The editor is untouched either way -- failing before
        // setItems is the point.
        console.error("open draft failed", error);
        pushToast("error", "We couldn't open that draft. Try again.");
      }
    },
    [clearPreview, companyCurrency, customerId, customerOptions, pushToast]
  );

  useEffect(
    () => () => {
      if (newConfirmTimer.current) clearTimeout(newConfirmTimer.current);
    },
    []
  );

  const handleOpenQuotation = useCallback((id: number, revision: number) => {
    setSelectedQuotation({ id, revision });
    setPage("detail");
  }, []);

  const handleEditQuotation = useCallback(
    async (quotation: ArchivedQuotation) => {
      // A revision is integers in the currency it was issued in. Editing it
      // under a different company currency would silently reinterpret every
      // amount onto the next revision, so the edit is blocked before the
      // editor is touched.
      const blocked = currencyEditBlockMessage(quotation.Currency, companyCurrency, {
        kind: "revision",
        quoteNumber: quotation.QuoteNumber,
        revision: quotation.RevisionNumber,
      });
      if (blocked) {
        pushToast("error", blocked);
        return;
      }
      try {
        await SetRevisionContext(
          quotation.QuoteNumber,
          quotation.RevisionNumber + 1
        );
        nextKey.current = quotation.Items.length;
        nextSubId.current = 1;
        setItems(
          quotation.Items.map((item, index) => {
            const parsed = parseSubItems(item.Description);
            const subs = (item.Subs ?? []).map((sub) => ({
              ...newSubItem(nextSubId.current++, sub.Description),
              qty: String(sub.Qty),
              uom: sub.UoM,
              unitCost: String(sub.OriginalUnitPrice),
              marginEnabled: sub.MarginEnabled,
              marginPercent: String(sub.MarginPercent),
              discountEnabled: sub.DiscountEnabled ?? false,
              discountPercent: String(sub.DiscountPercent ?? 0),
              bullets: [...(sub.Details ?? [])],
            }));
            return {
              key: index,
              description: parsed.description,
              bullets: parsed.subItems,
              qty: String(item.Qty),
              uom: item.UoM,
              unitPrice: String(item.UnitPrice),
              marginEnabled: item.MarginEnabled,
              marginPercent: String(item.MarginPercent),
              // An old revision has no per-row discount, which reads as "this
              // row carries none" -- so there is no legacy editor mode.
              discountEnabled: item.DiscountEnabled ?? false,
              discountPercent: String(item.DiscountPercent ?? 0),
              subItems: subs,
              showSubPrices: item.ShowSubPrices ?? false,
            };
          })
        );
        setDetails({
          subject: quotation.Subject,
          recipientName: quotation.RecipientName,
          validity: quotation.ValidityDays,
          paymentTerm: quotation.PaymentTermDays,
          leadTimeYears: quotation.LeadTimeYears,
          leadTimeMonths: quotation.LeadTimeMonths,
          leadTimeWeeks: quotation.LeadTimeWeeks,
          leadTimeDays: quotation.LeadTimeDays,
          discountPercent: quotation.DiscountPercent,
          incoterm: quotation.Incoterm,
          taxZone: quotation.TaxZone,
          taxType: quotation.TaxType || legacyTaxType(quotation.TaxCategory),
          serviceCategory:
            quotation.ServiceCategory || legacyServiceCategory(quotation.TaxCategory),
        });
        setCustomerId(quotation.CustomerCode);
        setEditingQuote({
          number: quotation.QuoteNumber,
          revision: quotation.RevisionNumber + 1,
        });
        // A revision is never a draft: the saved draft that may exist for this
        // editor is unrelated work and must not be consumed by the export.
        setActiveDraftId(0);
        setDraftSavedAt(null);
        void ClearDraftContext().catch(() => undefined);
        clearPreview();
        setStatus({ type: "idle", message: "" });
        setPage("create");
        pushToast(
          "info",
          `Editing ${quotation.QuoteNumber}. Exporting will create revision ${String(
            quotation.RevisionNumber + 1
          ).padStart(2, "0")}.`
        );
      } catch {
        pushToast("error", "We couldn't start editing this quotation. Try again.");
      }
    },
    [clearPreview, companyCurrency, pushToast]
  );

  // Any input edit invalidates the review: the user must re-Prepare.
  useEffect(() => {
    clearPreview();
    setStatus((current) => (current.type === "idle" ? current : { type: "idle", message: "" }));
  }, [items, details, customerId, clearPreview]);

  const handleAddItem = useCallback(() => {
    setItems((prev) => [...prev, newItem(nextKey.current++, canonicalUnit)]);
  }, [canonicalUnit]);

  const handleRemoveItem = useCallback((key: number) => {
    setItems((prev) => {
      if (prev.length <= 1) return prev;
      return prev.filter((item) => item.key !== key);
    });
  }, []);

  const handleItemChange = useCallback(
    (key: number, field: keyof EditorItem, value: string | string[] | boolean) => {
      setItems((prev) =>
        prev.map((item) => (item.key === key ? { ...item, [field]: value } : item))
      );
    },
    []
  );

  const handleAddBullet = useCallback((key: number) => {
    setItems((prev) =>
      prev.map((item) =>
        item.key === key ? { ...item, bullets: [...item.bullets, ""] } : item
      )
    );
  }, []);

  const handleRemoveBullet = useCallback(
    (key: number, bulletIndex: number) => {
      setItems((prev) =>
        prev.map((item) =>
          item.key === key
            ? {
                ...item,
                bullets: item.bullets.filter((_, i) => i !== bulletIndex),
              }
            : item
        )
      );
    },
    []
  );

  const handleBulletChange = useCallback(
    (key: number, bulletIndex: number, value: string) => {
      setItems((prev) =>
        prev.map((item) =>
          item.key === key
            ? {
                ...item,
                bullets: item.bullets.map((line, i) =>
                  i === bulletIndex ? value : line
                ),
              }
            : item
        )
      );
    },
    []
  );

  // Entering sub-item mode converts the line: existing description bullets
  // become sub-items with their text pre-filled, and the main item takes
  // the rollup shape (Qty 1, the canonical unit) before the user prices
  // anything. The parent and each new sub-item carry the served canonical
  // unit (#337), not a hardcoded "Lot", so the backend's package gate — which
  // requires the parent to be the company's canonical unit — accepts it and a
  // company that renamed the unit sees their own.
  const handleEnableSubItems = useCallback((key: number) => {
    setItems((prev) =>
      prev.map((item) => {
        if (item.key !== key) return item;
        const converted = item.bullets
          .filter((line) => line.trim() !== "")
          .map((line) => newSubItem(nextSubId.current++, line.trim(), canonicalUnit));
        return {
          ...item,
          bullets: [],
          qty: "1",
          uom: canonicalUnit,
          showSubPrices: false,
          subItems:
            converted.length > 0
              ? converted
              : [newSubItem(nextSubId.current++, "", canonicalUnit)],
        };
      })
    );
  }, [canonicalUnit]);

  // Leaving sub-item mode keeps the composition readable as description
  // lines; the per-sub values go away with the pricing they described.
  const handleDisableSubItems = useCallback((key: number) => {
    setItems((prev) =>
      prev.map((item) => {
        if (item.key !== key) return item;
        const lines = item.subItems
          .map((sub) => sub.description.trim())
          .filter((line) => line !== "");
        return { ...item, bullets: lines, subItems: [], showSubPrices: false };
      })
    );
  }, []);

  const handleAddSubRow = useCallback((key: number) => {
    setItems((prev) =>
      prev.map((item) =>
        item.key === key
          ? {
              ...item,
              subItems: [...item.subItems, newSubItem(nextSubId.current++, "", canonicalUnit)],
            }
          : item
      )
    );
  }, [canonicalUnit]);

  const handleRemoveSubRow = useCallback((key: number, subId: number) => {
    setItems((prev) =>
      prev.map((item) =>
        item.key === key
          ? {
              ...item,
              subItems: item.subItems.filter((sub) => sub.id !== subId),
            }
          : item
      )
    );
  }, []);

  const handlePatchSubRow = useCallback(
    (key: number, subId: number, patch: Partial<EditorSubItem>) => {
      setItems((prev) =>
        prev.map((item) =>
          item.key === key
            ? {
                ...item,
                subItems: item.subItems.map((sub) =>
                  sub.id === subId ? { ...sub, ...patch } : sub
                ),
              }
            : item
        )
      );
    },
    []
  );

  const handleMoveSubRow = useCallback(
    (key: number, subId: number, direction: -1 | 1) => {
      setItems((prev) =>
        prev.map((item) => {
          if (item.key !== key) return item;
          const at = item.subItems.findIndex((sub) => sub.id === subId);
          const target = at + direction;
          if (at === -1 || target < 0 || target >= item.subItems.length) {
            return item;
          }
          const next = [...item.subItems];
          [next[at], next[target]] = [next[target], next[at]];
          return { ...item, subItems: next };
        })
      );
    },
    []
  );

  const handleAddSubBullet = useCallback((key: number, subId: number) => {
    setItems((prev) =>
      prev.map((item) =>
        item.key === key
          ? {
              ...item,
              subItems: item.subItems.map((sub) =>
                sub.id === subId ? { ...sub, bullets: [...sub.bullets, ""] } : sub
              ),
            }
          : item
      )
    );
  }, []);

  const handleRemoveSubBullet = useCallback(
    (key: number, subId: number, bulletIndex: number) => {
      setItems((prev) =>
        prev.map((item) =>
          item.key === key
            ? {
                ...item,
                subItems: item.subItems.map((sub) =>
                  sub.id === subId
                    ? {
                        ...sub,
                        bullets: sub.bullets.filter((_, i) => i !== bulletIndex),
                      }
                    : sub
                ),
              }
            : item
        )
      );
    },
    []
  );

  const handleSubBulletChange = useCallback(
    (key: number, subId: number, bulletIndex: number, value: string) => {
      setItems((prev) =>
        prev.map((item) =>
          item.key === key
            ? {
                ...item,
                subItems: item.subItems.map((sub) =>
                  sub.id === subId
                    ? {
                        ...sub,
                        bullets: sub.bullets.map((line, i) =>
                          i === bulletIndex ? value : line
                        ),
                      }
                    : sub
                ),
              }
            : item
        )
      );
    },
    []
  );

  // Bulk entry: a block pasted from Excel becomes one sub-item per row.
  // Columns are positional, matching the header the panel shows.
  const handleSubRowPaste = useCallback(
    (key: number, event: React.ClipboardEvent<HTMLTextAreaElement>) => {
      // #390: no money is parsed under an entry that cannot be known. The
      // editor is unmounted while liveEntry is null, so this is unreachable
      // through the UI and exists to keep the type honest.
      if (liveEntry === null) return;
      const raw = event.clipboardData.getData("text");
      if (!raw.includes("\t") && !raw.includes("\n")) return;
      event.preventDefault();
      const rows = splitRows(raw)
        .filter((row) => row.trim())
        .map(parseRow);
      const parsed = rows
        .map((cols) => {
          const margin = Math.min(
            Number.parseInt((cols[4] || "").replace(/\D/g, ""), 10) || 0,
            MAX_MARGIN_PERCENT
          );
          return {
            ...newSubItem(nextSubId.current++, "", canonicalUnit),
            description: (cols[0] || "").trim(),
            qty: (cols[1] || "").trim() || "1",
            uom: normalizeUoM(cols[2] || "", storedUoMs ?? [], canonicalUnit),
            unitCost: parsePasteMoneyCell(liveEntry, cols[3]),
            marginPercent: margin.toString(),
            marginEnabled: margin > 0,
          };
        })
        .filter((sub) => sub.description !== "");
      if (parsed.length === 0) return;
      setItems((prev) =>
        prev.map((item) =>
          item.key === key
            ? { ...item, subItems: [...item.subItems, ...parsed] }
            : item
        )
      );
    },
    [liveEntry, storedUoMs, canonicalUnit]
  );

  const handlePaste = useCallback(
    (targetKey: number, event: React.ClipboardEvent<HTMLInputElement>) => {
      // #390: the same guard as the sub-row paste -- an unknown entry is
      // never the entry a pasted figure is parsed against.
      if (liveEntry === null) return;
      const raw = event.clipboardData.getData("text");
      if (!raw.includes("\t")) return;

      event.preventDefault();
      const rows = splitRows(raw)
        .filter((row) => row.trim())
        .map(parseRow);

      setItems((prev) => {
        const targetIndex = prev.findIndex((item) => item.key === targetKey);
        if (targetIndex === -1) return prev;

        const parsed = rows.map((cols) => {
          const { description, subItems: bullets } = parseSubItems(cols[0] || "");
          const margin = Math.min(
            Number.parseInt((cols[4] || "").replace(/\D/g, ""), 10) || 0,
            MAX_MARGIN_PERCENT
          );
          return {
            ...DEFAULT_ITEM,
            key: 0,
            description,
            bullets,
            uom: normalizeUoM(cols[1] || "", storedUoMs ?? [], canonicalUnit),
            qty: (cols[2] || "").trim() || "1",
            unitPrice: parsePasteMoneyCell(liveEntry, cols[3]),
            marginPercent: margin.toString(),
            marginEnabled: margin > 0,
          };
        });

        const first = { ...parsed[0], key: prev[targetIndex].key };
        const rest = parsed.slice(1).map((parsedItem, i) => ({
          ...parsedItem,
          key: nextKey.current + i,
        }));
        nextKey.current += rest.length;

        return [
          ...prev.slice(0, targetIndex),
          first,
          ...rest,
          ...prev.slice(targetIndex + 1),
        ];
      });
    },
    [liveEntry, storedUoMs, canonicalUnit]
  );

  const handlePrepare = useCallback(async () => {
    // #390: a keyboard shortcut bypasses the rendered gate, so the refusal is
    // repeated here. Preparing under an unknown entry would store figures the
    // editor cannot even display.
    if (liveEntry === null) {
      pushToast("error", CURRENCY_UNKNOWN_TOAST);
      return;
    }
    setStatus({ type: "loading", message: "Validating quotation…" });

    const lineItems = items
      .filter(
        (item) =>
          item.description.trim() !== "" ||
          item.bullets.some((line) => line.trim() !== "") ||
          item.subItems.some((sub) => sub.description.trim() !== "")
      )
      .map((item) => {
        if (item.subItems.length === 0) {
          return new backend.QuotationItemInput({
            Description: buildDescription(item.description, item.bullets),
            Qty: Number.parseInt(item.qty, 10) || 0,
            UoM: item.uom,
            UnitPrice: Number.parseInt(item.unitPrice, 10) || 0,
            MarginEnabled: item.marginEnabled,
            MarginPercent: Number.parseInt(item.marginPercent, 10) || 0,
            DiscountEnabled: item.discountEnabled,
            DiscountPercent: Number.parseInt(item.discountPercent, 10) || 0,
            SubItems: [],
            ShowSubPrices: false,
          });
        }
        // A main item with sub-items never prices itself: Qty 1 / the
        // canonical unit and a zero unit price are placeholders the backend
        // replaces with the sub-item rollup, so the margin controls do not
        // apply either. The unit is the served canonical (#337): the backend
        // package gate requires it to match the company's canonical, and a
        // hardcoded "Lot" would be refused for a company that removed it.
        return new backend.QuotationItemInput({
          Description: item.description,
          Qty: 1,
          UoM: canonicalUnit,
          UnitPrice: 0,
          MarginEnabled: false,
          MarginPercent: 0,
          DiscountEnabled: false,
          DiscountPercent: 0,
          ShowSubPrices: item.showSubPrices,
          SubItems: item.subItems.map(
            (sub) =>
              new backend.QuotationSubItemInput({
                Description: sub.description,
                Qty: Number.parseInt(sub.qty, 10) || 0,
                UoM: sub.uom,
                UnitCost: Number.parseInt(sub.unitCost, 10) || 0,
                MarginEnabled: sub.marginEnabled,
                MarginPercent: Number.parseInt(sub.marginPercent, 10) || 0,
                DiscountEnabled: sub.discountEnabled,
                DiscountPercent: Number.parseInt(sub.discountPercent, 10) || 0,
                SubSubItems: sub.bullets
                  .filter((line) => line.trim() !== "")
                  .map(
                    (line) =>
                      new backend.QuotationSubSubItemInput({
                        Description: line,
                      })
                  ),
              })
          ),
        });
      });

    const input = new backend.QuotationInput({
      Items: lineItems,
      Subject: details.subject,
      RecipientName: details.recipientName,
      DiscountPercent: details.discountPercent,
      TaxZone: details.taxZone,
      TaxType: details.taxType,
      ServiceCategory:
        details.taxType === "Services" ? details.serviceCategory : "",
      TaxCategory: "",
      Validity: details.validity,
      PaymentTerm: details.paymentTerm,
      LeadTimeYears: details.leadTimeYears,
      LeadTimeMonths: details.leadTimeMonths,
      LeadTimeWeeks: details.leadTimeWeeks,
      LeadTimeDays: details.leadTimeDays,
      CustomerID: customerId,
      Incoterm: details.incoterm,
    });

    try {
      await Prepare(input);
      const [customer, internal, previewHTML] = await Promise.all([
        GetCustomerPreview(),
        GetInternalBreakdown(),
        GetQuotationPreviewHTML(),
      ]);
      setCustomerPreview(customer as CustomerPreview);
      setBreakdown(internal as InternalBreakdown);
      setQuotationHTML(String(previewHTML));
      setStatus({ type: "idle", message: "" });
      pushToast("success", "Quotation prepared. Review the totals before exporting.");
    } catch (error: unknown) {
      clearPreview();
      const message = getErrorMessage(
        error,
        "We couldn't prepare the quotation. Check the form and try again."
      );
      setStatus({ type: "error", message });
      pushToast("error", message);
    }
  }, [items, details, customerId, clearPreview, pushToast, liveEntry, canonicalUnit]);

  const handlePasteDetails = useCallback(
    (event: React.ClipboardEvent<HTMLInputElement>) => {
      const raw = event.clipboardData.getData("text");
      if (!raw.includes("\t")) return;
      event.preventDefault();
      const rows = splitRows(raw)
        .filter((row) => row.trim())
        .map(parseRow);
      if (rows.length === 0) return;
      const cols = rows[0];
      const pastedZone = (cols[8] || "").trim();
      const pastedType = (cols[9] || "").trim();
      const pastedServiceCategory = (cols[10] || "").trim();

      setDetails((current) => {
        const next: DetailsState = {
          ...current,
          subject: (cols[0] || "").trim(),
          recipientName: (cols[1] || "").trim(),
          validity: Number.parseInt(cols[2], 10) || 30,
          paymentTerm: Number.parseInt(cols[3], 10) || 90,
          leadTimeYears: 0,
          leadTimeMonths: 0,
          leadTimeWeeks: 0,
          leadTimeDays: parsePastedLeadTimeDays(cols[4]),
          incoterm: (cols[7] || "").trim() || "EXW",
        };

        // Only touch the tax fields when the pasted row actually carries them,
        // so a plain details paste leaves the current selection alone.
        // Columns: 8 = zone, 9 = type (Goods/Services), 10 = service category.
        if (pastedZone !== "") {
          next.taxZone = normalizeTaxZone(pastedZone);
        }
        if (pastedType !== "") {
          next.taxType = normalizeTaxType(pastedType);
          next.serviceCategory = "";
        }
        if (next.taxType === "Services" && pastedServiceCategory !== "") {
          next.serviceCategory = normalizeServiceCategory(pastedServiceCategory);
        }

        return next;
      });
      setCustomerId(Number.parseInt(cols[6], 10) || 2);
    },
    []
  );

  const handleExport = useCallback(async () => {
    // #390: the export shortcut bypasses the rendered gate exactly as prepare
    // does, and a signed revision cannot be unsent.
    if (liveEntry === null) {
      pushToast("error", CURRENCY_UNKNOWN_TOAST);
      return;
    }
    setStatus({ type: "loading", message: "Rendering PDF…" });
    try {
      // Device-local export default from Settings; blank means OS default.
      const fromDraft = activeDraftId > 0;
      await Export(window.localStorage.getItem(EXPORT_DIR_KEY) ?? "");
      clearPreview();
      void ClearRevisionContext().catch(() => undefined);
      void ClearDraftContext().catch(() => undefined);
      setEditingQuote(null);
      // The server consumed the draft with the export. Locally it is gone too,
      // and the badge comes down with it.
      setActiveDraftId(0);
      setDraftSavedAt(null);
      if (fromDraft) setDraftCount((count) => Math.max(0, count - 1));
      setStatus({ type: "idle", message: "" });
      pushToast("success", "Quotation exported.");
    } catch (error: unknown) {
      const message = getErrorMessage(
        error,
        "We couldn't export the quotation. Check the save location and try again."
      );
      if (message.includes("user cancelled") || message.includes("cancelled")) {
        setStatus({ type: "idle", message: "" });
        pushToast("info", "Export cancelled. Your quotation is still ready.");
        return;
      }
      clearPreview();
      setStatus({ type: "error", message });
      pushToast("error", message);
    }
  }, [activeDraftId, clearPreview, pushToast, liveEntry]);

  const ready = customerPreview !== null && breakdown !== null && quotationHTML !== null;
  const hasRenderableItems = items.some(
    (item) =>
      item.description.trim() !== "" ||
      item.bullets.some((line) => line.trim() !== "") ||
      item.subItems.some((sub) => sub.description.trim() !== "")
  );
  const dirty =
    editingQuote !== null ||
    details.subject.trim() !== "" ||
    details.recipientName.trim() !== "" ||
    items.some(
      (item) =>
        item.description.trim() !== "" ||
        item.unitPrice.trim() !== "" ||
        item.bullets.some((line) => line.trim() !== "") ||
        item.subItems.some((sub) => sub.description.trim() !== "")
    );
  const loading = status.type === "loading";

  const requestNewQuotation = () => {
    if (!dirty) {
      handleNewQuotation();
      return;
    }
    setConfirmingNew(true);
    if (newConfirmTimer.current) clearTimeout(newConfirmTimer.current);
    newConfirmTimer.current = setTimeout(() => setConfirmingNew(false), 6000);
  };

  // Global shortcuts: mod+S export, mod+Enter prepare, mod+N new, Alt+1-5
  // pages, ? overlay. Bare keys never fire while typing (the registry
  // enforces that); grid/archive scopes own their containers. Esc layers:
  // overlay, discard confirm, then detail back to archive.
  const [showHelp, setShowHelp] = useState(false);
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      // The PO source selector owns the keyboard while it is open: its dialog
      // handles Escape and contains Tab, and no global shortcut may act on
      // the page behind the modal.
      if (poSelectorOpen) return;
      // The tab-navigation discard prompt is modal too: Escape is Keep
      // editing, and no shortcut may navigate behind it.
      if (poLeave.pendingPage !== null) {
        if (event.key === "Escape") poLeave.keepEditing();
        return;
      }
      if (event.key === "Escape" && !event.metaKey && !event.ctrlKey && !event.altKey) {
        const inGrid = (event.target as HTMLElement).closest?.("[data-grid-cell]");
        if (inGrid) return;
        if (whatsNew.length > 0) {
          dismissWhatsNew();
          return;
        }
        if (showHelp) {
          setShowHelp(false);
          return;
        }
        if (confirmingNew) {
          if (newConfirmTimer.current) clearTimeout(newConfirmTimer.current);
          setConfirmingNew(false);
          return;
        }
        if (page === "detail") {
          setPage("archive");
          return;
        }
        return;
      }
      const action = matchShortcut(event, loadBindings(), "global");
      if (!action) return;
      switch (action) {
        case "exportPdf":
          if (page !== "create") return;
          event.preventDefault();
          void handleExport();
          return;
        case "prepare":
          if (page !== "create") return;
          event.preventDefault();
          void handlePrepare();
          return;
        case "newQuotation":
          event.preventDefault();
          // On a dirty PO this asks first; once allowed, the Editor page is
          // shown before the existing new-quotation confirmation can render
          // in its action bar.
          poLeave.request("create", () => {
            setPage("create");
            requestNewQuotation();
          });
          return;
        case "gotoEditor":
          poLeave.request("create");
          return;
        case "gotoArchive":
          poLeave.request("archive");
          return;
        case "gotoClients":
          if (user?.Role === "admin") poLeave.request("clients");
          return;
        case "gotoTeam":
          if (user?.Role === "admin") poLeave.request("users");
          return;
        case "gotoSettings":
          poLeave.request("settings");
          return;
        case "toggleHelp":
          setShowHelp((visible) => !visible);
          return;
        default:
          return;
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [page, user, showHelp, confirmingNew, handleExport, handlePrepare, requestNewQuotation, whatsNew, dismissWhatsNew, poSelectorOpen, poLeave.request, poLeave.pendingPage, poLeave.keepEditing]);

  // PO source selector loaders (#467). The modal owns the three steps; these
  // callbacks are its only transport, so the component stays free of hidden
  // API assumptions and both entry paths reuse the same server data.
  const loadPOSourcePage = useCallback(
    async (query: POSourceQuery): Promise<POSourcePage> => {
      const page = await ListPOSourceQuotations({
        Search: query.search,
        Page: query.page,
        HideUsed: query.hideUsed,
      });
      return {
        quotations: page.Quotations.map((row) => ({
          id: row.ID,
          quoteNumber: row.QuoteNumber,
          subject: row.Subject,
          customerName: row.CustomerName,
          latestRevision: row.LatestRevision,
          issuedRevisionCount: row.IssuedRevisionCount,
          activePO:
            row.ActivePO === null || row.ActivePO === undefined
              ? null
              : {
                  id: row.ActivePO.ID,
                  state: row.ActivePO.State === "saved" ? "saved" : "draft",
                },
        })),
        page: page.Page,
        pageSize: page.PageSize,
        total: page.Total,
        totalPages: page.TotalPages,
      };
    },
    []
  );

  const loadPOSourceRevisions = useCallback(
    async (quotationId: number): Promise<POSourceRevision[]> => {
      const response = await ListPOSourceRevisions(quotationId);
      return response.Revisions.map((revision) => ({
        revision: revision.Revision,
        issuedAt: revision.IssuedAt,
      }));
    },
    []
  );

  const validatePOSourceSelection = useCallback(
    async (selection: POSourceSelection): Promise<POSourceValidation> => {
      const result = await ValidatePOSource({
        QuotationID: selection.quotationId,
        RevisionNumber: selection.revisionNumber,
      });
      return { valid: result.Valid, reason: result.Reason };
    },
    []
  );

  return (
    <>
      {showHelp && user !== null ? (
        <ShortcutsHelp onClose={() => setShowHelp(false)} />
      ) : null}
      {user !== null ? (
        <WhatsNewModal notes={whatsNew} onDismiss={dismissWhatsNew} />
      ) : null}
      <POQuotationSelector
        open={poSelectorOpen}
        onClose={() => setPOSelectorOpen(false)}
        onConfirm={async (selection) => {
          const draft = await CreatePurchaseOrderDraft({
            QuotationID: selection.quotationId,
            RevisionNumber: selection.revisionNumber,
            ReceivedDate: "",
          });
          poLeave.setDirty(false);
          setPOCorrecting(false);
          setPODraft(draft);
          setPOSelectorOpen(false);
          setPage("po");
          pushToast(
            "success",
            `Purchase order draft created from quotation ${draft.QuoteNumber}, revision ${draft.SourceRevision}.`
          );
        }}
        onOpenDraft={async (activePO) => {
          try {
            const draft = await GetPurchaseOrder(activePO.id);
            poLeave.setDirty(false);
            setPOCorrecting(false);
            setPODraft(draft);
            setPOSelectorOpen(false);
            setPage("po");
          } catch (error) {
            pushToast(
              "error",
              getErrorMessage(error, "The purchase order draft could not be opened.")
            );
          }
        }}
        loadPage={loadPOSourcePage}
        loadRevisions={loadPOSourceRevisions}
        validate={validatePOSourceSelection}
      />
      {checkingSession ? (
        <div className="flex min-h-full items-center justify-center px-5">
          <div className="h-8 w-48 animate-pulse rounded-md bg-paper-muted" aria-label="Checking session" />
        </div>
      ) : user === null ? (
        <LoginPage onSignedIn={setUser} />
      ) : (
    <>
      {updateState?.UpdateAvailable ? (
        <UpdateBanner state={updateState} onDismiss={dismissUpdate} />
      ) : null}
      <AppShell
      page={page}
      inert={poLeave.pendingPage !== null}
      onOpenHome={() => poLeave.request("home")}
      onOpenEditor={() => poLeave.request("create")}
      onOpenDrafts={() => poLeave.request("drafts")}
      onOpenArchive={() => poLeave.request("archive")}
      draftCount={draftCount}
      onOpenUsers={() => poLeave.request("users")}
      onOpenClients={() => poLeave.request("clients")}
      onOpenSettings={() => poLeave.request("settings")}
      showUsersTab={user?.Role === "admin"}
      showClientsTab={user?.Role === "admin"}
      companyName={companyName}
      user={user}
      onSignOut={() => void handleSignOut()}
      actionBar={
        page === "create" ? (
          <>
            <div className="flex min-w-0 items-center gap-2">
              {confirmingNew ? (
                <span className="flex items-center gap-2">
                  <span className="text-caption text-ink-600">
                    Discard this quotation and start a new one?
                  </span>
                  <Button variant="danger" size="sm" onClick={handleNewQuotation}>
                    Discard
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => {
                      if (newConfirmTimer.current) clearTimeout(newConfirmTimer.current);
                      setConfirmingNew(false);
                    }}
                  >
                    Keep editing
                  </Button>
                </span>
              ) : editingQuote ? (
                <span className="truncate text-caption text-ink-500">
                  Editing{" "}
                  <span className="tnum font-medium text-ink-800">
                    {editingQuote.number}
                  </span>{" "}
                  · next revision{" "}
                  <span className="tnum font-medium text-ink-800">
                    {String(editingQuote.revision).padStart(2, "0")}
                  </span>
                </span>
              ) : draftSavedAt ? (
                <span className="truncate text-caption text-ink-500">
                  Draft saved{" "}
                  <span className="text-ink-400">
                    {relativeSavedAt(draftSavedAt)} — save again to update it
                  </span>
                </span>
              ) : (
                <span className="text-caption text-ink-500">
                  {ready
                    ? "Review the three stages, then export."
                    : "Enter line items and quotation details."}
                </span>
              )}
            </div>
            <div className="ml-auto flex items-center gap-2.5">
              <Button
                variant="ghost"
                onClick={requestNewQuotation}
                disabled={loading}
              >
                New
              </Button>
              <Button
                variant="secondary"
                onClick={handleSaveDraft}
                disabled={loading || savingDraft || !dirty}
                title={
                  dirty
                    ? "Save this quotation to come back to it later"
                    : "Nothing to save yet"
                }
                leading={
                  <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
                    <path d="M2.667 2.667h8.666L13.333 4.667V12a1.333 1.333 0 01-1.333 1.333H4A1.333 1.333 0 012.667 12V2.667z" />
                    <path d="M5.333 2.667v3.333h5.333V2.667M5.333 13.333V9.333h5.333v4" />
                  </svg>
                }
              >
                {activeDraftId > 0 ? "Update draft" : "Save draft"}
              </Button>
              <Button
                variant="secondary"
                onClick={handlePrepare}
                disabled={loading || !hasRenderableItems}
                leading={
                  <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
                    <path d="M1.333 12.167V2.833a1 1 0 011-1h11.334a1 1 0 011 1v9.334a1 1 0 01-1 1H2.333a1 1 0 01-1-1z" />
                    <path d="M4.667 6.667L7.333 4l2.667 2.667M7.333 4v8" />
                  </svg>
                }
              >
                {ready ? "Prepare again" : "Prepare"}
              </Button>
              <Button
                variant="primary"
                onClick={handleExport}
                disabled={!ready || loading}
                leading={
                  <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
                    <path d="M2.667 11.333v1.334A1.333 1.333 0 004 14h8a1.333 1.333 0 001.333-1.333v-1.334M11.333 8L8 11.333 4.667 8M8 11.333V2" />
                  </svg>
                }
              >
                Export PDF
              </Button>
            </div>
          </>
        ) : null
      }
    >
      {page === "home" ? (
        <DocumentHomePage
          onOpenQuotation={() => setPage("create")}
          onOpenPurchaseOrder={() => setPage("po")}
        />
      ) : null}

      {page === "po" ? (
        poDraft === null ? (
          <PurchaseOrderListPage
            onAdd={() => setPOSelectorOpen(true)}
            onOpen={(id) => void openPurchaseOrder(id)}
          />
        ) : poEntry === null ? (
          <StatusBanner tone="danger">
            The purchase order's currency ({poDraft.Currency}) is not in the
            loaded currency catalogue, so money cannot be edited safely yet.
          </StatusBanner>
        ) : isSavedPurchaseOrder(poDraft) && !poCorrecting ? (
          // #471/#473: a saved record opens read-only; Correct is the one entry
          // into the correction editor, which asks for a reason and records the
          // replaced values.
          <PurchaseOrderSavedView
            order={poDraft}
            currency={poEntry}
            onCorrect={() => setPOCorrecting(true)}
            onAssigned={(updated) => setPODraft(updated)}
            onBack={() => {
              poLeave.setDirty(false);
              setPODraft(null);
              setPOCorrecting(false);
            }}
          />
        ) : (
          <PurchaseOrderEditorPage
            key={poDraft.ID}
            order={poDraft}
            currency={poEntry}
            uomOptions={storedUoMs ?? []}
            incotermOptions={INCOTERM_OPTIONS}
            mode={poCorrecting ? "correction" : "draft"}
            onDirtyChange={poLeave.setDirty}
            onSaved={(updated) => {
              setPODraft(updated);
              const corrected = poCorrecting;
              setPOCorrecting(false);
              pushToast(
                "success",
                corrected
                  ? "Purchase order corrected. The previous values are in the history."
                  : updated.State === "saved"
                    ? "Purchase order saved and finalized."
                    : "Purchase order draft saved."
              );
            }}
            onCancel={() => {
              // Cancel is the same discard decision as a tab click: the guard
              // owns the one dialog, and this cleanup runs only once it is
              // allowed. A correction returns to the saved view; a draft Cancel
              // returns to the archive after the guard allows it.
              if (poCorrecting) {
                poLeave.request("po", () => {
                  poLeave.setDirty(false);
                  setPOCorrecting(false);
                });
                return;
              }
              poLeave.request("home", () => {
                poLeave.setDirty(false);
                setPODraft(null);
              });
            }}
          />
        )
      ) : null}

      {page === "create" ? (
        <div className="flex flex-col gap-6">
          {status.type !== "idle" ? (
            <StatusBanner tone={status.type === "error" ? "danger" : "info"}>
              {loading ? (
                <span className="inline-flex items-center gap-2">
                  <span className="h-2.5 w-2.5 animate-pulse rounded-full bg-accent-500" />
                  {status.message}
                </span>
              ) : (
                status.message
              )}
            </StatusBanner>
          ) : null}

          <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_320px]">
            <div className="flex min-w-0 flex-col gap-6">
              <DetailsPanel
                value={details}
                customerId={customerId}
                customerOptions={customerOptions}
                incotermOptions={INCOTERM_OPTIONS}
                taxZoneOptions={TAX_ZONE_OPTIONS}
                taxTypeOptions={TAX_TYPE_OPTIONS}
                onChange={(patch) => setDetails((current) => ({ ...current, ...patch }))}
                onCustomerChange={setCustomerId}
                onPaste={handlePasteDetails}
              />

              {/* #390: no line-item pricing while the live entry is unknown.
                  Removing this gate reintroduces Rp over USD minor units. */}
              {liveEntry === null ? (
                <CurrencyUnknownCard
                  code={companyCurrency}
                  onRetry={() => void loadCatalogue()}
                />
              ) : (
                <div>
                  {/* #337: the served unit list failed to load. Say so and
                      offer a retry rather than leaving the selects disabled
                      on a pending label that would never resolve. */}
                  {unitsError && storedUoMs === null ? (
                    <div className="mb-4">
                      <StatusBanner tone="danger">
                        <span className="flex flex-wrap items-center gap-x-3 gap-y-2">
                          <span>
                            The unit list couldn't be loaded, so units can't be
                            chosen yet. New lines use the company's canonical unit.
                          </span>
                          <Button
                            variant="secondary"
                            size="sm"
                            onClick={() => void loadUnits()}
                          >
                            Try again
                          </Button>
                        </span>
                      </StatusBanner>
                    </div>
                  ) : null}
                  <SectionHeading
                    title="Line items"
                    action={
                      <span className="text-caption text-ink-500">
                        Toggle margin per line
                      </span>
                    }
                  />
                  <ItemEditor
                    items={items}
                    currency={liveEntry}
                    uomOptions={storedUoMs ?? []}
                    uomPendingLabel={
                      storedUoMs === null
                        ? unitsError
                          ? "Units unavailable"
                          : "Loading units…"
                        : undefined
                    }
                    onAddItem={handleAddItem}
                    onRemoveItem={handleRemoveItem}
                    onChange={handleItemChange}
                    onAddBullet={handleAddBullet}
                    onRemoveBullet={handleRemoveBullet}
                    onBulletChange={handleBulletChange}
                    onEnableSubItems={handleEnableSubItems}
                    onDisableSubItems={handleDisableSubItems}
                    onAddSubRow={handleAddSubRow}
                    onRemoveSubRow={handleRemoveSubRow}
                    onPatchSubRow={handlePatchSubRow}
                    onMoveSubRow={handleMoveSubRow}
                    onPasteSubRows={handleSubRowPaste}
                    onAddSubBullet={handleAddSubBullet}
                    onRemoveSubBullet={handleRemoveSubBullet}
                    onSubBulletChange={handleSubBulletChange}
                    onPasteItems={handlePaste}
                  />
                </div>
              )}
            </div>

            {/* #390: the rail prices what is typed above it; with no entry it
                would price in the wrong denomination. */}
            {liveEntry === null ? null : (
              <div className="xl:sticky xl:top-[80px] xl:self-start">
                <SummaryRail
                  items={items}
                  currency={liveEntry}
                  discountPercent={details.discountPercent}
                  taxZone={details.taxZone}
                  taxType={details.taxType}
                  serviceCategory={details.serviceCategory}
                />
              </div>
            )}
          </div>

          {/* #390: the review carries the customer preview in the live
              currency, margin and nett included. */}
          {ready && liveEntry !== null ? (
            <ReviewPanel
              customer={customerPreview}
              currency={liveEntry}
              internal={breakdown}
              quotationHTML={quotationHTML}
            />
          ) : null}
        </div>
      ) : null}

      {page === "drafts" ? (
        <DraftsPage
          onOpenDraft={handleOpenDraft}
          onNewQuotation={handleNewQuotation}
        />
      ) : null}

      {page === "archive" ? (
        // #390: every row's grand total is live-currency chrome until the
        // catalogue loads, so nothing renders while the entry is unknown.
        liveEntry === null ? (
          <CurrencyUnknownCard
            code={companyCurrency}
            onRetry={() => void loadCatalogue()}
          />
        ) : (
          // The currency prop is the initial-render fallback only: ArchivePage
          // fetches the catalogue itself and resolves each row's REVISION
          // currency in-page (row.Currency, filled by the server and plumbed
          // through archive.go). The live entry covers the window before the
          // catalogue loads, never a row's steady state.
          <ArchivePage
            currency={liveEntry}
            onNewQuotation={handleNewQuotation}
            onOpenQuotation={handleOpenQuotation}
          />
        )
      ) : null}

      {page === "detail" && selectedQuotation !== null ? (
        // #390: the detail page shows margin, tax and nett in the live
        // currency until the revision's own is resolved.
        liveEntry === null ? (
          <CurrencyUnknownCard
            code={companyCurrency}
            onRetry={() => void loadCatalogue()}
          />
        ) : (
          // Same fallback shape as ArchivePage: QuotationDetailPage fetches the
          // catalogue itself and resolves the revision's own currency
          // (quotation.Currency) in-page once GetQuotationRevision returns. The
          // live entry is only the initial render's currency until then.
          <QuotationDetailPage
            quotationID={selectedQuotation.id}
            currency={liveEntry}
            revision={selectedQuotation.revision}
            onBack={() => setPage("archive")}
            onEdit={handleEditQuotation}
            onAddPurchaseOrder={() => setPOSelectorOpen(true)}
          />
        )
      ) : null}

      {page === "users" && user?.Role === "admin" ? <UsersPage /> : null}

      {page === "clients" && user?.Role === "admin" ? <ClientsPage /> : null}

      {page === "settings" && user !== null ? (
        <SettingsPage
          isAdmin={user.Role === "admin"}
          user={user}
          onProfileChanged={setUser}
          onCompanyChanged={() => {
            // A company save (name, currency, canonical unit, or the unit list
            // itself) reaches the editor through here: re-read the profile and
            // the served list so the dropdown and the canonical default track
            // what was just saved.
            void loadCompany();
            void loadUnits();
          }}
        />
      ) : null}
    </AppShell>
    {poLeave.pendingPage !== null ? (
      <PurchaseOrderDiscardPrompt
        onKeepEditing={poLeave.keepEditing}
        onDiscard={poLeave.discard}
        returnFocus={poLeave.pendingReturnFocus}
      />
    ) : null}
    </>
      )}
    </>
  );
}

export default App;
