import { parseDraftPayload, countDraftItems, relativeSavedAt } from "./drafts";
import {
  DEFAULT_CURRENCY_CODE,
  currencyEditBlockMessage,
  normalizeCurrencyCode,
} from "./drafts";
import type { DetailsState } from "./components/DetailsPanel";
import type { EditorItem } from "./components/ItemEditor";

const DETAILS: DetailsState = {
  subject: "Roof replacement",
  recipientName: "PT Contoh",
  validity: 45,
  paymentTerm: 60,
  leadTimeYears: 1,
  leadTimeMonths: 2,
  leadTimeWeeks: 3,
  leadTimeDays: 4,
  discountPercent: 10,
  incoterm: "DAP",
  taxZone: "NonFTZ",
  taxType: "Goods",
  serviceCategory: "",
};

function sampleItems(): EditorItem[] {
  return [
    {
      key: 0,
      description: "Roof replacement",
      bullets: ["Old roof removed"],
      qty: "1",
      uom: "Lot",
      unitPrice: "25000000",
      marginEnabled: true,
      marginPercent: "15",
      discountEnabled: false,
      discountPercent: "0",
      subItems: [],
      showSubPrices: false,
    },
    {
      key: 1,
      description: "Control panel",
      bullets: [],
      qty: "1",
      uom: "Lot",
      unitPrice: "0",
      marginEnabled: false,
      marginPercent: "0",
      discountEnabled: true,
      discountPercent: "5",
      showSubPrices: true,
      subItems: [
        {
          id: 1,
          description: "Panel enclosure",
          qty: "2",
          uom: "Pc",
          unitCost: "3500000",
          marginEnabled: true,
          marginPercent: "20",
          discountEnabled: true,
          discountPercent: "10",
          bullets: ["Powder coated"],
        },
        {
          id: 2,
          description: "Wiring",
          qty: "1",
          uom: "LS",
          unitCost: "1200000",
          marginEnabled: false,
          marginPercent: "0",
          discountEnabled: false,
          discountPercent: "0",
          bullets: [],
        },
      ],
    },
  ];
}

function roundTrip(items: EditorItem[], details = DETAILS, customerId = 7) {
  return parseDraftPayload(JSON.stringify({ items, details, customerId }));
}

