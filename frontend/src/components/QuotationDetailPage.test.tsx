import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { ArchivedQuotation } from "../archive";
import { QuotationDetailHeader } from "./QuotationDetailPage";

// Issue #467 wave 2: the issued-Quotation detail surface is the second PO
// entry path. The header is rendered without a DOM, so this suite proves the
// action exists as a real enabled button beside Edit; the click that opens the
// shared selector, and returning focus to this button on close, stay
// hand-driven.
//
// renderToString runs no effects, which is why the header -- not the whole
// page -- is the unit here: the page fetches its revision in an effect and
// would only print its loading skeleton.

const QUOTATION = {
  QuoteNumber: "1502260001",
  RevisionNumber: 1,
  AvailableRevisions: [1, 0],
  CustomerName: "PT. Infineon Technologies Batam",
  Subject: "Electrical installation",
} as ArchivedQuotation;

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

function renderHeader(): string {
  return renderToString(
    <QuotationDetailHeader
      quotation={QUOTATION}
      activeRevision={1}
      revisionOptions={[{ value: 1, label: "Revision 01 — latest" }]}
      onRevisionChange={() => {}}
      onBack={() => {}}
      onEdit={() => {}}
      onAddPurchaseOrder={() => {}}
    />
  ).replace(/<!-- -->/g, "");
}

describe("QuotationDetailHeader PO entry path (issue #467)", () => {
  it("offers Add Purchase Order from a Quotation as an enabled action", () => {
    const html = renderHeader();
    const action = buttonTagFor(html, "Add Purchase Order from a Quotation");
    expect(action).not.toContain(" disabled=");
    expect(html).toContain("Add Purchase Order from a Quotation");
  });

  it("keeps the existing Edit action and quotation identity", () => {
    const html = renderHeader();
    expect(html).toContain("Edit revision 01");
    expect(html).toContain("1502260001");
    expect(html).toContain("PT. Infineon Technologies Batam");
    expect(html).toContain("Electrical installation");
  });
});
