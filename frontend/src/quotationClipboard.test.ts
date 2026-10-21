import { describe, expect, it } from "vitest";
import type { ArchivedQuotation } from "./archive";
import { computePricing } from "./components/SummaryRail";
import {
  QUOTATION_COPY_KIND,
  QUOTATION_COPY_VERSION,
  applyQuotationCopy,
  archivedToEditorDraft,
  buildQuotationCopy,
  copyQuotationToClipboard,
  currencyMismatchMessage,
  missingCurrencyMessage,
  parseQuotationCopy,
  resolveCopyPasteText,
  splitRawDescription,
  type QuotationCopyPayload,
} from "./quotationClipboard";

// A revision as the fixed detail endpoint returns it: raw stored values, the
// joined "- " description form plain lines are saved in, and priced subs.
function makeArchived(overrides: Partial<ArchivedQuotation> = {}): ArchivedQuotation {
  return {
    ID: 5,
    QuoteNumber: "1502260059",
    RevisionNumber: 3,
    CustomerName: "Acme",
    CustomerCode: 4,
    CustomerAddress: "Jl. Klien 12",
    Subject: "Pump package",
    Currency: "IDR",
    CreatedAt: "",
    GrandTotal: 0,
    DisplayGrand: "",
    RecipientName: "Jane",
    RecipientAddress: "Gedung Penerima 3",
    Incoterm: "DAP",
    ValidityDays: 45,
    PaymentTermDays: 60,
    DisplayLeadTime: "-",
    LeadTimeYears: 0,
    LeadTimeMonths: 1,
    LeadTimeWeeks: 0,
    LeadTimeDays: 0,
    DiscountPercent: 0,
    TaxZone: "FTZ",
    TaxCategory: "",
    TaxType: "Goods",
    ServiceCategory: "",
    TaxRateBps: 0,
    OriginalSubtotal: 0,
    OriginalDiscount: 0,
    OriginalGrand: 0,
    MarginalizedSubtotal: 0,
    MarginalizedDiscount: 0,
    MarginalizedGrand: 0,
    PpnAmount: 0,
    DisplayPpn: "",
    TaxAmount: 0,
    MarginalizedAfterTax: 0,
    ProfitAmount: 0,
    NettProfit: 0,
    DisplayOriginalSubtotal: "",
    DisplayOriginalDiscount: "",
    DisplayOriginalGrand: "",
    DisplayMarginalizedSubtotal: "",
    DisplayMarginalizedDiscount: "",
    DisplayMarginalizedGrand: "",
    DisplayTax: "",
    DisplayMarginalizedAfterTax: "",
    DisplayProfit: "",
    DisplayNett: "",
    HTMLSnapshot: "",
    AvailableRevisions: [3],
    Items: [
      {
        ItemNo: 1,
        // The raw stored form: main text plus "- " bullet lines, exactly as
        // buildDescription saved them.
        Description: "Supply pump\n- Cast iron casing\n- Mechanical seal",
        Qty: 2,
        UoM: "Lot",
        UnitPrice: 15000000,
        DisplayUnitPrice: "",
        MarginEnabled: true,
        MarginPercent: 20,
        MarginAmount: 0,
        DiscountEnabled: false,
        DiscountPercent: 0,
        ListUnitPrice: 0,
        DiscountAmount: 0,
        DisplayMarginAmount: "",
        OriginalTotal: 0,
        DisplayOriginalTotal: "",
        MarginalizedUnitPrice: 18000000,
        DisplayMarginalizedUnit: "",
        MarginalizedTotal: 0,
        DisplayMarginalizedTotal: "",
        ShowSubPrices: false,
        Subs: [],
      },
      {
        ItemNo: 2,
        Description: "Installation",
        Qty: 1,
        UoM: "Lot",
        UnitPrice: 0,
        DisplayUnitPrice: "",
        MarginEnabled: false,
        MarginPercent: 0,
        MarginAmount: 0,
        DiscountEnabled: true,
        DiscountPercent: 10,
        ListUnitPrice: 0,
        DiscountAmount: 0,
        DisplayMarginAmount: "",
        OriginalTotal: 0,
        DisplayOriginalTotal: "",
        MarginalizedUnitPrice: 0,
        DisplayMarginalizedUnit: "",
        MarginalizedTotal: 0,
        DisplayMarginalizedTotal: "",
        ShowSubPrices: true,
        Subs: [
          {
            SubNo: 1,
            Description: "Labour",
            Details: ["Two technicians", "Three days"],
            Qty: 1,
            UoM: "Lot",
            OriginalUnitPrice: 4000000,
            DisplayOriginalUnit: "",
            DisplayOriginalTotal: "",
            MarginEnabled: true,
            MarginPercent: 25,
            MarginAmount: 0,
            DisplayMarginAmount: "",
            MarginalizedUnitPrice: 5000000,
            DisplayMarginalizedUnit: "",
            MarginalizedTotal: 0,
            DisplayMarginalizedTotal: "",
            DiscountEnabled: false,
            DiscountPercent: 0,
            ListUnitPrice: 0,
            DiscountAmount: 0,
          },
        ],
      },
    ],
    ...overrides,
  };
}

