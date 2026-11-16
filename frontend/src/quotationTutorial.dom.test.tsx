// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App";
import { ToastProvider } from "./toast";
import {
  TUTORIAL_CUSTOMER,
  TUTORIAL_SUBJECT,
} from "./featuresTutorial/tutorialData";

// Issue #605: the Quotation tour runs on a client-only practice quotation.
// These cases drive the real App with only the desktop bindings replaced, so
// what is asserted is the production wiring: the practice panel replaces the
// real editor on first open, the real rail computes its totals, the shell
// Save/Prepare/Export handlers refuse without touching a binding, and
// leaving or replaying the tour drops every practice edit.
//
// What this cannot show: pixel placement (jsdom has no layout) and the
// browser's own paste behaviour inside the practice fields, which is left to
// the browser on purpose and is not asserted here.

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
  listDrafts: vi.fn(async (..._args: unknown[]) => [] as unknown[]),
  listCustomers: vi.fn(async (..._args: unknown[]) => [] as unknown[]),
  getDraft: vi.fn((..._args: unknown[]): unknown => undefined),
  saveDraft: vi.fn(async (..._args: unknown[]) => 1),
  prepare: vi.fn(async (..._args: unknown[]) => {}),
  exportPdf: vi.fn(async (..._args: unknown[]) => {}),
  getCustomerPreview: vi.fn(async (..._args: unknown[]) => ({})),
  getInternalBreakdown: vi.fn(async (..._args: unknown[]) => ({})),
  getQuotationPreviewHTML: vi.fn(async (..._args: unknown[]) => ""),
  setDraftContext: vi.fn(async (..._args: unknown[]) => {}),
  getCompanyProfile: vi.fn(async (..._args: unknown[]) => ({})),
  getCompanyLogo: vi.fn(async (..._args: unknown[]) => ({
    LogoBase64: "",
    LogoMime: "",
    BuiltIn: true,
  })),
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
    Logout: (...args: unknown[]) => mocks.logout(...args),
    GetCompany: (...args: unknown[]) => mocks.getCompany(...args),
    MyDeletionRequests: (...args: unknown[]) => mocks.myDeletionRequests(...args),
    MyEmailChangeRequests: (...args: unknown[]) =>
      mocks.myEmailChangeRequests(...args),
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
    ListEmailChangeRequests: (...args: unknown[]) =>
      mocks.listEmailChangeRequests(...args),
  };
});

