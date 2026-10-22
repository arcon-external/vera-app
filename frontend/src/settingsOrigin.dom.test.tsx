// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App";
import { ToastProvider } from "./toast";

// Issue #494: the profile menu's workspace settings entry follows the context
// the menu was opened in. This suite drives the real App with only the desktop
// bindings replaced, so the assertions are the production wiring:
//
//   - opening Account settings from a dirty Quotation captures "quotation"
//     only after Discard, and the settings page keeps that context;
//   - Keep editing neither navigates nor captures (nothing is recorded for a
//     navigation the user refused);
//   - a settings page reached from another settings page keeps the origin;
//   - returning Home clears it, so Account settings opened from Home offers no
//     workspace entry;
//   - signing out and back in does not carry a workspace entry into the next
//     session's Account settings.
//
// What this cannot show: the origin is not an observable by itself. It is read
// through the menu on a profile page, so "Keep did not capture" is asserted as
// "the app stayed on the editor and no settings surface rendered", plus the
// subsequent Discard capturing normally.

const mocks = vi.hoisted(() => ({
  restoreSession: vi.fn((..._args: unknown[]): unknown => undefined),
  currentUser: vi.fn((..._args: unknown[]): unknown => undefined),
  login: vi.fn((..._args: unknown[]): unknown => undefined),
  logout: vi.fn(async (..._args: unknown[]) => {}),
  getCompany: vi.fn((..._args: unknown[]): unknown => undefined),
  getCompanyUnits: vi.fn(async (..._args: unknown[]) => [{ Value: "Lot", Label: "Lot" }]),
  getCurrencyCatalogue: vi.fn(async (..._args: unknown[]) => [
    {
      code: "IDR",
      exponent: 0,
      symbol: "Rp",
      position: "before",
      groupSep: ".",
      decimalSep: ",",
      symbolSpaced: false,
    },
  ]),
  listDrafts: vi.fn(async (..._args: unknown[]) => [] as unknown[]),
  listCustomers: vi.fn(async (..._args: unknown[]) => [] as unknown[]),
  getCompanyProfile: vi.fn(async (..._args: unknown[]) => ({})),
  getCompanyLogo: vi.fn(async (..._args: unknown[]) => ({ LogoBase64: "", LogoMime: "", BuiltIn: true })),
  getCompanyLabels: vi.fn(async (..._args: unknown[]) => [] as unknown[]),
  listClients: vi.fn(async (..._args: unknown[]) => [] as unknown[]),
  listUsers: vi.fn(async (..._args: unknown[]) => [] as unknown[]),
  listDeletionRequests: vi.fn(async (..._args: unknown[]) => [] as unknown[]),
  listEmailChangeRequests: vi.fn(async (..._args: unknown[]) => [] as unknown[]),
  myDeletionRequests: vi.fn(async (..._args: unknown[]) => [] as unknown[]),
  myEmailChangeRequests: vi.fn(async (..._args: unknown[]) => [] as unknown[]),
  version: vi.fn(async (..._args: unknown[]) => ""),
  getChangelog: vi.fn(async (..._args: unknown[]) => ""),
  checkForUpdates: vi.fn(async (..._args: unknown[]) => ({ UpdateAvailable: false })),
}));

vi.mock("@backend/backend/AuthAPI", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@backend/backend/AuthAPI")>();
  return {
    ...actual,
    RestoreSession: (...args: unknown[]) => mocks.restoreSession(...args),
    CurrentUser: (...args: unknown[]) => mocks.currentUser(...args),
    Login: (...args: unknown[]) => mocks.login(...args),
    Logout: (...args: unknown[]) => mocks.logout(...args),
    GetCompany: (...args: unknown[]) => mocks.getCompany(...args),
    MyDeletionRequests: (...args: unknown[]) => mocks.myDeletionRequests(...args),
    MyEmailChangeRequests: (...args: unknown[]) => mocks.myEmailChangeRequests(...args),
  };
});

vi.mock("@backend/backend/AdminAPI", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@backend/backend/AdminAPI")>();
  return {
    ...actual,
    GetCompanyUnits: (...args: unknown[]) => mocks.getCompanyUnits(...args),
    GetCurrencyCatalogue: (...args: unknown[]) => mocks.getCurrencyCatalogue(...args),
    GetCompanyProfile: (...args: unknown[]) => mocks.getCompanyProfile(...args),
    GetCompanyLogo: (...args: unknown[]) => mocks.getCompanyLogo(...args),
    GetCompanyLabels: (...args: unknown[]) => mocks.getCompanyLabels(...args),
    ListClients: (...args: unknown[]) => mocks.listClients(...args),
    ListUsers: (...args: unknown[]) => mocks.listUsers(...args),
    ListDeletionRequests: (...args: unknown[]) => mocks.listDeletionRequests(...args),
    ListEmailChangeRequests: (...args: unknown[]) => mocks.listEmailChangeRequests(...args),
  };
});

