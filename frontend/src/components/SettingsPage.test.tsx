import { renderToString } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { DEFAULT_THEME } from "../company";
import {
  COMPANY_TABS,
  COMPANY_TAB_SAVES,
  companyTabDirty,
  errorMessage,
  generalFingerprint,
  labelsFingerprint,
  portalThemeFingerprint,
  quotationThemeFingerprint,
  saveGeneralTab,
  savePortalTab,
  saveQuotationTab,
  saveUnitsTab,
  tabSwitchGuard,
  unitsFingerprint,
} from "../companySaves";
import { ToastProvider } from "../toast";
import { companySaveDeps, CompanyProfileCard } from "./SettingsPage";
import { CompanyPortalTab } from "./CompanyPortalTab";
import { CompanyQuotationTab } from "./CompanyQuotationTab";
import { tabId, tabPanelId } from "./TabBar";

// The suite runs in node with no DOM (see vitest.config.ts), so the component
// is rendered to a string and asserted on as markup -- the same house pattern
// as ReviewPanel.test.tsx and CompanyGeneralPanel.test.tsx. No DOM, no
// testing-library, no new dependency.
//
// Effects never run under renderToString, so load()/loadLogo() never fire and
// `loading` stays true: the first render shows the tab bar, the guard's (empty)
// slot and the tabpanel shell holding the skeleton. The save rows live inside
// the tab panels, which are only reached once a load lands, so they are pinned
// through the panels themselves (CompanyGeneralPanel, CompanyQuotationTab,
// CompanyPortalTab, CompanyUnitsPanel) rather than through this loading render.
//
// What this test does NOT prove: tab switching, the saves and the guard all
// need events, which renderToString cannot fire. The decisions behind them are
// pinned as pure functions (companySaves.test.ts) and the page's endpoint
// wiring is pinned by driving companySaveDeps() with the bindings mocked --
// the two halves of the behaviour that a click would join together.

// A served set in the shape GET /v1/company/labels answers: one plain label and
// one terms sentence, which is all saveCompanyLabels looks at.
const SERVED = [
  { Key: "docTitle", Value: "PROPOSAL", Default: "QUOTATION", MaxLength: 24, Terms: false },
  {
    Key: "termsPayment",
    Value: "1. Payment terms agreed for this quotation is {payment} days.",
    Default: "1. Payment terms agreed for this quotation is {payment} days.",
    MaxLength: 300,
    Terms: true,
  },
];

function render(): string {
  return renderToString(
    <ToastProvider>
      <CompanyProfileCard />
    </ToastProvider>
  );
}

/** Splits the rendered output into one string per <button> element. */
function buttons(html: string): string[] {
  return html.split("<button").slice(1);
}

/** The three sub-tab buttons: role="tab" excludes the role="tablist" wrapper
 *  by matching the closing quote. */
function tabs(html: string): string[] {
  return buttons(html).filter((segment) => segment.includes('role="tab"'));
}

describe("CompanyProfileCard sub-tabs", () => {
  it("renders four sub-tabs labelled General, Units, Quotation and Portal, each role=tab", () => {
    // Template and Quotation are one thing (issue #429): the merged tab is
    // named Quotation and there is no Template tab.
    const html = render();
    const found = tabs(html);
    expect(found).toHaveLength(4);
    for (const label of ["General", "Units", "Quotation", "Portal"]) {
      expect(
        found.find((segment) => segment.includes(`>${label}</button>`)),
        `expected a tab labelled ${label} in: ${html}`
      ).toBeDefined();
    }
    expect(
      found.find((segment) => segment.includes(">Template</button>")),
      "the Template sub-tab was merged into Quotation and must be gone"
    ).toBeUndefined();
  });

  it("marks exactly one tab selected, and it is General on first render", () => {
    const html = render();
    expect(html.split('aria-selected="true"').length - 1).toBe(1);
    const general = tabs(html).find((segment) => segment.includes(">General</button>"));
    expect(general).toBeDefined();
    expect(general).toContain('aria-selected="true"');
    for (const label of ["Quotation", "Portal"]) {
      const button = tabs(html).find((segment) => segment.includes(`>${label}</button>`));
      expect(button).toBeDefined();
      expect(button).toContain('aria-selected="false"');
    }
  });

  it("labels the tablist Company sections and wires tab ids through tabId/tabPanelId", () => {
    const html = render();
    expect(html).toContain('role="tablist"');
    expect(html).toContain('aria-label="Company sections"');
    for (const id of COMPANY_TABS) {
      const button = tabs(html).find((segment) =>
        segment.includes(`>${COMPANY_TAB_SAVES[id].label}</button>`)
      );
      expect(button).toBeDefined();
      expect(button).toContain(`id="${tabId("company", id)}"`);
      expect(button).toContain(`aria-controls="${tabPanelId("company", id)}"`);
    }
  });

  it("renders a real tabpanel for the active tab, not a decorative one", () => {
    const html = render();
    // The general tab's button points at the panel...
    const general = tabs(html).find((segment) => segment.includes(">General</button>"));
    expect(general).toContain(`aria-controls="${tabPanelId("company", "general")}"`);
    // ...and a role=tabpanel element answers with the matching id and owner.
    expect(html).toContain('role="tabpanel"');
    expect(html).toContain(`id="${tabPanelId("company", "general")}"`);
    expect(html).toContain(`aria-labelledby="${tabId("company", "general")}"`);
  });
});

