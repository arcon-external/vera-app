// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App";
import { ToastProvider } from "./toast";

// Issue #491 follow-up: the profile menu's Sign out used to call handleSignOut
// directly, bypassing the leave guard, so a dirty Quotation or Company edit was
// discarded with no prompt. This suite renders the real App with only the
// desktop bindings replaced, so the assertion is the production wiring rather
// than a re-implementation of it.

const mocks = vi.hoisted(() => ({
  restoreSession: vi.fn((..._args: unknown[]): unknown => undefined),
  currentUser: vi.fn((..._args: unknown[]): unknown => undefined),
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
  listDrafts: vi.fn(async (..._args: unknown[]) => []),
  listCustomers: vi.fn(async (..._args: unknown[]) => []),
  getCompanyProfile: vi.fn(async (..._args: unknown[]) => ({})),
  getCompanyLogo: vi.fn(async (..._args: unknown[]) => ({ LogoBase64: "", LogoMime: "", BuiltIn: true })),
  getCompanyLabels: vi.fn(async (..._args: unknown[]) => []),
  listClients: vi.fn(async (..._args: unknown[]) => []),
  listUsers: vi.fn(async (..._args: unknown[]) => []),
  listDeletionRequests: vi.fn(async (..._args: unknown[]) => []),
  listEmailChangeRequests: vi.fn(async (..._args: unknown[]) => []),
  myDeletionRequests: vi.fn(async (..._args: unknown[]) => []),
  myEmailChangeRequests: vi.fn(async (..._args: unknown[]) => []),
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
  ID: 1,
  Email: "admin@example.com",
  DisplayName: "Ada Admin",
  Role: "admin",
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
});

async function flush(): Promise<void> {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

function button(label: string): HTMLButtonElement {
  const found = document.querySelector<HTMLButtonElement>(
    `button[aria-label="${label}"]`
  );
  if (found === null) throw new Error(`no button labelled ${label}`);
  return found;
}

function menuItem(label: string): HTMLButtonElement {
  const found = Array.from(
    document.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')
  ).find((node) => node.textContent?.trim() === label);
  if (found === undefined) throw new Error(`no menu item ${label}`);
  return found;
}

function prompt(): HTMLElement | null {
  return document.querySelector<HTMLElement>(
    '[role="alertdialog"][aria-label="Discard unsaved changes?"]'
  );
}

/** A tab in the shell's primary navigation (issue #494: the only Admin nav). */
function navTab(label: string): HTMLButtonElement {
  const nav = document.querySelector<HTMLElement>('nav[aria-label="Primary"]');
  const found = Array.from(
    nav?.querySelectorAll<HTMLButtonElement>("button") ?? []
  ).find((node) => node.textContent?.trim() === label);
  if (found === undefined) throw new Error(`no tab labelled ${label}`);
  return found;
}

function signInButton(): HTMLButtonElement | null {
  return Array.from(document.querySelectorAll<HTMLButtonElement>("button")).find(
    (node) => node.textContent?.trim() === "Sign in"
  ) ?? null;
}

async function openMenuAndSignOut(): Promise<void> {
  await act(async () => {
    button("Account menu for Ada Admin").click();
  });
  await act(async () => {
    menuItem("Sign out").click();
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

describe("Sign out is guarded (issue #491 follow-up)", () => {
  it("asks before discarding dirty Quotation work", async () => {
    await renderApp();

    await act(async () => {
      button("Open Quotation").click();
    });
    const subject = document.querySelector<HTMLInputElement>(
      'input[placeholder="Supply and installation of electrical and mechanical systems"]'
    );
    if (subject === null) throw new Error("subject field not found");
    typeInto(subject, "Unsaved quotation work");

    await openMenuAndSignOut();
    expect(prompt()).not.toBeNull();
    expect(signInButton()).toBeNull();

    // Keep editing cancels the sign-out and keeps the work.
    await act(async () => {
      button("Keep editing").click();
    });
    expect(prompt()).toBeNull();
    expect(signInButton()).toBeNull();
    expect(
      document.querySelector<HTMLInputElement>(
        'input[placeholder="Supply and installation of electrical and mechanical systems"]'
      )?.value
    ).toBe("Unsaved quotation work");

    // Discard clears the dirty state and signs out.
    await openMenuAndSignOut();
    await act(async () => {
      button("Discard changes").click();
    });
    await flush();
    expect(prompt()).toBeNull();
    expect(signInButton()).not.toBeNull();
    expect(mocks.logout).toHaveBeenCalledTimes(1);
  });

  it("asks before discarding dirty Company work", async () => {
    await renderApp();

    // Home -> Admin Dashboard -> Company (issue #494: the Admin workspace is
    // reached from Home; its sections are the shell's primary tabs).
    await act(async () => {
      button("Open Admin Dashboard").click();
    });
    await flush();
    await act(async () => {
      navTab("Company").click();
    });
    await flush();

    const legalName = document.querySelector<HTMLInputElement>(
      'input[placeholder="PT Arcon Penta Persada"]'
    );
    if (legalName === null) throw new Error("legal name field not found");
    typeInto(legalName, "PT Edited");
    expect(document.body.textContent).toContain("Unsaved changes");

    await openMenuAndSignOut();
    expect(prompt()).not.toBeNull();
    expect(signInButton()).toBeNull();

    await act(async () => {
      button("Keep editing").click();
    });
    expect(prompt()).toBeNull();
    expect(signInButton()).toBeNull();
    expect(document.querySelector('[aria-label="Company section"]')).not.toBeNull();
    expect(
      document.querySelector<HTMLInputElement>(
        'input[placeholder="PT Arcon Penta Persada"]'
      )?.value
    ).toBe("PT Edited");

    await openMenuAndSignOut();
    await act(async () => {
      button("Discard changes").click();
    });
    await flush();
    expect(prompt()).toBeNull();
    expect(signInButton()).not.toBeNull();
    expect(mocks.logout).toHaveBeenCalledTimes(1);
  });

  it("signs out immediately when nothing is dirty", async () => {
    await renderApp();
    await openMenuAndSignOut();
    await flush();
    expect(prompt()).toBeNull();
    expect(signInButton()).not.toBeNull();
  });

  it("signs out an untouched PO-first editor without a prompt", async () => {
    await renderApp();
    await act(async () => {
      button("Open Purchase Orders").click();
    });
    await flush();
    expect(document.body.textContent).toContain("New purchase order");

    await openMenuAndSignOut();
    await flush();
    expect(prompt()).toBeNull();
    expect(signInButton()).not.toBeNull();
    expect(mocks.logout).toHaveBeenCalledTimes(1);
  });

  it("guards a dirty PO-first editor and keeps its values on Keep editing", async () => {
    await renderApp();
    await act(async () => {
      button("Open Purchase Orders").click();
    });
    await flush();
    const poNumber = document.querySelector<HTMLInputElement>(
      'input[aria-label="Customer PO number"]'
    );
    if (poNumber === null) throw new Error("customer PO number field not found");
    typeInto(poNumber, "KEEP-PO-496");
    expect(document.body.textContent).toContain("Unsaved changes");

    await openMenuAndSignOut();
    expect(prompt()).not.toBeNull();
    expect(signInButton()).toBeNull();

    await act(async () => {
      button("Keep editing").click();
    });
    await flush();
    expect(prompt()).toBeNull();
    expect(signInButton()).toBeNull();
    expect(
      document.querySelector<HTMLInputElement>(
        'input[aria-label="Customer PO number"]'
      )?.value
    ).toBe("KEEP-PO-496");

    await openMenuAndSignOut();
    await act(async () => {
      button("Discard changes").click();
    });
    await flush();
    expect(prompt()).toBeNull();
    expect(signInButton()).not.toBeNull();
    expect(mocks.logout).toHaveBeenCalledTimes(1);
  });
});
