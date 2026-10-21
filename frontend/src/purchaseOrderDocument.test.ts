// Issue #472: pure decisions for the one optional customer PO PDF. The panel's
// markup and the final-save payload both read these, so the confirmation rules
// cannot diverge between what is shown and what is submitted.

import { describe, expect, it } from "vitest";
import {
  base64ToBytes,
  bytesToBase64,
  documentErrorMessage,
  documentStateOf,
  documentVersionLabel,
  extractAndCompareAvailability,
  finalSaveNeedsConfirmation,
  finalSaveSendsAbsenceConfirmation,
  formatDocumentSize,
  removalRequiresConfirmation,
  type PurchaseOrderDocumentStateDTO,
} from "./purchaseOrderDocument";

function stateWith(version: number | null, versions: number[]): PurchaseOrderDocumentStateDTO {
  return {
    Document:
      version === null
        ? null
        : {
            UUID: `doc-${version}`,
            SHA256: `sha-${version}`,
            ContentType: "application/pdf",
            OriginalFilename: `po-${version}.pdf`,
            UploadedAt: "2026-10-21T10:00:00Z",
            ID: version,
            PurchaseOrderID: 7,
            Version: version,
            SizeBytes: 1024 * version,
            UploadedBy: 4,
            Active: true,
          },
    Versions: versions.map((item) => ({
      UUID: `doc-${item}`,
      SHA256: `sha-${item}`,
      ContentType: "application/pdf",
      OriginalFilename: `po-${item}.pdf`,
      UploadedAt: "2026-10-21T10:00:00Z",
      ID: item,
      PurchaseOrderID: 7,
      Version: item,
      SizeBytes: 1024 * item,
      UploadedBy: 4,
      Active: item === version,
    })),
  };
}

describe("customer PO document state", () => {
  it("distinguishes no attachment from an attached PDF", () => {
    expect(documentStateOf(null)).toBe("none");
    expect(documentStateOf(stateWith(null, []))).toBe("none");
    expect(documentStateOf(stateWith(1, [1]))).toBe("attached");
  });

  it("labels the active version against the retained history", () => {
    expect(documentVersionLabel(stateWith(null, []))).toBe(
      "No customer PO PDF attached"
    );
    expect(documentVersionLabel(stateWith(2, [2, 1]))).toBe("Version 2 of 2");
    expect(documentVersionLabel(stateWith(1, [1]))).toBe("Version 1 of 1");
  });

  it("requires its own confirmation before removing the current PDF", () => {
    expect(removalRequiresConfirmation(stateWith(1, [1]))).toBe(true);
    expect(removalRequiresConfirmation(stateWith(null, [1]))).toBe(false);
    expect(removalRequiresConfirmation(null)).toBe(false);
  });

  it("needs the absence confirmation only when no PDF is attached", () => {
    expect(finalSaveNeedsConfirmation("none", false)).toBe(true);
    expect(finalSaveNeedsConfirmation("none", true)).toBe(false);
    expect(finalSaveNeedsConfirmation("attached", false)).toBe(false);
    expect(finalSaveNeedsConfirmation("attached", true)).toBe(false);
  });

  it("never sends an absence confirmation for an attached PDF", () => {
    expect(finalSaveSendsAbsenceConfirmation("none", true)).toBe(true);
    expect(finalSaveSendsAbsenceConfirmation("none", false)).toBe(false);
    expect(finalSaveSendsAbsenceConfirmation("attached", true)).toBe(false);
    expect(finalSaveSendsAbsenceConfirmation("attached", false)).toBe(false);
  });
});

describe("Extract and compare availability", () => {
  it("is honestly disabled without a PDF", () => {
    const availability = extractAndCompareAvailability("none");
    expect(availability.enabled).toBe(false);
    expect(availability.reason).toContain("Attach a customer PO PDF");
  });

  it("offers extraction with a PDF when the editor can apply proposals", () => {
    const availability = extractAndCompareAvailability("attached");
    expect(availability.enabled).toBe(true);
    expect(availability.hint).toContain("text layer");
  });

  it("explains the correction path on a read-only saved view", () => {
    const availability = extractAndCompareAvailability("attached", false);
    expect(availability.enabled).toBe(false);
    expect(availability.reason).toContain("Correct this record");
  });
});

