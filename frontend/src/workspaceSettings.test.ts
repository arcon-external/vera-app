import { describe, expect, it, vi } from "vitest";
import {
  DEFAULT_PURCHASE_ORDER_SETTINGS,
  DEFAULT_QUOTATION_SETTINGS,
  DEFAULT_WORKSPACE_SETTINGS,
  MAX_PO_LIST_PAGE_SIZE,
  MAX_QUOTATION_DAYS,
  MAX_SUBJECT_LENGTH,
  applyPurchaseOrderDefaults,
  clearPurchaseOrderSettings,
  clearQuotationSettings,
  purchaseOrderSettingsKey,
  quotationDefaultsFromSettings,
  quotationSettingsKey,
  readWorkspaceSettings,
  todayWIB,
  writePurchaseOrderSettings,
  writeQuotationSettings,
  type SettingsStorage,
} from "./workspaceSettings";
import {
  MAX_PAYMENT_TERMS_LENGTH,
  type PurchaseOrderDTO,
} from "./purchaseOrder";

// Issue #492: per-user, device-local Quotation and PO defaults. Every case
// drives the module's public interface -- storage in, effective settings out --
// so a change of storage key, record shape or validation rule has to keep the
// same promises. No React is imported here on purpose: the module is pure and
// the pages consume it.

function memoryStorage(): SettingsStorage & { keys(): string[] } {
  const map = new Map<string, string>();
  return {
    getItem: (key) => map.get(key) ?? null,
    setItem: (key, value) => {
      map.set(key, value);
    },
    removeItem: (key) => {
      map.delete(key);
    },
    keys: () => [...map.keys()],
  };
}

function blankOrder(): PurchaseOrderDTO {
  return {
    PODate: null,
    RequestedDeliveryDate: null,
    UUID: "uuid-blank",
    State: "draft",
    QuoteNumber: "1502260012",
    CustomerName: "PT. Infineon Technologies Batam",
    BillingAddress: "Batamindo Industrial Park",
    DeliveryAddress: "",
    ShipToCode: "",
    ContactEmail: "",
    ContactPhone: "",
    PONumber: "",
    ReceivedDate: "",
    Incoterms: "",
    Currency: "IDR",
    TaxZone: "FTZ",
    TaxCategory: "SupplyOnly",
    AdjustmentReason: "",
    CreatedAt: "2026-10-22T03:00:00Z",
    UpdatedAt: "2026-10-22T03:00:00Z",
    Items: [],
    ID: 5,
    QuotationID: 12,
    SourceRevision: 0,
    CustomerID: 2,
    PaymentTerms: "",
    DiscountPercent: 0,
    PpnBps: 0,
    SubtotalMinor: 0,
    DiscountMinor: 0,
    PpnMinor: 0,
    AdjustmentMinor: 0,
    GrandTotalMinor: 0,
    PrintedSubtotalMinor: 0,
    PrintedDiscountMinor: 0,
    PrintedPpnMinor: 0,
    PrintedGrandTotalMinor: 0,
    Version: 1,
  };
}

