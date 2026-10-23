// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AppShell, type SettingsOrigin } from "./AppShell";
import type { AppPage } from "../poLeaveGuard";
import type { SignedInUser } from "./LoginPage";
import { ToastProvider } from "../toast";

// Issue #494: the profile menu is contextual. Account settings and Sign out are
// always there; a workspace's settings entry appears only when the menu is
// opened in that workspace (or on a settings page that retained it). The
// callbacks alone no longer make an entry visible -- the old callback-driven
// rule showed both settings everywhere. The Admin Dashboard shortcut is gone:
// the workspace is reached from Home. Icons and separators are covered here;
// the pixels are read from the snapshot harness.

vi.mock("@backend/backend/AdminAPI", () => ({
  ListDeletionRequests: vi.fn(async () => []),
  ListEmailChangeRequests: vi.fn(async () => []),
}));
vi.mock("@backend/backend/AuthAPI", () => ({
  MyDeletionRequests: vi.fn(async () => []),
  MyEmailChangeRequests: vi.fn(async () => []),
}));

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

const ADMIN: SignedInUser = {
  ID: 1,
  Email: "admin@example.com",
  DisplayName: "Ada Admin",
  Role: "admin",
};
const STAFF: SignedInUser = { ...ADMIN, ID: 2, DisplayName: "Sam Staff", Role: "staff" };

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  document.body.innerHTML = "";
});

interface RenderOptions {
  page?: AppPage;
  settingsOrigin?: SettingsOrigin;
  quotationSettings?: () => void;
  poSettings?: () => void;
  onNavigate?: (page: AppPage) => void;
}

function render(user: SignedInUser, options: RenderOptions = {}) {
  act(() => {
    root.render(
      <ToastProvider>
        <AppShell
          page={options.page ?? "create"}
          settingsOrigin={options.settingsOrigin}
          onNavigate={options.onNavigate ?? (() => {})}
          user={user}
          onOpenQuotationSettings={options.quotationSettings}
          onOpenPurchaseOrderSettings={options.poSettings}
        >
          <div>body</div>
        </AppShell>
      </ToastProvider>
    );
  });
}

function trigger(user: SignedInUser): HTMLButtonElement {
  const found = document.querySelector<HTMLButtonElement>(
    `button[aria-label="Account menu for ${user.DisplayName}"]`
  );
  if (found === null) throw new Error("account menu trigger not found");
  return found;
}

function openMenu(user: SignedInUser): void {
  act(() => trigger(user).click());
}

function menuItems(): string[] {
  return Array.from(document.querySelectorAll('[role="menuitem"]')).map(
    (node) => node.textContent?.trim() ?? ""
  );
}

function menuItemNodes(): HTMLButtonElement[] {
  return Array.from(
    document.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')
  );
}

function menuItem(label: string): HTMLButtonElement {
  const found = menuItemNodes().find((node) => node.textContent?.trim() === label);
  if (found === undefined) throw new Error(`no menu item ${label}`);
  return found;
}

function separators(): HTMLElement[] {
  return Array.from(document.querySelectorAll<HTMLElement>('[role="separator"]'));
}

/** Focuses a node and lets React's synthetic focus/blur bookkeeping settle. */
function focus(node: HTMLElement): void {
  act(() => node.focus());
}

function pressKey(node: HTMLElement, key: string): void {
  act(() => {
    node.dispatchEvent(new KeyboardEvent("keydown", { key, bubbles: true, cancelable: true }));
  });
}

const SETTINGS = { quotationSettings: () => {}, poSettings: () => {} };

