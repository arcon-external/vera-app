// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AppShell } from "./AppShell";
import type { SignedInUser } from "./LoginPage";
import { ToastProvider } from "../toast";

// Issue #491: the profile menu carries Account settings, the optional
// issue-#492 settings entries (only when a caller supplies them), Admin
// Dashboard for admins, and Sign out. The notification bell runs its polling
// effect in jsdom, so its bindings are replaced at the module boundary.

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

function render(user: SignedInUser, extra?: { quotationSettings?: () => void; poSettings?: () => void }) {
  act(() => {
    root.render(
      <ToastProvider>
        <AppShell
          page="create"
          onNavigate={() => {}}
          user={user}
          onOpenQuotationSettings={extra?.quotationSettings}
          onOpenPurchaseOrderSettings={extra?.poSettings}
        >
          <div>body</div>
        </AppShell>
      </ToastProvider>
    );
  });
}

function openMenu(user: SignedInUser): void {
  const trigger = document.querySelector<HTMLButtonElement>(
    `button[aria-label="Account menu for ${user.DisplayName}"]`
  );
  if (trigger === null) throw new Error("account menu trigger not found");
  act(() => trigger.click());
}

function menuItems(): string[] {
  return Array.from(document.querySelectorAll('[role="menuitem"]')).map(
    (node) => node.textContent?.trim() ?? ""
  );
}

function menuItem(label: string): HTMLButtonElement {
  const found = Array.from(
    document.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')
  ).find((node) => node.textContent?.trim() === label);
  if (found === undefined) throw new Error(`no menu item ${label}`);
  return found;
}

describe("AppShell profile menu (issue #491)", () => {
  it("offers Account settings, Admin Dashboard and Sign out to an admin", () => {
    render(ADMIN);
    openMenu(ADMIN);
    expect(menuItems()).toEqual(["Account settings", "Admin Dashboard", "Sign out"]);
  });

  it("hides the Admin Dashboard from staff", () => {
    render(STAFF);
    openMenu(STAFF);
    expect(menuItems()).toEqual(["Account settings", "Sign out"]);
  });

  it("renders the issue-#492 settings entries only when callbacks are supplied", () => {
    render(ADMIN, { quotationSettings: () => {}, poSettings: () => {} });
    openMenu(ADMIN);
    expect(menuItems()).toEqual([
      "Account settings",
      "Quotation settings",
      "Purchase Order settings",
      "Admin Dashboard",
      "Sign out",
    ]);
  });

  it("gives staff the same settings entries without the Admin Dashboard", () => {
    render(STAFF, { quotationSettings: () => {}, poSettings: () => {} });
    openMenu(STAFF);
    expect(menuItems()).toEqual([
      "Account settings",
      "Quotation settings",
      "Purchase Order settings",
      "Sign out",
    ]);
  });

  it("calls the supplied settings callbacks instead of navigating", () => {
    const onNavigate = vi.fn();
    const quotationSettings = vi.fn();
    const poSettings = vi.fn();
    act(() => {
      root.render(
        <ToastProvider>
          <AppShell
            page="home"
            onNavigate={onNavigate}
            user={STAFF}
            onOpenQuotationSettings={quotationSettings}
            onOpenPurchaseOrderSettings={poSettings}
          >
            <div>body</div>
          </AppShell>
        </ToastProvider>
      );
    });
    openMenu(STAFF);
    act(() => menuItem("Quotation settings").click());
    expect(quotationSettings).toHaveBeenCalledTimes(1);
    openMenu(STAFF);
    act(() => menuItem("Purchase Order settings").click());
    expect(poSettings).toHaveBeenCalledTimes(1);
    // The caller's guard owns the route, so the shell itself navigates nowhere.
    expect(onNavigate).not.toHaveBeenCalled();
  });

  it("navigates through the caller's guard when Account settings is chosen", () => {
    const onNavigate = vi.fn();
    act(() => {
      root.render(
        <ToastProvider>
          <AppShell page="create" onNavigate={onNavigate} user={ADMIN}>
            <div>body</div>
          </AppShell>
        </ToastProvider>
      );
    });
    openMenu(ADMIN);
    const account = Array.from(
      document.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')
    ).find((node) => node.textContent?.trim() === "Account settings");
    if (account === undefined) throw new Error("Account settings item not found");
    act(() => account.click());
    expect(onNavigate).toHaveBeenCalledWith("settings");
  });
});