vi.mock("@backend/backend/Quotation", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@backend/backend/Quotation")>();
  return {
    ...actual,
    ListDrafts: (...args: unknown[]) => mocks.listDrafts(...args),
    ListCustomers: (...args: unknown[]) => mocks.listCustomers(...args),
    ClearDraftContext: vi.fn(async () => {}),
    ClearRevisionContext: vi.fn(async () => {}),
    SetDraftContext: vi.fn(async () => {}),
    SetRevisionContext: vi.fn(async () => {}),
  };
});

vi.mock("@backend/backend/UpdateAPI", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@backend/backend/UpdateAPI")>();
  return {
    ...actual,
    Version: (...args: unknown[]) => mocks.version(...args),
    GetChangelog: (...args: unknown[]) => mocks.getChangelog(...args),
    CheckForUpdates: (...args: unknown[]) => mocks.checkForUpdates(...args),
  };
});

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

const USER = {
  ID: 7,
  Email: "staff@example.com",
  DisplayName: "Sam Staff",
  Role: "staff",
};

const PROFILE = {
  LegalName: "PT Arcon Penta Persada",
  Address: "Jl. Test",
  Phone: "",
  Email: "",
  TaxID: "",
  Currency: "IDR",
  CanonicalUoM: "Lot",
};

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  window.localStorage.clear();
  for (const mock of Object.values(mocks)) mock.mockClear();
  mocks.restoreSession.mockResolvedValue(USER);
  mocks.currentUser.mockResolvedValue(USER);
  mocks.login.mockResolvedValue(USER);
  mocks.getCompany.mockResolvedValue(PROFILE);
  mocks.getCompanyProfile.mockResolvedValue(PROFILE);
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  document.body.innerHTML = "";
  window.localStorage.clear();
});

