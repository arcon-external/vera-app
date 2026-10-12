import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { ThemeColorField } from "./ThemeColorField";
import { fieldClass } from "./ui";

// The suite runs in node with no DOM (see vitest.config.ts), so the component
// is rendered to a string and asserted on as markup -- the same house pattern
// as ReviewPanel.test.tsx and TabBar.test.tsx.

const SECTION = "Quotation";
const LABEL = "Deep";
const HINT = "Headings, table headers, subject label, buttons, top ribbon on quotations";
const FALLBACK = "#1A237E";

function render(props?: Partial<Parameters<typeof ThemeColorField>[0]>): string {
  return renderToString(
    <ThemeColorField
      section={SECTION}
      label={LABEL}
      hint={HINT}
      value={FALLBACK}
      fallback={FALLBACK}
      onChange={() => {}}
      {...props}
    />
  );
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

describe("ThemeColorField", () => {
  it("renders both inputs with the exact accessible names", () => {
    const html = render();
    expect(html).toContain(`aria-label="${SECTION} ${LABEL} color picker"`);
    expect(html).toContain(`aria-label="${SECTION} ${LABEL} hex value"`);
  });

  it("shows the label text and the hint text", () => {
    const html = render();
    expect(html).toContain(LABEL);
    expect(html).toContain(HINT);
  });

  it("sets the title to exactly \"<label>: <hint>\"", () => {
    const html = render();
    expect(html).toContain(`title="${LABEL}: ${HINT}"`);
  });

  it("carries the raw value on the text input, with the fallback as placeholder", () => {
    const raw = render({ value: "#abc123" });
    const text = inputWithLabel(raw, `${SECTION} ${LABEL} hex value`);
    expect(text).toContain('value="#abc123"');
    expect(text).toContain(`placeholder="${FALLBACK}"`);

    const empty = render({ value: "" });
    const emptyText = inputWithLabel(empty, `${SECTION} ${LABEL} hex value`);
    expect(emptyText).toContain('value=""');
    expect(emptyText).toContain(`placeholder="${FALLBACK}"`);
  });

  it("shows the fallback on the picker unless value is a complete hex", () => {
    const pickerLabel = `${SECTION} ${LABEL} color picker`;
    for (const value of ["", "#12", "zzz"]) {
      const picker = inputWithLabel(render({ value }), pickerLabel);
      expect(picker, `value ${JSON.stringify(value)}`).toContain(
        `value="${FALLBACK}"`
      );
    }
    const exact = inputWithLabel(render({ value: "#1A237E" }), pickerLabel);
    expect(exact).toContain('value="#1A237E"');
  });

  it("trims the value before testing it for the picker, keeping the raw text in the input", () => {
    const pickerAria = `${SECTION} ${LABEL} color picker`;
    const textAria = `${SECTION} ${LABEL} hex value`;

    const padded = render({ value: "  #1A237E  " });
    const paddedPicker = inputWithLabel(padded, pickerAria);
    expect(paddedPicker).toContain('value="#1A237E"');
    const paddedText = inputWithLabel(padded, textAria);
    expect(paddedText).toContain('value="  #1A237E  "');

    const blank = render({ value: "   " });
    const blankPicker = inputWithLabel(blank, pickerAria);
    expect(blankPicker).toContain(`value="${FALLBACK}"`);
  });

  it("gives the text input exactly the shared fieldClass plus font-mono", () => {
    const text = inputWithLabel(render(), `${SECTION} ${LABEL} hex value`);
    expect(text).toContain(`class="${fieldClass} font-mono"`);
  });

  it("disables spellcheck and autocomplete on the text input", () => {
    const html = render();
    const text = inputWithLabel(html, `${SECTION} ${LABEL} hex value`);
    // Case-insensitive on purpose: the contract is that spellcheck is present
    // and false, not how the framework cases the attribute name. React 18's
    // server renderer lowercased spellCheck to spellcheck; React 19 emits the
    // canonical camelCase spellCheck. Match either, as CompanyGeneralPanel
    // already does for maxLength/tabIndex.
    expect(text.toLowerCase()).toContain('spellcheck="false"');
    expect(text).toContain('autoComplete="off"');
  });

  it("renders without throwing when value and fallback are both empty", () => {
    let html = "";
    expect(() => {
      html = render({ value: "", fallback: "" });
    }).not.toThrow();
    expect(html).toContain(`aria-label="${SECTION} ${LABEL} color picker"`);
    expect(html).toContain(`aria-label="${SECTION} ${LABEL} hex value"`);
  });
});
