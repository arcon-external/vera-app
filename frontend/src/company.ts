/**
 * Load→save mapping for the Company section of Settings. This module is the
 * real production path: CompanyProfileCard calls formFromProfile,
 * extrasFromProfile and themeFromProfile on load, and profilePayload on save.
 *
 * The company save is a single whole-record UpdateCompanyProfile call — the
 * legal name, the address, the hidden extras and all eight theme colours in
 * one payload — because the Go endpoint full-replaces the editable columns
 * and empty strings clear a field. A saver that sent only its own slice
 * would silently wipe everything else. So the page owns the whole record in
 * one place and the General/Theme panels are purely presentational slices:
 * they render their controls and forward edits back up, holding no state.
 * Phone, email and tax ID have no inputs at all and round-trip verbatim so
 * saving never wipes them. There is deliberately no save before load: saving
 * an empty record would clear those same columns.
 *
 * This repo has no DOM tests, so "a field was lost" cannot be asserted
 * through the UI — company.test.ts pins this mapping instead.
 */
export type CompanyForm = {
  legalName: string;
  address: string;
};

// Stored-but-unedited profile values. Phone, email, and tax ID print
// nowhere, so they have no inputs — but they still round-trip verbatim so
// saving never wipes them.
export type StoredCompanyExtras = {
  phone: string;
  email: string;
  taxId: string;
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

/** The exact payload CompanyProfileCard's save passes to UpdateCompanyProfile. */
export type CompanyProfilePayload = {
  LegalName: string; Address: string;
  Phone: string; Email: string; TaxID: string;
  // Vestigial on the save path, kept deliberately: neither update path
  // writes it — the desktop forwards thirteen string columns and the server
  // binds thirteen, while the store recomputes HasLogo from logo_bytes. It
  // stays because the Wails binding types the UpdateCompanyProfile argument
  // as the full backend.CompanyProfile, so tsc requires all fourteen fields
  // at the call site; sending the echoed value is harmless since both update
  // paths ignore it.
  HasLogo: boolean;
  ThemeQDeep: string; ThemeQAccent: string; ThemeQSoft: string; ThemeQInk: string;
  ThemePDeep: string; ThemePAccent: string; ThemePSoft: string; ThemePInk: string;
};

export function profilePayload(input: {
  form: CompanyForm; extras: StoredCompanyExtras; theme: CompanyTheme; hasLogo: boolean;
}): CompanyProfilePayload {
  return {
    LegalName: input.form.legalName.trim(),
    Address: input.form.address.trim(),
    Phone: input.extras.phone,
    Email: input.extras.email,
    TaxID: input.extras.taxId,
    HasLogo: input.hasLogo,
    ThemeQDeep: input.theme.qDeep.trim(),
    ThemeQAccent: input.theme.qAccent.trim(),
    ThemeQSoft: input.theme.qSoft.trim(),
    ThemeQInk: input.theme.qInk.trim(),
    ThemePDeep: input.theme.pDeep.trim(),
    ThemePAccent: input.theme.pAccent.trim(),
    ThemePSoft: input.theme.pSoft.trim(),
    ThemePInk: input.theme.pInk.trim(),
  };
}

/** True when the value is a complete six-digit hex colour — the test
 *  ThemeColorField's picker applies to the trimmed value to decide whether
 *  the stored value can feed the native colour input. */
export function isCompleteHex(value: string): boolean {
  return /^#[0-9a-fA-F]{6}$/.test(value);
}