describe("profile menu is contextual (issue #494)", () => {
  it("Home offers Account settings and Sign out only", () => {
    render(ADMIN, { page: "home", ...SETTINGS });
    openMenu(ADMIN);
    expect(menuItems()).toEqual(["Account settings", "Sign out"]);
  });

  it("a workspace offers Account settings, its own settings and Sign out", () => {
    render(STAFF, { page: "create", ...SETTINGS });
    openMenu(STAFF);
    expect(menuItems()).toEqual([
      "Account settings",
      "Quotation settings",
      "Sign out",
    ]);
  });

  it("Purchase Order offers only the Purchase Order settings entry", () => {
    render(STAFF, { page: "po", ...SETTINGS });
    openMenu(STAFF);
    expect(menuItems()).toEqual([
      "Account settings",
      "Purchase Order settings",
      "Sign out",
    ]);
  });

  it("Admin has no workspace settings entry, even when callbacks are supplied", () => {
    render(ADMIN, { page: "admin-company", ...SETTINGS });
    openMenu(ADMIN);
    expect(menuItems()).toEqual(["Account settings", "Sign out"]);
  });

  it("a settings page keeps the workspace context it was opened from", () => {
    render(STAFF, {
      page: "settings",
      settingsOrigin: "quotation",
      ...SETTINGS,
    });
    openMenu(STAFF);
    expect(menuItems()).toEqual([
      "Account settings",
      "Quotation settings",
      "Sign out",
    ]);
  });

  it("a settings page reached from Home has no workspace context", () => {
    render(STAFF, { page: "settings", settingsOrigin: null, ...SETTINGS });
    openMenu(STAFF);
    expect(menuItems()).toEqual(["Account settings", "Sign out"]);
  });

  it("a missing callback still suppresses an inoperable workspace entry", () => {
    // Quotation is the current context, but no callback reaches the page.
    render(STAFF, { page: "create", poSettings: () => {} });
    openMenu(STAFF);
    expect(menuItems()).toEqual(["Account settings", "Sign out"]);
  });

  it("never offers the Admin Dashboard shortcut", () => {
    render(ADMIN, { page: "admin-clients", ...SETTINGS });
    openMenu(ADMIN);
    expect(menuItems()).not.toContain("Admin Dashboard");
  });
});

describe("profile menu structure (issue #494)", () => {
  it("groups general settings, workspace settings and Sign out with separators", () => {
    render(STAFF, { page: "create", ...SETTINGS });
    openMenu(STAFF);
    const dividers = separators();
    expect(dividers).toHaveLength(2);
    for (const divider of dividers) {
      expect(divider.getAttribute("role")).toBe("separator");
      // A divider is not a menu item and carries no name of its own; its role
      // is what tells assistive technology the menu has groups.
      expect(divider.textContent).toBe("");
      expect(divider.getAttribute("aria-hidden")).toBeNull();
    }
    // Account settings | Quotation settings | Sign out.
    const items = menuItemNodes();
    expect(items.map((node) => node.previousElementSibling?.getAttribute("role"))).toEqual([
      null,
      "separator",
      "separator",
    ]);
  });

  it("keeps one separator before Sign out when there is no workspace entry", () => {
    render(STAFF, { page: "home", ...SETTINGS });
    openMenu(STAFF);
    expect(separators()).toHaveLength(1);
    expect(menuItem("Sign out").previousElementSibling?.getAttribute("role")).toBe(
      "separator"
    );
  });

  it("gives every entry a decorative inline icon", () => {
    render(STAFF, { page: "create", ...SETTINGS });
    openMenu(STAFF);
    for (const item of menuItemNodes()) {
      const icon = item.querySelector("svg");
      expect(icon).not.toBeNull();
      expect(icon?.getAttribute("aria-hidden")).toBe("true");
    }
  });
});

describe("profile menu activation (issue #491/#494)", () => {
  it("calls the supplied settings callback instead of navigating", () => {
    const onNavigate = vi.fn();
    const quotationSettings = vi.fn();
    const poSettings = vi.fn();
    render(STAFF, {
      page: "create",
      onNavigate,
      quotationSettings,
      poSettings,
    });
    openMenu(STAFF);
    act(() => menuItem("Quotation settings").click());
    expect(quotationSettings).toHaveBeenCalledTimes(1);
    // The Purchase Order entry is not in this workspace at all.
    expect(menuItemNodes().map((node) => node.textContent?.trim())).not.toContain(
      "Purchase Order settings"
    );
    expect(poSettings).not.toHaveBeenCalled();
    expect(onNavigate).not.toHaveBeenCalled();
  });

  it("navigates through the caller's guard when Account settings is chosen", () => {
    const onNavigate = vi.fn();
    render(ADMIN, { page: "create", onNavigate });
    openMenu(ADMIN);
    act(() => menuItem("Account settings").click());
    expect(onNavigate).toHaveBeenCalledWith("settings");
  });
});