// The shared <form> root and its footer save button are gone (#432): each tab
// saves itself, so there is nothing at the bottom of the page to press and no
// form for another tab's Enter to submit. The loading render is where that used
// to be asserted, so it is asserted here instead — the retired shape must not
// come back.
describe("the retired shared form and footer save", () => {
  it("renders no form root, no submit button and no footer save", () => {
    const html = render();
    expect(html).not.toContain("<form");
    expect(html).not.toContain("</form>");
    expect(html).not.toContain('type="submit"');
    expect(html).not.toContain(">Save company profile</button>");
    expect(buttons(html).filter((segment) => segment.includes(">Save"))).toHaveLength(0);
  });

  it("shows the loading skeleton inside the tabpanel, with no save button to press early", () => {
    // Saving before load() has landed would clear every hidden column, so the
    // rows only exist once the record is in hand.
    const html = render();
    expect(html).toContain('aria-label="Loading company profile"');
    expect(html).not.toContain(COMPANY_TAB_SAVES.general.button);
    expect(html).not.toContain(COMPANY_TAB_SAVES.units.button);
    expect(html).not.toContain(COMPANY_TAB_SAVES.quotation.button);
    expect(html).not.toContain(COMPANY_TAB_SAVES.portal.button);
  });
});

// The merged Quotation tab (issue #429): the Portal tab's shape — the document
// on top, the colours, the wording, the save row, then the two standing notes.
// CompanyProfileCard can only render its loading skeleton here, and the branch
// renders this component verbatim, so the composition is what gets pinned.
describe("the merged Quotation tab", () => {
  function renderTab(): string {
    return renderToString(
      <CompanyQuotationTab
        labels={SERVED}
        theme={DEFAULT_THEME}
        refreshKey={0}
        wordingStatus="saved"
        tabStatus="saved"
        labelsError={null}
        labelsNotice={null}
        themeError={null}
        invalid={null}
        busy={false}
        onLabelChange={() => undefined}
        onThemeChange={() => undefined}
        onSave={() => undefined}
        onFieldBlur={() => undefined}
        onFieldEnter={() => undefined}
      />
    );
  }

  it("stacks the document, the colours, the wording, the save row and the two notes", () => {
    const html = renderTab();
    const preview = html.indexOf('aria-label="Loading template preview"');
    const colours = html.indexOf("Quotation colors");
    const wording = html.indexOf("Document wording");
    const saveRow = html.indexOf(`>${COMPANY_TAB_SAVES.quotation.button}</button>`);
    const fixedColours = html.indexOf("These four colours paint the whole document.");
    const draftNote = html.indexOf("The preview above shows the draft");
    expect(preview).toBeGreaterThan(-1);
    // The reorder (#432): colours above the wording, the save row below both,
    // then the fixed-colour sentence and the draft sentence.
    expect(colours).toBeGreaterThan(preview);
    expect(wording).toBeGreaterThan(colours);
    expect(saveRow).toBeGreaterThan(wording);
    expect(fixedColours).toBeGreaterThan(saveRow);
    expect(draftNote).toBeGreaterThan(fixedColours);
    // The preview is the pending document, not an empty frame.
    expect(html).not.toContain("<iframe");
  });

  it("names its own save button, and only that one", () => {
    const html = renderTab();
    expect(html).toContain(`>${COMPANY_TAB_SAVES.quotation.button}</button>`);
    // One save row for the whole tab: the wording and the colours are one save.
    expect(html.split(`>${COMPANY_TAB_SAVES.quotation.button}</button>`).length - 1).toBe(1);
    expect(html).not.toContain(`>${COMPANY_TAB_SAVES.general.button}</button>`);
    expect(html).not.toContain(`>${COMPANY_TAB_SAVES.portal.button}</button>`);
  });

  it("describes the save model truthfully: no autosave, one button, the draft in the preview", () => {
    const html = renderTab();
    // The old caption promised an autosave and pointed at a footer button that
    // no longer exists. Under per-tab saves both halves save together, and the
    // copy says exactly that.
    expect(html).toContain(
      "The wording and the four colours save together with this button."
    );
    expect(html).not.toContain("saves automatically");
    expect(html).not.toContain("Wording above saves automatically");
    expect(html).not.toContain("Save company profile");
    // The palette-coverage sentence, frozen by the plan: what the four colours
    // paint, and the two things no rebrand may move.
    expect(html).toContain("The white page and the green discount row stay fixed");
    // The draft sentence, and the fact the preview follows blur/Enter only.
    expect(html).toContain("before either is saved");
    expect(html).toContain("never while you type");
  });

  it("still carries the wording inputs and the four quotation colours", () => {
    const html = renderTab();
    expect(html).toContain('aria-label="Document wording: QUOTATION"');
    // The served override (PROPOSAL over the built-in QUOTATION) shows its
    // Reset affordance, so the wording panel is the live one.
    expect(html).toContain(`aria-label="Reset QUOTATION to the built-in wording"`);
    expect(html).toContain('aria-label="Quotation Deep hex value"');
    expect(html).toContain('aria-label="Quotation Ink hex value"');
  });

  it("disables the save row and says why when a wording value would be refused", () => {
    // The store's rules are mirrored, so the button refuses a set the server
    // would bounce — the "held" state the autosave used to carry.
    const html = renderToString(
      <CompanyQuotationTab
        labels={[{ ...SERVED[0], Value: "{{.DocTitle}}" }]}
        theme={DEFAULT_THEME}
        refreshKey={0}
        wordingStatus="dirty"
        tabStatus="dirty"
        labelsError={null}
        labelsNotice={null}
        themeError={null}
        invalid="One field is highlighted below and must be fixed before anything saves."
        busy={false}
        onLabelChange={() => undefined}
        onThemeChange={() => undefined}
        onSave={() => undefined}
        onFieldBlur={() => undefined}
        onFieldEnter={() => undefined}
      />
    );
    const save = buttons(html).find((segment) =>
      segment.includes(`>${COMPANY_TAB_SAVES.quotation.button}</button>`)
    );
    expect(save).toBeDefined();
    expect(save).toContain('disabled=""');
    expect(html).toContain("must be fixed before anything saves");
  });

  it("refuses an incomplete colour hex before the store has to", () => {
    // A colour that is neither empty nor #RRGGBB can only be refused by the
    // server; the row names the field here instead, the way the preview panel
    // does. A half-typed hex must not send anything.
    const html = renderToString(
      <CompanyQuotationTab
        labels={SERVED}
        theme={{ ...DEFAULT_THEME, qAccent: "#284AF" }}
        refreshKey={0}
        wordingStatus="saved"
        tabStatus="dirty"
        labelsError={null}
        labelsNotice={null}
        themeError={null}
        invalid={null}
        busy={false}
        onLabelChange={() => undefined}
        onThemeChange={() => undefined}
        onSave={() => undefined}
        onFieldBlur={() => undefined}
        onFieldEnter={() => undefined}
      />
    );
    expect(html).toContain("The Quotation Accent colour is not a complete hex — fix it before saving.");
    expect(
      buttons(html).find((segment) =>
        segment.includes(`>${COMPANY_TAB_SAVES.quotation.button}</button>`)
      )
    ).toContain('disabled=""');
  });
});