const CUSTOMERS = [
  { value: 2, label: "Default" },
  { value: 4, label: "Acme" },
];

describe("splitRawDescription", () => {
  it("splits '- ' lines into bullets and joins the rest as the main text", () => {
    expect(splitRawDescription("Supply pump\n- Cast iron casing\n- Mechanical seal")).toEqual({
      description: "Supply pump",
      bullets: ["Cast iron casing", "Mechanical seal"],
    });
  });

  it("treats '* ' lines as bullets too", () => {
    expect(splitRawDescription("Title\n* First\n* Second")).toEqual({
      description: "Title",
      bullets: ["First", "Second"],
    });
  });

  it("keeps a plain description whole and skips blank lines", () => {
    expect(splitRawDescription("Just a line\n\n   \nSecond line")).toEqual({
      description: "Just a line Second line",
      bullets: [],
    });
  });
});

describe("buildQuotationCopy / parseQuotationCopy", () => {
  it("round-trips the whole revision through text", () => {
    const text = buildQuotationCopy(makeArchived());
    const parsed = parseQuotationCopy(text);
    expect(parsed).not.toBeNull();
    const payload = parsed as QuotationCopyPayload;
    expect(payload.kind).toBe(QUOTATION_COPY_KIND);
    expect(payload.version).toBe(QUOTATION_COPY_VERSION);
    expect(payload.source).toEqual({ quoteNumber: "1502260059", revision: 3 });
    expect(payload.quotation.customerCode).toBe(4);
    expect(payload.quotation.recipientName).toBe("Jane");
    expect(payload.quotation.recipientAddress).toBe("Gedung Penerima 3");
    expect(payload.quotation.subject).toBe("Pump package");
    expect(payload.quotation.incoterm).toBe("DAP");
    expect(payload.quotation.validityDays).toBe(45);
    expect(payload.quotation.paymentTermDays).toBe(60);
    expect(payload.quotation.leadTimeMonths).toBe(1);
    expect(payload.quotation.taxZone).toBe("FTZ");
    expect(payload.quotation.taxType).toBe("Goods");
    expect(payload.quotation.items).toHaveLength(2);
  });

  it("carries the revision currency", () => {
    const parsed = parseQuotationCopy(buildQuotationCopy(makeArchived())) as QuotationCopyPayload;
    expect(parsed.quotation.currency).toBe("IDR");
  });

  it("rejects a payload with missing currency", () => {
    const payload = JSON.parse(buildQuotationCopy(makeArchived()));
    const quotation = { ...payload.quotation };
    delete quotation.currency;
    expect(parseQuotationCopy(JSON.stringify({ ...payload, quotation }))).toBeNull();
  });

  it("rejects a payload with blank currency", () => {
    const payload = JSON.parse(buildQuotationCopy(makeArchived()));
    const quotation = { ...payload.quotation, currency: "   " };
    expect(parseQuotationCopy(JSON.stringify({ ...payload, quotation }))).toBeNull();
  });

  it("carries raw costs, never display or marginalised figures", () => {
    const parsed = parseQuotationCopy(buildQuotationCopy(makeArchived())) as QuotationCopyPayload;
    const [plain, split] = parsed.quotation.items;
    expect(plain.unitCost).toBe(15000000);
    expect(plain.marginPercent).toBe(20);
    expect(plain.description).toBe("Supply pump\n- Cast iron casing\n- Mechanical seal");
    expect(split.discountPercent).toBe(10);
    expect(split.subItems[0].unitCost).toBe(4000000);
    expect(split.subItems[0].details).toEqual(["Two technicians", "Three days"]);
  });

  it.each([
    ["empty text", ""],
    ["ordinary prose", "Supply pump for the new plant"],
    ["a description field paste", "Line one\n- bullet one"],
    ["tab-separated Excel rows", "Pump\tLot\t2\t15000000\t20"],
    ["valid JSON of another shape", '{"subject":"x"}'],
    ["a JSON array", "[]"],
  ])("ignores %s", (_label, text) => {
    expect(parseQuotationCopy(text)).toBeNull();
  });

  it("rejects the wrong kind", () => {
    const payload = JSON.parse(buildQuotationCopy(makeArchived()));
    expect(parseQuotationCopy(JSON.stringify({ ...payload, kind: "vera.draft" }))).toBeNull();
  });

  it.each([[0], [2], ["1"], [null]])("rejects version %s", (version) => {
    const payload = JSON.parse(buildQuotationCopy(makeArchived()));
    expect(parseQuotationCopy(JSON.stringify({ ...payload, version }))).toBeNull();
  });

  it("rejects a payload missing the items array or a row cost", () => {
    const payload = JSON.parse(buildQuotationCopy(makeArchived()));
    const quotation = { ...payload.quotation };
    delete quotation.items;
    expect(parseQuotationCopy(JSON.stringify({ ...payload, quotation }))).toBeNull();
    const badItems = [{ ...payload.quotation.items[0], unitCost: "15000000" }];
    expect(
      parseQuotationCopy(
        JSON.stringify({ ...payload, quotation: { ...payload.quotation, items: badItems } })
      )
    ).toBeNull();
  });

  it("rejects non-string sub-item details", () => {
    const payload = JSON.parse(buildQuotationCopy(makeArchived()));
    const items = JSON.parse(JSON.stringify(payload.quotation.items));
    items[1].subItems[0].details = [42];
    expect(
      parseQuotationCopy(
        JSON.stringify({ ...payload, quotation: { ...payload.quotation, items } })
      )
    ).toBeNull();
  });
});

