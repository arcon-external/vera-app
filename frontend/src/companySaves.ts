/**
 * The per-tab save model (issue #432).
 *
 * The Company settings section used to save one whole record: one
 * UpdateCompanyProfile call full-replacing every editable column, so a client
 * that saved one tab had to resend the eight theme colours and the canonical
 * unit or clear them. The settings are now saved one tab at a time through
 * narrow endpoints — UpdateCompanyGeneral, UpdateCompanyUnits (which carries
 * the canonical unit), UpdateCompanyLabels, UpdateQuotationTheme and
 * UpdatePortalTheme — none of which can clear a column another tab owns.
 *
 * What lives here is everything about that model that can be decided without a
 * DOM: which tab owns which values, what "dirty" means per tab, what a blocked
 * tab switch says, and the save sequences themselves with the Wails bindings
 * injected as a seam (the pattern companyLabels.ts set with saveCompanyLabels).
 * The page owns the state and the wiring; this module owns the decisions, so
 * the node suite can pin them — there are no DOM tests in this repo, and a save
 * model pinned only through clicks is a save model nobody tested.
 *
 * The one obligation every successful save carries is the identity refresh:
 * the desktop renders its own PDFs from the identity it cached at startup, so
 * a save that lands without RefreshCompanyIdentity keeps printing the previous
 * wording, colours or header on this device until the next sign-in. It runs
 * once, at the end, and only when something actually saved.
 */

import {
  companyLabelPayload,
  serializeCompanyLabels,
  type CompanyLabelEntry,
} from "./companyLabels";
import type { CompanyForm, CompanyTheme, StoredCompanyExtras } from "./company";
import type { CompanyUnitOption } from "./companyUnits";

export type CompanyTabId = "general" | "units" | "quotation" | "portal";

/** The tabs, in the order the bar renders them. */
export const COMPANY_TABS: CompanyTabId[] = ["general", "units", "quotation", "portal"];

/**
 * Each tab's name and its save button. The guard names both when it blocks a
 * switch, so the message a person reads is the one that tells them which tab is
 * unsaved and which button saves it.
 */
export const COMPANY_TAB_SAVES: Record<CompanyTabId, { label: string; button: string; busy: string }> = {
  general: { label: "General", button: "Save company details", busy: "Saving…" },
  units: { label: "Units", button: "Save units", busy: "Saving…" },
  quotation: { label: "Quotation", button: "Save wording and colours", busy: "Saving…" },
  portal: { label: "Portal", button: "Save colours", busy: "Saving…" },
};

/** The Wails rejection's message when it sent one, or the caller's fallback.
 *  The runtime rejects with the Go error — a string, or an Error wrapping it —
 *  and those messages are descriptive ("company legal name is required"). An
 *  Error carrying an empty message, or anything else, keeps the fallback. */
export function errorMessage(err: unknown, fallback: string): string {
  if (typeof err === "string") return err.trim() === "" ? fallback : err;
  if (err instanceof Error) return err.message.trim() === "" ? fallback : err.message;
  return fallback;
}

/** The six General-tab values, trimmed where the store trims: the legal name
 *  and address are trimmed, the hidden extras (phone, email, tax ID) and the
 *  currency go through verbatim, exactly as the old whole-record save did. */
export function generalSaveValues(
  form: CompanyForm,
  extras: StoredCompanyExtras
): {
  legalName: string;
  address: string;
  phone: string;
  email: string;
  taxID: string;
  currency: string;
} {
  return {
    legalName: form.legalName.trim(),
    address: form.address.trim(),
    phone: extras.phone,
    email: extras.email,
    taxID: extras.taxId,
    currency: extras.currency,
  };
}

/** The four quotation colours, trimmed, in the order the endpoint takes them. */
export function quotationThemeValues(theme: CompanyTheme): [string, string, string, string] {
  return [theme.qDeep.trim(), theme.qAccent.trim(), theme.qSoft.trim(), theme.qInk.trim()];
}

/** The four portal colours, trimmed, in the order the endpoint takes them. */
export function portalThemeValues(theme: CompanyTheme): [string, string, string, string] {
  return [theme.pDeep.trim(), theme.pAccent.trim(), theme.pSoft.trim(), theme.pInk.trim()];
}

/** The live draft, as the dirty decision sees it. */
export type CompanyDraft = {
  form: CompanyForm;
  extras: StoredCompanyExtras;
  units: CompanyUnitOption[] | null;
  labels: CompanyLabelEntry[] | null;
  theme: CompanyTheme;
};

/**
 * What the server holds, per tab: the fingerprint of the last loaded or saved
 * state, or null while that piece has not loaded. A null fingerprint never
 * reads as dirty — there is nothing on screen to lose yet — and the page also
 * keeps every save disabled until the first load has landed.
 */
export type CompanyFingerprints = {
  general: string | null;
  units: string | null;
  labels: string | null;
  quotationTheme: string | null;
  portalTheme: string | null;
};

