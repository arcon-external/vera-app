import {
  EMPTY_THEME,
  THEME_FIELDS,
  extrasFromProfile,
  formFromProfile,
  isCompleteHex,
  themeFromProfile,
} from "./company";
import type { CompanyProfileLike } from "./company";
import { backend } from "@backend/models";

// The company save is per-tab (#432): the General tab writes its six columns
// through UpdateCompanyGeneral, the Units tab writes the list plus the
// canonical unit, the Quotation tab writes the wording and its four colours,
// the Portal tab writes its four. Those payloads are pinned in
// companySaves.test.ts, against the endpoints that receive them.
//
// What is pinned here is the read side, and it is still the whole record: the
// narrow endpoints each write one slice, so the page must hold every served
// value — including the phone, email, tax ID and currency that have no inputs
// of their own — or the General save would clear them. "A field was lost"
// cannot be asserted through the UI (no DOM tests), so it fails here instead.

// Every field carries a distinct, recognisable value, so a swap or a drop
// points at exactly the field that broke.
const FULL_PROFILE: CompanyProfileLike = {
  LegalName: "PT Arcon Penta Persada",
  Address: "Jl. Merdeka 1, Jakarta",
  Phone: "+62-21-5550192",
  Email: "admin@arconpentapersada.co.id",
  TaxID: "01.234.567.8-901.234",
  Currency: "USD",
  CanonicalUoM: "Week",
  ThemeQDeep: "#111111",
  ThemeQAccent: "#222222",
  ThemeQSoft: "#333333",
  ThemeQInk: "#444444",
  ThemePDeep: "#555555",
  ThemePAccent: "#666666",
  ThemePSoft: "#777777",
  ThemePInk: "#888888",
};

describe("company profile load", () => {
  it("reads every field the tabs edit, and the hidden ones they re-send", () => {
    // The whole record reaches the form: the two displayed fields, the six the
    // General save re-sends without showing, and the eight colours split
    // between the Quotation and Portal tabs.
    expect(formFromProfile(FULL_PROFILE)).toEqual({
      legalName: "PT Arcon Penta Persada",
      address: "Jl. Merdeka 1, Jakarta",
    });
    expect(extrasFromProfile(FULL_PROFILE)).toEqual({
      phone: "+62-21-5550192",
      email: "admin@arconpentapersada.co.id",
      taxId: "01.234.567.8-901.234",
      currency: "USD",
      canonicalUoM: "Week",
    });
    expect(themeFromProfile(FULL_PROFILE)).toEqual({
      qDeep: "#111111",
      qAccent: "#222222",
      qSoft: "#333333",
      qInk: "#444444",
      pDeep: "#555555",
      pAccent: "#666666",
      pSoft: "#777777",
      pInk: "#888888",
    });
  });

  it.each([["undefined", undefined] as const, ["null", null] as const])(
    "turns an absent profile with %s fields into empty strings",
    (_label, absent) => {
      const profile: CompanyProfileLike = {
        LegalName: absent,
        Address: absent,
        Phone: absent,
        Email: absent,
        TaxID: absent,
        Currency: absent,
        ThemeQDeep: absent,
        ThemeQAccent: absent,
        ThemeQSoft: absent,
        ThemeQInk: absent,
        ThemePDeep: absent,
        ThemePAccent: absent,
        ThemePSoft: absent,
        ThemePInk: absent,
      };
      const form = formFromProfile(profile);
      const extras = extrasFromProfile(profile);
      const theme = themeFromProfile(profile);

      for (const value of [...Object.values(form), ...Object.values(extras), ...Object.values(theme)]) {
        expect(value).toBe("");
      }
      // Belt and suspenders: the coercion bug this guards against is
      // String(undefined) === "undefined" leaking into a saved column.
      expect(JSON.stringify({ form, extras, theme })).not.toContain("undefined");
      expect(JSON.stringify({ form, extras, theme })).not.toContain("null");
    }
  );

  it("keeps the quotation and portal colours apart, so one tab cannot edit the other's", () => {
    // The two halves of the brand are four columns each and two saves; a
    // reader that crossed them would let the Portal tab paint the document.
    const theme = themeFromProfile(FULL_PROFILE);
    expect(theme.qDeep).not.toBe(theme.pDeep);
    expect(theme.qInk).not.toBe(theme.pInk);
  });
});

