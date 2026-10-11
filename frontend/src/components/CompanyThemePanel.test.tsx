import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { CompanyTheme } from "../company";
import { DEFAULT_THEME, EMPTY_THEME, THEME_FIELDS } from "../company";
import { CompanyThemePanel } from "./CompanyThemePanel";

// The suite runs in node with no DOM (see vitest.config.ts), so the component
// is rendered to a string and asserted on as markup -- the same house pattern
// as ReviewPanel.test.tsx and ThemeColorField.test.tsx.

const QUOTATION_LABELS = THEME_FIELDS.filter((f) => f.section === "Quotation").map(
  (f) => f.label
);
const PORTAL_LABELS = THEME_FIELDS.filter((f) => f.section === "Portal").map(
  (f) => f.label
);

// Distinct raw values per field, so a mis-wired value or a swapped field
// would fail the assertions below rather than passing by coincidence.
const DISTINCT_QUOTATION_THEME: CompanyTheme = {
  ...EMPTY_THEME,
  qDeep: "#111111",
  qAccent: "#222222",
  qSoft: "#333333",
  qInk: "#444444",
};

const DISTINCT_PORTAL_THEME: CompanyTheme = {
  ...EMPTY_THEME,
  pDeep: "#555555",
  pAccent: "#666666",
  pSoft: "#777777",
  pInk: "#888888",
};

function render(
  section: "Quotation" | "Portal",
  theme: CompanyTheme = EMPTY_THEME
): string {
  return renderToString(
    <CompanyThemePanel section={section} theme={theme} onChange={() => {}} />
  );
}

function countInputs(html: string, type: string): number {
  return html.split(`type="${type}"`).length - 1;
}

/** Returns the raw "<input ...>" segment carrying the given aria-label. */
function inputWithLabel(html: string, ariaLabel: string): string {
  const segments = html.split("<input").slice(1);
  const found = segments.find((segment) =>
    segment.includes(`aria-label="${ariaLabel}"`)
  );
  expect(found, `expected an input with aria-label="${ariaLabel}" in: ${html}`).toBeDefined();
  return found as string;
}

describe("CompanyThemePanel", () => {
  it("renders exactly four color inputs and four text inputs for Quotation", () => {
    const html = render("Quotation", DISTINCT_QUOTATION_THEME);
    expect(countInputs(html, "color")).toBe(4);
    expect(countInputs(html, "text")).toBe(4);
  });

  it("renders exactly four color inputs and four text inputs for Portal", () => {
    const html = render("Portal", DISTINCT_PORTAL_THEME);
    expect(countInputs(html, "color")).toBe(4);
    expect(countInputs(html, "text")).toBe(4);
  });

  it("heads the Quotation panel exactly 'Quotation colors'", () => {
    const html = render("Quotation", DISTINCT_QUOTATION_THEME);
    expect(html).toContain("Quotation colors");
  });

  it("renders the four quotation fields and none of the portal ones", () => {
    const html = render("Quotation", DISTINCT_QUOTATION_THEME);
    for (const label of QUOTATION_LABELS) {
      expect(html).toContain(`aria-label="Quotation ${label} color picker"`);
      expect(html).toContain(`aria-label="Quotation ${label} hex value"`);
    }
    for (const label of PORTAL_LABELS) {
      expect(html).not.toContain(`aria-label="Portal ${label} color picker"`);
      expect(html).not.toContain(`aria-label="Portal ${label} hex value"`);
    }
  });

  it("renders the four portal fields and none of the quotation ones", () => {
    const html = render("Portal", DISTINCT_PORTAL_THEME);
    expect(html).toContain("Portal colors");
    for (const label of PORTAL_LABELS) {
      expect(html).toContain(`aria-label="Portal ${label} color picker"`);
      expect(html).toContain(`aria-label="Portal ${label} hex value"`);
    }
    for (const label of QUOTATION_LABELS) {
      expect(html).not.toContain(`aria-label="Quotation ${label} color picker"`);
      expect(html).not.toContain(`aria-label="Quotation ${label} hex value"`);
    }
  });

  it("carries each field's raw theme value on its text input", () => {
    const html = render("Quotation", DISTINCT_QUOTATION_THEME);
    const pairs: [string, string][] = [
      ["Deep", "#111111"],
      ["Accent", "#222222"],
      ["Soft", "#333333"],
      ["Ink", "#444444"],
    ];
    for (const [label, raw] of pairs) {
      const text = inputWithLabel(html, `Quotation ${label} hex value`);
      expect(text).toContain(`value="${raw}"`);
    }
  });

  it("falls back to DEFAULT_THEME on the picker when the stored value is empty or invalid", () => {
    const theme: CompanyTheme = {
      ...EMPTY_THEME,
      qDeep: "#111111",
      qAccent: "",
      qSoft: "zzz",
    };
    const html = render("Quotation", theme);
    expect(inputWithLabel(html, "Quotation Deep color picker")).toContain(
      'value="#111111"'
    );
    expect(inputWithLabel(html, "Quotation Accent color picker")).toContain(
      `value="${DEFAULT_THEME.qAccent}"`
    );
    expect(inputWithLabel(html, "Quotation Soft color picker")).toContain(
      `value="${DEFAULT_THEME.qSoft}"`
    );
    expect(inputWithLabel(html, "Quotation Ink color picker")).toContain(
      `value="${DEFAULT_THEME.qInk}"`
    );
    // The text inputs still carry the raw values, even when empty/invalid.
    expect(inputWithLabel(html, "Quotation Accent hex value")).toContain('value=""');
    expect(inputWithLabel(html, "Quotation Soft hex value")).toContain('value="zzz"');
  });

  it("keeps the card and field-grid layout classes", () => {
    for (const section of ["Quotation", "Portal"] as const) {
      const html = render(section);
      expect(html).toContain("p-5");
      expect(html).toContain("grid grid-cols-2 gap-3");
    }
  });

  it("renders without throwing when every theme value is empty", () => {
    let quotation = "";
    let portal = "";
    expect(() => {
      quotation = render("Quotation", EMPTY_THEME);
      portal = render("Portal", EMPTY_THEME);
    }).not.toThrow();
    expect(quotation).toContain("Quotation colors");
    expect(portal).toContain("Portal colors");
  });
});
