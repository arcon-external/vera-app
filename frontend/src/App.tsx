import { useState, useCallback, useRef, useEffect } from "react";
import {
  Prepare,
  Export,
  GetCustomerPreview,
  GetInternalBreakdown,
  GetQuotationPreviewHTML,
  SetRevisionContext,
  ClearRevisionContext,
} from "@backend/backend/Quotation";
import { backend } from "@backend/models";
import { Select } from "./components/Select";
import { ReviewPanel } from "./components/ReviewPanel";
import type { CustomerPreview, InternalBreakdown } from "./preview";
import { useToast } from "./toast";
import { ArchivePage } from "./components/ArchivePage";
import { QuotationDetailPage } from "./components/QuotationDetailPage";
import type { ArchivedQuotation } from "./archive";

interface ItemRow {
  key: number;
  description: string;
  subItems: string[];
  qty: string;
  uom: string;
  unitPrice: string;
  marginEnabled: boolean;
  marginPercent: string;
}

interface StatusState {
  type: "idle" | "loading" | "ready" | "error";
  message: string;
}

const CUSTOMERS: Record<number, string> = {
  2: "Infineon Technologies",
  3: "H-Tech Oilfield NOV",
  4: "Datalec Global",
  5: "Panasonic Industrial",
  6: "Surya Teknologi",
};

const CUSTOMER_OPTIONS = Object.entries(CUSTOMERS).map(([id, name]) => ({
  value: Number(id),
  label: name,
}));

const UOM_OPTIONS = [
  { value: "Lot", label: "Lot" },
  { value: "PC", label: "PC" },
  { value: "EA", label: "EA" },
  { value: "Set", label: "Set" },
  { value: "Meter", label: "Meter" },
] as const;

const VALIDITY_OPTIONS = [
  { value: 30, label: "30 Days" },
  { value: 60, label: "60 Days" },
  { value: 90, label: "90 Days" },
] as const;

const INCOTERM_OPTIONS = [
  { value: "EXW", label: "EXW" },
  { value: "FCA", label: "FCA" },
  { value: "FAS", label: "FAS" },
  { value: "FOB", label: "FOB" },
  { value: "CFR", label: "CFR" },
  { value: "CIF", label: "CIF" },
  { value: "CPT", label: "CPT" },
  { value: "CIP", label: "CIP" },
  { value: "DPU", label: "DPU" },
  { value: "DAP", label: "DAP" },
  { value: "DDP", label: "DDP" },
] as const;

const TAX_ZONE_OPTIONS = [
  { value: "FTZ", label: "FTZ — 0% tax" },
  { value: "NonFTZ", label: "Non-FTZ" },
] as const;

const TAX_CATEGORY_OPTIONS = [
  { value: "SupplyOnly", label: "Supply Only — 11%" },
  { value: "SupplyInstall", label: "Supply and Install Services — 2%" },
  {
    value: "SupplyInstallConstruction",
    label: "Supply and Install Services (Construction) — 1.75%",
  },
] as const;

const DEFAULT_ITEM: ItemRow = {
  key: 0,
  description: "",
  subItems: [],
  qty: "1",
  uom: "Lot",
  unitPrice: "",
  marginEnabled: false,
  marginPercent: "0",
};

