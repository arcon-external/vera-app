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
