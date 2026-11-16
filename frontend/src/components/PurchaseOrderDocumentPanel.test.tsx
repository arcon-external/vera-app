import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import {
  PurchaseOrderDocumentPanelView,
  type PurchaseOrderDocumentPanelViewProps,
} from "./PurchaseOrderDocumentPanel";
import type { PurchaseOrderDocumentStateDTO } from "../purchaseOrderDocument";

// Issue #472: the attachment panel's assembled markup. The SSR pass holds the
// exact empty-state sentence, the version/history label, and the inline removal
// confirmation. Interactions (picker, view, remove) live in the DOM suite.

function stateWith(version: number | null): PurchaseOrderDocumentStateDTO {
  const versions = version === null ? [1] : Array.from({ length: version }, (_, index) => version - index);
  return {
    Document:
      version === null
        ? null
        : {
            UUID: `doc-${version}`,
            SHA256: `sha-${version}`,
            ContentType: "application/pdf",
            OriginalFilename: `Customer PO ${version}.pdf`,
            UploadedAt: "2026-10-21T10:00:00Z",
            ID: version,
            PurchaseOrderID: 7,
            Version: version,
            SizeBytes: 1536,
            UploadedBy: 4,
            Active: true,
          },
    Versions: versions.map((item) => ({
      UUID: `doc-${item}`,
      SHA256: `sha-${item}`,
      ContentType: "application/pdf",
      OriginalFilename: `Customer PO ${item}.pdf`,
      UploadedAt: "2026-10-21T10:00:00Z",
      ID: item,
      PurchaseOrderID: 7,
      Version: item,
      SizeBytes: 1536,
      UploadedBy: 4,
      Active: item === version,
    })),
  };
}

function render(overrides: Partial<PurchaseOrderDocumentPanelViewProps> = {}): string {
  return renderToString(
    <PurchaseOrderDocumentPanelView
      state={stateWith(null)}
      onChooseFile={() => undefined}
      onView={() => undefined}
      onRemove={() => undefined}
      onConfirmRemove={() => undefined}
      onCancelRemove={() => undefined}
      {...overrides}
    />
  );
}

describe("customer PO PDF panel", () => {
  it("states the no-attachment condition exactly and offers Attach", () => {
    const html = render();
    expect(html).toContain("No customer PO PDF attached");
    expect(html).toContain("Attach");
    // Earlier versions are document versions, never correction history.
    expect(html).toContain("1 earlier document version is kept.");
    expect(html).not.toContain("correction history");
    expect(html).not.toContain("View</button>");
    expect(html).not.toContain("Replace");
  });

  it("shows the active filename, size, uploader and version against the retained versions", () => {
    const html = render({ state: stateWith(2) });
    expect(html).toContain("Customer PO 2.pdf");
    expect(html).toContain("Version 2 of 2");
    expect(html).toContain("1.5 KB");
    // The DTO carries only the uploader id; no name is invented.
    expect(html).toContain("user #4");
    expect(html).toContain("View</button>");
    expect(html).toContain("Replace");
    expect(html).toContain("Remove</button>");
    expect(html).not.toContain("No customer PO PDF attached");
  });

  it("lists earlier versions as metadata only, and omits the list when none exist", () => {
    const html = render({ state: stateWith(2) });
    expect(html).toContain("<details");
    expect(html).toContain("Earlier versions (1)");
    const start = html.indexOf("<details");
    expect(start).toBeGreaterThanOrEqual(0);
    const earlier = html.slice(start, html.indexOf("</details>", start));
    expect(earlier).toContain("Version 1");
    expect(earlier).toContain("1.5 KB");
    expect(earlier).toContain("2026-10-21");
    // No content control inside the disclosure: the list never fetches bytes.
    expect(earlier).not.toContain("<button");

    const only = render({ state: stateWith(1) });
    expect(only).not.toContain("<details");
    expect(only).not.toContain("Earlier versions");
  });

  it("asks its own confirmation before removing the current PDF", () => {
    const html = render({ state: stateWith(1), pendingRemoval: true });
    expect(html).toContain('role="alertdialog"');
    expect(html).toContain("Remove the customer PO PDF?");
    expect(html).toContain("Keep PDF");
    expect(html).toContain("Remove PDF");
    // Not correction history: these are versions of the document.
    expect(html).toContain("Earlier document versions are kept");
    expect(html).not.toContain("correction history");
  });

  it("records a deliberate omission on a saved record with no PDF", () => {
    // `savedRecord` alone implies the compact card; the pairing cannot drift.
    const html = render({ absenceConfirmed: true, savedRecord: true });
    expect(html).toContain("The final save recorded this omission deliberately.");
    // #610: the saved record makes the later attachment the normal next step,
    // and states that it is a document change rather than a correction.
    expect(html).toContain("Attach it later; this does not create a correction.");
    // The compact header's Optional pill, not the draft card's heading.
    expect(html).toContain(">Optional<");
  });

  it("keeps the draft card's document copy unchanged", () => {
    const html = render({ compact: true });
    expect(html).toContain("One original document, kept unchanged.");
    expect(html).not.toContain("Attach it later");
  });

  it("renders the content preview when the caller has one", () => {
    const html = render({
      state: stateWith(1),
      preview: { url: "blob:test", filename: "Customer PO 1.pdf" },
      onClosePreview: () => undefined,
    });
    expect(html).toContain('title="Customer PO PDF preview"');
    expect(html).toContain("blob:test");
    expect(html).toContain("Close preview");
  });
});