describe("profile menu keyboard operation (issue #494)", () => {
  it("opens from the trigger with an arrow and lands on the requested end", () => {
    render(STAFF, { page: "create", ...SETTINGS });
    focus(trigger(STAFF));
    pressKey(trigger(STAFF), "ArrowDown");
    expect(menuItems()).toHaveLength(3);
    expect(document.activeElement).toBe(menuItem("Account settings"));

    // Escape closes and returns to the trigger, from where ArrowUp opens at
    // the last entry.
    pressKey(menuItem("Account settings"), "Escape");
    expect(menuItems()).toHaveLength(0);
    expect(document.activeElement).toBe(trigger(STAFF));
    pressKey(trigger(STAFF), "ArrowUp");
    const opened = menuItemNodes();
    expect(document.activeElement).toBe(opened[opened.length - 1]);
  });

  it("moves through the entries with arrows, Home and End", () => {
    render(STAFF, { page: "create", ...SETTINGS });
    openMenu(STAFF);
    const [account, quotation, signOut] = menuItemNodes();
    focus(account);
    pressKey(account, "ArrowDown");
    expect(document.activeElement).toBe(quotation);
    pressKey(quotation, "ArrowDown");
    expect(document.activeElement).toBe(signOut);
    pressKey(signOut, "ArrowDown");
    expect(document.activeElement).toBe(account);
    pressKey(account, "ArrowUp");
    expect(document.activeElement).toBe(signOut);
    pressKey(signOut, "Home");
    expect(document.activeElement).toBe(account);
    pressKey(account, "End");
    expect(document.activeElement).toBe(signOut);
  });

  it("closes on Escape and returns focus to the trigger", () => {
    render(STAFF, { page: "create", ...SETTINGS });
    openMenu(STAFF);
    const account = menuItem("Account settings");
    focus(account);
    pressKey(account, "Escape");
    expect(menuItems()).toHaveLength(0);
    expect(document.activeElement).toBe(trigger(STAFF));
  });

  it.each(["Enter", " "])(
    "opens from the trigger with %j on the first entry and closes with Escape there",
    (key) => {
      render(STAFF, { page: "create", ...SETTINGS });
      focus(trigger(STAFF));
      pressKey(trigger(STAFF), key);
      expect(menuItems()).toHaveLength(3);
      expect(document.activeElement).toBe(menuItem("Account settings"));

      // A keyboard user may Shift+Tab back to the trigger while the menu is
      // open; Escape there must dismiss it, not only Escape from an entry.
      focus(trigger(STAFF));
      pressKey(trigger(STAFF), "Escape");
      expect(menuItems()).toHaveLength(0);
      expect(document.activeElement).toBe(trigger(STAFF));
    }
  );

  it("closes when focus leaves an arrow-opened menu", () => {
    render(STAFF, { page: "create", ...SETTINGS });
    const outside = document.createElement("button");
    outside.textContent = "outside";
    document.body.appendChild(outside);
    try {
      focus(trigger(STAFF));
      pressKey(trigger(STAFF), "ArrowDown");
      expect(menuItems()).toHaveLength(3);
      // Tab-out: focus moves to a control outside the trigger+popover group.
      focus(outside);
      expect(menuItems()).toHaveLength(0);
    } finally {
      outside.remove();
    }
  });

  it("closes on a pointerdown outside an arrow-opened menu", () => {
    render(STAFF, { page: "create", ...SETTINGS });
    const outside = document.createElement("button");
    outside.textContent = "outside";
    document.body.appendChild(outside);
    try {
      focus(trigger(STAFF));
      pressKey(trigger(STAFF), "ArrowDown");
      expect(menuItems()).toHaveLength(3);
      // A click on a non-focusable area still has to dismiss the popover.
      act(() => {
        outside.dispatchEvent(new MouseEvent("pointerdown", { bubbles: true }));
      });
      expect(menuItems()).toHaveLength(0);
    } finally {
      outside.remove();
    }
  });

  it("keeps the click-open blur close and returns focus on Escape", () => {
    render(STAFF, { page: "create", ...SETTINGS });
    const outside = document.createElement("button");
    outside.textContent = "outside";
    document.body.appendChild(outside);
    try {
      openMenu(STAFF);
      expect(menuItems()).toHaveLength(3);
      // A pointer press focuses the trigger first (jsdom's click() does not),
      // then the pre-#494 dismissal applies: focus leaving the trigger group
      // closes it.
      focus(trigger(STAFF));
      focus(outside);
      expect(menuItems()).toHaveLength(0);

      openMenu(STAFF);
      focus(trigger(STAFF));
      pressKey(trigger(STAFF), "Escape");
      expect(menuItems()).toHaveLength(0);
      expect(document.activeElement).toBe(trigger(STAFF));
    } finally {
      outside.remove();
    }
  });
});