// The Portal tab carries the same shape: the page on top, its four colours, its
// own save row, and the frozen sentence about the verdict colours that stay
// fixed. It is a component of its own so it can be pinned without a DOM.
describe("the Portal tab", () => {
  function renderTab(): string {
    return renderToString(
      <CompanyPortalTab
        theme={DEFAULT_THEME}
        onChange={() => undefined}
        status="saved"
        busy={false}
        error={null}
        onSave={() => undefined}
      />
    );
  }

  it("stacks the page, the colours, the save row and the fixed-colour sentence", () => {
    const html = renderTab();
    const preview = html.indexOf('aria-label="Loading portal preview"');
    const colours = html.indexOf("Portal colors");
    const saveRow = html.indexOf(`>${COMPANY_TAB_SAVES.portal.button}</button>`);
    const fixed = html.indexOf("These four colours paint the page around the verdict.");
    expect(preview).toBeGreaterThan(-1);
    expect(colours).toBeGreaterThan(preview);
    expect(saveRow).toBeGreaterThan(colours);
    expect(fixed).toBeGreaterThan(saveRow);
    expect(html).toContain("green valid, amber superseded, red mismatch, grey not-found");
  });

  it("names the Portal save and not the profile save it used to ride", () => {
    const html = renderTab();
    expect(html).toContain(`>${COMPANY_TAB_SAVES.portal.button}</button>`);
    expect(html).not.toContain("Save company profile");
    expect(html).not.toContain("saves automatically");
  });
});