describe("archivedToEditorDraft", () => {
  it("maps the revision into editor state with the raw description split", () => {
    const draft = archivedToEditorDraft(makeArchived());
    expect(draft.customerCode).toBe(4);
    expect(draft.details).toMatchObject({
      subject: "Pump package",
      recipientName: "Jane",
      validity: 45,
      paymentTerm: 60,
      leadTimeMonths: 1,
      incoterm: "DAP",
      taxZone: "FTZ",
      taxType: "Goods",
    });
    expect(draft.items).toHaveLength(2);
    expect(draft.items[0]).toMatchObject({
      description: "Supply pump",
      bullets: ["Cast iron casing", "Mechanical seal"],
      qty: "2",
      uom: "Lot",
      unitPrice: "15000000",
      marginEnabled: true,
      marginPercent: "20",
    });
    expect(draft.items[1].subItems[0]).toMatchObject({
      description: "Labour",
      unitCost: "4000000",
      marginPercent: "25",
      bullets: ["Two technicians", "Three days"],
    });
  });

  it("falls back to the legacy tax mapping when the revision predates taxType", () => {
    const draft = archivedToEditorDraft(
      makeArchived({ TaxType: "", ServiceCategory: "", TaxCategory: "SupplyInstall" })
    );
    expect(draft.details.taxType).toBe("Services");
    expect(draft.details.serviceCategory).toBe("NonConstruction");
  });
});

