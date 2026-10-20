import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import {
  PurchaseOrderDocumentPanelView,
  type PurchaseOrderDocumentPanelViewProps,
} from "./PurchaseOrderDocumentPanel";
import type { PurchaseOrderDocumentStateDTO } from "../purchaseOrderDocument";

// Issue #472: the attachment panel's assembled markup. The SSR pass holds the
// exact empty-state sentence, the version/history label, the disabled
// Extract and compare action with its stated reason, and the inline removal
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
    expect(html).not.toContain("View</button>");
    expect(html).not.toContain("Replace");
  });

  it("shows the active filename, size and version against the retained history", () => {
    const html = render({ state: stateWith(2) });
    expect(html).toContain("Customer PO 2.pdf");
    expect(html).toContain("Version 2 of 2");
    expect(html).toContain("1.5 KB");
    expect(html).toContain("View</button>");
    expect(html).toContain("Replace");
    expect(html).toContain("Remove</button>");
    expect(html).not.toContain("No customer PO PDF attached");
  });

  it("keeps Extract and compare visibly disabled and says why", () => {
    const empty = render();
    expect(empty).toContain("Extract and compare");
    expect(empty).toContain("disabled");
    expect(empty).toContain("Attach a customer PO PDF before extracting its data.");

    const attached = render({ state: stateWith(1) });
    expect(attached).toContain("Extraction is not available in this release.");
  });

  it("asks its own confirmation before removing the current PDF", () => {
    const html = render({ state: stateWith(1), pendingRemoval: true });
    expect(html).toContain('role="alertdialog"');
    expect(html).toContain("Remove the customer PO PDF?");
    expect(html).toContain("Keep PDF");
    expect(html).toContain("Remove PDF");
    expect(html).toContain("Earlier versions stay in the correction history");
  });

  it("records a deliberate omission on a saved record with no PDF", () => {
    const html = render({ absenceConfirmed: true });
    expect(html).toContain("The final save recorded this omission deliberately.");
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
