import { useState, useCallback, useRef, useEffect } from "react";
import {
  Prepare,
  Export,
  GetCustomerPreview,
  GetInternalBreakdown,
  GetQuotationPreviewHTML,
  ListCustomers,
  SetRevisionContext,
  ClearRevisionContext,
} from "@backend/backend/Quotation";
import { backend } from "@backend/models";
import { AppShell } from "./components/AppShell";
import { ArchivePage } from "./components/ArchivePage";
import { QuotationDetailPage } from "./components/QuotationDetailPage";
import { DetailsPanel } from "./components/DetailsPanel";
import type { DetailsState } from "./components/DetailsPanel";
import { ItemEditor } from "./components/ItemEditor";
import type { EditorItem } from "./components/ItemEditor";
import { ReviewPanel } from "./components/ReviewPanel";
import { SummaryRail } from "./components/SummaryRail";
import { Button, SectionHeading, StatusBanner } from "./components/ui";
import type { CustomerPreview, InternalBreakdown } from "./preview";
import type { ArchivedQuotation } from "./archive";
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
import { UpdateBanner, useUpdateState } from "./components/UpdateBanner";
import { UsersPage } from "./components/UsersPage";
import { ClientsPage } from "./components/ClientsPage";
import { EXPORT_DIR_KEY, SettingsPage } from "./components/SettingsPage";
import { CurrentUser, GetCompany, Logout, RestoreSession } from "@backend/backend/AuthAPI";
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

const UOM_OPTIONS = [
  { value: "Lot", label: "Lot" },
  { value: "PC", label: "PC" },
  { value: "EA", label: "EA" },
  { value: "Set", label: "Set" },
  { value: "Meter", label: "Meter" },
] as const;

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
  subItems: [],
  qty: "1",
  uom: "Lot",
  unitPrice: "",
  marginEnabled: false,
  marginPercent: "0",
};

const DEFAULT_DETAILS: DetailsState = {
  subject: "",
  recipientName: "",
  validity: 30,
  paymentTerm: 90,
  leadTime: 60,
  discountPercent: 5,
  incoterm: "EXW",
  taxZone: "FTZ",
  taxType: "Goods",
  serviceCategory: "",
};

