import { describe, expect, it } from "vitest";
import {
  backTarget,
  isPurchaseOrderWorkspacePage,
  isQuotationWorkspacePage,
  isSettingsPage,
  lastWorkspacePage,
  popHistory,
  pushHistory,
  shouldPushHistory,
  workspaceKeyOf,
} from "./pageHistory";

// Ticket 02: the page-history stack behind Home → workspace → settings.
// Pure navigation rules, no React: App holds the stack, these functions are
// the decisions it applies. Every case pins the ticket's acceptance rules.

describe("settings pages", () => {
  it("names the three settings surfaces", () => {
    expect(isSettingsPage("settings")).toBe(true);
    expect(isSettingsPage("quotation-settings")).toBe(true);
    expect(isSettingsPage("po-settings")).toBe(true);
  });

  it("does not mistake workspaces or home for settings", () => {
    for (const page of [
      "home",
      "create",
      "drafts",
      "archive",
      "detail",
      "po",
      "po-drafts",
      "po-archive",
      "admin-clients",
    ] as const) {
      expect(isSettingsPage(page)).toBe(false);
    }
  });
});

describe("workspace pages", () => {
  it("names the PO workspace pages", () => {
    expect(isPurchaseOrderWorkspacePage("po")).toBe(true);
    expect(isPurchaseOrderWorkspacePage("po-drafts")).toBe(true);
    expect(isPurchaseOrderWorkspacePage("po-archive")).toBe(true);
    expect(isPurchaseOrderWorkspacePage("create")).toBe(false);
    expect(isPurchaseOrderWorkspacePage("home")).toBe(false);
    expect(isPurchaseOrderWorkspacePage("po-settings")).toBe(false);
  });

  it("names the Quotation workspace pages", () => {
    expect(isQuotationWorkspacePage("create")).toBe(true);
    expect(isQuotationWorkspacePage("drafts")).toBe(true);
    expect(isQuotationWorkspacePage("archive")).toBe(true);
    expect(isQuotationWorkspacePage("detail")).toBe(true);
    expect(isQuotationWorkspacePage("po")).toBe(false);
    expect(isQuotationWorkspacePage("home")).toBe(false);
    expect(isQuotationWorkspacePage("quotation-settings")).toBe(false);
  });
});

describe("shouldPushHistory", () => {
  it("pushes across workspaces: home to workspace, workspace to settings", () => {
    expect(shouldPushHistory("home", "po")).toBe(true);
    expect(shouldPushHistory("home", "create")).toBe(true);
    expect(shouldPushHistory("po", "po-settings")).toBe(true);
    expect(shouldPushHistory("create", "quotation-settings")).toBe(true);
    expect(shouldPushHistory("po-drafts", "po-settings")).toBe(true);
  });

  it("replaces within one workspace so Back leaves the workspace, not the tab", () => {
    expect(shouldPushHistory("po", "po-drafts")).toBe(false);
    expect(shouldPushHistory("po-drafts", "po-archive")).toBe(false);
    expect(shouldPushHistory("create", "drafts")).toBe(false);
    expect(shouldPushHistory("drafts", "archive")).toBe(false);
    expect(shouldPushHistory("archive", "detail")).toBe(false);
    expect(shouldPushHistory("settings", "settings")).toBe(false);
  });

  it("never pushes staying on the same page", () => {
    expect(shouldPushHistory("po", "po")).toBe(false);
    expect(shouldPushHistory("home", "home")).toBe(false);
  });
});

describe("pushHistory / popHistory / backTarget", () => {
  it("pushes the page being left and pops back to it", () => {
    const afterOpen = pushHistory([], "home", "po");
    expect(afterOpen).toEqual(["home"]);
    const afterSettings = pushHistory(afterOpen, "po", "po-settings");
    expect(afterSettings).toEqual(["home", "po"]);
    expect(backTarget("po-settings", afterSettings)).toBe("po");
    const { history: afterBack, page } = popHistory(afterSettings);
    expect(page).toBe("po");
    expect(afterBack).toEqual(["home"]);
    expect(backTarget("po", afterBack)).toBe("home");
  });

  it("replaces within one workspace instead of growing the stack", () => {
    const opened = pushHistory([], "home", "po");
    const tabbed = pushHistory(opened, "po", "po-drafts");
    expect(tabbed).toEqual(["home"]);
  });

  it("Back from a settings page never jumps straight home", () => {
    const history = pushHistory(pushHistory([], "home", "po"), "po", "po-settings");
    expect(backTarget("po-settings", history)).not.toBe("home");
    expect(backTarget("po-settings", history)).toBe("po");
  });

  it("Back with an empty stack lands on home", () => {
    expect(backTarget("po", [])).toBe("home");
    const { history, page } = popHistory([]);
    expect(page).toBe("home");
    expect(history).toEqual([]);
  });
});

describe("workspaceKeyOf / lastWorkspacePage", () => {
  it("groups the tabs of one workspace", () => {
    expect(workspaceKeyOf("po")).toBe(workspaceKeyOf("po-drafts"));
    expect(workspaceKeyOf("po-drafts")).toBe(workspaceKeyOf("po-archive"));
    expect(workspaceKeyOf("create")).toBe(workspaceKeyOf("drafts"));
    expect(workspaceKeyOf("drafts")).toBe(workspaceKeyOf("archive"));
    expect(workspaceKeyOf("archive")).toBe(workspaceKeyOf("detail"));
    expect(workspaceKeyOf("po")).not.toBe(workspaceKeyOf("create"));
    expect(workspaceKeyOf("home")).not.toBe(workspaceKeyOf("po"));
  });

  it("finds the workspace to keep alive behind settings pages", () => {
    expect(lastWorkspacePage(["home", "po"], "po-settings")).toBe("po");
    expect(lastWorkspacePage(["home", "po-drafts"], "po-settings")).toBe("po-drafts");
    expect(lastWorkspacePage(["home", "create"], "quotation-settings")).toBe("create");
    expect(lastWorkspacePage(["home"], "settings")).toBeNull();
    expect(lastWorkspacePage(["home", "po"], "po")).toBeNull();
  });
});
