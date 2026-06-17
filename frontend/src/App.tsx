import { useState, useCallback, useRef } from "react";
import { Prepare, Export } from "@backend/backend/Quotation";
import { backend } from "@backend/models";

interface ItemRow {
  key: number;
  description: string;
  subItems: string[];
  qty: string;
  uom: string;
  unitPrice: string;
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

const DEFAULT_ITEM: ItemRow = {
  key: 0,
  description: "",
  subItems: [],
  qty: "1",
  uom: "Lot",
  unitPrice: "",
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

function App() {
  const nextKey = useRef(1);
  const [items, setItems] = useState<ItemRow[]>([newItem(0)]);
  const [subject, setSubject] = useState("");
  const [recipientName, setRecipientName] = useState("");
  const [customerId, setCustomerId] = useState(2);
  const [validity, setValidity] = useState(30);
  const [paymentTerm, setPaymentTerm] = useState(90);
  const [leadTime, setLeadTime] = useState(60);
  const [discountPercent, setDiscountPercent] = useState(5);
  const [status, setStatus] = useState<StatusState>({ type: "idle", message: "" });

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
    (key: number, field: keyof ItemRow, value: string | string[]) => {
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
        })
      );

    const input = new backend.QuotationInput({
      Items: lineItems,
      Subject: subject,
      RecipientName: recipientName,
      DiscountPercent: discountPercent,
      Validity: validity,
      PaymentTerm: paymentTerm,
      LeadTime: leadTime,
      CustomerID: customerId,
    });

    try {
      await Prepare(input);
      setStatus({ type: "ready", message: "Quotation prepared — review and export" });
    } catch (err: unknown) {
      const msg = typeof err === "string" ? err : "Validation failed";
      setStatus({ type: "error", message: msg });
    }
  }, [items, subject, recipientName, discountPercent, validity, paymentTerm, leadTime, customerId]);

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
    },
    []
  );

  const handleExport = useCallback(async () => {
    setStatus({ type: "loading", message: "Generating PDF..." });
    try {
      await Export();
      setStatus({ type: "idle", message: "" });
    } catch (err: unknown) {
      const msg = typeof err === "string" ? err : "Export failed";
      if (msg.includes("user cancelled") || msg.includes("cancelled")) {
        setStatus({ type: "ready", message: "Quotation prepared — review and export" });
        return;
      }
      setStatus({ type: "error", message: msg });
    }
  }, []);

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
          <div className="flex items-center gap-3">
            <div className="h-[1px] w-8 bg-slate-200" />
            <span className="text-[11px] tracking-[0.15em] uppercase text-slate-400">
              PT Arcon Penta Persada
            </span>
          </div>
        </header>

        <main className="flex flex-col gap-8">
          <section>
            <div className="mb-4 flex items-center gap-3">
              <span className={sectionLabel}>Customer</span>
              <div className={divider} />
            </div>
            <div className={`${card} p-5`}>
              <select
                value={customerId}
                onChange={(e) => setCustomerId(Number(e.target.value))}
                className="w-full rounded-xl border border-slate-200 bg-white px-4 py-3 text-[15px] text-slate-800 outline-none transition duration-200 hover:border-slate-300 focus:border-emerald-500/60 focus:ring-2 focus:ring-emerald-500/10 cursor-pointer"
              >
                {Object.entries(CUSTOMERS).map(([id, name]) => (
                  <option key={id} value={id} className="text-slate-800">
                    {name}
                  </option>
                ))}
              </select>
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
              <div className="grid grid-cols-[1fr_0.6fr_0.8fr_1fr_0.2fr] gap-3 border-b border-slate-100 px-6 py-3 text-[11px] font-medium tracking-[0.08em] uppercase text-slate-400">
                <span>Description</span>
                <span>UoM</span>
                <span>Qty</span>
                <span>Unit Price (IDR)</span>
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
                    <div className="grid grid-cols-[1fr_0.6fr_0.8fr_1fr_0.2fr] gap-3 px-6 pt-3 items-start group">
                      <input
                        type="text"
                        placeholder="Service or product description. Paste from Excel"
                        value={item.description}
                        onChange={(e) =>
                          handleItemChange(item.key, "description", e.target.value)
                        }
                        onPaste={(e) => handlePaste(item.key, e)}
                        className="w-full bg-transparent py-2 text-[14px] text-slate-800 placeholder-slate-400 outline-none transition-colors duration-200"
                      />
                      <input
                        type="text"
                        placeholder="e.g. Lot"
                        value={item.uom}
                        onChange={(e) =>
                          handleItemChange(item.key, "uom", e.target.value)
                        }
                        className="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 text-[14px] text-slate-800 placeholder-slate-400 outline-none transition duration-200 hover:border-slate-300 focus:border-emerald-500/60 focus:ring-2 focus:ring-emerald-500/10"
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
                      <div className="ml-12 mr-6 mb-2 border-l-2 border-slate-100 pl-4">
                        {item.subItems.map((sub, si) => (
                          <div key={si} className="flex items-center gap-2 py-1">
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
                            <button
                              onClick={() => handleRemoveSubItem(item.key, si)}
                              className="flex-shrink-0 flex h-5 w-5 items-center justify-center rounded text-slate-300 transition-all duration-200 hover:bg-red-50 hover:text-red-400"
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

                <div className="grid grid-cols-4 gap-4">
                  <div>
                    <label className="mb-2 block text-[12px] font-medium tracking-[0.06em] uppercase text-slate-400">
                      Validity
                    </label>
                    <div className={inputGroup}>
                      <input
                        type="text"
                        inputMode="numeric"
                        value={validity}
                        onChange={(e) =>
                          setValidity(Number(e.target.value.replace(/\D/g, "")) || 0)
                        }
                        className={inputFieldCenter}
                      />
                      <span className={inputSuffix}>days</span>
                    </div>
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
                </div>
              </div>
            </div>
          </section>

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
                disabled={status.type !== "ready"}
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
