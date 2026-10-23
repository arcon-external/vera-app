import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { ArchivedQuotation } from "../archive";
import {
  QuotationDetailHeader,
  type QuotationDetailHeaderProps,
} from "./QuotationDetailPage";

// Issue #499: the issued-Quotation detail action. It is no longer a second
// entry point into the general selector -- it acts on the revision on screen.
// The header is rendered without a DOM (the page fetches in an effect), so
// these pins prove the three labels exist exclusively, and that the action is
// disabled while the occupancy answer is pending or the revision is switching.
// The click-through lives in quotationDetailPOApp.dom.test.tsx.

const QUOTATION = {
  QuoteNumber: "1502260001",
  RevisionNumber: 1,
  AvailableRevisions: [1, 0],
  CustomerName: "PT. Infineon Technologies Batam",
  Subject: "Electrical installation",
} as ArchivedQuotation;

const noop = () => {};

function buttonTagFor(html: string, label: string): string {
  const button = html
    .split("<button")
    .slice(1)
    .find((chunk) =>
      chunk.slice(0, chunk.indexOf(">")).includes(`aria-label="${label}"`)
    );
  if (!button) throw new Error(`no <button aria-label="${label}"> in: ${html}`);
  return button.slice(0, button.indexOf(">") + 1);
}

function renderHeader(
  overrides: Partial<QuotationDetailHeaderProps> = {}
): string {
  return renderToString(
    <QuotationDetailHeader
      quotation={QUOTATION}
      activeRevision={1}
      revisionOptions={[{ value: 1, label: "Revision 01 — latest" }]}
      onRevisionChange={noop}
      onBack={noop}
      onEdit={noop}
      purchaseOrder={null}
      purchaseOrderPending={false}
      onCreatePurchaseOrder={noop}
      onOpenPurchaseOrder={noop}
      {...overrides}
    />
  ).replace(/<!-- -->/g, "");
}

describe("QuotationDetailHeader from-this-Quotation action (issue #499)", () => {
  it("offers exactly Create PO from this Quotation when the Quotation is free", () => {
    const html = renderHeader();
    const action = buttonTagFor(html, "Create PO from this Quotation");
    expect(action).not.toContain(" disabled=");
    expect(html).toContain("Create PO from this Quotation");
    expect(html).not.toContain("Open PO draft");
    expect(html).not.toContain("View PO");
  });

  it("offers Open PO draft, and no creation attempt, when a draft occupies it", () => {
    const html = renderHeader({
      purchaseOrder: { id: 41, state: "draft" },
    });
    expect(buttonTagFor(html, "Open PO draft")).not.toContain(" disabled=");
    expect(html).not.toContain("Create PO from this Quotation");
    expect(html).not.toContain("View PO");
  });

  it("offers View PO when the occupying record is finalized", () => {
    const html = renderHeader({
      purchaseOrder: { id: 41, state: "saved" },
    });
    expect(buttonTagFor(html, "View PO")).not.toContain(" disabled=");
    expect(html).not.toContain("Create PO from this Quotation");
    expect(html).not.toContain("Open PO draft");
  });

  it("disables creation until the occupancy answer arrives", () => {
    const html = renderHeader({ purchaseOrderPending: true });
    expect(buttonTagFor(html, "Create PO from this Quotation")).toContain(
      " disabled="
    );
  });

  it("disables creation while the displayed revision is being replaced", () => {
    const html = renderHeader({ revisionSwitching: true });
    expect(buttonTagFor(html, "Create PO from this Quotation")).toContain(
      " disabled="
    );
  });

  it("keeps the existing Edit action and quotation identity", () => {
    const html = renderHeader();
    expect(html).toContain("Edit revision 01");
    expect(html).toContain("1502260001");
    expect(html).toContain("PT. Infineon Technologies Batam");
    expect(html).toContain("Electrical installation");
  });
});