// The guard's decision, pinned where the page makes it. A click cannot be fired
// in node, so the switch itself stays hand-driven; what is pinned is that a
// dirty outgoing tab blocks the switch with a sentence naming the tab and its
// button, and that a clean tab never blocks — the two halves of "edit wording →
// switch tab → warned, nothing sent".
describe("the tab-switch guard", () => {
  const clean = {
    general: false,
    units: false,
    quotation: false,
    portal: false,
  };

  it("blocks a switch out of a dirty tab and names the tab and its button", () => {
    const message = tabSwitchGuard("quotation", { ...clean, quotation: true }, "portal");
    expect(message).toBe(
      "The Quotation tab has unsaved changes. Save them with “Save wording and colours” before switching tabs."
    );
    // The page applies this by not switching: `message !== null` is the only
    // branch that keeps the current tab, and it never calls a save. Nothing is
    // sent on the way out.
    expect(companyTabDirty("quotation", dirtyDraft(), savedAll())).toBe(true);
  });

  it("lets a clean tab switch, and never blocks re-selecting the current tab", () => {
    expect(tabSwitchGuard("general", { ...clean }, "quotation")).toBeNull();
    expect(tabSwitchGuard("quotation", { ...clean, quotation: true }, "quotation")).toBeNull();
  });
});

/** A draft whose Quotation wording has moved, and every tab's saved baseline. */
function dirtyDraft() {
  return {
    form: { legalName: "PT Arcon Penta Persada", address: "Jl. Merdeka 1" },
    extras: {
      phone: "+62-21-5550192",
      email: "admin@arconpentapersada.co.id",
      taxId: "01.234.567.8-901.234",
      currency: "IDR",
      canonicalUoM: "Lot",
    },
    units: [
      { value: "Lot", label: "Lot" },
      { value: "m2", label: "m²" },
    ],
    labels: SERVED.map((entry) => ({ ...entry, Value: `${entry.Value} edited` })),
    theme: DEFAULT_THEME,
  };
}

function savedAll() {
  const base = dirtyDraft();
  return {
    general: generalFingerprint(base.form, base.extras),
    units: unitsFingerprint(base.units, base.extras.canonicalUoM),
    labels: labelsFingerprint(SERVED),
    quotationTheme: quotationThemeFingerprint(base.theme),
    portalTheme: portalThemeFingerprint(base.theme),
  };
}

// The page's wiring: which endpoint each tab's save answers, and with which
// arguments. The Bindings are mocked, so these run in node; the sequences they
// feed are pinned in companySaves.test.ts.
describe("the page's save wiring answers one endpoint per tab", () => {
  it("sends the General tab's six columns through UpdateCompanyGeneral", async () => {
    const deps = companySaveDeps();
    const result = await saveGeneralTab(
      { legalName: "PT Arcon Penta Persada", address: "Jl. Merdeka 1" },
      {
        phone: "+62-21-5550192",
        email: "admin@arconpentapersada.co.id",
        taxId: "01.234.567.8-901.234",
        currency: "USD",
        canonicalUoM: "Lot",
      },
      deps
    );
    expect(mocked.updateGeneral).toHaveBeenCalledWith(
      "PT Arcon Penta Persada",
      "Jl. Merdeka 1",
      "+62-21-5550192",
      "admin@arconpentapersada.co.id",
      "01.234.567.8-901.234",
      "USD"
    );
    // No theme and no unit call: a General save cannot clear a colour or a unit.
    expect(mocked.updateQuotationTheme).not.toHaveBeenCalled();
    expect(mocked.updatePortalTheme).not.toHaveBeenCalled();
    expect(mocked.updateUnits).not.toHaveBeenCalled();
    expect(mocked.updateLabels).not.toHaveBeenCalled();
    expect(result.saved).toBe(true);
    expect(mocked.refreshIdentity).toHaveBeenCalledTimes(1);
  });

  it("sends the list and the canonical through UpdateCompanyUnits", async () => {
    const deps = companySaveDeps();
    await saveUnitsTab([{ value: "Lot", label: "Lot" }], "Lot", deps);
    expect(mocked.updateUnits).toHaveBeenCalledWith(
      [expect.objectContaining({ Value: "Lot", Label: "Lot" })],
      "Lot"
    );
    expect(mocked.updateGeneral).not.toHaveBeenCalled();
  });

  it("sends the wording through UpdateCompanyLabels and the colours through UpdateQuotationTheme", async () => {
    const deps = companySaveDeps();
    await saveQuotationTab(
      {
        labels: SERVED,
        labelsDirty: true,
        theme: DEFAULT_THEME,
        themeDirty: true,
      },
      deps
    );
    expect(mocked.updateLabels).toHaveBeenCalledWith([
      expect.objectContaining({ Key: "docTitle", Value: "PROPOSAL" }),
      expect.objectContaining({ Key: "termsPayment" }),
    ]);
    expect(mocked.updateQuotationTheme).toHaveBeenCalledWith(
      "#1A237E",
      "#284AF3",
      "#D6E4FF",
      "#1a1a1a"
    );
    // The portal's four are a different endpoint.
    expect(mocked.updatePortalTheme).not.toHaveBeenCalled();
    expect(mocked.updateGeneral).not.toHaveBeenCalled();
  });

  it("sends the portal's four colours through UpdatePortalTheme", async () => {
    const deps = companySaveDeps();
    await savePortalTab(DEFAULT_THEME, deps);
    expect(mocked.updatePortalTheme).toHaveBeenCalledWith(
      "#1A237E",
      "#284AF3",
      "#D6E4FF",
      "#1a1a1a"
    );
    expect(mocked.updateQuotationTheme).not.toHaveBeenCalled();
    expect(mocked.updateLabels).not.toHaveBeenCalled();
  });
});