describe("draft payloads", () => {
  it("restores every editor field exactly", () => {
    const restored = roundTrip(sampleItems());
    expect(restored).not.toBeNull();
    expect(restored!.items).toHaveLength(2);
    expect(restored!.items[0]).toMatchObject({
      description: "Roof replacement",
      bullets: ["Old roof removed"],
      qty: "1",
      uom: "Lot",
      unitPrice: "25000000",
      marginEnabled: true,
      marginPercent: "15",
      discountEnabled: false,
      discountPercent: "0",
    });
    expect(restored!.items[1].showSubPrices).toBe(true);
    expect(restored!.items[1].subItems).toHaveLength(2);
    expect(restored!.items[1].subItems[0]).toMatchObject({
      description: "Panel enclosure",
      qty: "2",
      uom: "Pc",
      unitCost: "3500000",
      marginEnabled: true,
      marginPercent: "20",
      discountEnabled: true,
      discountPercent: "10",
      bullets: ["Powder coated"],
    });
    expect(restored!.details).toEqual(DETAILS);
    expect(restored!.customerId).toBe(7);
  });

  it("reissues row keys so they never collide with the live editor", () => {
    // The payload stores whatever keys the editor happened to be using. Two
    // rows saved under the same key, or keys left over from another
    // quotation, would otherwise edit each other when restored.
    const collided = sampleItems();
    collided[0].key = 5;
    collided[1].key = 5;
    const restored = roundTrip(collided);
    expect(restored!.items.map((item) => item.key)).toEqual([0, 1]);
  });

  it("reissues sub-item ids per line without colliding", () => {
    // Ids only need to be unique within a line, and the editor addresses a
    // sub-item by (line key, sub id) -- so per-line numbering is correct and a
    // shared counter would be wrong.
    const shared = sampleItems();
    shared[1].subItems[1].id = 1;
    const restored = roundTrip(shared);
    expect(restored!.items[1].subItems.map((sub) => sub.id)).toEqual([1, 2]);
  });

  it("fills in defaults for fields an older build did not write", () => {
    // A draft written by a previous version must still open into something
    // usable. Missing fields fall back to what a new row would carry -- which
    // is how a draft saved before per-row discounts existed reads back as "no
    // discount on any row".
    const restored = parseDraftPayload(
      JSON.stringify({ items: [{ description: "Legacy line" }], customerId: 3 })
    );
    expect(restored).not.toBeNull();
    expect(restored!.items[0]).toMatchObject({
      description: "Legacy line",
      qty: "1",
      uom: "Lot",
      marginPercent: "0",
      discountEnabled: false,
      discountPercent: "0",
    });
    expect(restored!.items[0].bullets).toEqual([]);
    expect(restored!.items[0].subItems).toEqual([]);
    expect(restored!.details).toMatchObject({
      validity: 30,
      paymentTerm: 90,
      leadTimeYears: 0,
      leadTimeMonths: 0,
      leadTimeWeeks: 0,
      leadTimeDays: 0,
      incoterm: "EXW",
      taxZone: "FTZ",
      taxType: "Goods",
    });
    expect(restored!.customerId).toBe(3);
  });

  it("maps a pre-unit draft's single lead time into the days segment", () => {
    // Drafts written before per-unit lead times carried one days-only figure
    // under `leadTime`. It restores into days so old work keeps its duration;
    // the other segments read as unstated, and the legacy key is dropped
    // rather than re-saved alongside the new shape.
    const restored = parseDraftPayload(
      JSON.stringify({
        items: [],
        details: { subject: "Legacy", leadTime: 90 },
        customerId: 3,
      })
    );
    expect(restored).not.toBeNull();
    expect(restored!.details.leadTimeYears).toBe(0);
    expect(restored!.details.leadTimeMonths).toBe(0);
    expect(restored!.details.leadTimeWeeks).toBe(0);
    expect(restored!.details.leadTimeDays).toBe(90);
    expect("leadTime" in restored!.details).toBe(false);
  });

  it("prefers the four segments over a stale legacy lead time", () => {
    // A draft carrying both (saved across the migration) restores the units
    // as entered; the legacy figure must not overwrite the days segment.
    const restored = parseDraftPayload(
      JSON.stringify({
        items: [],
        details: {
          leadTimeYears: 2,
          leadTimeMonths: 10,
          leadTimeWeeks: 4,
          leadTimeDays: 5,
          leadTime: 90,
        },
        customerId: 3,
      })
    );
    expect(restored).not.toBeNull();
    expect(restored!.details).toMatchObject({
      leadTimeYears: 2,
      leadTimeMonths: 10,
      leadTimeWeeks: 4,
      leadTimeDays: 5,
    });
    expect("leadTime" in restored!.details).toBe(false);
  });

  it("ignores fields of the wrong type rather than storing them", () => {
    // The editor controls are bound to strings and selects. A number where a
    // string belongs would render as nothing at all and fail on export.
    const restored = parseDraftPayload(
      JSON.stringify({
        items: [
          {
            description: 42,
            qty: null,
            marginEnabled: "yes",
            discountPercent: 15,
            bullets: "not an array",
            subItems: "not an array",
          },
        ],
      })
    );
    expect(restored!.items[0].description).toBe("");
    expect(restored!.items[0].qty).toBe("1");
    expect(restored!.items[0].marginEnabled).toBe(false);
    expect(restored!.items[0].discountPercent).toBe("0");
    expect(restored!.items[0].bullets).toEqual([]);
    expect(restored!.items[0].subItems).toEqual([]);
  });

  it("refuses a payload that is not an object", () => {
    // Returning null is what lets the editor say so, rather than silently
    // blanking the quotation the user was about to keep typing into.
    expect(parseDraftPayload("")).toBeNull();
    expect(parseDraftPayload("not json at all")).toBeNull();
    expect(parseDraftPayload("null")).toBeNull();
    expect(parseDraftPayload("42")).toBeNull();
    expect(parseDraftPayload('"a string"')).toBeNull();
    expect(parseDraftPayload("[1,2,3]")).toBeNull();
  });

  it("opens a draft that has nothing in it yet", () => {
    // The common case: someone hit save on an empty editor.
    const restored = parseDraftPayload("{}");
    expect(restored).not.toBeNull();
    expect(restored!.items).toEqual([]);
    expect(restored!.customerId).toBe(0);
  });
});