async function flush(): Promise<void> {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

async function renderApp(): Promise<void> {
  await act(async () => {
    root.render(
      <ToastProvider>
        <App />
      </ToastProvider>
    );
  });
  await flush();
}

function button(label: string): HTMLButtonElement {
  const found = document.querySelector<HTMLButtonElement>(
    `button[aria-label="${label}"]`
  );
  if (found === null) throw new Error(`no button labelled ${label}`);
  return found;
}

function buttonWithText(label: string): HTMLElement {
  const found = Array.from(
    document.querySelectorAll<HTMLElement>("button")
  ).find((node) => node.textContent?.trim() === label);
  if (found === undefined) throw new Error(`no button reading ${label}`);
  return found;
}

function openMenu(): void {
  act(() => button("Account menu for Sam Staff").click());
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

async function chooseMenuItem(label: string): Promise<void> {
  await act(async () => {
    menuItem(label).click();
  });
  await flush();
}

async function openMenuItem(label: string): Promise<void> {
  openMenu();
  await chooseMenuItem(label);
}

function prompt(): HTMLElement | null {
  return document.querySelector<HTMLElement>(
    '[role="alertdialog"][aria-label="Discard unsaved changes?"]'
  );
}

function workspace(): string | null {
  return (
    document
      .querySelector<HTMLElement>("[data-app-shell]")
      ?.getAttribute("data-workspace") ?? null
  );
}

/** The origin the shell was handed; absent means "no remembered context". */
function originAttr(): string | null {
  return (
    document
      .querySelector<HTMLElement>("[data-app-shell]")
      ?.getAttribute("data-settings-origin") ?? null
  );
}

function typeInto(element: HTMLInputElement, value: string): void {
  act(() => {
    const setter = Object.getOwnPropertyDescriptor(
      HTMLInputElement.prototype,
      "value"
    )?.set;
    setter?.call(element, value);
    element.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

function subjectField(): HTMLInputElement {
  const found = document.querySelector<HTMLInputElement>(
    'input[placeholder="Supply and installation of electrical and mechanical systems"]'
  );
  if (found === null) throw new Error("subject field not found");
  return found;
}

/** Opens the Quotation editor and makes it dirty. */
async function dirtyQuotation(): Promise<void> {
  await act(async () => {
    button("Open Quotation").click();
  });
  typeInto(subjectField(), "Unsaved quotation work");
}

/** Quotation -> Account settings, discarding the dirty editor. */
async function reachSettingsFromQuotation(): Promise<void> {
  await dirtyQuotation();
  await openMenuItem("Account settings");
  expect(prompt()).not.toBeNull();
  await act(async () => {
    button("Discard changes").click();
  });
  await flush();
  expect(workspace()).toBe("account");
}

describe("settings origin capture (issue #494)", () => {
  it("captures the workspace on Discard and keeps it through settings pages", async () => {
    await renderApp();
    await reachSettingsFromQuotation();

    expect(document.querySelector("#settings-heading")).not.toBeNull();
    expect(originAttr()).toBe("quotation");
    openMenu();
    expect(menuItems()).toEqual([
      "Account settings",
      "Quotation settings",
      "Sign out",
    ]);

    // Settings -> settings retains the origin the first visit captured.
    await act(async () => {
      menuItem("Quotation settings").click();
    });
    await flush();
    expect(workspace()).toBe("quotation-settings");
    expect(document.querySelector("#quotation-settings-heading")).not.toBeNull();
    expect(originAttr()).toBe("quotation");
    openMenu();
    expect(menuItems()).toEqual([
      "Account settings",
      "Quotation settings",
      "Sign out",
    ]);
  });

  it("does not navigate or capture while the user keeps editing", async () => {
    await renderApp();
    await dirtyQuotation();
    // No context is remembered yet: the app started on Home.
    expect(originAttr()).toBeNull();

    await openMenuItem("Account settings");
    expect(prompt()).not.toBeNull();
    // The origin is recorded in the allowed-navigation callback, so asking the
    // question must not record anything yet.
    expect(originAttr()).toBeNull();
    await act(async () => {
      button("Keep editing").click();
    });
    expect(prompt()).toBeNull();
    // The refused navigation left the editor mounted, rendered no settings
    // surface and captured nothing; the menu still speaks for the current
    // workspace.
    expect(originAttr()).toBeNull();
    expect(workspace()).toBe("quotation");
    expect(document.querySelector("#settings-heading")).toBeNull();
    expect(subjectField().value).toBe("Unsaved quotation work");
    openMenu();
    expect(menuItems()).toEqual([
      "Account settings",
      "Quotation settings",
      "Sign out",
    ]);

    // The next attempt, now discarded, captures normally.
    await chooseMenuItem("Account settings");
    await act(async () => {
      button("Discard changes").click();
    });
    await flush();
    expect(workspace()).toBe("account");
    expect(originAttr()).toBe("quotation");
    openMenu();
    expect(menuItems()).toEqual([
      "Account settings",
      "Quotation settings",
      "Sign out",
    ]);
  });

  it("clears the origin on a return to Home", async () => {
    await renderApp();
    await reachSettingsFromQuotation();
    expect(originAttr()).toBe("quotation");

    await act(async () => {
      button("Back to Home").click();
    });
    await flush();
    expect(workspace()).toBe("home");
    // The successful return to Home ends the remembered context itself, not
    // merely hiding it behind Home's own null context.
    expect(originAttr()).toBeNull();
    openMenu();
    expect(menuItems()).toEqual(["Account settings", "Sign out"]);

    await chooseMenuItem("Account settings");
    expect(workspace()).toBe("account");
    expect(originAttr()).toBeNull();
    openMenu();
    expect(menuItems()).toEqual(["Account settings", "Sign out"]);
  });

  it("does not carry the origin across sign-out and sign-in", async () => {
    await renderApp();
    await reachSettingsFromQuotation();
    expect(originAttr()).toBe("quotation");

    openMenu();
    await act(async () => {
      menuItem("Sign out").click();
    });
    await flush();
    expect(workspace()).toBeNull();
    expect(buttonWithText("Sign in")).not.toBeNull();

    const form = document.querySelector("form");
    if (form === null) throw new Error("sign-in form not found");
    const email = document.querySelector<HTMLInputElement>(
      'input[placeholder="you@company.com"]'
    );
    const password = document.querySelector<HTMLInputElement>(
      'input[placeholder="••••••••"]'
    );
    if (email === null || password === null) throw new Error("sign-in fields not found");
    typeInto(email, USER.Email);
    typeInto(password, "secret");
    await act(async () => {
      form.dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
    });
    await flush();

    expect(workspace()).toBe("home");
    expect(originAttr()).toBeNull();
    await openMenuItem("Account settings");
    expect(workspace()).toBe("account");
    expect(originAttr()).toBeNull();
    openMenu();
    expect(menuItems()).toEqual(["Account settings", "Sign out"]);
  });
});

describe("Purchase Order settings origin (issue #494)", () => {
  async function openPurchaseOrders(): Promise<void> {
    await act(async () => {
      button("Open Purchase Orders").click();
    });
    await flush();
    expect(workspace()).toBe("purchase-order");
  }

  it("captures purchase-order from the Purchase Order settings entry", async () => {
    await renderApp();
    await openPurchaseOrders();
    // The PO workspace names its own context before anything is remembered.
    expect(originAttr()).toBeNull();

    await openMenuItem("Purchase Order settings");
    expect(workspace()).toBe("purchase-order-settings");
    expect(originAttr()).toBe("purchase-order");
    openMenu();
    expect(menuItems()).toEqual([
      "Account settings",
      "Purchase Order settings",
      "Sign out",
    ]);
  });

  it("retains purchase-order through Account settings", async () => {
    await renderApp();
    await openPurchaseOrders();
    expect(originAttr()).toBeNull();

    await openMenuItem("Account settings");
    expect(workspace()).toBe("account");
    expect(originAttr()).toBe("purchase-order");
    openMenu();
    expect(menuItems()).toEqual([
      "Account settings",
      "Purchase Order settings",
      "Sign out",
    ]);
  });
});
