// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App";
import { ToastProvider } from "./toast";
import { RECOVERY_DEBOUNCE_MS, parseRecoverySnapshot } from "./recovery";
import type { CustomerPreview, InternalBreakdown } from "./preview";

// Issue #537: the local recovery snapshot, driven through the real App. Only
// the desktop bindings are replaced, so these tests exercise the production
// sign-in load, the production banner, the debounce and every clear path.

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
  listCustomers: vi.fn(async (..._args: unknown[]): Promise<unknown[]> => []),
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
  prepare: vi.fn(async (..._args: unknown[]) => {}),
  exportQuotation: vi.fn(async (..._args: unknown[]) => {}),
  getCustomerPreview: vi.fn(async (..._args: unknown[]) => ({})),
  getInternalBreakdown: vi.fn(async (..._args: unknown[]) => ({})),
  getQuotationPreviewHTML: vi.fn(async (..._args: unknown[]) => "<html></html>"),
  saveDraft: vi.fn(async (..._args: unknown[]) => 5),
  loadRecovery: vi.fn(async (..._args: unknown[]): Promise<string> => ""),
  saveRecovery: vi.fn(async (..._args: unknown[]) => {}),
  clearRecovery: vi.fn(async (..._args: unknown[]) => {}),
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
    Prepare: (...args: unknown[]) => mocks.prepare(...args),
    Export: (...args: unknown[]) => mocks.exportQuotation(...args),
    GetCustomerPreview: (...args: unknown[]) => mocks.getCustomerPreview(...args),
    GetInternalBreakdown: (...args: unknown[]) => mocks.getInternalBreakdown(...args),
    GetQuotationPreviewHTML: (...args: unknown[]) => mocks.getQuotationPreviewHTML(...args),
    SaveDraft: (...args: unknown[]) => mocks.saveDraft(...args),
    ClearDraftContext: vi.fn(async () => {}),
    ClearRevisionContext: vi.fn(async () => {}),
    SetDraftContext: vi.fn(async () => {}),
    SetRevisionContext: vi.fn(async () => {}),
  };
});

vi.mock("@backend/backend/RecoveryAPI", () => ({
  LoadRecovery: (...args: unknown[]) => mocks.loadRecovery(...args),
  SaveRecovery: (...args: unknown[]) => mocks.saveRecovery(...args),
  ClearRecovery: (...args: unknown[]) => mocks.clearRecovery(...args),
}));

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

const CUSTOMER_PREVIEW: CustomerPreview = {
  Items: [],
  Subject: "Unsaved quotation work",
  RecipientName: "",
  RecipientCompany: "",
  RecipientAddress: "",
  QuoteNumber: "",
  RevisionID: 0,
  Incoterm: "EXW",
  DisplaySubTotal: "Rp0",
  DisplayDiscountAmount: "Rp0",
  DisplayGrandTotal: "Rp0",
  DisplayPpnAmount: "Rp0",
  SubTotal: 0,
  DiscountAmount: 0,
  GrandTotal: 0,
  PpnAmount: 0,
  OriginalSubTotal: 0,
  OriginalDiscountAmount: 0,
  OriginalGrandTotal: 0,
  DiscountPercent: 0,
  DisplayLeadTime: "-",
  LeadTimeYears: 0,
  LeadTimeMonths: 0,
  LeadTimeWeeks: 0,
  LeadTimeDays: 0,
};