describe("applyQuotationCopy", () => {
  it("fills the whole editor state and recalculates from raw numbers", () => {
    const parsed = parseQuotationCopy(buildQuotationCopy(makeArchived())) as QuotationCopyPayload;
    const applied = applyQuotationCopy(parsed, CUSTOMERS);
    expect(applied.ok).toBe(true);
    if (!applied.ok) return;
    expect(applied.customerCode).toBe(4);
    expect(applied.details.subject).toBe("Pump package");
    expect(applied.details.validity).toBe(45);
    expect(applied.items).toHaveLength(2);
    expect(applied.items[0].key).toBe(0);
    expect(applied.items[1].subItems[0].unitCost).toBe("4000000");
    // React keys and sub ids are assigned, one past the last used id handed
    // back so the editor's counters continue without collision.
    expect(applied.items[1].subItems[0].id).toBe(1);
    expect(applied.nextSubId).toBe(2);
  });

  it("recalculates from the applied state exactly", () => {
    const parsed = parseQuotationCopy(buildQuotationCopy(makeArchived())) as QuotationCopyPayload;
    const applied = applyQuotationCopy(parsed, CUSTOMERS);
    expect(applied.ok).toBe(true);
    if (!applied.ok) return;
    // Plain line: 2 x 15,000,000 + 20% = 36,000,000 sell on 30,000,000 cost.
    // Split line via its sub: 1 x 4,000,000 + 25% = 5,000,000 sell on
    // 4,000,000 cost. FTZ goods charges no PPN and no services tax.
    const priced = computePricing(
      applied.items,
      applied.details.discountPercent,
      applied.details.taxZone,
      applied.details.taxType,
      applied.details.serviceCategory
    );
    expect(priced.costSubtotal).toBe(34000000);
    expect(priced.sellSubtotal).toBe(41000000);
    expect(priced.sellGrand).toBe(41000000);
    expect(priced.customerGrand).toBe(41000000);
  });

  it("never applies the archived number or revision", () => {
    const parsed = parseQuotationCopy(buildQuotationCopy(makeArchived())) as QuotationCopyPayload;
    const applied = applyQuotationCopy(parsed, CUSTOMERS);
    expect(applied.ok).toBe(true);
    expect(JSON.stringify(applied)).not.toContain("1502260059");
  });

  it("refuses an unknown customer code without partial changes", () => {
    const archived = makeArchived({ CustomerCode: 99, CustomerName: "Gone" });
    const parsed = parseQuotationCopy(buildQuotationCopy(archived)) as QuotationCopyPayload;
    const applied = applyQuotationCopy(parsed, CUSTOMERS);
    expect(applied).toEqual({ ok: false, reason: "unknown-customer", customerCode: 99 });
  });

  it("carries the addresses as information only, never as editor values", () => {
    const parsed = parseQuotationCopy(buildQuotationCopy(makeArchived())) as QuotationCopyPayload;
    // The payload keeps what the revision stored; the editor has no address
    // inputs, so the applied state must not grow any.
    expect(parsed.quotation.recipientAddress).toBe("Gedung Penerima 3");
    const applied = applyQuotationCopy(parsed, CUSTOMERS);
    expect(applied.ok).toBe(true);
    if (!applied.ok) return;
    expect(JSON.stringify(applied.details)).not.toContain("Gedung Penerima 3");
    expect(JSON.stringify(applied.details)).not.toContain("Jl. Klien 12");
  });
});