export function generalFingerprint(form: CompanyForm, extras: StoredCompanyExtras): string {
  return JSON.stringify(generalSaveValues(form, extras));
}

export function unitsFingerprint(units: CompanyUnitOption[] | null, canonicalUoM: string): string {
  if (units === null) return "";
  return JSON.stringify({
    units: units.map((unit) => ({ value: unit.value, label: unit.label })),
    canonical: canonicalUoM.trim(),
  });
}

export function labelsFingerprint(labels: CompanyLabelEntry[] | null): string {
  return labels === null ? "" : serializeCompanyLabels(labels);
}

export function quotationThemeFingerprint(theme: CompanyTheme): string {
  return quotationThemeValues(theme).join("\n");
}

export function portalThemeFingerprint(theme: CompanyTheme): string {
  return portalThemeValues(theme).join("\n");
}

/**
 * Whether a tab holds unsaved edits.
 *
 * The halves are kept apart on purpose: the Quotation tab's wording, the
 * Quotation tab's four colours and the Portal tab's four colours are three
 * different saves against three different columns, so editing a portal colour
 * must not make the Quotation tab dirty (and must not send a quotation save).
 * A tab is dirty when any of its halves moved. The canonical unit belongs to
 * the Units tab, so moving it dirties Units and nothing else.
 *
 * Each half is exported as well, because the save sends only the halves that
 * moved — and a decision that lives in one function and a save that lives in
 * another is how the two drift apart.
 */
export function generalDirty(draft: CompanyDraft, saved: CompanyFingerprints): boolean {
  return saved.general !== null && saved.general !== generalFingerprint(draft.form, draft.extras);
}

export function unitsDirty(draft: CompanyDraft, saved: CompanyFingerprints): boolean {
  // A list that has not loaded is not an edit: the fingerprint of "nothing" is
  // empty, and comparing it to a served list would read as one unsaved change.
  if (draft.units === null) return false;
  return (
    saved.units !== null && saved.units !== unitsFingerprint(draft.units, draft.extras.canonicalUoM)
  );
}

export function labelsDirty(draft: CompanyDraft, saved: CompanyFingerprints): boolean {
  // Same rule: a failed wording fetch must not read as an edit nobody made,
  // and must never offer to save an empty set over a served one.
  if (draft.labels === null) return false;
  return saved.labels !== null && saved.labels !== labelsFingerprint(draft.labels);
}

export function quotationColoursDirty(draft: CompanyDraft, saved: CompanyFingerprints): boolean {
  return (
    saved.quotationTheme !== null &&
    saved.quotationTheme !== quotationThemeFingerprint(draft.theme)
  );
}

export function portalColoursDirty(draft: CompanyDraft, saved: CompanyFingerprints): boolean {
  return (
    saved.portalTheme !== null && saved.portalTheme !== portalThemeFingerprint(draft.theme)
  );
}

export function companyTabDirty(
  tab: CompanyTabId,
  draft: CompanyDraft,
  saved: CompanyFingerprints
): boolean {
  switch (tab) {
    case "general":
      return generalDirty(draft, saved);
    case "units":
      return unitsDirty(draft, saved);
    case "quotation":
      return labelsDirty(draft, saved) || quotationColoursDirty(draft, saved);
    case "portal":
      return portalColoursDirty(draft, saved);
  }
}

/**
 * The tab-switch guard. Leaving a dirty tab loses its edits, so the switch is
 * intercepted and the tab stays: the message names the tab and the button that
 * saves it, which is the sentence that makes "unsaved changes" actionable. It
 * returns null when the switch may proceed — the outgoing tab is clean, or the
 * request is the tab already showing.
 *
 * A browser confirm() was the obvious mechanism and is deliberately not used:
 * it blocks the whole window, names nothing, and is the dialog people learn to
 * dismiss without reading.
 */
export function tabSwitchGuard(
  current: CompanyTabId,
  dirty: Record<CompanyTabId, boolean>,
  requested: CompanyTabId
): string | null {
  if (requested === current || !dirty[current]) return null;
  const { label, button } = COMPANY_TAB_SAVES[current];
  return `The ${label} tab has unsaved changes. Save them with “${button}” before switching tabs.`;
}

/**
 * The two halves of the Quotation save, in the order they are sent. Wording
 * first, then the four colours — the wording is the document's content and the
 * colours are its paint, and the labels endpoint is the one with the stricter
 * validation, so its refusal is the one a person reads first. A clean half is
 * not sent at all: a save that only touched a colour must not rewrite the
 * wording, and a save that only reworded must not touch the colours.
 */
export function quotationSaveHalves(
  labelsDirty: boolean,
  themeDirty: boolean
): ("labels" | "theme")[] {
  const halves: ("labels" | "theme")[] = [];
  if (labelsDirty) halves.push("labels");
  if (themeDirty) halves.push("theme");
  return halves;
}

