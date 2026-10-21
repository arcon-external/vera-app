// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { CurrencyInfo } from "../format";
import type {
  PurchaseOrderExtractionResultDTO,
  PurchaseOrderExtractionRowDTO,
} from "../poExtraction";
import type { PurchaseOrderHeader, PurchaseOrderLine } from "../purchaseOrder";
import { PurchaseOrderExtractionPanelView } from "./PurchaseOrderExtractionPanel";

// Issue #477: the extraction review's interactions. This is the runtime half
// the pure test cannot reach: which control is enabled, what a checkbox sends
// to the selection, and that Apply is unreachable with nothing ticked.

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

const USD: CurrencyInfo = {
  code: "USD",
  exponent: 2,
  symbol: "$",
  position: "before",
  groupSep: ",",
  decimalSep: ".",
  symbolSpaced: false,
};

function cell(value: string, status: string, page: number | null = 1) {
  return {
    Value: value,
    Raw: value,
    Status: status,
    Page: page,
    BBox: page === null ? null : { X: 86, Y: 82, Width: 56, Height: 9 },
    Confidence: null,
    Note: "",
  };
}

function row(index: number): PurchaseOrderExtractionRowDTO {
  return {
    LineNumber: cell(String(index + 1), "extracted"),
    Description: cell(`Extracted line ${index + 1}`, "extracted"),
    Quantity: cell("2", "extracted"),
    UoM: cell("Pc", "extracted"),
    UnitPrice: cell("1250.00", "extracted"),
    Amount: cell("2500.00", "extracted"),
    Status: "extracted",
    Page: 1,
    BBox: { X: 50, Y: 225, Width: 490, Height: 10 },
    Note: "",
  };
}

function result(): PurchaseOrderExtractionResultDTO {
  return {
    Engine: "poppler",
    EngineVersion: "25.03.0",
    PageCount: 1,
    PagesWithText: 1,
    PagesWithoutText: 0,
    PagesWithOCR: 0,
    DurationMs: 20,
    SchemaVersion: 2,
    Pages: [{ Number: 1, Width: 595, Height: 842 }],
    PageModes: [
      { Number: 1, Mode: "text", TextWords: 20, TextChars: 180, OCRWords: 0, DPI: 0, Confidence: null },
    ],
    Fields: [
      { Key: "poNumber", Cell: cell("4500123456", "extracted") },
      { Key: "poDate", Cell: cell("2026-10-12", "extracted") },
      { Key: "requestedDeliveryDate", Cell: cell("", "absent", null) },
      { Key: "currency", Cell: cell("USD", "extracted") },
      { Key: "paymentTerms", Cell: cell("30 days after delivery", "needs_review") },
      { Key: "incoterms", Cell: cell("CIF Batam", "extracted") },
    ],
    Rows: [row(0), row(1)],
    Abstentions: { Fields: 2, Rows: 0 },
  };
}

function emptyHeader(): PurchaseOrderHeader {
  return {
    billingAddress: "",
    deliveryAddress: "",
    shipToCode: "",
    contactEmail: "",
    contactPhone: "",
    poNumber: "",
    poDate: "",
    receivedDate: "2026-10-02",
    requestedDeliveryDate: "",
    paymentTerms: "",
    incoterms: "",
    discountPercent: "0",
    adjustment: "",
    adjustmentReason: "",
  };
}

function lines(): PurchaseOrderLine[] {
  return [
    {
      key: 0,
      description: "Quotation line",
      quantity: "1",
      uom: "Lot",
      unitPrice: "$100.00",
      discountEnabled: false,
      discountPercent: "0",
      quotationItemNo: 1,
      quotationSubNo: null,
    },
  ];
}

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

function button(label: string): HTMLButtonElement {
  const found = Array.from(container.querySelectorAll("button")).find(
    (candidate) => candidate.getAttribute("aria-label") === label || candidate.textContent === label
  );
  if (!(found instanceof HTMLButtonElement)) {
    throw new Error(`button ${label} not found`);
  }
  return found;
}

function checkbox(label: string): HTMLInputElement {
  const found = container.querySelector(`input[aria-label="${label}"]`);
  if (!(found instanceof HTMLInputElement)) {
    throw new Error(`checkbox ${label} not found`);
  }
  return found;
}

