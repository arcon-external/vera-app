import { renderToString } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import type { CompanyUnitOption } from "../companyUnits";
import { validateCompanyUnits } from "../companyUnits";
import { CompanyUnitsPanel } from "./CompanyUnitsPanel";

// Same house pattern as CompanyGeneralPanel.test.tsx: node, no DOM, the panel
// rendered to a string and asserted on as markup. The panel is a presentational
// slice (parent owns state), so a loaded render needs only props. What is NOT
// proven here: the save round-trip (needs the backend) and reorder-click
// behaviour (needs a DOM) — those stay hand-driven; the reorder math itself is
// pinned in companyUnits.test.ts.

const UNITS: CompanyUnitOption[] = [
  { value: "Lot", label: "Lot" },
  { value: "m2", label: "m²" },
  { value: "Pc", label: "Pc" },
];

function render(overrides: Partial<Parameters<typeof CompanyUnitsPanel>[0]> = {}): string {
  return renderToString(
    <CompanyUnitsPanel
      units={UNITS}
      canonical="Lot"
      onUnitsChange={() => {}}
      onCanonicalChange={() => {}}
      busy={false}
      error={null}
      onSaveUnits={() => {}}
      {...overrides}
    />
  );
}

/** Returns the raw "<select ...>...</select>" segments. */
function selects(html: string): string[] {
  return html.split("<select").slice(1).map((segment) => segment.split("</select>")[0]);
}

/** The button segment whose aria-label matches exactly. */
function buttonWithAria(html: string, label: string): string {
  const found = html.split("<button").slice(1).find((segment) =>
    segment.includes(`aria-label="${label}"`)
  );
  expect(found, `expected a button aria-label="${label}" in: ${html}`).toBeDefined();
  return found as string;
}