vi.mock("@backend/backend/Quotation", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@backend/backend/Quotation")>();
  return {
    ...actual,
    ListDrafts: (...args: unknown[]) => mocks.listDrafts(...args),
    ListCustomers: (...args: unknown[]) => mocks.listCustomers(...args),
    GetDraft: (...args: unknown[]) => mocks.getDraft(...args),
    SaveDraft: (...args: unknown[]) => mocks.saveDraft(...args),
    Prepare: (...args: unknown[]) => mocks.prepare(...args),
    Export: (...args: unknown[]) => mocks.exportPdf(...args),
    GetCustomerPreview: (...args: unknown[]) => mocks.getCustomerPreview(...args),
    GetInternalBreakdown: (...args: unknown[]) => mocks.getInternalBreakdown(...args),
    GetQuotationPreviewHTML: (...args: unknown[]) =>
      mocks.getQuotationPreviewHTML(...args),
    SetDraftContext: (...args: unknown[]) => mocks.setDraftContext(...args),
    ClearDraftContext: vi.fn(async () => {}),
    SetRevisionContext: vi.fn(async () => {}),
    ClearRevisionContext: vi.fn(async () => {}),
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

const REAL_CUSTOMER = "PT. Real Customer";

let container: HTMLDivElement;
let root: Root;
let scrollToSpy: ReturnType<typeof vi.spyOn>;

beforeEach(() => {
  window.localStorage.clear();
  for (const mock of Object.values(mocks)) mock.mockClear();
  mocks.restoreSession.mockResolvedValue(USER);
  mocks.currentUser.mockResolvedValue(USER);
  mocks.getCompany.mockResolvedValue(PROFILE);
  mocks.getCompanyProfile.mockResolvedValue(PROFILE);
  mocks.listCustomers.mockResolvedValue([
    { Code: 2, Name: REAL_CUSTOMER, PrimaryAddress: "", ID: 2, QuotationCount: 0 },
  ]);
  scrollToSpy = vi.spyOn(window, "scrollTo").mockImplementation(() => {});
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  scrollToSpy.mockRestore();
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

async function waitFor(ms: number): Promise<void> {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, ms));
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

function buttonWithText(label: string): HTMLButtonElement {
  const found = Array.from(document.querySelectorAll<HTMLButtonElement>("button")).find(
    (node) => node.textContent?.trim() === label
  );
  if (found === undefined) throw new Error(`no button reading ${label}`);
  return found;
}

/** The app settles the session check asynchronously; poll for the control
 * rather than assuming one flush is enough under a loaded suite. */
async function waitForButton(label: string): Promise<HTMLButtonElement> {
  const started = Date.now();
  while (Date.now() - started < 8000) {
    const found = document.querySelector<HTMLButtonElement>(
      `button[aria-label="${label}"]`
    );
    if (found !== null) return found;
    await waitFor(100);
  }
  throw new Error(`no button labelled ${label}`);
}

async function openMenuItem(label: string): Promise<void> {
  await act(async () => {
    button("Account menu for Sam Staff").click();
  });
  await act(async () => {
    const found = Array.from(
      document.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')
    ).find((node) => node.textContent?.trim() === label);
    if (found === undefined) throw new Error(`no menu item ${label}`);
    found.click();
  });
  await flush();
}

function input(ariaLabel: string): HTMLInputElement {
  const found = document.querySelector<HTMLInputElement>(
    `input[aria-label="${ariaLabel}"]`
  );
  if (found === null) throw new Error(`no input labelled ${ariaLabel}`);
  return found;
}

/** The Subject field carries a visible label, not an aria-label. */
function subjectInput(): HTMLInputElement {
  const found = document.querySelector<HTMLInputElement>(
    'input[placeholder="Supply and installation of electrical and mechanical systems"]'
  );
  if (found === null) throw new Error("no Subject input");
  return found;
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

function practiceWorkspace(): HTMLElement | null {
  return document.querySelector<HTMLElement>('[data-tutorial-practice="quotation"]');
}

function customerLabel(): string {
  return (
    document
      .querySelector<HTMLButtonElement>('button[aria-label="Customer"]')
      ?.textContent?.trim() ?? ""
  );
}

function railText(): string {
  return (
    document.querySelector<HTMLElement>('[data-tutorial="quotation-totals"]')
      ?.textContent ?? ""
  );
}

function practiceHero(): HTMLElement {
  const found = document.querySelector<HTMLElement>(
    '[data-testid="quotation-hero"]'
  );
  if (found === null) throw new Error("no quotation hero");
  return found;
}

function practiceHeroText(): string {
  return practiceHero().textContent ?? "";
}

function practiceHeroTotal(): string {
  const found = document.querySelector<HTMLElement>(
    '[data-testid="quotation-hero-total"]'
  );
  if (found === null) throw new Error("no quotation hero total");
  return found.textContent?.trim() ?? "";
}

/** The practice details panel renders its tax basis as a radio group. */
function pickTaxZone(label: string): void {
  const group = document.querySelector<HTMLElement>(
    '[role="radiogroup"][aria-label="Tax zone"]'
  );
  if (group === null) throw new Error("no Tax zone radiogroup");
  const radio = Array.from(
    group.querySelectorAll<HTMLButtonElement>('[role="radio"]')
  ).find((candidate) => candidate.textContent?.trim() === label);
  if (radio === undefined) throw new Error(`no radio reading ${label}`);
  act(() => radio.click());
}

async function openQuotationTour(): Promise<void> {
  await renderApp();
  const open = await waitForButton("Open Quotation");
  await act(async () => {
    open.click();
  });
  await flush();
  await waitFor(650);
}

/** The active tour step, as the guide itself names it. */
function tutorialStep(): string | null {
  return (
    document.querySelector<HTMLElement>("[data-features-tutorial]")?.dataset
      .featuresTutorialStep ?? null
  );
}

/** Walks the guide with Next until it reaches the named step. */
async function advanceTutorialTo(stepId: string): Promise<void> {
  const started = Date.now();
  while (Date.now() - started < 25000) {
    if (tutorialStep() === stepId) return;
    const next = document.querySelector<HTMLButtonElement>(
      'button[aria-label="Next step"]'
    );
    if (next === null) {
      throw new Error(
        `tutorial has no Next button while waiting for ${stepId} (on ${tutorialStep()})`
      );
    }
    await act(async () => {
      next.click();
    });
    await waitFor(600);
  }
  throw new Error(`tutorial never reached ${stepId} (on ${tutorialStep()})`);
}

describe("Quotation tutorial practice (issues #605, #607)", () => {
  it(
    "opens on practice values and computes totals through the real rail",
    { timeout: 20000 },
    async () => {
      await openQuotationTour();

      expect(practiceWorkspace()).not.toBeNull();
      expect(
        document.body.textContent
      ).toContain("This is a sandbox with made-up lines");
      // The tour's Quotation steps must keep resolving inside the practice
      // composition, and to exactly one target each -- a hidden real editor
      // carrying the same anchors would make the guide measure the wrong one.
      for (const anchor of [
        "quotation-customer",
        "quotation-line-cost",
        "quotation-totals",
      ]) {
        expect(
          document.querySelectorAll(`[data-tutorial="${anchor}"]`)
        ).toHaveLength(1);
        expect(
          practiceWorkspace()?.querySelector(`[data-tutorial="${anchor}"]`)
        ).not.toBeNull();
      }
      expect(subjectInput().value).toBe(TUTORIAL_SUBJECT);
      expect(customerLabel()).toContain(TUTORIAL_CUSTOMER);
      // The practice customer is a local option, never a row the client
      // directory supplied: the real directory row does not leak in.
      expect(customerLabel()).not.toContain(REAL_CUSTOMER);
      expect(input("Description for line 1").value).toBe(
        "Tutorial installation service (MOCK)"
      );
      expect(input("Description for line 2").value).toBe(
        "Tutorial equipment package (MOCK)"
      );

      // Worked example, independent of computePricing:
      // line 1: 2 x (5.000.000 + 15%) = 11.500.000 sell / 10.000.000 cost
      // split:  (2 x 2.750.000) + (1.750.000 - 5%) = 7.162.500 sell / 6.750.000 cost
      // FTZ goods: no PPN, so the customer grand is the sell subtotal.
      const rail = railText();
      expect(rail).toContain("Rp16.750.000");
      expect(rail).toContain("Rp18.662.500");
      expect(rail).toContain("Rp1.912.500");

      // Editing the practice line reprices the real rail. Qty 4 makes the
      // first line 23.000.000, so the grand becomes 30.162.500.
      typeInto(input("Quantity for line 1"), "4");
      await flush();
      expect(railText()).toContain("Rp30.162.500");

      act(() => button("Skip Features Tutorial").click());
      await flush();
    }
  );

  it(
    "mounts the practice workspace even when the currency catalogue cannot resolve the profile",
    { timeout: 20000 },
    async () => {
      // A USD company whose entry is missing from the catalogue resolves to
      // "currency unknown". The tour must still replace the real editor: the
      // practice lines are sandbox data, and showing the real editor under
      // the guide would let a practice step point at real controls.
      mocks.getCompany.mockResolvedValue({ ...PROFILE, Currency: "USD" });
      mocks.getCurrencyCatalogue.mockResolvedValue([]);
      await openQuotationTour();

      expect(practiceWorkspace()).not.toBeNull();
      expect(document.body.textContent).not.toContain("Amounts are hidden");
      expect(document.body.textContent).toContain(
        "This is a sandbox with made-up lines"
      );
      // The fallback entry still renders money: it is the denomination every
      // stored integer was written in, and the practice data is not stored.
      expect(railText()).toContain("Rp18.662.500");

      act(() => button("Skip Features Tutorial").click());
      await flush();
    }
  );

  it(
    "guards Save, Prepare and Export without calling a binding",
    { timeout: 20000 },
    async () => {
      await openQuotationTour();

      // In tutorial mode the shell actions are pressable on purpose: the
      // point is that staff learn where they are and that the data is
      // practice, not that the buttons are dead.
      act(() => buttonWithText("Save draft").click());
      await flush();
      act(() => buttonWithText("Prepare").click());
      await flush();
      act(() => buttonWithText("Export PDF").click());
      await flush();

      expect(document.body.textContent).toContain(
        "Features Tutorial practice — nothing is saved to your workspace."
      );
      expect(mocks.saveDraft).not.toHaveBeenCalled();
      expect(mocks.prepare).not.toHaveBeenCalled();
      expect(mocks.exportPdf).not.toHaveBeenCalled();
      expect(mocks.getCustomerPreview).not.toHaveBeenCalled();
      expect(mocks.getInternalBreakdown).not.toHaveBeenCalled();
      expect(mocks.getQuotationPreviewHTML).not.toHaveBeenCalled();
      expect(mocks.setDraftContext).not.toHaveBeenCalled();

      act(() => button("Skip Features Tutorial").click());
      await flush();
    }
  );

  it(
    "drops practice state on leave and restores fresh practice data on replay",
    { timeout: 20000 },
    async () => {
      await openQuotationTour();
      expect(practiceWorkspace()).not.toBeNull();
      typeInto(input("Quantity for line 1"), "4");
      await flush();
      expect(input("Quantity for line 1").value).toBe("4");

      // Leaving mid-tour unmounts the sandbox without marking it seen. The
      // next entry starts the tour again -- on fresh practice data, never on
      // the values that were edited here.
      act(() => button("Back to Home").click());
      await flush();
      expect(practiceWorkspace()).toBeNull();

      await act(async () => {
        button("Open Quotation").click();
      });
      await flush();
      await waitFor(650);
      // The tour restarts because leaving did not mark it seen.
      expect(practiceWorkspace()).not.toBeNull();
      expect(subjectInput().value).toBe(TUTORIAL_SUBJECT);
      expect(input("Quantity for line 1").value).toBe("2");

      // Finish the tour for real, then replay from Quotation settings.
      act(() => button("Skip Features Tutorial").click());
      await flush();
      expect(practiceWorkspace()).toBeNull();
      expect(subjectInput().value).toBe("");
      expect(customerLabel()).toContain(REAL_CUSTOMER);
      expect(input("Description for line 1").value).toBe("");

      await openMenuItem("Quotation settings");
      act(() => button("Replay Features Tutorial").click());
      await flush();
      await waitFor(650);

      expect(practiceWorkspace()).not.toBeNull();
      expect(subjectInput().value).toBe(TUTORIAL_SUBJECT);
      expect(input("Description for line 1").value).toBe(
        "Tutorial installation service (MOCK)"
      );
      expect(input("Quantity for line 1").value).toBe("2");
      expect(mocks.saveDraft).not.toHaveBeenCalled();
      expect(mocks.prepare).not.toHaveBeenCalled();
      expect(mocks.exportPdf).not.toHaveBeenCalled();
    }
  );

  it(
    "never touches a real draft that was already in the editor",
    { timeout: 20000 },
    async () => {
      // Mark the tour seen first so the editor underneath stays real.
      await openQuotationTour();
      act(() => button("Skip Features Tutorial").click());
      await flush();
      expect(practiceWorkspace()).toBeNull();

      typeInto(subjectInput(), "REAL DRAFT 605");
      typeInto(input("Description for line 1"), "REAL LINE 605");
      await flush();
      expect(subjectInput().value).toBe("REAL DRAFT 605");

      // Replay starts the practice panel over that real draft.
      await openMenuItem("Quotation settings");
      act(() => button("Replay Features Tutorial").click());
      await flush();
      await waitFor(650);
      expect(practiceWorkspace()).not.toBeNull();
      expect(subjectInput().value).toBe(TUTORIAL_SUBJECT);
      expect(input("Description for line 1").value).toBe(
        "Tutorial installation service (MOCK)"
      );

      typeInto(input("Description for line 1"), "PRACTICE EDIT 605");
      typeInto(input("Quantity for line 1"), "9");
      await flush();
      expect(input("Quantity for line 1").value).toBe("9");

      act(() => button("Skip Features Tutorial").click());
      await flush();

      // The real draft is exactly as it was: its own subject, its own line,
      // its own quantity -- no practice value survived the tour.
      expect(practiceWorkspace()).toBeNull();
      expect(subjectInput().value).toBe("REAL DRAFT 605");
      expect(input("Description for line 1").value).toBe("REAL LINE 605");
      expect(input("Quantity for line 1").value).toBe("1");
      expect(customerLabel()).toContain(REAL_CUSTOMER);
      expect(mocks.saveDraft).not.toHaveBeenCalled();
      expect(mocks.prepare).not.toHaveBeenCalled();
      expect(mocks.exportPdf).not.toHaveBeenCalled();
      expect(mocks.setDraftContext).not.toHaveBeenCalled();
    }
  );

  it(
    "guards Update draft too, while a real save still reaches the binding",
    { timeout: 20000 },
    async () => {
      // Leave the tour, then save a real draft so the shell label becomes
      // "Update draft" -- the same handler, a different label.
      await openQuotationTour();
      act(() => button("Skip Features Tutorial").click());
      await flush();
      typeInto(subjectInput(), "DRAFT 605");
      await flush();
      act(() => buttonWithText("Save draft").click());
      await flush();
      expect(mocks.saveDraft).toHaveBeenCalledTimes(1);
      expect(buttonWithText("Update draft").textContent?.trim()).toBe(
        "Update draft"
      );

      // Replay the tour over that draft: Update draft must now refuse.
      await openMenuItem("Quotation settings");
      act(() => button("Replay Features Tutorial").click());
      await flush();
      await waitFor(650);
      act(() => buttonWithText("Update draft").click());
      await flush();
      expect(mocks.saveDraft).toHaveBeenCalledTimes(1);
      expect(document.body.textContent).toContain(
        "Features Tutorial practice — nothing is saved to your workspace."
      );

      act(() => button("Skip Features Tutorial").click());
      await flush();
      // The draft context itself is untouched and still reads "Update draft".
      expect(buttonWithText("Update draft").textContent?.trim()).toBe(
        "Update draft"
      );
    }
  );

  it(
    "focuses the note to reveal its formatting toolbar, then focuses the subject",
    { timeout: 30000 },
    async () => {
      await openQuotationTour();
      await advanceTutorialTo("quotation-note");

      // The practice note starts with its toolbar hidden; the step's action
      // is focusing the note, which is what reveals it.
      expect(
        document.querySelector('[data-tutorial="quotation-note-toolbar"]')
      ).toBeNull();
      const note = document.querySelector<HTMLElement>(
        '[data-tutorial="quotation-note"]'
      );
      if (note === null) throw new Error("no practice note");
      const noteEditor = note.querySelector<HTMLElement>('[role="textbox"]');
      if (noteEditor === null) throw new Error("no practice note editor");

      act(() => noteEditor.focus());
      await waitFor(650);
      expect(tutorialStep()).toBe("quotation-note-toolbar");
      const toolbar = document.querySelector<HTMLElement>(
        '[data-tutorial="quotation-note-toolbar"]'
      );
      expect(toolbar).not.toBeNull();
      for (const label of ["Bold", "Italic", "Bullet list"]) {
        expect(
          toolbar?.querySelector(`button[aria-label="${label}"]`)
        ).not.toBeNull();
      }
      // The read-only step explains the toolbar rather than asking for a click.
      expect(document.querySelector("[data-tutorial-beacon]")).toBeNull();

      await advanceTutorialTo("quotation-subject");
      expect(document.body.textContent).toContain("Excel");
      act(() => subjectInput().focus());
      await waitFor(650);
      expect(tutorialStep()).toBe("quotation-details");

      act(() => button("Skip Features Tutorial").click());
      await flush();
    }
  );

  it(
    "types a cost, then toggles margin and row discount and splits the line",
    { timeout: 40000 },
    async () => {
      await openQuotationTour();
      await advanceTutorialTo("quotation-line-cost");

      // The step says to type the cost and then press Next. Its cue is Type,
      // but it never advances on a keystroke: the sell price updates in place
      // while the guide stays put.
      expect(
        document.querySelector("[data-tutorial-beacon]")?.textContent
      ).toContain("Type");

      typeInto(input("Unit price for line 1"), "7500000");
      // Give the guide the full leave window: if a keystroke advanced the
      // step, the id would have changed by now.
      await waitFor(700);
      expect(tutorialStep()).toBe("quotation-line-cost");
      // Worked example: 2 x (7.500.000 + 15%) = 17.250.000 for line 1, plus
      // the split line's 7.162.500. FTZ goods carry no PPN, so the customer
      // grand total is that sum.
      expect(railText()).toContain("Rp24.412.500");

      await act(async () => button("Next step").click());
      await waitFor(700);
      expect(tutorialStep()).toBe("quotation-line-sell");
      expect(document.body.textContent).toContain("sell price");

      await advanceTutorialTo("quotation-line-margin");
      const margin = document.querySelector<HTMLButtonElement>(
        '[data-tutorial="quotation-line-margin"]'
      );
      if (margin === null) throw new Error("no first-line margin switch");
      expect(margin.getAttribute("aria-checked")).toBe("true");
      act(() => margin.click());
      await waitFor(650);
      expect(tutorialStep()).toBe("quotation-line-discount");
      expect(
        document
          .querySelector('[data-tutorial="quotation-line-margin"]')
          ?.getAttribute("aria-checked")
      ).toBe("false");
      // Margin off: 2 x 7.500.000 = 15.000.000, plus the split line.
      expect(railText()).toContain("Rp22.162.500");

      const discount = document.querySelector<HTMLButtonElement>(
        '[data-tutorial="quotation-line-discount"]'
      );
      if (discount === null) throw new Error("no first-line discount switch");
      expect(discount.getAttribute("aria-checked")).toBe("false");
      act(() => discount.click());
      await waitFor(650);
      expect(tutorialStep()).toBe("quotation-line-split");
      expect(
        document
          .querySelector('[data-tutorial="quotation-line-discount"]')
          ?.getAttribute("aria-checked")
      ).toBe("true");
      // The row carries 10% ready: 15.000.000 less 10% = 13.500.000, plus the
      // split line's 7.162.500.
      expect(railText()).toContain("Rp20.662.500");

      const split = document.querySelector<HTMLButtonElement>(
        '[data-tutorial="quotation-line-split"]'
      );
      if (split === null) throw new Error("no first-line split button");
      act(() => split.click());
      await waitFor(650);
      expect(tutorialStep()).toBe("quotation-totals");
      const row = document.querySelector<HTMLElement>(
        '[data-tutorial="quotation-line-row"]'
      );
      expect(row?.textContent).toContain("Sub-items");
      // The line now prices its parts. The converted scope line carries no
      // cost yet, so line 1 contributes nothing and the split package's
      // 7.162.500 is the whole customer total.
      expect(railText()).toContain("Rp7.162.500");
      expect(railText()).not.toContain("Rp20.662.500");

      act(() => button("Skip Features Tutorial").click());
      await flush();
    }
  );

  it(
    "points at Save and Export without a cue and never presses them",
    { timeout: 30000 },
    async () => {
      await openQuotationTour();
      await advanceTutorialTo("quotation-actions");

      // The actions step is read-only: the spotlight marks the controls, but
      // there is no beacon because the guide is not asking for a press. It
      // advances with Next and never invokes either control.
      expect(
        document.querySelector('[data-tutorial-spotlight="target"]')
      ).not.toBeNull();
      expect(document.querySelector("[data-tutorial-beacon]")).toBeNull();
      expect(mocks.saveDraft).not.toHaveBeenCalled();
      expect(mocks.prepare).not.toHaveBeenCalled();
      expect(mocks.exportPdf).not.toHaveBeenCalled();

      await act(async () => button("Next step").click());
      await waitFor(700);
      expect(tutorialStep()).toBe("quotation-drafts");
      expect(practiceWorkspace()).toBeNull();
      expect(mocks.saveDraft).not.toHaveBeenCalled();
      expect(mocks.prepare).not.toHaveBeenCalled();
      expect(mocks.exportPdf).not.toHaveBeenCalled();
      expect(mocks.setDraftContext).not.toHaveBeenCalled();

      act(() => button("Skip Features Tutorial").click());
      await flush();
    }
  );

  // Issue #624: the practice quotation renders the same dark hero the real
  // editor opens with, over its own local values. These cases drive the real
  // App, so what is asserted is the production wiring: the practice customer,
  // details and tax basis reach the hero, the practice lines price through the
  // same computePricing call the Summary rail uses, and no real quotation is
  // read or written to get there.
  describe("practice hero (issue #624)", () => {
    it(
      "mirrors the practice quotation with draft semantics and a live total",
      { timeout: 20000 },
      async () => {
        await openQuotationTour();

        const workspace = practiceWorkspace();
        if (workspace === null) throw new Error("no practice workspace");
        // The hero lives inside the practice sandbox -- not above it, where
        // the real editor's hero would sit -- so the whole card is practice
        // data by construction.
        expect(workspace.contains(practiceHero())).toBe(true);
        expect(practiceHero().parentElement).toBe(workspace);

        // Draft semantics: the practice quotation has never been issued, and
        // nothing in the tour can issue one.
        const hero = practiceHeroText();
        expect(hero).toContain("DRAFT");
        expect(hero).toContain("New quotation");
        expect(hero).not.toContain("CURRENT REV");
        expect(hero).not.toContain("NEXT REV");

        // Practice values, and never the real client directory's row: the
        // hero names the fixture customer the panel itself offers.
        expect(hero).toContain(TUTORIAL_CUSTOMER);
        expect(hero).not.toContain(REAL_CUSTOMER);
        expect(hero).toContain("30 days");
        expect(hero).toContain("90 days");
        expect(hero).toContain("EXW · IDR");
        expect(hero).toContain("FTZ · Goods");

        // Worked example, independent of computePricing (the same figures the
        // rail case uses above): line 1 is 2 x (5.000.000 + 15%) = 11.500.000,
        // the split package is 7.162.500, and FTZ goods charge no PPN, so the
        // customer total is their sum.
        expect(practiceHeroTotal()).toBe("Rp18.662.500");
        expect(hero).not.toContain("Includes PPN");

        // The sandbox says so in the hero too: Save draft refuses in the tour,
        // so the hero must not promise that saving stores the work.
        expect(hero).toContain(
          "Features Tutorial practice — nothing is saved to your workspace."
        );
        expect(hero).not.toContain("Save draft stores it");

        // Read-only mirror: the practice form stays the only editing surface.
        expect(
          practiceHero().querySelectorAll("input, button, select, textarea")
        ).toHaveLength(0);

        // The workspace is capped at the real editor's 1280px and the hero
        // spans it above the details/lines/summary composition.
        expect(workspace.className).toContain("max-w-[1280px]");
        const details = document.querySelector('[data-tutorial="quotation-details"]');
        if (details === null) throw new Error("no practice details panel");
        expect(
          practiceHero().compareDocumentPosition(details) &
            Node.DOCUMENT_POSITION_FOLLOWING
        ).not.toBe(0);

        // Live: editing the practice line reprices the hero through the same
        // pricing path as the rail. Qty 4 makes line 1 23.000.000, so the
        // customer total becomes 30.162.500 -- and the rail agrees.
        typeInto(input("Quantity for line 1"), "4");
        await flush();
        expect(practiceHeroTotal()).toBe("Rp30.162.500");
        expect(railText()).toContain("Rp30.162.500");

        // The practice details are live too: validity, payment and tax basis
        // reach the hero without touching a real quotation.
        typeInto(input("Validity"), "45");
        await flush();
        expect(practiceHeroText()).toContain("45 days");

        // Non-FTZ goods charge 11% PPN on top, so the hero's customer total
        // includes it. The line is still at qty 4, so the subtotal is
        // 30.162.500 and PPN adds 3.317.875: 33.480.375. The card says PPN is
        // included, exactly as the real editor's hero would.
        pickTaxZone("Non-FTZ");
        await flush();
        expect(practiceHeroText()).toContain("Non-FTZ · Goods");
        expect(practiceHeroText()).toContain("Includes PPN");
        expect(practiceHeroTotal()).toBe("Rp33.480.375");

        expect(customerLabel()).toContain(TUTORIAL_CUSTOMER);
        expect(mocks.getDraft).not.toHaveBeenCalled();
        expect(mocks.saveDraft).not.toHaveBeenCalled();
        expect(mocks.prepare).not.toHaveBeenCalled();
        expect(mocks.exportPdf).not.toHaveBeenCalled();
        expect(mocks.setDraftContext).not.toHaveBeenCalled();

        act(() => button("Skip Features Tutorial").click());
        await flush();
      }
    );
  });
});