describe("extraction review panel", () => {
  it("shows proposals with status, evidence and a relative region", () => {
    act(() =>
      root.render(
        <PurchaseOrderExtractionPanelView
          result={result()}
          header={emptyHeader()}
          lines={lines()}
          currency={USD}
          selection={{}}
          onToggle={() => undefined}
          onApply={() => undefined}
          onDiscard={() => undefined}
        />
      )
    );
    const text = container.textContent ?? "";
    expect(text).toContain("Extracted from the customer PO PDF");
    expect(text).toContain("poppler 25.03.0");
    expect(text).toContain("4500123456");
    expect(text).toContain("Needs review");
    expect(text).toContain("Page 1 · 86–142 × 82–91 pt");
    expect(container.querySelectorAll('[data-testid="po-extraction-region"]').length).toBeGreaterThan(0);
    expect(text).toContain("2 of 6 header fields");
    expect(text).toContain("replaces the current 1 line");
  });

  it("disables absent and protected proposals and allows the rest", () => {
    act(() =>
      root.render(
        <PurchaseOrderExtractionPanelView
          result={result()}
          header={emptyHeader()}
          lines={lines()}
          currency={USD}
          selection={{}}
          onToggle={() => undefined}
          onApply={() => undefined}
          onDiscard={() => undefined}
        />
      )
    );
    expect(checkbox("Apply extracted Customer PO number").disabled).toBe(false);
    expect(checkbox("Apply extracted Requested delivery date").disabled).toBe(true);
    expect(checkbox("Apply extracted Currency").disabled).toBe(true);
    expect(checkbox("Apply extracted Payment terms").disabled).toBe(false);
    expect(checkbox("Apply extracted line 1").disabled).toBe(false);
  });

  it("reports a tick to the selection and enables Apply only when something is chosen", () => {
    const onToggle = vi.fn();
    const onApply = vi.fn();
    const render = (selection: Record<string, boolean>) =>
      act(() =>
        root.render(
          <PurchaseOrderExtractionPanelView
            result={result()}
            header={emptyHeader()}
            lines={lines()}
            currency={USD}
            selection={selection}
            onToggle={onToggle}
            onApply={onApply}
            onDiscard={() => undefined}
          />
        )
      );
    render({});
    expect(button("Apply selected").disabled).toBe(true);

    act(() => checkbox("Apply extracted Customer PO number").click());
    expect(onToggle).toHaveBeenCalledWith("header:poNumber", true);

    render({ "header:poNumber": true });
    const apply = Array.from(container.querySelectorAll("button")).find((candidate) =>
      candidate.textContent?.startsWith("Apply selected")
    );
    expect(apply?.textContent).toContain("1 field");
    act(() => apply?.click());
    expect(onApply).toHaveBeenCalled();
  });

  it("shows OCR provenance and low-confidence evidence without auto-applying", () => {
    const extraction = result();
    extraction.PageCount = 2;
    extraction.Pages = [
      { Number: 1, Width: 595, Height: 842 },
      { Number: 2, Width: 595, Height: 842 },
    ];
    extraction.PageModes = [
      { Number: 1, Mode: "text", TextWords: 20, TextChars: 180, OCRWords: 0, DPI: 0, Confidence: null },
      {
        Number: 2,
        Mode: "ocr",
        TextWords: 2,
        TextChars: 11,
        OCRWords: 24,
        DPI: 300,
        Confidence: 88,
        Reason: "below the text floor",
      },
    ];
    extraction.PagesWithoutText = 1;
    extraction.PagesWithOCR = 1;
    extraction.OCREngine = "tesseract";
    extraction.OCREngineVersion = "5.5.0";
    extraction.Rows[1] = {
      ...extraction.Rows[1]!,
      Quantity: {
        Value: "10.5",
        Raw: "10.5",
        Status: "needs_review",
        Page: 2,
        BBox: { X: 300, Y: 225, Width: 30, Height: 10 },
        Confidence: 55,
        Note: "low OCR confidence",
      },
      Page: 2,
      Status: "needs_review",
    };
    act(() =>
      root.render(
        <PurchaseOrderExtractionPanelView
          result={extraction}
          header={emptyHeader()}
          lines={lines()}
          currency={USD}
          selection={{}}
          onToggle={() => undefined}
          onApply={() => undefined}
          onDiscard={() => undefined}
        />
      )
    );
    const text = container.textContent ?? "";
    expect(text).toContain("OCR: tesseract 5.5.0");
    expect(text).toContain("Page 2: OCR at 300 DPI");
    expect(text).toContain("24 words");
    expect(text).toContain("88% mean confidence");
    expect(text).toContain("OCR confidence 55%");
    expect(text).toContain("Low OCR confidence: quantity 55%");
    // Evidence is never applied by itself: the low-confidence row starts
    // unticked and Apply stays disabled with nothing selected.
    expect(checkbox("Apply extracted line 2").checked).toBe(false);
    expect(button("Apply selected").disabled).toBe(true);
  });

  it("opens no apply path when nothing can be represented", () => {
    const extraction = result();
    extraction.Rows[0] = {
      ...extraction.Rows[0]!,
      Quantity: cell("abc", "needs_review"),
    };
    act(() =>
      root.render(
        <PurchaseOrderExtractionPanelView
          result={extraction}
          header={emptyHeader()}
          lines={lines()}
          currency={USD}
          selection={{ "row:0": true }}
          onToggle={() => undefined}
          onApply={() => undefined}
          onDiscard={() => undefined}
        />
      )
    );
    const apply = Array.from(container.querySelectorAll("button")).find((candidate) =>
      candidate.textContent?.startsWith("Apply selected")
    );
    expect(apply?.textContent).toBe("Apply selected");
    expect(checkbox("Apply extracted line 1").disabled).toBe(true);
  });
});