function newItem(key: number): EditorItem {
  return { ...DEFAULT_ITEM, key };
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

// normalizeUoM turns a pasted cell into a unit of measure, or into the default
// when it is not one.
//
// A pasted row is positional, and parseRow splits on tabs while honouring quotes,
// so a description cell containing an unquoted tab arrives as two columns and the
// tail of the description is read as the UoM. Quantity, price and margin come
// from later columns and land correctly, which is what made the row look nearly
// right while the UoM carried a sentence.
//
// Anything that is not a UoM becomes "Lot" rather than being stored: the UoM
// control is a <select> bound to this value, so it cannot display a value
// outside UOM_OPTIONS and would show "Lot" while the state held the text. That
// mismatch is what let the value reach a customer quotation unnoticed, so a
// value that cannot be displayed must not be stored either.
//
// The match is case-insensitive and returns the canonical spelling, so "lot"
// and "LOT" both become "Lot".
function normalizeUoM(raw: string): string {
  const candidate = raw.trim();
  if (candidate === "") return "Lot";

  const match = UOM_OPTIONS.find(
    (option) => option.value.toLowerCase() === candidate.toLowerCase()
  );
  return match ? match.value : "Lot";
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

function buildDescription(description: string, subItems: string[]): string {
  const parts = [description];
  for (const sub of subItems) {
    parts.push("- " + sub);
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
  const [page, setPage] = useState<"create" | "archive" | "detail" | "users" | "clients" | "settings">("create");
  const [selectedQuotation, setSelectedQuotation] = useState<{
    id: number;
    revision: number;
  } | null>(null);
  const [editingQuote, setEditingQuote] = useState<{
    number: string;
    revision: number;
  } | null>(null);

  const nextKey = useRef(1);
  const [items, setItems] = useState<EditorItem[]>([newItem(0)]);
  const [details, setDetails] = useState<DetailsState>(DEFAULT_DETAILS);
  const [customerId, setCustomerId] = useState(2);
  const [customerOptions, setCustomerOptions] = useState<{ value: number; label: string }[]>([]);
  const [status, setStatus] = useState<StatusState>({ type: "idle", message: "" });
  const [confirmingNew, setConfirmingNew] = useState(false);
  const newConfirmTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [customerPreview, setCustomerPreview] = useState<CustomerPreview | null>(null);
  const [breakdown, setBreakdown] = useState<InternalBreakdown | null>(null);
  const [quotationHTML, setQuotationHTML] = useState<string | null>(null);
  const { pushToast } = useToast();
  const [user, setUser] = useState<SignedInUser | null>(null);
  const [checkingSession, setCheckingSession] = useState(true);
  const [companyName, setCompanyName] = useState(DEFAULT_COMPANY_NAME);

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
    } catch {
      // Offline or plain-browser dev: keep the default.
    }
  }, []);

  useEffect(() => {
    if (signedIn) void loadCompany();
    else setCompanyName(DEFAULT_COMPANY_NAME);
  }, [signedIn, loadCompany]);

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
    setItems([newItem(0)]);
    setDetails(DEFAULT_DETAILS);
    setCustomerId((current) => pickCustomerId(customerOptions, current));
    setEditingQuote(null);
    clearPreview();
    setStatus({ type: "idle", message: "" });
  }, [clearPreview, customerOptions]);

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
      setPage("create");
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
          setPage("create");
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
      try {
        await SetRevisionContext(
          quotation.QuoteNumber,
          quotation.RevisionNumber + 1
        );
        nextKey.current = quotation.Items.length;
        setItems(
          quotation.Items.map((item, index) => {
            const parsed = parseSubItems(item.Description);
            return {
              key: index,
              description: parsed.description,
              subItems: parsed.subItems,
              qty: String(item.Qty),
              uom: item.UoM,
              unitPrice: String(item.UnitPrice),
              marginEnabled: item.MarginEnabled,
              marginPercent: String(item.MarginPercent),
            };
          })
        );
        setDetails({
          subject: quotation.Subject,
          recipientName: quotation.RecipientName,
          validity: quotation.ValidityDays,
          paymentTerm: quotation.PaymentTermDays,
          leadTime: quotation.LeadTimeDays,
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
    [clearPreview, pushToast]
  );

  // Any input edit invalidates the review: the user must re-Prepare.
  useEffect(() => {
    clearPreview();
    setStatus((current) => (current.type === "idle" ? current : { type: "idle", message: "" }));
  }, [items, details, customerId, clearPreview]);

  const handleAddItem = useCallback(() => {
    setItems((prev) => [...prev, newItem(nextKey.current++)]);
  }, []);

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

  const handleAddSubItem = useCallback((key: number) => {
    setItems((prev) =>
      prev.map((item) =>
        item.key === key ? { ...item, subItems: [...item.subItems, ""] } : item
      )
    );
  }, []);

  const handleRemoveSubItem = useCallback((key: number, subIndex: number) => {
    setItems((prev) =>
      prev.map((item) =>
        item.key === key
          ? { ...item, subItems: item.subItems.filter((_, i) => i !== subIndex) }
          : item
      )
    );
  }, []);

  const handleSubItemChange = useCallback(
    (key: number, subIndex: number, value: string) => {
      setItems((prev) =>
        prev.map((item) =>
          item.key === key
            ? {
                ...item,
                subItems: item.subItems.map((s, i) => (i === subIndex ? value : s)),
              }
            : item
        )
      );
    },
    []
  );

  const handlePaste = useCallback(
    (targetKey: number, event: React.ClipboardEvent<HTMLInputElement>) => {
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
          const { description, subItems } = parseSubItems(cols[0] || "");
          const margin = Math.min(
            Number.parseInt((cols[4] || "").replace(/\D/g, ""), 10) || 0,
            100
          );
          return {
            ...DEFAULT_ITEM,
            key: 0,
            description,
            subItems,
            uom: normalizeUoM(cols[1] || ""),
            qty: (cols[2] || "").trim() || "1",
            unitPrice: (cols[3] || "").replace(/\D/g, "").trim(),
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
    []
  );

  const handlePrepare = useCallback(async () => {
    setStatus({ type: "loading", message: "Validating quotation…" });

    const lineItems = items
      .filter(
        (item) =>
          item.description.trim() !== "" || item.subItems.some((sub) => sub.trim())
      )
      .map(
        (item) =>
          new backend.QuotationItemInput({
            Description: buildDescription(item.description, item.subItems),
            Qty: Number.parseInt(item.qty, 10) || 0,
            UoM: item.uom,
            UnitPrice: Number.parseInt(item.unitPrice, 10) || 0,
            MarginEnabled: item.marginEnabled,
            MarginPercent: Number.parseInt(item.marginPercent, 10) || 0,
          })
      );

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
      LeadTime: details.leadTime,
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
  }, [items, details, customerId, clearPreview, pushToast]);

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
          leadTime: Number.parseInt(cols[4], 10) || 60,
          discountPercent: Math.min(Number.parseInt(cols[5], 10) || 5, 100),
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
    setStatus({ type: "loading", message: "Rendering PDF…" });
    try {
      // Device-local export default from Settings; blank means OS default.
      await Export(window.localStorage.getItem(EXPORT_DIR_KEY) ?? "");
      clearPreview();
      void ClearRevisionContext().catch(() => undefined);
      setEditingQuote(null);
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
  }, [clearPreview, pushToast]);

  const ready = customerPreview !== null && breakdown !== null && quotationHTML !== null;
  const hasRenderableItems = items.some(
    (item) => item.description.trim() !== "" || item.subItems.some((sub) => sub.trim())
  );
  const dirty =
    editingQuote !== null ||
    details.subject.trim() !== "" ||
    details.recipientName.trim() !== "" ||
    items.some(
      (item) =>
        item.description.trim() !== "" ||
        item.unitPrice.trim() !== "" ||
        item.subItems.some((sub) => sub.trim() !== "")
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
          requestNewQuotation();
          return;
        case "gotoEditor":
          setPage("create");
          return;
        case "gotoArchive":
          setPage("archive");
          return;
        case "gotoClients":
          if (user?.Role === "admin") setPage("clients");
          return;
        case "gotoTeam":
          if (user?.Role === "admin") setPage("users");
          return;
        case "gotoSettings":
          setPage("settings");
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
  }, [page, user, showHelp, confirmingNew, handleExport, handlePrepare, requestNewQuotation, whatsNew, dismissWhatsNew]);

  return (
    <>
      {showHelp && user !== null ? (
        <ShortcutsHelp onClose={() => setShowHelp(false)} />
      ) : null}
      {user !== null ? (
        <WhatsNewModal notes={whatsNew} onDismiss={dismissWhatsNew} />
      ) : null}
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
      onOpenEditor={() => setPage("create")}
      onOpenArchive={() => setPage("archive")}
      onOpenUsers={() => setPage("users")}
      onOpenClients={() => setPage("clients")}
      onOpenSettings={() => setPage("settings")}
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

              <div>
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
                  uomOptions={UOM_OPTIONS}
                  onAddItem={handleAddItem}
                  onRemoveItem={handleRemoveItem}
                  onChange={handleItemChange}
                  onAddSubItem={handleAddSubItem}
                  onRemoveSubItem={handleRemoveSubItem}
                  onSubItemChange={handleSubItemChange}
                  onPasteItems={handlePaste}
                />
              </div>
            </div>

            <div className="xl:sticky xl:top-[80px] xl:self-start">
              <SummaryRail
                items={items}
                discountPercent={details.discountPercent}
                taxZone={details.taxZone}
                taxType={details.taxType}
                serviceCategory={details.serviceCategory}
              />
            </div>
          </div>

          {ready ? (
            <ReviewPanel
              customer={customerPreview}
              internal={breakdown}
              quotationHTML={quotationHTML}
            />
          ) : null}
        </div>
      ) : null}

      {page === "archive" ? (
        <ArchivePage
          onNewQuotation={handleNewQuotation}
          onOpenQuotation={handleOpenQuotation}
        />
      ) : null}

      {page === "detail" && selectedQuotation !== null ? (
        <QuotationDetailPage
          quotationID={selectedQuotation.id}
          revision={selectedQuotation.revision}
          onBack={() => setPage("archive")}
          onEdit={handleEditQuotation}
        />
      ) : null}

      {page === "users" && user?.Role === "admin" ? <UsersPage /> : null}

      {page === "clients" && user?.Role === "admin" ? <ClientsPage /> : null}

      {page === "settings" && user !== null ? (
        <SettingsPage
          isAdmin={user.Role === "admin"}
          user={user}
          onProfileChanged={setUser}
          onCompanyChanged={() => void loadCompany()}
        />
      ) : null}
    </AppShell>
    </>
      )}
    </>
  );
}

export default App;
