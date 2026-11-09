import { describe, expect, it } from "vitest";
import { displayVersion } from "./version";

// Issue #546: the login update notice prepended its own `v` to a value that
// already carried one, so staff saw "Vera vv0.15.0". The helper is the one
// place that decides the label; both the login notice and the in-app banner
// read it.
describe("displayVersion", () => {
  it("keeps a single v on a prefixed release", () => {
    expect(displayVersion("v0.15.0")).toBe("v0.15.0");
    expect(displayVersion("v0.14.5")).toBe("v0.14.5");
  });

  it("adds the v when the source omits it", () => {
    expect(displayVersion("0.15.0")).toBe("v0.15.0");
    expect(displayVersion("0.14.5")).toBe("v0.14.5");
  });

  it("collapses a doubled prefix instead of printing vv", () => {
    expect(displayVersion("vv0.15.0")).toBe("v0.15.0");
    expect(displayVersion("vvv0.15.0")).toBe("v0.15.0");
  });

  it("keeps the dev sentinel unprefixed", () => {
    expect(displayVersion("dev")).toBe("dev");
    expect(displayVersion("vdev")).toBe("dev");
    expect(displayVersion("")).toBe("dev");
    expect(displayVersion(null)).toBe("dev");
    expect(displayVersion(undefined)).toBe("dev");
  });

  it("trims surrounding whitespace", () => {
    expect(displayVersion(" v0.15.0 ")).toBe("v0.15.0");
  });
});
