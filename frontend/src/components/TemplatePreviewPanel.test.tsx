import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { DEFAULT_THEME, EMPTY_THEME, type CompanyTheme } from "../company";
import type { CompanyLabelEntry } from "../companyLabels";
import {
  TemplatePreviewPanel,
  templatePreviewArgs,
  templatePreviewColorError,
  templatePreviewColors,
  templatePreviewFingerprint,
  templatePreviewRefresh,
} from "./TemplatePreviewPanel";

// Same house pattern as the portal preview: node, no DOM, the panel rendered
// to a string and asserted on as markup.
//
// What this test does NOT prove, stated plainly: the fetch, the blur/Enter
// re-render and the iframe all need effects, which renderToString never runs —
// so the rendered markup is the panel's loading state. What is pinned instead
// is everything pinnable without them: the pending document, the download
// button, the sample disclaimer, and the pure mapping the binding is called
// with (a wrong colour set here would preview a brand the document never
// shows).

function label(Key: string, Value: string): CompanyLabelEntry {
  return { Key, Value, Default: Key, MaxLength: 60, Terms: false };
}

const LABELS: CompanyLabelEntry[] = [label("docTitle", "PROPOSAL"), label("subject", "Scope")];

function render(props?: Partial<Parameters<typeof TemplatePreviewPanel>[0]>): string {
  return renderToString(
    <TemplatePreviewPanel labels={LABELS} theme={DEFAULT_THEME} refreshKey={0} {...props} />
  );
}

describe("template preview panel", () => {
  it("shows a pending document, not an empty frame, until the first render lands", () => {
    const html = render();
    expect(html).toContain('aria-label="Loading template preview"');
    expect(html).not.toContain("<iframe");
  });

  it("offers the sample download and says what the sample is", () => {
    const html = render();
    expect(html).toContain(">Download sample</button>");
    expect(html).toContain("fake quote number");
    expect(html).toContain("does not verify");
  });

  it("says the preview follows blur and Enter, not keystrokes", () => {
    expect(render()).toContain("never while you type");
  });

  it("paints the pending document even before the wording loads", () => {
    // Effects never run under renderToString, so with labels null the panel
    // is exactly where a slow fetch would leave it: the pending rectangle,
    // not an empty frame and not an error (the error is set by the effect).
    const html = render({ labels: null });
    expect(html).toContain('aria-label="Loading template preview"');
    expect(html).not.toContain("<iframe");
  });
});

describe("template preview mapping", () => {
  it("reads the quotation's four colours, not the portal's", () => {
    const theme: CompanyTheme = {
      ...EMPTY_THEME,
      qDeep: " #111111 ",
      qAccent: "#222222",
      qSoft: "#333333",
      qInk: "#444444",
      pDeep: "#1A237E",
      pAccent: "#284AF3",
      pSoft: "#D6E4FF",
      pInk: "#1a1a1a",
    };
    expect(templatePreviewColors(theme)).toEqual(["#111111", "#222222", "#333333", "#444444"]);
    expect(templatePreviewArgs(LABELS, theme)).toEqual([
      LABELS,
      "#111111",
      "#222222",
      "#333333",
      "#444444",
    ]);
  });

  it("names the first incomplete hex and passes empty colours through", () => {
    expect(templatePreviewColorError(DEFAULT_THEME)).toBeNull();
    expect(templatePreviewColorError({ ...DEFAULT_THEME, qSoft: "" })).toBeNull();
    expect(templatePreviewColorError({ ...DEFAULT_THEME, qAccent: "red" })).toBe("Accent");
    expect(templatePreviewColorError({ ...DEFAULT_THEME, qDeep: "#12345" })).toBe("Deep");
  });

  it("fingerprints wording and colours together, and nothing while loading", () => {
    const base = templatePreviewFingerprint(LABELS, DEFAULT_THEME);
    expect(base).not.toBe("");
    expect(templatePreviewFingerprint(null, DEFAULT_THEME)).toBe("");
    expect(templatePreviewFingerprint(LABELS, { ...DEFAULT_THEME, qDeep: "#000000" })).not.toBe(base);
    expect(
      templatePreviewFingerprint(
        LABELS.map((entry) => ({ ...entry, Value: `${entry.Value} x` })),
        DEFAULT_THEME
      )
    ).not.toBe(base);
  });

  it("refreshes on blur/Enter only when the draft moved", () => {
    const shown = templatePreviewFingerprint(LABELS, DEFAULT_THEME);
    // Unchanged fields: tabbing through renders nothing.
    expect(templatePreviewRefresh(shown, LABELS, DEFAULT_THEME)).toBeNull();
    // Still loading: nothing to render from.
    expect(templatePreviewRefresh("", null, DEFAULT_THEME)).toBeNull();
    // A rewording or a recolour returns the fingerprint to render.
    const reworded = LABELS.map((entry) =>
      entry.Key === "docTitle" ? { ...entry, Value: "OFFER" } : entry
    );
    const next = templatePreviewRefresh(shown, reworded, DEFAULT_THEME);
    expect(next).not.toBeNull();
    expect(next).toBe(templatePreviewFingerprint(reworded, DEFAULT_THEME));
    expect(
      templatePreviewRefresh(shown, LABELS, { ...DEFAULT_THEME, qInk: "#000000" })
    ).toBe(templatePreviewFingerprint(LABELS, { ...DEFAULT_THEME, qInk: "#000000" }));
  });
});
