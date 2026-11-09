// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App";
import { ToastProvider } from "./toast";

// Issue #531: the 60-second session check is typed data, not an error string.
// These tests render the real App with only the desktop bindings replaced, so
// the assertions are the production poll, the production reset paths and the
// production re-login surface rather than a re-implementation of them.

const mocks = vi.hoisted(() => ({
  restoreSession: vi.fn((..._args: unknown[]): unknown => undefined),
  checkSession: vi.fn((..._args: unknown[]): unknown => undefined),
  renewSession: vi.fn((..._args: unknown[]): unknown => undefined),
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
    CheckSession: (...args: unknown[]) => mocks.checkSession(...args),
    RenewSession: (...args: unknown[]) => mocks.renewSession(...args),
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
  ID: 1,
  Email: "admin@example.com",
  DisplayName: "Ada Admin",
  Role: "admin",
};

const OTHER_USER = {
  ID: 2,
  Email: "grace@example.com",
  DisplayName: "Grace Staff",
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

const SUBJECT_PLACEHOLDER =
  "Supply and installation of electrical and mechanical systems";

let container: HTMLDivElement;
let root: Root;
let intervals: Map<number, () => void>;
let nextIntervalID: number;

beforeEach(() => {
  window.localStorage.clear();
  for (const mock of Object.values(mocks)) mock.mockClear();
  mocks.restoreSession.mockResolvedValue(USER);
  mocks.checkSession.mockResolvedValue({ State: "active", User: USER });
  mocks.renewSession.mockResolvedValue({ State: "active", User: USER });
  mocks.login.mockResolvedValue(USER);
  mocks.getCompany.mockResolvedValue(PROFILE);
  mocks.getCompanyProfile.mockResolvedValue(PROFILE);
  intervals = new Map();
  nextIntervalID = 1;
  vi.spyOn(window, "setInterval").mockImplementation((handler) => {
    const id = nextIntervalID++;
    intervals.set(id, handler as () => void);
    return id as unknown as ReturnType<typeof window.setInterval>;
  });
  vi.spyOn(window, "clearInterval").mockImplementation((id) => {
    intervals.delete(Number(id));
  });
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  document.body.innerHTML = "";
  vi.restoreAllMocks();
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

function signInButton(): HTMLButtonElement | null {
  return Array.from(document.querySelectorAll<HTMLButtonElement>("button")).find(
    (node) => node.textContent?.trim() === "Sign in"
  ) ?? null;
}

function subjectInput(): HTMLInputElement | null {
  return document.querySelector<HTMLInputElement>(
    `input[placeholder="${SUBJECT_PLACEHOLDER}"]`
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

async function openQuotationWithWork(): Promise<void> {
  await act(async () => {
    button("Open Quotation").click();
  });
  await flush();
  const subject = subjectInput();
  if (subject === null) throw new Error("subject field not found");
  typeInto(subject, "Unsaved quotation work");
}

/** Fires every live 60-second interval: the session poll and the notification
 *  bell. The bell's read is mocked, so only the poll's state transition is
 *  under test. */
async function runPolls(): Promise<void> {
  const handlers = [...intervals.values()];
  await act(async () => {
    for (const handler of handlers) handler();
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
  await flush();
}

async function signIn(email: string, password: string): Promise<void> {
  const emailInput = document.querySelector<HTMLInputElement>(
    'input[type="email"]'
  );
  const passwordInput = document.querySelector<HTMLInputElement>(
    'input[type="password"]'
  );
  if (emailInput === null || passwordInput === null) {
    throw new Error("sign-in fields not found");
  }
  typeInto(emailInput, email);
  typeInto(passwordInput, password);
  const submit = signInButton();
  if (submit === null) throw new Error("sign-in button not found");
  await act(async () => {
    submit.click();
  });
  await flush();
}

describe("Typed session poll (issue #531)", () => {
  it("does not sign out on a transport failure and keeps the editor", async () => {
    await renderApp();
    await openQuotationWithWork();

    mocks.checkSession.mockResolvedValue({
      State: "unreachable",
      Code: "session_unreachable",
      Message: "reach the quotation server: EOF",
    });
    await runPolls();

    expect(signInButton()).toBeNull();
    expect(subjectInput()?.value).toBe("Unsaved quotation work");
  });

  it("does not sign out on a 5xx, a 429, or a rejected binding", async () => {
    await renderApp();
    await openQuotationWithWork();

    for (const status of [
      { State: "server_error", Code: "session_check_failed" },
      { State: "rate_limited", Code: "rate_limited" },
    ]) {
      mocks.checkSession.mockResolvedValue(status);
      await runPolls();
      expect(signInButton(), status.State).toBeNull();
      expect(subjectInput()?.value, status.State).toBe("Unsaved quotation work");
    }

    mocks.checkSession.mockRejectedValue(new Error("Wails rejected the binding"));
    await runPolls();
    expect(signInButton()).toBeNull();
    expect(subjectInput()?.value).toBe("Unsaved quotation work");
  });

  it.each(["expired", "revoked", "inactive", "invalid"])(
    "signs out on a real %s session end, preserves the work, and restores it for the same user",
    async (state) => {
      await renderApp();
      await openQuotationWithWork();

      mocks.checkSession.mockResolvedValue({
        State: state,
        Code: `session_${state}`,
        Message: `session ${state}`,
      });
      await runPolls();

      expect(signInButton()).not.toBeNull();
      expect(document.body.textContent).toContain("Your session ended");

      mocks.login.mockResolvedValue(USER);
      await signIn("admin@example.com", "correct horse");

      expect(signInButton()).toBeNull();
      expect(subjectInput()?.value).toBe("Unsaved quotation work");
    }
  );

  it("clears the preserved work when a different user signs in", async () => {
    await renderApp();
    await openQuotationWithWork();

    mocks.checkSession.mockResolvedValue({
      State: "revoked",
      Code: "session_revoked",
      Message: "session was signed out",
    });
    await runPolls();
    expect(signInButton()).not.toBeNull();

    mocks.login.mockResolvedValue(OTHER_USER);
    await signIn("grace@example.com", "correct horse");

    expect(signInButton()).toBeNull();
    expect(subjectInput()).toBeNull();
  });

  it("manual sign-out still clears the workspace", async () => {
    await renderApp();
    await openQuotationWithWork();

    await act(async () => {
      button("Account menu for Ada Admin").click();
    });
    await act(async () => {
      menuItem("Sign out").click();
    });
    await act(async () => {
      button("Discard changes").click();
    });
    await flush();
    expect(signInButton()).not.toBeNull();

    mocks.login.mockResolvedValue(USER);
    await signIn("admin@example.com", "correct horse");
    expect(subjectInput()).toBeNull();
  });
});

// Ticket #536: the "Stay signed in" window. The banner appears only inside
// the last ten minutes of a live, renewable session; renewal extends it, and
// a renewal that reports a real end uses #531's preservation path rather than
// the destructive reset.
describe("Stay signed in (#536)", () => {
  function expiryIn(ms: number): string {
    return new Date(Date.now() + ms).toISOString();
  }

  function renewButton(): HTMLButtonElement | null {
    return Array.from(document.querySelectorAll<HTMLButtonElement>("button")).find(
      (node) => node.textContent?.trim() === "Stay signed in"
    ) ?? null;
  }

  function nearExpiry(canRenew = true): Record<string, unknown> {
    return {
      State: "active",
      User: USER,
      Code: "session_active",
      ExpiresAt: expiryIn(5 * 60 * 1000),
      CanRenew: canRenew,
    };
  }

  it("offers renewal inside the window and extends the session on click", async () => {
    mocks.checkSession.mockResolvedValue(nearExpiry());
    mocks.renewSession.mockResolvedValue({
      State: "active",
      User: USER,
      Code: "session_active",
      ExpiresAt: expiryIn(8 * 60 * 60 * 1000),
      CanRenew: true,
    });
    await renderApp();
    await runPolls();

    expect(document.body.textContent).toContain("Your session ends");
    const renew = renewButton();
    if (renew === null) throw new Error("no Stay signed in button");

    await act(async () => {
      renew.click();
    });
    await flush();

    expect(mocks.renewSession).toHaveBeenCalledTimes(1);
    expect(document.body.textContent).not.toContain("Your session ends");
  });

  it("stays hidden before the window and when the server forbids renewal", async () => {
    mocks.checkSession.mockResolvedValue({
      ...nearExpiry(),
      ExpiresAt: expiryIn(30 * 60 * 1000),
    });
    await renderApp();
    await runPolls();
    expect(document.body.textContent).not.toContain("Your session ends");

    mocks.checkSession.mockResolvedValue(nearExpiry(false));
    await runPolls();
    expect(document.body.textContent).not.toContain("Your session ends");
  });

  it("preserves the work when renewal reports a real session end", async () => {
    mocks.checkSession.mockResolvedValue(nearExpiry());
    await renderApp();
    await runPolls();
    await openQuotationWithWork();

    mocks.renewSession.mockResolvedValue({
      State: "revoked",
      Code: "session_revoked",
      Message: "session was signed out",
    });
    const renew = renewButton();
    if (renew === null) throw new Error("no Stay signed in button");
    await act(async () => {
      renew.click();
    });
    await flush();

    expect(signInButton()).not.toBeNull();
    expect(document.body.textContent).toContain("Your session ended");

    mocks.login.mockResolvedValue(USER);
    await signIn("admin@example.com", "correct horse");
    expect(signInButton()).toBeNull();
    expect(subjectInput()?.value).toBe("Unsaved quotation work");
  });

  it("keeps the session and the work when renewal fails operationally", async () => {
    mocks.checkSession.mockResolvedValue(nearExpiry());
    await renderApp();
    await runPolls();
    await openQuotationWithWork();

    for (const failure of [
      { State: "unreachable", Code: "session_unreachable", Message: "reach the quotation server: EOF" },
      { State: "server_error", Code: "session_check_failed", Message: "session renewal failed" },
    ]) {
      mocks.renewSession.mockResolvedValue(failure);
      const renew = renewButton();
      if (renew === null) throw new Error("no Stay signed in button");
      await act(async () => {
        renew.click();
      });
      await flush();

      expect(signInButton(), failure.State).toBeNull();
      expect(subjectInput()?.value, failure.State).toBe("Unsaved quotation work");
      expect(document.body.textContent, failure.State).toContain("Your session ends");
    }

    mocks.renewSession.mockRejectedValue(new Error("Wails rejected the binding"));
    const renew = renewButton();
    if (renew === null) throw new Error("no Stay signed in button");
    await act(async () => {
      renew.click();
    });
    await flush();
    expect(signInButton()).toBeNull();
    expect(subjectInput()?.value).toBe("Unsaved quotation work");
  });
});
