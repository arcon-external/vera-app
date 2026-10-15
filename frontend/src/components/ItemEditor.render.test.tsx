import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { EditorItem } from "./ItemEditor";
import { ItemEditor } from "./ItemEditor";

// Issue #337: the editor's unit-of-measure dropdown is served, not hardcoded.
// These render-to-string pins prove two things the change must hold: the
// dropdown offers exactly the options handed to it (the served list), and
// while that list is empty it shows a disabled pending state instead of a
// stale or invented list. Interaction (choosing a unit) stays hand-driven.

const ITEM: EditorItem = {
  key: 0,
  description: "Widget",
  bullets: [],
  qty: "1",
  uom: "Pc",
  unitPrice: "1000",
  marginEnabled: false,
  marginPercent: "0",
  discountEnabled: false,
  discountPercent: "0",
  subItems: [],
  showSubPrices: false,
};

const noop = () => {};

function render(overrides: Partial<Parameters<typeof ItemEditor>[0]> = {}): string {
  return renderToString(
    <ItemEditor
      items={[ITEM]}
      uomOptions={[
        { value: "Lot", label: "Lot" },
        { value: "Pc", label: "Pc" },
        { value: "m2", label: "m²" },
      ]}
      onAddItem={noop}
      onRemoveItem={noop}
      onChange={noop}
      onAddBullet={noop}
      onRemoveBullet={noop}
      onBulletChange={noop}
      onEnableSubItems={noop}
      onDisableSubItems={noop}
      onAddSubRow={noop}
      onRemoveSubRow={noop}
      onPatchSubRow={noop}
      onMoveSubRow={noop}
      onPasteSubRows={noop}
      onAddSubBullet={noop}
      onRemoveSubBullet={noop}
      onSubBulletChange={noop}
      onPasteItems={noop}
      {...overrides}
    />
  );
}

/** The UoM <select> for line 1, by its aria-label. */
function uomSelect(html: string): string {
  const start = html.indexOf('aria-label="Unit of measure for line 1"');
  expect(start, `expected the line UoM select in: ${html}`).toBeGreaterThan(0);
  const selectStart = html.lastIndexOf("<select", start);
  const selectEnd = html.indexOf("</select>", start);
  return html.slice(selectStart, selectEnd);
}

/** Returns the decoded background-image data URI out of a markup segment's
 *  inline style: React SSR escapes both the attribute's double quotes and the
 *  URI's single quotes as entities, so they are undone before decoding. The
 *  same helper as the Company panels' chevron pins (#429). */
function decodedChevron(segment: string): string {
  const match = segment.match(/background-image:url\((.*?)\)/);
  expect(match, `expected a background-image url in: ${segment}`).toBeDefined();
  const escaped = (match as RegExpMatchArray)[1].replace(/&quot;/g, '"').replace(/&#x27;/g, "'");
  return decodeURIComponent(escaped);
}

describe("ItemEditor served UoM dropdown", () => {
  it("offers exactly the options it is handed, and selects the stored unit", () => {
    const select = uomSelect(render());
    expect(select).toContain('value="Lot"');
    expect(select).toContain('value="Pc"');
    expect(select).toContain('value="m2"');
    expect(select).toContain("m²"); // the label, not the value, prints
    const pc = select.split("<option").slice(1).find((segment) => segment.includes('value="Pc"'));
    expect(pc).toContain("selected");
    expect(select).not.toContain('disabled=""');
  });

  it("renders a stored unit that is off the served list as (not recognised)", () => {
    const select = uomSelect(render({ items: [{ ...ITEM, uom: "Meter" }] }));
    expect(select).toContain("Meter (not recognised)");
  });

  it("shows a disabled pending state, not a stale or invented list, while the served list is empty", () => {
    const select = uomSelect(render({ uomOptions: [] }));
    expect(select).toContain('disabled=""');
    expect(select).toContain("Loading units…");
    // No real unit can be offered from an empty list.
    expect(select).not.toContain('value="Lot"');
    expect(select).not.toContain("not recognised");
  });

  it("honours an explicit pending label (the failure case names it)", () => {
    const select = uomSelect(render({ uomOptions: [], uomPendingLabel: "Units unavailable" }));
    expect(select).toContain("Units unavailable");
    expect(select).toContain('disabled=""');
  });
});

// Issue #430: the chevron is a data-URI SVG drawn as a CSS background. Parsed
// in image mode nothing implies the SVG namespace, so without an explicit
// xmlns the browser paints nothing at all — silently, with no console error —
// and every select using this idiom loses its arrow. The pin decodes the URI
// (the same bytes the browser parses, so an escaped or misplaced attribute
// cannot pass) and asserts the xmlns is present and nothing else moved: only
// the attribute was added to the broken copy.
describe("ItemEditor UoM chevron (#430)", () => {
  const ITEM_WITH_SUBITEM: EditorItem = {
    ...ITEM,
    key: 1,
    description: "Installation",
    subItems: [
      {
        id: 1,
        description: "Cable tray",
        qty: "10",
        uom: "m",
        unitCost: "5000",
        marginEnabled: false,
        marginPercent: "0",
        discountEnabled: false,
        discountPercent: "0",
        bullets: [],
      },
    ],
  };

  it("draws the chevron from an image-mode SVG carrying its xmlns, on every select that uses it", () => {
    // One plain line, plus one line whose panel is open with a sub-item, so
    // both UoMSelect call sites — the line row and the sub-item row — are on
    // the page and both are pinned by the same assertions.
    const html = render({ items: [ITEM, ITEM_WITH_SUBITEM] });
    const selects = html.split("<select").slice(1).map((segment) => segment.split("</select>")[0]);
    expect(selects.length).toBe(2);
    for (const select of selects) {
      expect(select).toContain("background-image");
      expect(select).toContain("data:image/svg+xml");
      // The decoded URI must carry an svg element that names its namespace:
      // this is the assertion the broken copy fails.
      expect(decodedChevron(select)).toContain("<svg xmlns='http://www.w3.org/2000/svg'");
      // …and byte-for-byte the intended SVG, so nothing else may move with
      // it: the path, size, position and encoding are pinned by the same
      // assertion. The only difference from the broken copy is the xmlns.
      expect(decodedChevron(select)).toContain(
        "<svg xmlns='http://www.w3.org/2000/svg' width='12' height='12' viewBox='0 0 24 24' " +
          "fill='none' stroke='#767469' stroke-width='2' stroke-linecap='round'>" +
          "<path d='m6 9 6 6 6-6'/></svg>"
      );
    }
  });
});