function newItem(key: number): ItemRow {
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

function parseSubItems(text: string): { description: string; subItems: string[] } {
  const lines = text.split("\n").filter((l) => l.trim());
  const mainParts: string[] = [];
  const bullets: string[] = [];

  for (const line of lines) {
    const trimmed = line.trimStart();
    if (trimmed.startsWith("- ") || trimmed.startsWith("* ") || trimmed.startsWith("* ")) {
      bullets.push(trimmed.slice(2).trim());
    } else {
      mainParts.push(trimmed);
    }
  }

  return {
    description: mainParts.join(" "),
    subItems: bullets,
  };
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
  const [page, setPage] = useState<"create" | "archive" | "detail">("create");
  const [selectedQuotation, setSelectedQuotation] = useState<{ id: number; revision: number } | null>(null);
  const [editingQuote, setEditingQuote] = useState<{ number: string; revision: number } | null>(null);
  const nextKey = useRef(1);
  const [items, setItems] = useState<ItemRow[]>([newItem(0)]);
  const [subject, setSubject] = useState("");
  const [recipientName, setRecipientName] = useState("");
  const [customerId, setCustomerId] = useState(2);
  const [validity, setValidity] = useState(30);
  const [paymentTerm, setPaymentTerm] = useState(90);
  const [leadTime, setLeadTime] = useState(60);
  const [discountPercent, setDiscountPercent] = useState(5);
  const [taxZone, setTaxZone] = useState("FTZ");
  const [taxCategory, setTaxCategory] = useState("");
  const [incoterm, setIncoterm] = useState("EXW");
  const [status, setStatus] = useState<StatusState>({ type: "idle", message: "" });
  const [customerPreview, setCustomerPreview] =
    useState<CustomerPreview | null>(null);
  const [breakdown, setBreakdown] = useState<InternalBreakdown | null>(null);
  const [quotationHTML, setQuotationHTML] = useState<string | null>(null);
  const { pushToast } = useToast();

  const clearPreview = useCallback(() => {
    setCustomerPreview(null);
    setBreakdown(null);
    setQuotationHTML(null);
  }, []);

  const handleNewQuotation = useCallback(() => {
    void ClearRevisionContext().catch(() => undefined);
    nextKey.current = 1;
    setItems([newItem(0)]);
    setSubject("");
    setRecipientName("");
    setCustomerId(2);
    setValidity(30);
    setPaymentTerm(90);
    setLeadTime(60);
    setDiscountPercent(5);
    setTaxZone("FTZ");
    setTaxCategory("");
    setIncoterm("EXW");
    setEditingQuote(null);
    clearPreview();
    setStatus({ type: "idle", message: "" });
    setPage("create");
  }, [clearPreview]);

  const handleOpenQuotation = useCallback((id: number, revision: number) => {
    setSelectedQuotation({ id, revision });
    setPage("detail");
  }, []);

  const handleEditQuotation = useCallback(
    async (quotation: ArchivedQuotation) => {
      try {
        await SetRevisionContext(quotation.QuoteNumber, quotation.RevisionNumber + 1);
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
        setSubject(quotation.Subject);
        setRecipientName(quotation.RecipientName);
        setCustomerId(quotation.CustomerCode);
        setValidity(quotation.ValidityDays);
        setPaymentTerm(quotation.PaymentTermDays);
        setLeadTime(quotation.LeadTimeDays);
        setDiscountPercent(quotation.DiscountPercent);
        setTaxZone(quotation.TaxZone);
        setTaxCategory(quotation.TaxCategory);
        setIncoterm(quotation.Incoterm);
        setEditingQuote({ number: quotation.QuoteNumber, revision: quotation.RevisionNumber + 1 });
        clearPreview();
        setStatus({ type: "idle", message: "" });
        setPage("create");
        pushToast("info", `Editing ${quotation.QuoteNumber}. Saving will create revision ${String(quotation.RevisionNumber + 1).padStart(2, "0")}.`);
      } catch {
        pushToast("error", "We couldn't start editing this quotation. Try again.");
      }
    },
    [clearPreview, pushToast]
  );

  // Any input edit invalidates the review: the user must re-Prepare.
  useEffect(() => {
    clearPreview();
    setStatus((s) => (s.type === "ready" ? { type: "idle", message: "" } : s));
  }, [
    items,
    subject,
    recipientName,
    discountPercent,
    taxZone,
    taxCategory,
    validity,
    paymentTerm,
    leadTime,
    customerId,
    incoterm,
    clearPreview,
  ]);

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
    (key: number, field: keyof ItemRow, value: string | string[] | boolean) => {
      setItems((prev) =>
        prev.map((item) => (item.key === key ? { ...item, [field]: value } : item))
      );
    },
    []
  );

  const handleAddSubItem = useCallback((key: number) => {
    setItems((prev) =>
      prev.map((item) =>
        item.key === key
          ? { ...item, subItems: [...item.subItems, ""] }
          : item
      )
    );
  }, []);

  const handleRemoveSubItem = useCallback((key: number, subIdx: number) => {
    setItems((prev) =>
      prev.map((item) =>
        item.key === key
          ? { ...item, subItems: item.subItems.filter((_, i) => i !== subIdx) }
          : item
      )
    );
  }, []);

  const handleSubItemChange = useCallback(
    (key: number, subIdx: number, value: string) => {
      setItems((prev) =>
        prev.map((item) =>
          item.key === key
            ? {
                ...item,
                subItems: item.subItems.map((s, i) => (i === subIdx ? value : s)),
              }
            : item
        )
      );
    },
    []
  );

  const handlePaste = useCallback(
    (targetKey: number, e: React.ClipboardEvent<HTMLInputElement>) => {
      const raw = e.clipboardData.getData("text");
      if (!raw.includes("\t")) return;

      e.preventDefault();
      const rows = splitRows(raw)
        .filter((r) => r.trim())
        .map(parseRow);

      setItems((prev) => {
        const targetIdx = prev.findIndex((item) => item.key === targetKey);
        if (targetIdx === -1) return prev;

        const parsed = rows.map((cols) => {
          const { description, subItems } = parseSubItems(cols[0] || "");
          return {
            ...DEFAULT_ITEM,
            key: 0,
            description,
            subItems,
            uom: (cols[1] || "").trim() || "Lot",
            qty: (cols[2] || "").trim() || "1",
            unitPrice: (cols[3] || "").replace(/\D/g, "").trim(),
            marginPercent: Math.min(
              Number.parseInt((cols[4] || "").replace(/\D/g, "")) || 0,
              100
            ).toString(),
            marginEnabled: Number.parseInt((cols[4] || "").replace(/\D/g, "")) > 0,
          };
        });

        const first = { ...parsed[0], key: prev[targetIdx].key };
        const rest = parsed.slice(1).map((p, i) => ({
          ...p,
          key: nextKey.current + i,
        }));
        nextKey.current += rest.length;

        return [
          ...prev.slice(0, targetIdx),
          first,
          ...(rest.length > 0 ? rest : []),
          ...prev.slice(targetIdx + 1),
        ];
      });
    },
    []
  );

  const handlePrepare = useCallback(async () => {
    setStatus({ type: "loading", message: "Validating..." });

    const lineItems = items
      .filter((item) => item.description.trim() !== "" || item.subItems.some((s) => s.trim()))
      .map((item) =>
        new backend.QuotationItemInput({
          Description: buildDescription(item.description, item.subItems),
          Qty: Number.parseInt(item.qty) || 0,
          UoM: item.uom,
          UnitPrice: Number.parseInt(item.unitPrice) || 0,
          MarginEnabled: item.marginEnabled,
          MarginPercent: Number.parseInt(item.marginPercent) || 0,
        })
      );

    const input = new backend.QuotationInput({
      Items: lineItems,
      Subject: subject,
      RecipientName: recipientName,
      DiscountPercent: discountPercent,
      TaxZone: taxZone,
      TaxCategory: taxZone === "NonFTZ" ? taxCategory : "",
      Validity: validity,
      PaymentTerm: paymentTerm,
      LeadTime: leadTime,
      CustomerID: customerId,
      Incoterm: incoterm,
    });

    try {
      await Prepare(input);
      const [customer, internal, quotationPreviewHTML] = await Promise.all([
        GetCustomerPreview(),
        GetInternalBreakdown(),
        GetQuotationPreviewHTML(),
      ]);
      setCustomerPreview(customer as CustomerPreview);
      setBreakdown(internal as InternalBreakdown);
      setQuotationHTML(String(quotationPreviewHTML));
      setStatus({ type: "ready", message: "Quotation prepared — review and export" });
      pushToast("success", "Quotation prepared. Review the totals before exporting.");
    } catch (err: unknown) {
      clearPreview();
      const msg = getErrorMessage(err, "We couldn't prepare the quotation. Check the form and try again.");
      setStatus({ type: "error", message: msg });
      pushToast("error", msg);
    }
  }, [items, subject, recipientName, discountPercent, taxZone, taxCategory, validity, paymentTerm, leadTime, customerId, incoterm, clearPreview, pushToast]);

  const handlePasteDetails = useCallback(
    (e: React.ClipboardEvent<HTMLInputElement>) => {
      const raw = e.clipboardData.getData("text");
      if (!raw.includes("\t")) return;
      e.preventDefault();
      const rows = splitRows(raw).filter((r) => r.trim()).map(parseRow);
      if (rows.length === 0) return;
      const cols = rows[0];
      setSubject((cols[0] || "").trim());
      setRecipientName((cols[1] || "").trim());
      setValidity(Number.parseInt(cols[2]) || 30);
      setPaymentTerm(Number.parseInt(cols[3]) || 90);
      setLeadTime(Number.parseInt(cols[4]) || 60);
      setDiscountPercent(Math.min(Number.parseInt(cols[5]) || 5, 100));
      setCustomerId(Number.parseInt(cols[6]) || 2);
      setIncoterm((cols[7] || "").trim() || "EXW");
      const pastedZone = (cols[9] || "").trim();
      setTaxZone(pastedZone === "NonFTZ" ? "NonFTZ" : "FTZ");
      setTaxCategory((cols[10] || "").trim());
    },
    []
  );

  const handleExport = useCallback(async () => {
    setStatus({ type: "loading", message: "Generating PDF..." });
    try {
      await Export();
      clearPreview();
      void ClearRevisionContext().catch(() => undefined);
      setEditingQuote(null);
      setStatus({ type: "idle", message: "" });
      pushToast("success", "Quotation exported successfully.");
    } catch (err: unknown) {
      const msg = getErrorMessage(err, "We couldn't export the quotation. Check the save location and try again.");
      if (msg.includes("user cancelled") || msg.includes("cancelled")) {
        setStatus({ type: "ready", message: "Quotation prepared — review and export" });
        pushToast("info", "Export cancelled. Your quotation is still ready to review.");
        return;
      }
      clearPreview();
      setStatus({ type: "error", message: msg });
      pushToast("error", msg);
    }
  }, [clearPreview, pushToast]);

  const statusStyles: Record<StatusState["type"], string> = {
    idle: "hidden",
    loading: "border-slate-200 text-slate-600 bg-white",
    ready: "border-emerald-200 text-emerald-800 bg-emerald-50",
    error: "border-red-200 text-red-700 bg-red-50",
  };

  const sectionLabel = "text-[11px] tracking-[0.2em] uppercase text-slate-400";
  const divider = "h-[1px] flex-1 bg-slate-200";
  const card =
    "rounded-2xl border border-slate-200/80 bg-white shadow-[0_1px_3px_rgba(0,0,0,0.03)]";
  const inputField =
    "w-full rounded-xl border border-slate-200 bg-white px-4 py-3 text-[14px] text-slate-800 placeholder-slate-400 outline-none transition duration-200 hover:border-slate-300 focus:border-emerald-500/60 focus:ring-2 focus:ring-emerald-500/10";
  const inputFieldCenter =
    "w-full bg-transparent text-[14px] text-slate-800 text-center outline-none";
  const inputGroup =
    "flex items-center rounded-xl border border-slate-200 bg-white px-4 py-3 transition duration-200 hover:border-slate-300 focus-within:border-emerald-500/60 focus-within:ring-2 focus-within:ring-emerald-500/10";
  const inputSuffix = "ml-2 text-[12px] text-slate-400 select-none";

  return (
    <div className="min-h-screen bg-[#f7f8fa] text-slate-800 font-sans antialiased selection:bg-emerald-100 selection:text-emerald-900">
      <div className="mx-auto max-w-[1040px] px-8 py-10">
        <header className="mb-10 flex items-center justify-between">
          <div>
            <h1 className="text-[24px] font-semibold tracking-[-0.03em] text-slate-900">
              Vera
            </h1>
            <p className="mt-1 text-[13px] text-slate-400">Quotation Generator</p>
          </div>
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={handleNewQuotation}
              className={`min-h-10 rounded-lg px-3 text-[12px] font-medium transition active:scale-[0.98] focus:outline-none focus:ring-2 focus:ring-emerald-500/20 ${
                page === "create" ? "bg-slate-900 text-white" : "text-slate-500 hover:bg-white hover:text-slate-900"
              }`}
            >
              New quotation
            </button>
            <button
              type="button"
              onClick={() => setPage("archive")}
              className={`min-h-10 rounded-lg px-3 text-[12px] font-medium transition active:scale-[0.98] focus:outline-none focus:ring-2 focus:ring-emerald-500/20 ${
                page !== "create" ? "bg-slate-900 text-white" : "text-slate-500 hover:bg-white hover:text-slate-900"
              }`}
            >
              Archive
            </button>
            <div className="ml-2 hidden h-[1px] w-8 bg-slate-200 sm:block" />
            <span className="hidden text-[11px] tracking-[0.15em] uppercase text-slate-400 lg:block">
              PT Arcon Penta Persada
            </span>
          </div>
        </header>

        <main className="flex flex-col gap-8">
          {page === "create" && (
            <>
          <section>
            <div className="mb-4 flex items-center gap-3">
              <span className={sectionLabel}>Customer</span>
              <div className={divider} />
            </div>
            <div className={`${card} p-5`}>
              <Select
                value={customerId}
                options={CUSTOMER_OPTIONS}
                onChange={(v) => setCustomerId(Number(v))}
              />
            </div>
          </section>

          <section>
            <div className="mb-4 flex items-center gap-3">
              <span className={sectionLabel}>Items</span>
              <div className={divider} />
              <span className="text-[11px] text-slate-400">
                {items.filter((i) => i.description.trim() || i.subItems.some((s) => s.trim())).length} / {items.length}
              </span>
            </div>

            <div className={`${card} overflow-hidden`}>
              <div className="grid grid-cols-[1.4fr_0.58fr_0.52fr_0.9fr_1fr_auto] gap-3 border-b border-slate-100 px-6 py-3 text-[11px] font-medium tracking-[0.08em] uppercase text-slate-400">
                <span>Description</span>
                <span>UoM</span>
                <span>Qty</span>
                <span>Cost Price (IDR)</span>
                <span>Margin</span>
                <span />
              </div>

              <div className="divide-y divide-slate-100">
                {items.map((item, idx) => (
                  <div
                    key={item.key}
                    style={{
                      animation: `itemIn 300ms ease-out forwards`,
                      animationDelay: `${Math.min(idx * 40, 200)}ms`,
                      opacity: 0,
                    }}
                  >
                    <div className="grid grid-cols-[1.4fr_0.58fr_0.52fr_0.9fr_1fr_auto] gap-3 px-6 pt-3 items-start group">
                      <input
                        type="text"
                        placeholder="Service or product description. Paste from Excel"
                        value={item.description}
                        onChange={(e) =>
                          handleItemChange(item.key, "description", e.target.value)
                        }
                        onPaste={(e) => handlePaste(item.key, e)}
                        className="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 text-[14px] text-slate-800 placeholder-slate-400 outline-none transition duration-200 hover:border-slate-300 focus:border-emerald-500/60 focus:ring-2 focus:ring-emerald-500/10"
                      />
                      <Select
                        value={item.uom}
                        options={UOM_OPTIONS}
                        className="!py-2 !px-3 !rounded-lg"
                        onChange={(v) =>
                          handleItemChange(item.key, "uom", String(v))
                        }
                      />
                      <input
                        type="text"
                        inputMode="numeric"
                        placeholder="1"
                        value={item.qty}
                        onChange={(e) =>
                          handleItemChange(item.key, "qty", e.target.value.replace(/\D/g, ""))
                        }
                        className="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 text-[14px] text-slate-800 placeholder-slate-400 text-center outline-none transition duration-200 hover:border-slate-300 focus:border-emerald-500/60 focus:ring-2 focus:ring-emerald-500/10"
                      />
                      <input
                        type="text"
                        inputMode="numeric"
                        placeholder="0"
                        value={item.unitPrice}
                        onChange={(e) =>
                          handleItemChange(
                            item.key,
                            "unitPrice",
                            e.target.value.replace(/\D/g, "")
                          )
                        }
                        className="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 text-[14px] text-slate-800 placeholder-slate-400 outline-none transition duration-200 hover:border-slate-300 focus:border-emerald-500/60 focus:ring-2 focus:ring-emerald-500/10"
                      />
                      <div className="flex min-w-0 flex-wrap items-center gap-1.5 pt-0.5">
                        <button
                          type="button"
                          role="switch"
                          aria-checked={item.marginEnabled}
                          aria-label={`Enable profit margin for item ${idx + 1}`}
                          onClick={() =>
                            handleItemChange(item.key, "marginEnabled", !item.marginEnabled)
                          }
                          className={`inline-flex min-h-10 items-center gap-1.5 rounded-lg border px-2.5 text-[12px] font-medium transition duration-200 active:scale-[0.97] focus:outline-none focus:ring-2 focus:ring-emerald-500/20 ${
                            item.marginEnabled
                              ? "border-emerald-200 bg-emerald-50 text-emerald-700"
                              : "border-slate-200 bg-slate-50 text-slate-500 hover:border-slate-300"
                          }`}
                        >
                          <span
                            aria-hidden="true"
                            className={`h-1.5 w-1.5 rounded-full ${
                              item.marginEnabled ? "bg-emerald-500" : "bg-slate-300"
                            }`}
                          />
                          {item.marginEnabled ? "On" : "Off"}
                        </button>
                        {item.marginEnabled && (
                          <div className="flex items-center rounded-lg border border-emerald-200 bg-white px-2 py-2">
                            <input
                              type="text"
                              inputMode="numeric"
                              aria-label={`Profit margin percentage for item ${idx + 1}`}
                              value={item.marginPercent}
                              onChange={(e) =>
                                handleItemChange(
                                  item.key,
                                  "marginPercent",
                                  Math.min(
                                    Number(e.target.value.replace(/\D/g, "")) || 0,
                                    100
                                  ).toString()
                                )
                              }
                              className="w-9 bg-transparent text-center text-[13px] text-slate-800 outline-none"
                            />
                            <span className="text-[12px] text-slate-400">%</span>
                          </div>
                        )}
                      </div>
                      <button
                        onClick={() => handleRemoveItem(item.key)}
                        disabled={items.length <= 1}
                        className="mx-auto flex h-8 w-8 items-center justify-center rounded-lg text-slate-300 transition-all duration-200 hover:bg-red-50 hover:text-red-500 active:scale-[0.94] disabled:opacity-0 disabled:cursor-default"
                      >
                        <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
                          <path d="M2 4h12M5.333 4V2.667a1.333 1.333 0 011.334-1.334h2.666a1.333 1.333 0 011.334 1.334V4m2 0v9.333a1.333 1.333 0 01-1.334 1.334H4.667a1.333 1.333 0 01-1.334-1.334V4h9.334z" />
                        </svg>
                      </button>
                    </div>

                    {item.subItems.length > 0 && (
                      <div className="mb-2 ml-12 border-l-2 border-slate-100 pl-4">
                        {item.subItems.map((sub, si) => (
                          <div
                            key={si}
                            className="grid grid-cols-[1.4fr_0.58fr_0.52fr_0.9fr_1fr_auto] gap-3 pr-6 py-1 items-center"
                          >
                            <div className="col-span-5 flex items-center gap-2">
                              <span className="flex-shrink-0 text-[11px] text-slate-400 select-none">
                                &mdash;
                              </span>
                              <input
                                type="text"
                                placeholder={`Sub-item ${si + 1}`}
                                value={sub}
                                onChange={(e) =>
                                  handleSubItemChange(item.key, si, e.target.value)
                                }
                                className="flex-1 bg-transparent py-1 text-[13px] text-slate-600 placeholder-slate-300 outline-none transition-colors duration-200"
                              />
                            </div>
                            <button
                              onClick={() => handleRemoveSubItem(item.key, si)}
                              className="mx-auto flex h-8 w-8 items-center justify-center rounded-lg text-slate-300 transition-all duration-200 hover:bg-red-50 hover:text-red-500 active:scale-[0.94]"
                            >
                              <svg width="10" height="10" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2">
                                <path d="M4 4l8 8M12 4l-8 8" />
                              </svg>
                            </button>
                          </div>
                        ))}
                        <button
                          onClick={() => handleAddSubItem(item.key)}
                          className="flex items-center gap-1 py-1 text-[11px] text-slate-400 transition-colors duration-200 hover:text-slate-600"
                        >
                          <svg width="10" height="10" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
                            <path d="M8 3.333v9.334M3.333 8h9.334" />
                          </svg>
                          Add sub-item
                        </button>
                      </div>
                    )}

                    {item.subItems.length === 0 && (
                      <div className="ml-6 mr-6 mb-2">
                        <button
                          onClick={() => handleAddSubItem(item.key)}
                          className="flex items-center gap-1 py-1 text-[11px] text-slate-300 transition-colors duration-200 hover:text-slate-500"
                        >
                          <svg width="10" height="10" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
                            <path d="M8 3.333v9.334M3.333 8h9.334" />
                          </svg>
                          Add sub-item
                        </button>
                      </div>
                    )}
                  </div>
                ))}
              </div>

              <button
                onClick={handleAddItem}
                className="flex w-full items-center justify-center gap-2 border-t border-slate-100 px-6 py-3 text-[13px] font-medium text-slate-400 transition-all duration-200 hover:bg-slate-50 hover:text-slate-600 active:scale-[0.99]"
              >
                <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
                  <path d="M8 3.333v9.334M3.333 8h9.334" />
                </svg>
                Add item
              </button>
            </div>
          </section>

          <section>
            <div className="mb-4 flex items-center gap-3">
              <span className={sectionLabel}>Details</span>
              <div className={divider} />
            </div>

            <div className={`${card} p-6`}>
              <div className="grid grid-cols-1 gap-5">
                <div>
                  <label className="mb-2 block text-[12px] font-medium tracking-[0.06em] uppercase text-slate-400">
                    Subject
                  </label>
                  <input
                    type="text"
                    placeholder="e.g. Supply & Installation of Electrical Systems. Paste from Excel"
                    value={subject}
                    onChange={(e) => setSubject(e.target.value)}
                    onPaste={handlePasteDetails}
                    className={inputField}
                  />
                </div>

                <div>
                  <label className="mb-2 block text-[12px] font-medium tracking-[0.06em] uppercase text-slate-400">
                    Recipient name
                  </label>
                  <input
                    type="text"
                    placeholder="Full name of the recipient"
                    value={recipientName}
                    onChange={(e) => setRecipientName(e.target.value)}
                    className={inputField}
                  />
                </div>

                <div className="grid grid-cols-2 gap-4 sm:grid-cols-3">
                  <div>
                    <label className="mb-2 block text-[12px] font-medium tracking-[0.06em] uppercase text-slate-400">
                      Validity
                    </label>
                    <Select
                      value={validity}
                      options={VALIDITY_OPTIONS}
                      onChange={(v) => setValidity(Number(v))}
                    />
                  </div>

                  <div>
                    <label className="mb-2 block text-[12px] font-medium tracking-[0.06em] uppercase text-slate-400">
                      Payment
                    </label>
                    <div className={inputGroup}>
                      <input
                        type="text"
                        inputMode="numeric"
                        value={paymentTerm}
                        onChange={(e) =>
                          setPaymentTerm(Number(e.target.value.replace(/\D/g, "")) || 0)
                        }
                        className={inputFieldCenter}
                      />
                      <span className={inputSuffix}>days</span>
                    </div>
                  </div>

                  <div>
                    <label className="mb-2 block text-[12px] font-medium tracking-[0.06em] uppercase text-slate-400">
                      Lead time
                    </label>
                    <div className={inputGroup}>
                      <input
                        type="text"
                        inputMode="numeric"
                        value={leadTime}
                        onChange={(e) =>
                          setLeadTime(Number(e.target.value.replace(/\D/g, "")) || 0)
                        }
                        className={inputFieldCenter}
                      />
                      <span className={inputSuffix}>days</span>
                    </div>
                  </div>

                  <div>
                    <label className="mb-2 block text-[12px] font-medium tracking-[0.06em] uppercase text-slate-400">
                      Discount
                    </label>
                    <div className={inputGroup}>
                      <input
                        type="text"
                        inputMode="numeric"
                        value={discountPercent}
                        onChange={(e) =>
                          setDiscountPercent(
                            Math.min(Number(e.target.value.replace(/\D/g, "")) || 0, 100)
                          )
                        }
                        className={inputFieldCenter}
                      />
                      <span className={inputSuffix}>%</span>
                    </div>
                  </div>

                  <div>
                    <label className="mb-2 block text-[12px] font-medium tracking-[0.06em] uppercase text-slate-400">
                      Incoterm
                    </label>
                    <Select
                      value={incoterm}
                      options={INCOTERM_OPTIONS}
                      onChange={(v) => setIncoterm(String(v))}
                    />
                  </div>
                </div>

                <div className="mt-5 grid grid-cols-2 gap-4">
                  <div>
                    <label className="mb-2 block text-[12px] font-medium tracking-[0.06em] uppercase text-slate-400">
                      Tax zone
                    </label>
                    <div className="grid grid-cols-2 gap-2">
                      {TAX_ZONE_OPTIONS.map((opt) => (
                        <button
                          key={opt.value}
                          type="button"
                          onClick={() => {
                            setTaxZone(opt.value);
                            if (opt.value === "FTZ") setTaxCategory("");
                          }}
                          className={`rounded-xl border px-4 py-3 text-[14px] transition duration-200 active:scale-[0.98] ${
                            taxZone === opt.value
                              ? "border-emerald-500/60 bg-emerald-50 font-medium text-emerald-700 ring-2 ring-emerald-500/10"
                              : "border-slate-200 bg-white text-slate-600 hover:border-slate-300"
                          }`}
                        >
                          {opt.label}
                        </button>
                      ))}
                    </div>
                  </div>

                  {taxZone === "NonFTZ" && (
                    <div>
                      <label className="mb-2 block text-[12px] font-medium tracking-[0.06em] uppercase text-slate-400">
                        Tax category
                      </label>
                      <Select
                        value={taxCategory}
                        options={[
                          { value: "", label: "Select category…" },
                          ...TAX_CATEGORY_OPTIONS,
                        ]}
                        onChange={(v) => setTaxCategory(String(v))}
                      />
                    </div>
                  )}
                </div>
              </div>
            </div>
          </section>

          {customerPreview !== null && breakdown !== null && quotationHTML !== null && (
            <ReviewPanel
              customer={customerPreview}
              internal={breakdown}
              quotationHTML={quotationHTML}
            />
          )}

          <section className="flex flex-col items-center gap-3">
            {status.type !== "idle" && (
              <div
                className={`w-full rounded-2xl border px-5 py-3 text-[13px] font-medium transition-all duration-300 ${statusStyles[status.type]}`}
                style={{
                  animation: "statusIn 200ms ease-out forwards",
                }}
              >
                {status.type === "loading" && (
                  <span className="inline-flex items-center gap-2">
                    <span className="h-3 w-3 animate-pulse rounded-full bg-current opacity-30" />
                    {status.message}
                  </span>
                )}
                {status.type !== "loading" && status.message}
              </div>
            )}

            <div className="flex gap-3">
              <button
                onClick={handlePrepare}
                disabled={status.type === "loading"}
                className="flex items-center gap-2 rounded-xl border border-slate-200 bg-white px-5 py-2.5 text-[13px] font-semibold text-slate-600 shadow-[0_1px_2px_rgba(0,0,0,0.03)] transition-all duration-200 hover:border-slate-300 hover:text-slate-800 hover:shadow-[0_2px_4px_rgba(0,0,0,0.04)] active:scale-[0.97] disabled:cursor-not-allowed disabled:opacity-40"
              >
                <svg
                  width="15"
                  height="15"
                  viewBox="0 0 16 16"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="1.5"
                >
                  <path d="M1.333 12.167V2.833a1 1 0 011-1h11.334a1 1 0 011 1v9.334a1 1 0 01-1 1H2.333a1 1 0 01-1-1z" />
                  <path d="M4.667 6.667L7.333 4l2.667 2.667M7.333 4v8" />
                </svg>
                Prepare
              </button>

              <button
                onClick={handleExport}
                disabled={
                  status.type !== "ready" ||
                  customerPreview === null ||
                  breakdown === null
                }
                className="flex items-center gap-2 rounded-xl bg-emerald-600 px-5 py-2.5 text-[13px] font-semibold text-white shadow-[0_1px_2px_rgba(5,150,105,0.2)] transition-all duration-200 hover:bg-emerald-500 hover:shadow-[0_2px_6px_rgba(5,150,105,0.25)] active:scale-[0.97] disabled:cursor-not-allowed disabled:bg-slate-200 disabled:text-slate-400 disabled:shadow-none"
              >
                <svg
                  width="15"
                  height="15"
                  viewBox="0 0 16 16"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="1.5"
                >
                  <path d="M2.667 11.333v1.334A1.333 1.333 0 004 14h8a1.333 1.333 0 001.333-1.333v-1.334M11.333 8L8 11.333 4.667 8M8 11.333V2" />
                </svg>
                Export PDF
              </button>
            </div>
          </section>
            </>
          )}
          {page === "archive" && (
            <ArchivePage
              onNewQuotation={handleNewQuotation}
              onOpenQuotation={handleOpenQuotation}
            />
          )}
          {page === "detail" && selectedQuotation !== null && (
            <QuotationDetailPage
              quotationID={selectedQuotation.id}
              revision={selectedQuotation.revision}
              onBack={() => setPage("archive")}
              onEdit={handleEditQuotation}
            />
          )}
        </main>

        <footer className="mt-16 border-t border-slate-100 pt-6 text-center">
          <p className="text-[11px] text-slate-400">
            Vera — Quotation Generator for PT Arcon Penta Persada
          </p>
        </footer>
      </div>

      <style>{`
        @keyframes itemIn {
          from {
            opacity: 0;
            transform: translateY(6px);
          }
          to {
            opacity: 1;
            transform: translateY(0);
          }
        }
        @keyframes statusIn {
          from {
            opacity: 0;
            transform: translateY(-4px);
          }
          to {
            opacity: 1;
            transform: translateY(0);
          }
        }
        @keyframes toastIn {
          from {
            opacity: 0;
            transform: translateY(-6px);
          }
          to {
            opacity: 1;
            transform: translateY(0);
          }
        }
        @media (prefers-reduced-motion: reduce) {
          *, *::before, *::after {
            animation-duration: 0.01ms !important;
            transition-duration: 0.01ms !important;
          }
        }
      `}</style>
    </div>
  );
}

export default App;