describe("CompanyUnitsPanel", () => {
  it("renders the heading and the value/label explainer", () => {
    const html = render();
    expect(html).toContain("Units of measure");
    expect(html).toContain("value");
    expect(html).toContain("what prints");
    expect(html).toContain("m²");
  });

  it("shows a loading skeleton while the served list is null", () => {
    const html = render({ units: null });
    expect(html).toContain('aria-label="Loading units"');
    expect(html).not.toContain('aria-label="Canonical unit"');
  });

  it("renders the canonical picker with every value and the current one selected", () => {
    const canonical = selects(render()).find((segment) => segment.includes('aria-label="Canonical unit"'));
    expect(canonical, `expected a canonical select in: ${render()}`).toBeDefined();
    for (const unit of UNITS) {
      expect(canonical).toContain(`value="${unit.value}"`);
    }
    const lotOption = (canonical as string).split("<option").slice(1).find((segment) => segment.includes('value="Lot"'));
    expect(lotOption).toContain("selected");
  });

  it("shows the printable label beside a value that differs from it", () => {
    const canonical = selects(render()).find((segment) => segment.includes('aria-label="Canonical unit"')) as string;
    expect(canonical).toContain("m2 — m²");
  });

  it("renders a value and a label input for every unit, in order", () => {
    const html = render();
    UNITS.forEach((unit, i) => {
      expect(html).toContain(`aria-label="Value for unit ${i + 1}"`);
      expect(html).toContain(`aria-label="Label for unit ${i + 1}"`);
      expect(html).toContain(`value="${unit.value}"`);
      expect(html).toContain(`value="${unit.label}"`);
    });
  });

  it("marks the canonical row and disables only its remove button", () => {
    const html = render();
    expect(html).toContain("Canonical");
    expect(buttonWithAria(html, "Remove unit 1")).toContain('disabled=""'); // Lot is canonical
    expect(buttonWithAria(html, "Remove unit 2")).not.toContain('disabled=""'); // m2
    expect(buttonWithAria(html, "Remove unit 3")).not.toContain('disabled=""'); // Pc
  });

  it("clamps the up and down move buttons at the list edges", () => {
    const html = render();
    expect(buttonWithAria(html, "Move unit 1 up")).toContain('disabled=""');
    expect(buttonWithAria(html, "Move unit 1 down")).not.toContain('disabled=""');
    expect(buttonWithAria(html, "Move unit 3 down")).toContain('disabled=""');
    expect(buttonWithAria(html, "Move unit 3 up")).not.toContain('disabled=""');
  });

  it("offers an Add unit button and a Save units button, neither submit type", () => {
    const html = render();
    const add = html.split("<button").slice(1).find((segment) => segment.includes(">Add unit</button>"));
    expect(add).toBeDefined();
    expect(add).toContain('type="button"');
    const save = html.split("<button").slice(1).find((segment) => segment.includes(">Save units</button>"));
    expect(save).toBeDefined();
    expect(save).toContain('type="button"');
    // The panel is a slice: it must not collapse into a form of its own.
    expect(html).not.toContain("<form");
    expect(html).not.toContain('type="submit"');
  });

  it("disables Save units and shows the reason when the list is invalid", () => {
    const html = render({ units: [{ value: "", label: "Lot" }] });
    expect(html).toContain("value is required");
    const save = html.split("<button").slice(1).find((segment) => segment.includes(">Save units</button>"));
    expect(save).toContain('disabled=""');
  });

  it("enables Save units for a valid list and shows the order hint", () => {
    const html = render();
    const save = html.split("<button").slice(1).find((segment) => segment.includes(">Save units</button>"));
    expect(save).not.toContain('disabled=""');
    expect(html).toContain("Order is the order the editor dropdown shows.");
  });

  it("shows a save error banner when one is passed", () => {
    const html = render({ error: "the canonical unit \"Week\" must stay on the list." });
    expect(html).toContain("must stay on the list");
    expect(html).toContain('role="alert"');
  });

  it("wires the save handler without calling it during render", () => {
    const onSaveUnits = vi.fn();
    const onUnitsChange = vi.fn();
    const onCanonicalChange = vi.fn();
    render({ onSaveUnits, onUnitsChange, onCanonicalChange });
    expect(onSaveUnits).not.toHaveBeenCalled();
    expect(onUnitsChange).not.toHaveBeenCalled();
    expect(onCanonicalChange).not.toHaveBeenCalled();
  });

  it("renders without throwing when every value is blank", () => {
    let html = "";
    expect(() => {
      html = render({ units: [{ value: "", label: "" }], canonical: "" });
    }).not.toThrow();
    expect(html).toContain("Units of measure");
  });

  it("keeps a canonical off the offered list visible on the picker (#337)", () => {
    // The canonical was "Week"; the list dropped it. The picker must still name
    // Week, or the next profile save would silently clear it.
    const html = render({ canonical: "Week" });
    const canonical = selects(html).find((segment) => segment.includes('aria-label="Canonical unit"')) as string;
    expect(canonical).toContain('value="Week"');
  });
});

describe("CompanyUnitsPanel validation is the store's", () => {
  // The panel feeds validateCompanyUnits(units, canonical); this pins that a
  // representative refusal agrees with the store boundary it mirrors, so the
  // UI never offers a save the server would bounce.
  it("refuses an empty list, a bad value, a long label and a dropped canonical", () => {
    expect(validateCompanyUnits([], "Lot")).toBe("At least one unit of measure is required.");
    expect(validateCompanyUnits([{ value: "m²", label: "m²" }], "Lot")).toContain("not shaped");
    expect(validateCompanyUnits([{ value: "Lot", label: "x".repeat(21) }], "Lot")).toContain("too long");
    expect(validateCompanyUnits([{ value: "Pc", label: "Pc" }], "Lot")).toContain("must stay on the list");
  });

  it("accepts a shaped list that keeps the canonical", () => {
    expect(
      validateCompanyUnits(
        [{ value: "Lot", label: "Lot" }, { value: "m2", label: "m²" }],
        "Lot"
      )
    ).toBeNull();
  });
});