describe("countDraftItems", () => {
  it("counts rows worth listing and ignores blank ones", () => {
    const blank: EditorItem = {
      key: 0,
      description: "   ",
      bullets: ["  "],
      qty: "1",
      uom: "Lot",
      unitPrice: "",
      marginEnabled: false,
      marginPercent: "0",
      discountEnabled: false,
      discountPercent: "0",
      subItems: [],
      showSubPrices: false,
    };
    const items = sampleItems();
    expect(countDraftItems([blank])).toBe(0);
    expect(countDraftItems([...items, blank])).toBe(2);
  });

  it("counts a line whose only content is its sub-items", () => {
    // A split line often has an empty main description: everything real is in
    // the parts. It is still a line worth saving and listing.
    const items = sampleItems();
    items[0].description = "";
    items[0].bullets = [];
    expect(countDraftItems(items)).toBe(1);

    const withTitle = sampleItems();
    withTitle[0].description = "Control panel";
    withTitle[0].bullets = [];
    expect(countDraftItems(withTitle)).toBe(2);
  });
});

describe("relativeSavedAt", () => {
  const now = Date.now();
  afterEach(() => {
    vi.useRealTimers();
  });

  it("reads as recently for recent stamps", () => {
    vi.useFakeTimers();
    vi.setSystemTime(now);
    expect(relativeSavedAt(new Date(now - 5_000).toISOString())).toBe("just now");
    expect(relativeSavedAt(new Date(now - 60_000).toISOString())).toBe("1 minute ago");
    expect(relativeSavedAt(new Date(now - 3 * 3_600_000).toISOString())).toBe("3 hours ago");
    expect(relativeSavedAt(new Date(now - 2 * 86_400_000).toISOString())).toBe("2 days ago");
  });

  it("falls back to a date, and to nothing, rather than printing NaN", () => {
    vi.useFakeTimers();
    vi.setSystemTime(now);
    expect(relativeSavedAt(new Date(now - 90 * 86_400_000).toISOString())).toMatch(
      /202\d/
    );
    expect(relativeSavedAt("not a date")).toBe("");
    expect(relativeSavedAt("")).toBe("");
  });
});
describe("currencyEditBlockMessage", () => {
  it("lets a matching currency through on both paths", () => {
    expect(
      currencyEditBlockMessage("IDR", "IDR", { kind: "draft", name: "Roof" })
    ).toBeNull();
    expect(
      currencyEditBlockMessage("usd", "USD", {
        kind: "revision",
        quoteNumber: "1502250007",
        revision: 3,
      })
    ).toBeNull();
  });

  it("blocks a mismatched draft naming both currencies", () => {
    const message = currencyEditBlockMessage("IDR", "USD", {
      kind: "draft",
      name: "Roof works",
    });
    expect(message).not.toBeNull();
    // Both codes, so the reader knows which stored currency contradicts
    // which live one -- and raw codes only, never a formatted figure.
    expect(message!).toContain("IDR");
    expect(message!).toContain("USD");
    expect(message!).toContain("Roof works");
    expect(message!).toContain("cannot be opened");
    expect(message!).toContain("Switch the company currency back to IDR");
  });

  it("blocks a mismatched revision naming both currencies", () => {
    const message = currencyEditBlockMessage("USD", "IDR", {
      kind: "revision",
      quoteNumber: "1502250007",
      revision: 3,
    });
    expect(message).not.toBeNull();
    expect(message!).toContain("USD");
    expect(message!).toContain("IDR");
    expect(message!).toContain("1502250007");
    expect(message!).toContain("03");
    expect(message!).toContain("cannot be edited");
    expect(message!).toContain("Switch the company currency back to USD");
  });

  it("reads a missing code as the default rather than blocking", () => {
    // A draft or revision that predates the currency column carries nothing;
    // that is the default it was written in, not a mismatch.
    expect(
      currencyEditBlockMessage("", "IDR", { kind: "draft", name: "" })
    ).toBeNull();
    expect(
      currencyEditBlockMessage(undefined, "IDR", {
        kind: "revision",
        quoteNumber: "1502250007",
        revision: 0,
      })
    ).toBeNull();
    expect(DEFAULT_CURRENCY_CODE).toBe("IDR");
  });

  it("normalises case and whitespace before comparing", () => {
    expect(
      currencyEditBlockMessage(" usd ", "USD", { kind: "draft", name: "x" })
    ).toBeNull();
    expect(normalizeCurrencyCode(" idr")).toBe("IDR");
    expect(normalizeCurrencyCode(undefined)).toBe("IDR");
  });

  it("names an unnamed draft without leaving a blank", () => {
    const message = currencyEditBlockMessage("IDR", "USD", {
      kind: "draft",
      name: "   ",
    });
    expect(message!).toContain("This draft was saved in IDR");
  });
});