describe("storage round trip", () => {
  it("returns the built-in defaults when nobody is signed in, without touching storage", () => {
    const storage = memoryStorage();
    const getItem = vi.spyOn(storage, "getItem");
    expect(readWorkspaceSettings(storage, null)).toEqual(DEFAULT_WORKSPACE_SETTINGS);
    expect(readWorkspaceSettings(null, 7)).toEqual(DEFAULT_WORKSPACE_SETTINGS);
    expect(getItem).not.toHaveBeenCalled();
  });

  it("keeps a written section and reads it back", () => {
    const storage = memoryStorage();
    const quotation = {
      ...DEFAULT_QUOTATION_SETTINGS,
      validityDays: 45,
      paymentTermDays: 30,
      taxZone: "NonFTZ" as const,
      taxType: "Services" as const,
      serviceCategory: "Construction" as const,
      subject: "Turbine overhaul",
    };
    writeQuotationSettings(storage, 7, quotation);
    expect(readWorkspaceSettings(storage, 7).quotation).toEqual(quotation);
    // The PO section was never written and stays at its defaults.
    expect(readWorkspaceSettings(storage, 7).purchaseOrder).toEqual(
      DEFAULT_PURCHASE_ORDER_SETTINGS
    );
  });

  it("isolates two accounts on one device", () => {
    const storage = memoryStorage();
    writeQuotationSettings(storage, 7, {
      ...DEFAULT_QUOTATION_SETTINGS,
      subject: "Seven",
    });
    writeQuotationSettings(storage, 8, {
      ...DEFAULT_QUOTATION_SETTINGS,
      subject: "Eight",
    });
    expect(readWorkspaceSettings(storage, 7).quotation.subject).toBe("Seven");
    expect(readWorkspaceSettings(storage, 8).quotation.subject).toBe("Eight");
    expect(quotationSettingsKey(7)).not.toBe(quotationSettingsKey(8));
  });

  it("keeps the two sections independent", () => {
    const storage = memoryStorage();
    writeQuotationSettings(storage, 7, {
      ...DEFAULT_QUOTATION_SETTINGS,
      subject: "Quotation only",
    });
    writePurchaseOrderSettings(storage, 7, {
      ...DEFAULT_PURCHASE_ORDER_SETTINGS,
      paymentTerms: "50% advance",
    });
    expect(readWorkspaceSettings(storage, 7).quotation.subject).toBe("Quotation only");
    expect(readWorkspaceSettings(storage, 7).purchaseOrder.paymentTerms).toBe(
      "50% advance"
    );
  });

  it("clears one section back to the defaults and leaves the other alone", () => {
    const storage = memoryStorage();
    writeQuotationSettings(storage, 7, {
      ...DEFAULT_QUOTATION_SETTINGS,
      validityDays: 7,
    });
    writePurchaseOrderSettings(storage, 7, {
      ...DEFAULT_PURCHASE_ORDER_SETTINGS,
      listPageSize: 30,
    });

    clearQuotationSettings(storage, 7);
    expect(storage.getItem(quotationSettingsKey(7))).toBeNull();
    expect(readWorkspaceSettings(storage, 7).quotation).toEqual(
      DEFAULT_QUOTATION_SETTINGS
    );
    expect(readWorkspaceSettings(storage, 7).purchaseOrder.listPageSize).toBe(30);

    clearPurchaseOrderSettings(storage, 7);
    expect(storage.getItem(purchaseOrderSettingsKey(7))).toBeNull();
    expect(readWorkspaceSettings(storage, 7)).toEqual(DEFAULT_WORKSPACE_SETTINGS);
  });

  it("does not write when signed out", () => {
    const storage = memoryStorage();
    writeQuotationSettings(storage, null, {
      ...DEFAULT_QUOTATION_SETTINGS,
      subject: "nobody",
    });
    writePurchaseOrderSettings(storage, null, DEFAULT_PURCHASE_ORDER_SETTINGS);
    expect(storage.keys()).toEqual([]);
  });
});

describe("corrupt or invalid storage falls back safely", () => {
  it("treats unreadable JSON as absent", () => {
    const storage = memoryStorage();
    storage.setItem(quotationSettingsKey(7), "{not json");
    storage.setItem(purchaseOrderSettingsKey(7), "[]");
    expect(readWorkspaceSettings(storage, 7)).toEqual(DEFAULT_WORKSPACE_SETTINGS);
  });

  it("ignores a record written by another version", () => {
    const storage = memoryStorage();
    storage.setItem(
      quotationSettingsKey(7),
      JSON.stringify({ version: 2, settings: { ...DEFAULT_QUOTATION_SETTINGS, validityDays: 45 } })
    );
    expect(readWorkspaceSettings(storage, 7).quotation.validityDays).toBe(30);
  });

  it("falls back field by field, never wholesale, for invalid values", () => {
    const storage = memoryStorage();
    storage.setItem(
      quotationSettingsKey(7),
      JSON.stringify({
        version: 1,
        settings: {
          validityDays: 0,
          paymentTermDays: 999,
          taxZone: "Moon",
          taxType: "Widgets",
          serviceCategory: "Construction",
          subject: "x".repeat(MAX_SUBJECT_LENGTH + 1),
        },
      })
    );
    storage.setItem(
      purchaseOrderSettingsKey(7),
      JSON.stringify({
        version: 1,
        settings: {
          paymentTerms: "x".repeat(MAX_PAYMENT_TERMS_LENGTH + 1),
          incoterms: "XYZ",
          receivedDate: "2026-02-30",
          listPageSize: 0,
        },
      })
    );

    expect(readWorkspaceSettings(storage, 7)).toEqual({
      quotation: DEFAULT_QUOTATION_SETTINGS,
      purchaseOrder: DEFAULT_PURCHASE_ORDER_SETTINGS,
    });
  });

  it("gives Services a category when the stored one is missing", () => {
    const storage = memoryStorage();
    storage.setItem(
      quotationSettingsKey(7),
      JSON.stringify({
        version: 1,
        settings: {
          ...DEFAULT_QUOTATION_SETTINGS,
          taxType: "Services",
          serviceCategory: "",
        },
      })
    );
    // The server refuses a service without a category, so an unreadable one
    // must not become a default every new quotation fails to Prepare.
    expect(readWorkspaceSettings(storage, 7).quotation.serviceCategory).toBe(
      "NonConstruction"
    );
  });

  it("drops a service category stored under Goods, as the editor does", () => {
    const storage = memoryStorage();
    storage.setItem(
      quotationSettingsKey(7),
      JSON.stringify({
        version: 1,
        settings: {
          ...DEFAULT_QUOTATION_SETTINGS,
          taxType: "Goods",
          serviceCategory: "Construction",
        },
      })
    );
    expect(readWorkspaceSettings(storage, 7).quotation.serviceCategory).toBe("");
  });

  it("accepts the bounds the editors enforce, and a 500-code-point payment term", () => {
    const storage = memoryStorage();
    writeQuotationSettings(storage, 7, {
      ...DEFAULT_QUOTATION_SETTINGS,
      validityDays: MAX_QUOTATION_DAYS,
      paymentTermDays: 1,
      subject: "x".repeat(MAX_SUBJECT_LENGTH),
    });
    writePurchaseOrderSettings(storage, 7, {
      ...DEFAULT_PURCHASE_ORDER_SETTINGS,
      paymentTerms: "😀".repeat(MAX_PAYMENT_TERMS_LENGTH),
      incoterms: "DDP",
      receivedDate: "2026-10-22",
      listPageSize: MAX_PO_LIST_PAGE_SIZE,
    });
    const read = readWorkspaceSettings(storage, 7);
    expect(read.quotation.validityDays).toBe(MAX_QUOTATION_DAYS);
    expect(read.quotation.paymentTermDays).toBe(1);
    expect(read.quotation.subject).toHaveLength(MAX_SUBJECT_LENGTH);
    // Code points, not UTF-16 units: 500 astral characters are 500, not 1000.
    expect(read.purchaseOrder.paymentTerms).toBe("😀".repeat(MAX_PAYMENT_TERMS_LENGTH));
    expect(read.purchaseOrder.incoterms).toBe("DDP");
    expect(read.purchaseOrder.receivedDate).toBe("2026-10-22");
    expect(read.purchaseOrder.listPageSize).toBe(MAX_PO_LIST_PAGE_SIZE);
  });
});