describe("THEME_FIELDS", () => {
  it("covers every CompanyTheme key exactly once, four per section", () => {
    // Keys are derived from EMPTY_THEME (typed CompanyTheme), so adding a
    // colour to CompanyTheme without adding a THEME_FIELDS entry fails this
    // test instead of silently leaving the new colour unsaveable/unrendered.
    const themeKeys = Object.keys(EMPTY_THEME).sort();
    const fieldKeys = THEME_FIELDS.map((field) => field.key).sort();

    expect(THEME_FIELDS).toHaveLength(8);
    expect(fieldKeys).toEqual(themeKeys);
    expect(new Set(fieldKeys).size).toBe(fieldKeys.length);
    expect(THEME_FIELDS.filter((field) => field.section === "Quotation")).toHaveLength(4);
    expect(THEME_FIELDS.filter((field) => field.section === "Portal")).toHaveLength(4);
  });
});

describe("CompanyProfile load shape", () => {
  it("reads every generated backend.CompanyProfile field except HasLogo", () => {
    // CompanyProfileLike plus the three *FromProfile readers are a
    // hand-written mirror of the generated backend.CompanyProfile: a Go field
    // missing from the readers never reaches the form, and the General save —
    // which writes the six columns it knows about — would clear the column it
    // never read. This test fails — naming the field — the moment the two
    // drift apart.
    //
    // Wails model constructors assign every declared field (to undefined
    // when the source omits it, so JSON.stringify shows {}), which is why
    // Object.keys(new backend.CompanyProfile()) is the full field list.
    // Each model field gets a distinct sentinel, so a reader that dropped
    // or swapped a field fails here naming exactly which one was lost.
    //
    // HasLogo is the one deliberate exception: it is not part of the profile
    // load at all. CompanyProfileCard's load ignores profile.HasLogo and takes
    // hasLogo from GetCompanyLogo instead (setHasLogo(!logo.BuiltIn)), and the
    // update endpoints never write it either — the store recomputes HasLogo
    // from logo_bytes (len(LogoBytes) > 0). A reader that skipped HasLogo
    // therefore cannot wipe the column, so excluding it here is safe. The
    // assertion below pins that the readers genuinely do not consume it.
    const modelKeys = Object.keys(new backend.CompanyProfile()).sort();
    const expected = modelKeys.filter((key) => key !== "HasLogo");
    const probe: Record<string, string> = {};
    for (const key of modelKeys) {
      probe[key] = `sentinel:${key}`;
    }
    const profile = probe as unknown as CompanyProfileLike;
    const seen = new Set([
      ...Object.values(formFromProfile(profile)),
      ...Object.values(extrasFromProfile(profile)),
      ...Object.values(themeFromProfile(profile)),
    ]);
    const unread = expected.filter((key) => !seen.has(`sentinel:${key}`));
    expect(
      unread,
      `CompanyProfile readers stopped reading [${unread.join(", ")}] ` +
        `from backend.CompanyProfile. formFromProfile, extrasFromProfile and ` +
        `themeFromProfile must cover every model field except HasLogo, or the ` +
        `next General save clears the column.`
    ).toEqual([]);
    expect(
      seen.has("sentinel:HasLogo"),
      `CompanyProfile readers started consuming HasLogo, which belongs to ` +
        `GetCompanyLogo (see CompanyProfileCard's load). Keep it out of the ` +
        `profile readers.`
    ).toBe(false);
  });
});

describe("isCompleteHex", () => {
  it.each(["#1A237E", "#abcdef"])("accepts %s", (value) => {
    expect(isCompleteHex(value)).toBe(true);
  });

  it.each(["#abc", "1A237E", "#GGGGGG", ""])("rejects %s", (value) => {
    expect(isCompleteHex(value)).toBe(false);
  });
});