describe("document validation messages", () => {
  it("asks for an unlocked copy of an encrypted PDF and never mentions a password field", () => {
    const message = documentErrorMessage(
      new Error("the customer PO PDF is password-protected; an unlocked copy is required")
    );
    expect(message).toBe(
      "This PDF is password-protected. Attach an unlocked copy; Vera does not store PDF passwords."
    );
    expect(message).not.toContain("enter the password");
  });

  it("names the upload bound", () => {
    expect(
      documentErrorMessage(new Error("the customer PO PDF is larger than 20 MB"))
    ).toBe("This PDF is larger than 20 MB. Choose a smaller file.");
  });

  it("names an invalid PDF", () => {
    expect(
      documentErrorMessage(new Error("the customer PO document is not a valid PDF: it has no trailer"))
    ).toBe("This file is not a valid PDF. Choose a PDF that opens in a PDF reader.");
  });

  it("names an unreadable base64 payload", () => {
    expect(
      documentErrorMessage(new Error("the customer PO PDF content is not valid base64"))
    ).toBe("The attached PDF could not be opened. Attach it again.");
  });

  it("keeps an unknown server message rather than hiding it", () => {
    expect(documentErrorMessage(new Error("purchase order operation failed"))).toBe(
      "purchase order operation failed"
    );
    expect(documentErrorMessage(null)).toBe("The customer PO PDF could not be updated. Try again.");
  });
});

describe("document size formatting", () => {
  it("uses binary units with one decimal and no float arithmetic", () => {
    expect(formatDocumentSize(0)).toBe("0 B");
    expect(formatDocumentSize(900)).toBe("900 B");
    expect(formatDocumentSize(1024)).toBe("1 KB");
    expect(formatDocumentSize(1536)).toBe("1.5 KB");
    expect(formatDocumentSize(1024 * 1024)).toBe("1 MB");
    expect(formatDocumentSize(1024 * 1024 + 524288)).toBe("1.5 MB");
    expect(formatDocumentSize(20 * 1024 * 1024)).toBe("20 MB");
  });
});

// Problem: Wails serializes a Go []byte as base64 text, while the generated
// TypeScript said number[]; the panel therefore previewed zero bytes. The
// binding now carries an explicit base64 string, and these helpers are the only
// encode/decode path the panel uses.
//
// The literal below is the exact value a Go test pins:
// backend/po_documents_http_test.go's TestPurchaseOrderDocumentContentWireShape
// marshals %PDF-1.7\n and requires "JVBERi0xLjcK".
describe("base64 binding codec", () => {
  const pdfHeader = new TextEncoder().encode("%PDF-1.7\n");

  it("decodes the wire value the Go binding emits to the exact PDF bytes", () => {
    expect(Array.from(base64ToBytes("JVBERi0xLjcK"))).toEqual(Array.from(pdfHeader));
    expect(new TextDecoder().decode(base64ToBytes("JVBERi0xLjcK"))).toBe("%PDF-1.7\n");
  });

  it("encodes the exact bytes the Go side decodes", () => {
    expect(bytesToBase64(pdfHeader)).toBe("JVBERi0xLjcK");
    expect(bytesToBase64(new Uint8Array(0))).toBe("");
  });

  it("handles padding for every remainder", () => {
    expect(Array.from(base64ToBytes("AA=="))).toEqual([0]);
    expect(Array.from(base64ToBytes("AAA="))).toEqual([0, 0]);
    expect(Array.from(base64ToBytes("AAEC"))).toEqual([0, 1, 2]);
  });

  it("round-trips every byte value", () => {
    const all = new Uint8Array(256);
    for (let value = 0; value < 256; value += 1) all[value] = value;
    expect(Array.from(base64ToBytes(bytesToBase64(all)))).toEqual(Array.from(all));
  });

  it("refuses malformed base64 instead of silently returning nothing", () => {
    expect(() => base64ToBytes("JVBERi0xLjc")).toThrow();
    expect(() => base64ToBytes("JVBERi0xLjc$")).toThrow();
    expect(() => base64ToBytes("J=BERi0xLjcK")).toThrow();
  });
});