const INTERNAL_BREAKDOWN: InternalBreakdown = {
  Items: [],
  TaxLabel: "",
  TaxType: "Goods",
  ServiceCategory: "",
  PpnAmount: 0,
  DisplayPpn: "Rp0",
  DisplayCost: "Rp0",
  DisplaySell: "Rp0",
  DisplayCostDisc: "Rp0",
  DisplaySellDisc: "Rp0",
  DisplayCostGrd: "Rp0",
  DisplayGrand: "Rp0",
  DisplayProfit: "Rp0",
  DisplayTax: "Rp0",
  DisplayNett: "Rp0",
  DisplayOriginalSubtotal: "Rp0",
  DisplayOriginalDiscount: "Rp0",
  DisplayOriginalGrand: "Rp0",
  DisplayMarginalizedSubtotal: "Rp0",
  DisplayMarginalizedDiscount: "Rp0",
  DisplayMarginalizedGrand: "Rp0",
  DisplayMarginalizedAfterTax: "Rp0",
  CostSubtotal: 0,
  SellSubtotal: 0,
  DiscountCost: 0,
  DiscountSell: 0,
  CostGrand: 0,
  GrandTotal: 0,
  ProfitAmount: 0,
  TaxAmount: 0,
  NettProfit: 0,
  TaxRateBps: 0,
  DiscountPercent: 0,
  OriginalSubtotal: 0,
  OriginalDiscount: 0,
  OriginalGrand: 0,
  MarginalizedSubtotal: 0,
  MarginalizedDiscount: 0,
  MarginalizedGrand: 0,
  MarginalizedGrandAfterTax: 0,
};

function snapshotJSON(): string {
  return JSON.stringify({
    version: 1,
    savedAt: "2026-11-09T14:14:00",
    draft: {
      items: [
        {
          key: 0,
          description: "Recovered cable tray",
          bullets: [],
          qty: "2",
          uom: "Lot",
          unitPrice: "1500000",
          marginEnabled: false,
          marginPercent: "0",
          discountEnabled: false,
          discountPercent: "0",
          subItems: [],
          showSubPrices: false,
        },
      ],
      details: {
        subject: "Recovered quotation",
        recipientName: "Ada",
        validity: 30,
        paymentTerm: 90,
        leadTimeYears: 0,
        leadTimeMonths: 0,
        leadTimeWeeks: 0,
        leadTimeDays: 0,
        discountPercent: 0,
        incoterm: "EXW",
        taxZone: "FTZ",
        taxType: "Goods",
        serviceCategory: "",
        note: "Deliver by Friday",
      },
      customerId: 5,
    },
  });
}

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
  mocks.getCustomerPreview.mockResolvedValue(CUSTOMER_PREVIEW);
  mocks.getInternalBreakdown.mockResolvedValue(INTERNAL_BREAKDOWN);
  mocks.loadRecovery.mockResolvedValue("");
  intervals = new Map();
  nextIntervalID = 1;
  vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
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
  vi.useRealTimers();
  vi.restoreAllMocks();
});

async function flush(): Promise<void> {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(0);
  });
}

function button(label: string): HTMLButtonElement {
  const found = document.querySelector<HTMLButtonElement>(
    `button[aria-label="${label}"]`
  );
  if (found === null) throw new Error(`no button labelled ${label}`);
  return found;
}