describe("resolveCopyPasteText", () => {
  function decide(text: string | null, dirty: boolean, liveCurrency = "IDR") {
    return resolveCopyPasteText(text, { dirty, customerOptions: CUSTOMERS, liveCurrency });
  }

  it("ignores ordinary text, including tab-separated and multiline pastes", () => {
    expect(decide(null, false)).toEqual({ action: "ignore" });
    expect(decide("", false)).toEqual({ action: "ignore" });
    expect(decide("just typing", true)).toEqual({ action: "ignore" });
    expect(decide("Pump\tLot\t2\t15000000", false)).toEqual({ action: "ignore" });
    expect(decide("Line one\n- bullet one", true)).toEqual({ action: "ignore" });
  });

  it("applies immediately into a blank editor", () => {
    const decision = decide(buildQuotationCopy(makeArchived()), false);
    expect(decision.action).toBe("apply");
    if (decision.action !== "apply") return;
    expect(decision.payload.source.quoteNumber).toBe("1502260059");
  });

  it("asks for confirmation when the editor already has content", () => {
    expect(decide(buildQuotationCopy(makeArchived()), true).action).toBe("confirm");
  });

  it("refuses an unknown customer before any confirm step", () => {
    const archived = makeArchived({ CustomerCode: 99 });
    expect(decide(buildQuotationCopy(archived), true)).toEqual({
      action: "refuse-unknown-customer",
      customerCode: 99,
    });
  });
});

describe("paste currency gate", () => {
  function decideWith(text: string | null, dirty: boolean, liveCurrency: string) {
    return resolveCopyPasteText(text, { dirty, customerOptions: CUSTOMERS, liveCurrency });
  }

  it("fills when the payload currency matches the live currency", () => {
    const text = buildQuotationCopy(makeArchived());
    expect(decideWith(text, false, "IDR").action).toBe("apply");
    expect(decideWith(text, true, "IDR").action).toBe("confirm");
  });

  it("accepts a case-insensitive currency match", () => {
    expect(decideWith(buildQuotationCopy(makeArchived({ Currency: "idr" })), false, "IDR").action).toBe(
      "apply"
    );
    expect(decideWith(buildQuotationCopy(makeArchived({ Currency: "Idr" })), true, "IDR").action).toBe(
      "confirm"
    );
  });

  it("refuses a mismatched currency before any confirm step, naming both codes", () => {
    const text = buildQuotationCopy(makeArchived({ Currency: "USD" }));
    expect(decideWith(text, true, "IDR")).toEqual({
      action: "refuse-currency-mismatch",
      payloadCurrency: "USD",
      liveCurrency: "IDR",
    });
    expect(currencyMismatchMessage("USD", "IDR")).toBe(
      "This quotation was issued in USD; the editor is set to IDR. " +
        "Change the company currency or copy from a matching revision."
    );
  });

  it("refuses a marked payload with no currency", () => {
    const payload = JSON.parse(buildQuotationCopy(makeArchived()));
    const quotation = { ...payload.quotation };
    delete quotation.currency;
    const text = JSON.stringify({ ...payload, quotation });
    expect(decideWith(text, false, "IDR")).toEqual({ action: "refuse-missing-currency" });
    expect(missingCurrencyMessage()).toBe(
      "This clipboard item has no currency and cannot be applied."
    );
  });

  it("still ignores ordinary text that is not a payload", () => {
    expect(decideWith("just typing", true, "IDR")).toEqual({ action: "ignore" });
    expect(decideWith("Pump\tLot\t2\t15000000", false, "IDR")).toEqual({ action: "ignore" });
  });
});

describe("copyQuotationToClipboard", () => {
  it("writes the exact payload text and reports success", async () => {
    const archived = makeArchived();
    const written: string[] = [];
    const toasts: Array<{ type: string; message: string }> = [];
    await copyQuotationToClipboard(
      archived,
      { writeText: async (text: string) => void written.push(text) },
      (type, message) => void toasts.push({ type, message })
    );
    expect(written).toEqual([buildQuotationCopy(archived)]);
    expect(toasts).toHaveLength(1);
    expect(toasts[0].type).toBe("success");
    expect(toasts[0].message).toContain("1502260059");
    expect(toasts[0].message).toContain("03");
  });

  it("reports failure without throwing when the clipboard refuses", async () => {
    const toasts: Array<{ type: string; message: string }> = [];
    await copyQuotationToClipboard(
      makeArchived(),
      {
        writeText: async () => {
          throw new Error("denied");
        },
      },
      (type, message) => void toasts.push({ type, message })
    );
    expect(toasts).toEqual([
      { type: "error", message: "We couldn't copy this quotation. Try again." },
    ]);
  });
});