/** The bindings a tab save is allowed to call, injected so the sequences can
 *  be driven with spies in node. Each save calls only its own endpoints — a
 *  General save that reached a theme endpoint would clear a colour the
 *  Quotation tab owns, which is the whole reason the split exists. */
export interface CompanySaveDeps {
  updateGeneral: (values: ReturnType<typeof generalSaveValues>) => Promise<unknown>;
  updateUnits: (units: CompanyUnitOption[], canonicalUoM: string) => Promise<unknown>;
  updateLabels: (rows: { Key: string; Value: string }[]) => Promise<unknown>;
  updateQuotationTheme: (deep: string, accent: string, soft: string, ink: string) => Promise<unknown>;
  updatePortalTheme: (deep: string, accent: string, soft: string, ink: string) => Promise<unknown>;
  refreshIdentity: () => Promise<unknown>;
}

/** What one save reports back. `saved` is true when the store accepted the
 *  write; `refreshFailed` is a landed save whose identity refresh did not —
 *  the data is correct and this device keeps the old copy until the next
 *  sign-in, which is a note, not an error. */
export type TabSaveResult = { error: string | null; saved: boolean; refreshFailed: boolean };

/** Runs one write and closes with the refresh when it landed. */
async function runSave(
  write: () => Promise<unknown>,
  fallback: string,
  deps: CompanySaveDeps
): Promise<TabSaveResult> {
  try {
    await write();
  } catch (err: unknown) {
    return { error: errorMessage(err, fallback), saved: false, refreshFailed: false };
  }
  try {
    await deps.refreshIdentity();
    return { error: null, saved: true, refreshFailed: false };
  } catch {
    return { error: null, saved: true, refreshFailed: true };
  }
}

/** The General tab: one call, six columns, no theme and no canonical unit. */
export function saveGeneralTab(
  form: CompanyForm,
  extras: StoredCompanyExtras,
  deps: CompanySaveDeps
): Promise<TabSaveResult> {
  return runSave(
    () => deps.updateGeneral(generalSaveValues(form, extras)),
    "We couldn't save the company details.",
    deps
  );
}

/** The Units tab: the list and the canonical unit in one call, because the
 *  server writes them in one transaction — a canonical the new list does not
 *  carry is refused there, not here. */
export function saveUnitsTab(
  units: CompanyUnitOption[],
  canonicalUoM: string,
  deps: CompanySaveDeps
): Promise<TabSaveResult> {
  return runSave(
    () => deps.updateUnits(units, canonicalUoM.trim()),
    "We couldn't save the units.",
    deps
  );
}

/** The Portal tab: its four colours, and only its four. */
export function savePortalTab(theme: CompanyTheme, deps: CompanySaveDeps): Promise<TabSaveResult> {
  const [deep, accent, soft, ink] = portalThemeValues(theme);
  return runSave(
    () => deps.updatePortalTheme(deep, accent, soft, ink),
    "We couldn't save the portal colours.",
    deps
  );
}

/** The two halves of the Quotation save, each with its own verdict, so a
 *  wording refusal does not silently drop the colour half and a colour refusal
 *  does not hide behind the wording's success. */
export type QuotationSaveResult = {
  labelsError: string | null;
  themeError: string | null;
  saved: boolean;
  refreshFailed: boolean;
};

/**
 * The Quotation tab: labels and colours, sequentially, only the dirty halves.
 *
 * Each half is attempted whatever happened to the other: a wording refusal
 * leaves the colour half unsaved and says so, which is more honest than
 * aborting the second half on the first half's failure. The refresh runs once
 * at the end, and only when at least one half landed.
 */
export async function saveQuotationTab(
  input: {
    labels: CompanyLabelEntry[];
    labelsDirty: boolean;
    theme: CompanyTheme;
    themeDirty: boolean;
  },
  deps: CompanySaveDeps
): Promise<QuotationSaveResult> {
  const result: QuotationSaveResult = {
    labelsError: null,
    themeError: null,
    saved: false,
    refreshFailed: false,
  };
  for (const half of quotationSaveHalves(input.labelsDirty, input.themeDirty)) {
    try {
      if (half === "labels") {
        await deps.updateLabels(companyLabelPayload(input.labels));
      } else {
        const [deep, accent, soft, ink] = quotationThemeValues(input.theme);
        await deps.updateQuotationTheme(deep, accent, soft, ink);
      }
      result.saved = true;
    } catch (err: unknown) {
      const fallback =
        half === "labels"
          ? "We couldn't save the document wording."
          : "We couldn't save the quotation colours.";
      if (half === "labels") result.labelsError = errorMessage(err, fallback);
      else result.themeError = errorMessage(err, fallback);
    }
  }
  if (!result.saved) return result;
  try {
    await deps.refreshIdentity();
  } catch {
    result.refreshFailed = true;
  }
  return result;
}