// The bindings, mocked. Every AdminAPI export the page imports is listed, or
// the import itself fails before any assertion runs.
const mocked = vi.hoisted(() => ({
  updateGeneral: vi.fn(async () => ({})),
  updateUnits: vi.fn(async () => []),
  updateLabels: vi.fn(async () => []),
  updateQuotationTheme: vi.fn(async () => ({})),
  updatePortalTheme: vi.fn(async () => ({})),
  refreshIdentity: vi.fn(async () => {}),
  updateProfile: vi.fn(async () => ({})),
}));

vi.mock("@backend/backend/AdminAPI", () => ({
  ClearCompanyLogo: vi.fn(async () => {}),
  DownloadBackup: vi.fn(async () => ""),
  GetCompanyLabels: vi.fn(async () => []),
  GetCompanyLogo: vi.fn(async () => ({})),
  GetCompanyProfile: vi.fn(async () => ({})),
  GetCompanyUnits: vi.fn(async () => []),
  GetCurrencyCatalogue: vi.fn(async () => []),
  RestoreBackup: vi.fn(async () => ({})),
  SetCompanyLogo: vi.fn(async () => {}),
  SetUserEmail: vi.fn(async () => ({})),
  UpdateCompanyGeneral: mocked.updateGeneral,
  UpdateCompanyUnits: mocked.updateUnits,
  UpdateCompanyLabels: mocked.updateLabels,
  UpdateCompanyProfile: mocked.updateProfile,
  UpdatePortalTheme: mocked.updatePortalTheme,
  UpdateQuotationTheme: mocked.updateQuotationTheme,
}));

vi.mock("@backend/backend/Quotation", () => ({
  PickDirectory: vi.fn(async () => ""),
  RefreshCompanyIdentity: mocked.refreshIdentity,
}));

// The identity refresh is the obligation every save carries: the desktop
// renders its own PDFs from the identity it cached at startup, so a save that
// lands without it keeps printing the previous copy on this device until the
// next sign-in. Pinned through the deps the page hands over.
describe("the page hands every save the identity refresh", () => {
  it("refreshes once per landed save, and never after a refusal", async () => {
    const deps = companySaveDeps();
    await saveGeneralTab(
      { legalName: "PT Arcon", address: "" },
      {
        phone: "",
        email: "",
        taxId: "",
        currency: "IDR",
        canonicalUoM: "Lot",
      },
      deps
    );
    expect(mocked.refreshIdentity).toHaveBeenCalledTimes(1);

    mocked.refreshIdentity.mockClear();
    mocked.updateGeneral.mockImplementationOnce(async () => {
      throw new Error("company legal name is required");
    });
    const refused = await saveGeneralTab(
      { legalName: "", address: "" },
      { phone: "", email: "", taxId: "", currency: "IDR", canonicalUoM: "" },
      deps
    );
    expect(refused.saved).toBe(false);
    expect(mocked.refreshIdentity).not.toHaveBeenCalled();
    // The refusal's message reaches the banner through errorMessage, which the
    // page and the seam share.
    expect(errorMessage(refused.error, "fallback")).toBe("company legal name is required");
  });
});