function textButton(label: string): HTMLButtonElement {
  const found = Array.from(document.querySelectorAll<HTMLButtonElement>("button")).find(
    (node) => node.textContent?.trim() === label
  );
  if (found === undefined) throw new Error(`no button ${label}`);
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

function lineDescription(): HTMLInputElement | null {
  return document.querySelector<HTMLInputElement>(
    'input[aria-label="Description for line 1"]'
  );
}

function noteBox(): HTMLElement | null {
  return document.querySelector<HTMLElement>('[role="textbox"][aria-label="Note"]');
}

function typeInto(
  element: HTMLInputElement | HTMLTextAreaElement,
  value: string
): void {
  act(() => {
    const prototype =
      element instanceof HTMLTextAreaElement
        ? HTMLTextAreaElement.prototype
        : HTMLInputElement.prototype;
    const setter = Object.getOwnPropertyDescriptor(prototype, "value")?.set;
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
  const description = lineDescription();
  if (description === null) throw new Error("line description not found");
  typeInto(description, "Cable tray");
  await flush();
}

async function signIn(email: string, password: string): Promise<void> {
  const emailInput = document.querySelector<HTMLInputElement>('input[type="email"]');
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

async function advance(ms: number): Promise<void> {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
  });
}

function recoveryBannerShown(): boolean {
  return (document.body.textContent ?? "").includes("Vera found unsaved work");
}

describe("Local recovery snapshot (#537)", () => {
  it("offers a saved snapshot after sign-in and restores it", async () => {
    mocks.listCustomers.mockResolvedValue([
      { Code: 2, Name: "Default Customer" },
      { Code: 5, Name: "Acme" },
    ]);
    mocks.loadRecovery.mockResolvedValue(snapshotJSON());
    await renderApp();

    expect(mocks.loadRecovery).toHaveBeenCalledWith(USER.ID);
    expect(recoveryBannerShown()).toBe(true);
    expect(document.body.textContent).toContain("2:14 PM");

    await act(async () => {
      button("Open Quotation").click();
    });
    await flush();
    expect(button("Customer").textContent).toContain("Default Customer");

    await act(async () => {
      textButton("Restore").click();
    });
    await flush();

    expect(recoveryBannerShown()).toBe(false);
    expect(subjectInput()?.value).toBe("Recovered quotation");
    expect(lineDescription()?.value).toBe("Recovered cable tray");
    expect(noteBox()?.textContent).toContain("Deliver by Friday");
    expect(button("Customer").textContent).toContain("Acme");
    expect(mocks.clearRecovery).toHaveBeenCalledWith(USER.ID);
  });

  it("discards the snapshot and leaves the editor untouched", async () => {
    mocks.loadRecovery.mockResolvedValue(snapshotJSON());
    await renderApp();
    expect(recoveryBannerShown()).toBe(true);

    await act(async () => {
      textButton("Discard").click();
    });
    await flush();

    expect(recoveryBannerShown()).toBe(false);
    expect(mocks.clearRecovery).toHaveBeenCalledWith(USER.ID);

    await act(async () => {
      button("Open Quotation").click();
    });
    await flush();
    expect(subjectInput()?.value).toBe("");
    expect(lineDescription()?.value).toBe("");
  });

  it("saves once after the debounce and not again while idle", async () => {
    await renderApp();
    await openQuotationWithWork();

    await advance(RECOVERY_DEBOUNCE_MS);
    expect(mocks.saveRecovery).toHaveBeenCalledTimes(1);
    const [userID, payload] = mocks.saveRecovery.mock.calls[0] as [number, string];
    expect(userID).toBe(USER.ID);
    const snapshot = parseRecoverySnapshot(payload);
    expect(snapshot).not.toBeNull();
    expect(snapshot?.draft.details.subject).toBe("Unsaved quotation work");
    expect(snapshot?.draft.items[0].description).toBe("Cable tray");

    await advance(5000);
    expect(mocks.saveRecovery).toHaveBeenCalledTimes(1);
  });

  it("does not overwrite a pending snapshot while the banner waits", async () => {
    mocks.loadRecovery.mockResolvedValue(snapshotJSON());
    await renderApp();
    expect(recoveryBannerShown()).toBe(true);

    await advance(RECOVERY_DEBOUNCE_MS * 2);
    expect(mocks.saveRecovery).not.toHaveBeenCalled();
  });

  it("logs a failed save once per session and never toasts it", async () => {
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => {});
    mocks.saveRecovery.mockRejectedValue(new Error("disk full"));
    await renderApp();
    await openQuotationWithWork();

    await advance(RECOVERY_DEBOUNCE_MS);
    expect(mocks.saveRecovery).toHaveBeenCalledTimes(1);
    expect(consoleError).toHaveBeenCalledTimes(1);
    expect(document.body.textContent).not.toContain("disk full");

    typeInto(subjectInput()!, "Unsaved quotation work, more");
    await advance(RECOVERY_DEBOUNCE_MS);
    expect(mocks.saveRecovery).toHaveBeenCalledTimes(2);
    expect(consoleError).toHaveBeenCalledTimes(1);
  });

  it("clears the snapshot after a successful draft save", async () => {
    await renderApp();
    await openQuotationWithWork();

    await act(async () => {
      textButton("Save draft").click();
    });
    await flush();

    expect(mocks.saveDraft).toHaveBeenCalledTimes(1);
    expect(mocks.clearRecovery).toHaveBeenCalledWith(USER.ID);

    // The pending debounce was cancelled: a saved draft is not crash work.
    await advance(RECOVERY_DEBOUNCE_MS * 2);
    expect(mocks.saveRecovery).not.toHaveBeenCalled();
  });

  it("clears the snapshot after a successful export", async () => {
    await renderApp();
    await openQuotationWithWork();

    await act(async () => {
      textButton("Prepare").click();
    });
    await flush();
    expect(mocks.prepare).toHaveBeenCalledTimes(1);

    await act(async () => {
      textButton("Export PDF").click();
    });
    await flush();

    expect(mocks.exportQuotation).toHaveBeenCalledTimes(1);
    expect(mocks.clearRecovery).toHaveBeenCalledWith(USER.ID);

    // The debounce that was already running when the export succeeded must not
    // write the just-exported quotation back as crash work.
    await advance(RECOVERY_DEBOUNCE_MS * 2);
    expect(mocks.saveRecovery).not.toHaveBeenCalled();
  });

  it("clears the snapshot on a manual sign-out", async () => {
    await renderApp();
    await openQuotationWithWork();

    await act(async () => {
      button("Account menu for Ada Admin").click();
    });
    await act(async () => {
      menuItem("Sign out").click();
    });
    await act(async () => {
      textButton("Discard changes").click();
    });
    await flush();

    expect(signInButton()).not.toBeNull();
    expect(mocks.clearRecovery).toHaveBeenCalledWith(USER.ID);
  });

  it("clears the snapshot when the user deliberately starts a new quotation", async () => {
    await renderApp();
    await openQuotationWithWork();

    await act(async () => {
      textButton("New").click();
    });
    await act(async () => {
      textButton("Discard").click();
    });
    await flush();

    expect(mocks.clearRecovery).toHaveBeenCalledWith(USER.ID);
    expect(subjectInput()?.value).toBe("");
  });

  it("keeps the snapshot on an involuntary session end and prefers the live editor", async () => {
    mocks.loadRecovery.mockResolvedValue(snapshotJSON());
    mocks.restoreSession.mockResolvedValue(USER);
    await renderApp();

    // The banner was answered by starting work: the editor now holds newer
    // work than the file, and the banner must not return.
    await act(async () => {
      textButton("Discard").click();
    });
    await flush();
    mocks.clearRecovery.mockClear();
    mocks.saveRecovery.mockClear();
    await openQuotationWithWork();

    mocks.checkSession.mockResolvedValue({
      State: "revoked",
      Code: "session_revoked",
      Message: "session was signed out",
    });
    await act(async () => {
      for (const handler of [...intervals.values()]) handler();
      await vi.advanceTimersByTimeAsync(0);
    });
    await flush();

    expect(signInButton()).not.toBeNull();
    expect(mocks.clearRecovery).not.toHaveBeenCalled();
    expect(subjectInput()).toBeNull();

    // The same user signs back in: the in-memory editor is still newer than
    // any file, so no restore prompt appears and nothing is cleared.
    mocks.loadRecovery.mockResolvedValue(snapshotJSON());
    await signIn("admin@example.com", "correct horse");
    expect(mocks.loadRecovery).toHaveBeenCalledWith(USER.ID);
    expect(recoveryBannerShown()).toBe(false);
    expect(mocks.clearRecovery).not.toHaveBeenCalled();
    expect(subjectInput()?.value).toBe("Unsaved quotation work");
  });

  it("scopes load and clear to the account that signed in", async () => {
    mocks.restoreSession.mockResolvedValue(null);
    mocks.login.mockResolvedValue(OTHER_USER);
    mocks.loadRecovery.mockImplementation(async (userID: unknown) =>
      userID === OTHER_USER.ID ? snapshotJSON() : ""
    );
    await renderApp();
    expect(signInButton()).not.toBeNull();

    await signIn("grace@example.com", "correct horse");

    expect(mocks.loadRecovery).toHaveBeenCalledWith(OTHER_USER.ID);
    expect(mocks.loadRecovery).not.toHaveBeenCalledWith(USER.ID);
    expect(recoveryBannerShown()).toBe(true);

    await act(async () => {
      textButton("Discard").click();
    });
    await flush();
    expect(mocks.clearRecovery).toHaveBeenCalledWith(OTHER_USER.ID);
    expect(mocks.clearRecovery).not.toHaveBeenCalledWith(USER.ID);
  });
});