describe("quotationDefaultsFromSettings", () => {
  it("maps the settings onto the editor's details names", () => {
    expect(
      quotationDefaultsFromSettings({
        ...DEFAULT_QUOTATION_SETTINGS,
        validityDays: 45,
        paymentTermDays: 30,
        taxZone: "NonFTZ",
        taxType: "Services",
        serviceCategory: "NonConstruction",
        subject: "Turbine overhaul",
      })
    ).toEqual({
      subject: "Turbine overhaul",
      validity: 45,
      paymentTerm: 30,
      taxZone: "NonFTZ",
      taxType: "Services",
      serviceCategory: "NonConstruction",
    });
  });

  it("clears the service category for Goods", () => {
    expect(
      quotationDefaultsFromSettings({
        ...DEFAULT_QUOTATION_SETTINGS,
        taxType: "Goods",
        serviceCategory: "Construction",
      }).serviceCategory
    ).toBe("");
  });
});

describe("todayWIB", () => {
  it("is the calendar day in WIB, not the machine's UTC day", () => {
    // 18:30 UTC is 01:30 the next day in Jakarta (UTC+7, no DST).
    expect(todayWIB(new Date("2026-10-21T18:30:00Z"))).toBe("2026-10-22");
    expect(todayWIB(new Date("2026-10-21T16:59:59Z"))).toBe("2026-10-21");
  });
});

describe("applyPurchaseOrderDefaults", () => {
  const settings = {
    ...DEFAULT_PURCHASE_ORDER_SETTINGS,
    paymentTerms: "50% advance, balance 30 days after delivery",
    incoterms: "FOB",
    receivedDate: "2026-10-01",
    listPageSize: 25,
  };

  it("fills a blank manual draft from the settings", () => {
    const applied = applyPurchaseOrderDefaults(blankOrder(), settings);
    expect(applied.PaymentTerms).toBe(settings.paymentTerms);
    expect(applied.Incoterms).toBe("FOB");
    expect(applied.ReceivedDate).toBe("2026-10-01");
  });

  it("never overwrites a value the source supplied", () => {
    const sourced = {
      ...blankOrder(),
      PaymentTerms: "45 days",
      Incoterms: "CIF",
      ReceivedDate: "2026-09-30",
    };
    const applied = applyPurchaseOrderDefaults(sourced, settings);
    expect(applied.PaymentTerms).toBe("45 days");
    expect(applied.Incoterms).toBe("CIF");
    expect(applied.ReceivedDate).toBe("2026-09-30");
  });

  it("uses today in WIB when the received-date setting is blank", () => {
    const applied = applyPurchaseOrderDefaults(
      blankOrder(),
      { ...settings, receivedDate: "" },
      new Date("2026-10-21T18:30:00Z")
    );
    expect(applied.ReceivedDate).toBe("2026-10-22");
  });
});
