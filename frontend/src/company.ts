/**
 * Load→save mapping for the Company section of Settings. This module is the
 * real production path: CompanyProfileCard calls formFromProfile,
 * extrasFromProfile and themeFromProfile on load.
 *
 * The company save used to be one whole-record UpdateCompanyProfile call — the
 * legal name, the address, the hidden extras and all eight theme colours in a
 * single full-replacing payload, which meant a client saving one tab had to
 * resend every column another tab owned or clear it. Issue #432 replaced it
 * with per-tab saves (see companySaves.ts): the General tab writes its six
 * columns, Units writes the list plus the canonical unit, the Quotation tab
 * writes the wording and its four colours, the Portal tab writes its four.
 *
 * What survives here is the read side, and it is still the whole record: the
 * narrow endpoints write one slice, so the page must hold every served value —
 * including the phone, email, tax ID and currency that have no inputs of their
 * own — or the General save would clear them.
 *
 * This repo has no DOM tests, so "a field was lost" cannot be asserted
 * through the UI — company.test.ts pins this mapping instead.
 */
export type CompanyForm = {
  legalName: string;
  address: string;
};

// Stored-but-unedited profile values. Phone, email, and tax ID print
// nowhere, so they have no inputs — but the General save sends them (it
// writes all six of its columns), so they must be read on load and held.
// Currency has its picker on the General tab. CanonicalUoM belongs to the
// Units tab (#432): it is written in the same transaction as the unit list, so
// the General save neither sends nor clears it.
export type StoredCompanyExtras = {
  phone: string;
  email: string;
  taxId: string;
  currency: string;
  canonicalUoM: string;
};

export type CompanyTheme = {
  qDeep: string;
  qAccent: string;
  qSoft: string;
  qInk: string;
  pDeep: string;
  pAccent: string;
  pSoft: string;
  pInk: string;
};

export const EMPTY_THEME: CompanyTheme = {
  qDeep: "",
  qAccent: "",
  qSoft: "",
  qInk: "",
  pDeep: "",
  pAccent: "",
  pSoft: "",
  pInk: "",
};

// Baked defaults mirror the migration seeds: shown when the stored value is
// empty, so the picker never renders an invalid color.
export const DEFAULT_THEME: CompanyTheme = {
  qDeep: "#1A237E",
  qAccent: "#284AF3",
  qSoft: "#D6E4FF",
  qInk: "#1a1a1a",
  pDeep: "#1A237E",
  pAccent: "#284AF3",
  pSoft: "#D6E4FF",
  pInk: "#1a1a1a",
};

export const THEME_FIELDS: {
  key: keyof CompanyTheme;
  section: "Quotation" | "Portal";
  label: string;
  hint: string;
}[] = [
  { key: "qDeep", section: "Quotation", label: "Deep", hint: "Headings, table headers, subject label, buttons, top ribbon on quotations" },
  { key: "qAccent", section: "Quotation", label: "Accent", hint: "Company name, links and highlights on quotations" },
  { key: "qSoft", section: "Quotation", label: "Soft", hint: "Card and section backgrounds on quotations" },
  { key: "qInk", section: "Quotation", label: "Ink", hint: "Body text on quotations" },
  { key: "pDeep", section: "Portal", label: "Deep", hint: "Ribbon, headings, buttons and eyebrow on the verification page" },
  { key: "pAccent", section: "Portal", label: "Accent", hint: "Issuer name, links and button hover on the verification page" },
  { key: "pSoft", section: "Portal", label: "Soft", hint: "Lookup panel, page gradient, field dividers and sheet outline on the verification page" },
  { key: "pInk", section: "Portal", label: "Ink", hint: "Body text on the verification page" },
];

/** The fields this module reads from a stored profile. Structural, so tests can
 *  pass a literal without constructing the generated Wails type. */
export type CompanyProfileLike = {
  LegalName?: string | null; Address?: string | null;
  Phone?: string | null; Email?: string | null; TaxID?: string | null;
  Currency?: string | null; CanonicalUoM?: string | null;
  ThemeQDeep?: string | null; ThemeQAccent?: string | null;
  ThemeQSoft?: string | null; ThemeQInk?: string | null;
  ThemePDeep?: string | null; ThemePAccent?: string | null;
  ThemePSoft?: string | null; ThemePInk?: string | null;
};

export function formFromProfile(profile: CompanyProfileLike): CompanyForm {
  return {
    legalName: profile.LegalName ?? "",
    address: profile.Address ?? "",
  };
}

export function extrasFromProfile(profile: CompanyProfileLike): StoredCompanyExtras {
  return {
    phone: profile.Phone ?? "",
    email: profile.Email ?? "",
    taxId: profile.TaxID ?? "",
    currency: profile.Currency ?? "",
    canonicalUoM: profile.CanonicalUoM ?? "",
  };
}

export function themeFromProfile(profile: CompanyProfileLike): CompanyTheme {
  return {
    qDeep: profile.ThemeQDeep ?? "",
    qAccent: profile.ThemeQAccent ?? "",
    qSoft: profile.ThemeQSoft ?? "",
    qInk: profile.ThemeQInk ?? "",
    pDeep: profile.ThemePDeep ?? "",
    pAccent: profile.ThemePAccent ?? "",
    pSoft: profile.ThemePSoft ?? "",
    pInk: profile.ThemePInk ?? "",
  };
}

/** True when the value is a complete six-digit hex colour — the test
 *  ThemeColorField's picker applies to the trimmed value to decide whether
 *  the stored value can feed the native colour input. */
export function isCompleteHex(value: string): boolean {
  return /^#[0-9a-fA-F]{6}$/.test(value);
}
