import { renderToString } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { DEFAULT_THEME, EMPTY_THEME } from "../company";
import { CompanyPortalTab } from "./CompanyPortalTab";

// Same house pattern as the Quotation tab's: node, no DOM, the tab rendered to
// a string and asserted on as markup. The Portal tab is a component of its own
// (#432) so the shape the Settings page composes can be pinned here rather than
// inferred from a loading render.
//
// What this test does NOT prove, stated plainly: the preview's own fetch and the
// save round trip need effects and the backend. The verdict states a person
// reads in the iframe stay hand-driven.

function render(overrides: Partial<Parameters<typeof CompanyPortalTab>[0]> = {}): string {
  return renderToString(
    <CompanyPortalTab
      theme={DEFAULT_THEME}
      onChange={() => undefined}
      status="saved"
      busy={false}
      error={null}
      onSave={() => undefined}
      {...overrides}
    />
  );
}

/** Returns the raw "<button ...>" segments in the markup. */
function buttonSegments(html: string): string[] {
  return html.split("<button").slice(1);
}

describe("CompanyPortalTab", () => {
  it("renders the portal preview above the colours, with the state picker pending", () => {
    // renderToString runs no effects, so the preview is its pending state —
    // the panel fetches the real document from the server.
    const html = render();
    expect(html).toContain('aria-label="Loading portal preview"');
    expect(html).toContain("Verification state");
    expect(html).not.toContain("<iframe");
  });

  it("saves its own four colours and only its own four", () => {
    const html = render();
    expect(html).toContain(">Save colours</button>");
    // The row is a plain button inside no form, so nothing can submit it.
    expect(html).not.toContain("<form");
    expect(html).not.toContain('type="submit"');
    for (const other of ["Save company details", "Save units", "Save wording and colours"]) {
      expect(html).not.toContain(`>${other}</button>`);
    }
  });

  it("shows the live save state, the busy label and a failed save's banner", () => {
    expect(render({ status: "saved" })).toContain("Saved");
    const dirty = render({ status: "dirty" });
    expect(dirty).toContain("Unsaved changes");
    expect(dirty).toContain('role="status"');

    const saving = render({ status: "saving", busy: true });
    expect(saving).toContain(">Saving…</button>");
    expect(
      buttonSegments(saving).find((segment) => segment.includes(">Saving…</button>"))
    ).toContain('disabled=""');

    const failed = render({ error: "The portal colours were refused." });
    expect(failed).toContain("The portal colours were refused.");
    expect(failed).toContain('role="alert"');
  });

  it("keeps the frozen sentence about the verdict colours no rebrand may move", () => {
    // The palette-coverage note: what the four colours paint, and the four
    // verdict colours that stay fixed so one verdict cannot look like another.
    const html = render();
    expect(html).toContain("These four colours paint the page around the verdict.");
    expect(html).toContain(
      "The verdict colours stay fixed — green valid, amber superseded, red mismatch, grey not-found"
    );
    // The old caption promised the colours rode the profile save, which they no
    // longer do.
    expect(html).not.toContain("Save company profile");
    expect(html).not.toContain("saved with");
  });

  it("keeps the preview pointing at the draft colours, and never at the quotation's", () => {
    const html = render({ theme: { ...EMPTY_THEME, pDeep: "#abcdef" } });
    // The preview panel's own mapping (portalColors) is pinned in
    // PortalPreviewPanel.test.tsx; what is pinned here is that the tab hands
    // the whole theme through and the field names are the portal's.
    expect(html).toContain('aria-label="Portal Deep hex value"');
    expect(html).not.toContain('aria-label="Quotation Deep hex value"');
  });

  it("wires the save handler without calling it during render", () => {
    const onSave = vi.fn();
    const onChange = vi.fn();
    render({ onSave, onChange });
    expect(onSave).not.toHaveBeenCalled();
    expect(onChange).not.toHaveBeenCalled();
  });
});
