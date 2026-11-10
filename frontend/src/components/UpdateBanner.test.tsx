import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { backend } from "@backend/models";
import { UpdateBanner } from "./UpdateBanner";
import { UPDATE_NEWER, UPDATE_REQUIRED } from "../updateFixture";

// Ticket #558: the in-app banner and the login notice read the same served
// check, so they must print the same labels. The versions come from
// ../updateFixture.ts — the live `v`-prefixed shape — because a bare `0.x`
// fixture let the doubled-`v` regression (#546: "Vera vv0.15.0") pass the
// suite without ever looking like the contract.
//
// What this does NOT prove, stated so it is not assumed: the suite runs in
// node with no DOM (vitest.config.ts), so the banner is rendered to a string
// with its state handed in. The mount check, the cache fallback, the dismissal
// and the download click are effects and events; they stay hand-driven, and
// their wiring is App.tsx's (useUpdateState) rather than this component's.

/** The banner's state is UpdateInfo plus the two launch flags. */
function renderBanner(info: backend.UpdateInfo): string {
  return renderToStaticMarkup(
    <UpdateBanner
      state={{ ...info, checked: true, offline: false }}
      onDismiss={() => undefined}
    />
  );
}

describe("UpdateBanner version labels", () => {
  it("prints exactly one v for both versions", () => {
    const html = renderBanner(UPDATE_NEWER);
    expect(html).toContain("Vera v0.13.1 is available (you have v0.12.1).");
    expect(html).not.toContain("vv");
  });

  it("prints one v in the required-build sentence", () => {
    const html = renderBanner(UPDATE_REQUIRED);
    expect(html).toContain(
      "This Vera (v0.12.1) is below the minimum supported version. Please update to v0.13.1 to keep working."
    );
    expect(html).not.toContain("vv");
  });

  it("normalises a doubled v instead of printing it", () => {
    const html = renderBanner({
      ...UPDATE_NEWER,
      Current: "vv0.12.1",
      Latest: "vv0.13.1",
    });
    expect(html).toContain("Vera v0.13.1 is available (you have v0.12.1).");
    expect(html).not.toContain("vv");
  });
});
