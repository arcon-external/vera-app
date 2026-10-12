import {
  EMPTY_THEME,
  THEME_FIELDS,
  extrasFromProfile,
  formFromProfile,
  isCompleteHex,
  profilePayload,
  themeFromProfile,
} from "./company";
import type { CompanyProfileLike } from "./company";
import { backend } from "@backend/models";

// The company save must stay whole-record — see company.ts for why. These
// tests pin the load/payload mapping so "a field was lost" fails here instead.

// Every field carries a distinct, recognisable value, so a swap or a drop
// points at exactly the field that broke.
const FULL_PROFILE: CompanyProfileLike = {
  LegalName: "PT Arcon Penta Persada",
  Address: "Jl. Merdeka 1, Jakarta",
  Phone: "+62-21-5550192",
  Email: "admin@arconpentapersada.co.id",
  TaxID: "01.234.567.8-901.234",
  Currency: "USD",
  ThemeQDeep: "#111111",
  ThemeQAccent: "#222222",
  ThemeQSoft: "#333333",
  ThemeQInk: "#444444",
  ThemePDeep: "#555555",
  ThemePAccent: "#666666",
  ThemePSoft: "#777777",
  ThemePInk: "#888888",
};

describe("company profile round trip", () => {
  it("loses nothing from load to payload", () => {
    const payload = profilePayload({
      form: formFromProfile(FULL_PROFILE),
      extras: extrasFromProfile(FULL_PROFILE),
      theme: themeFromProfile(FULL_PROFILE),
      hasLogo: true,
    });

    expect(payload.LegalName).toBe("PT Arcon Penta Persada");
    expect(payload.Address).toBe("Jl. Merdeka 1, Jakarta");
    expect(payload.Phone).toBe("+62-21-5550192");
    expect(payload.Email).toBe("admin@arconpentapersada.co.id");
    expect(payload.TaxID).toBe("01.234.567.8-901.234");
    expect(payload.Currency).toBe("USD");
    expect(payload.HasLogo).toBe(true);
    expect(payload.ThemeQDeep).toBe("#111111");
    expect(payload.ThemeQAccent).toBe("#222222");
    expect(payload.ThemeQSoft).toBe("#333333");
    expect(payload.ThemeQInk).toBe("#444444");
    expect(payload.ThemePDeep).toBe("#555555");
    expect(payload.ThemePAccent).toBe("#666666");
    expect(payload.ThemePSoft).toBe("#777777");
    expect(payload.ThemePInk).toBe("#888888");
  });

  it("never drops a theme section when only one section changed", () => {
    // The wipe scenario: the Quotation tab edits four colours, the Portal
    // colours ride along untouched in the same theme object. A builder that
    // saved only the edited slice would send the other four as empty and
    // wipe them server-side.
    const theme = themeFromProfile(FULL_PROFILE);
    theme.qDeep = "#abcdef";
    theme.qAccent = "#123456";
    theme.qSoft = "#654321";
    theme.qInk = "#fedcba";

    const payload = profilePayload({
      form: formFromProfile(FULL_PROFILE),
      extras: extrasFromProfile(FULL_PROFILE),
      theme,
      hasLogo: false,
    });

    expect(payload.ThemeQDeep).toBe("#abcdef");
    expect(payload.ThemeQAccent).toBe("#123456");
    expect(payload.ThemeQSoft).toBe("#654321");
    expect(payload.ThemeQInk).toBe("#fedcba");
    // Distinct portal values: a swap with the quotation colours, or an
    // empty-section wipe, would both fail here.
    expect(payload.ThemePDeep).toBe("#555555");
    expect(payload.ThemePAccent).toBe("#666666");
    expect(payload.ThemePSoft).toBe("#777777");
    expect(payload.ThemePInk).toBe("#888888");
  });

  it("carries the hidden phone, email, tax id and currency even though no control renders them", () => {
    const payload = profilePayload({
      form: formFromProfile(FULL_PROFILE),
      extras: extrasFromProfile(FULL_PROFILE),
      theme: themeFromProfile(FULL_PROFILE),
      hasLogo: false,
    });

    expect(payload.Phone).toBe("+62-21-5550192");
    expect(payload.Email).toBe("admin@arconpentapersada.co.id");
    expect(payload.TaxID).toBe("01.234.567.8-901.234");
    expect(payload.Currency).toBe("USD");
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

      const payload = profilePayload({ form, extras, theme, hasLogo: false });
      for (const value of Object.values(payload)) {
        if (typeof value === "string") {
          expect(value).toBe("");
        }
      }
      // Belt and suspenders: the coercion bug this guards against is
      // String(undefined) === "undefined" leaking into a saved column.
      expect(JSON.stringify(payload)).not.toContain("undefined");
      expect(JSON.stringify(payload)).not.toContain("null");
    }
  );

  it("trims the legal name and address but passes extras through verbatim", () => {
    const payload = profilePayload({
      form: { legalName: "  PT Arcon Penta Persada  ", address: "\tJl. Merdeka 1\n" },
      extras: { phone: "  +62-21-5550192  ", email: " admin@example.id ", taxId: " 01.234 ", currency: "USD" },
      theme: themeFromProfile(FULL_PROFILE),
      hasLogo: false,
    });

    expect(payload.LegalName).toBe("PT Arcon Penta Persada");
    expect(payload.Address).toBe("Jl. Merdeka 1");
    expect(payload.Phone).toBe("  +62-21-5550192  ");
    expect(payload.Email).toBe(" admin@example.id ");
    expect(payload.TaxID).toBe(" 01.234 ");
    expect(payload.Currency).toBe("USD");
  });

  it("trims colour values exactly as the current save does", () => {
    // profilePayload applies .trim() to all eight colours. Kept
    // here (rather than "colours are not trimmed") so the module reproduces
    // the current behaviour byte-for-byte; a pasted "  #1A237E  " must save
    // as "#1A237E" before and after the refactor.
    const payload = profilePayload({
      form: formFromProfile(FULL_PROFILE),
      extras: extrasFromProfile(FULL_PROFILE),
      theme: {
        qDeep: "  #111111  ",
        qAccent: "#222222",
        qSoft: "#333333",
        qInk: "#444444",
        pDeep: "#555555",
        pAccent: "#666666",
        pSoft: "#777777",
        pInk: "\t#888888\n",
      },
      hasLogo: false,
    });

    expect(payload.ThemeQDeep).toBe("#111111");
    expect(payload.ThemePInk).toBe("#888888");
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

describe("CompanyProfilePayload shape", () => {
  it("covers every generated backend.CompanyProfile field exactly once", () => {
    // CompanyProfilePayload is a hand-written mirror of the generated
    // backend.CompanyProfile, and UpdateCompanyProfile full-replaces: a Go
    // field missing from the payload is cleared on every save. This test
    // fails — naming the field — the moment the two drift apart.
    //
    // Wails model constructors assign every declared field (to undefined
    // when the source omits it, so JSON.stringify shows {}), which is why
    // Object.keys(new backend.CompanyProfile()) is the full field list.
    // The keys under test come from a real profilePayload output, not a
    // hand-written list, so a builder that dropped a field fails here too.
    // Boundary: this pins the builder's runtime output only, not the
    // CompanyProfilePayload declaration — that is pinned by tsc, so run the
    // type check and read its exit code (never through a pipe).
    const modelKeys = Object.keys(new backend.CompanyProfile()).sort();
    const payloadKeys = Object.keys(
      profilePayload({
        form: { legalName: "", address: "" },
        extras: { phone: "", email: "", taxId: "", currency: "" },
        theme: EMPTY_THEME,
        hasLogo: false,
      })
    ).sort();
    const missing = modelKeys.filter((key) => !payloadKeys.includes(key));
    const extra = payloadKeys.filter((key) => !modelKeys.includes(key));
    expect(
      payloadKeys,
      `CompanyProfilePayload drifted from backend.CompanyProfile: ` +
        `missing [${missing.join(", ")}]` +
        (extra.length ? `, unknown [${extra.join(", ")}]` : "") +
        `. UpdateCompanyProfile full-replaces, so a missing field is cleared on every save.`
    ).toEqual(modelKeys);
  });
});

describe("CompanyProfile load shape", () => {
  it("reads every generated backend.CompanyProfile field except HasLogo", () => {
    // CompanyProfileLike plus the three *FromProfile readers are a
    // hand-written mirror of the generated backend.CompanyProfile, exactly
    // like the payload: a Go field missing from the readers never reaches
    // the form, and the next whole-record save writes it back as "". This
    // test fails — naming the field — the moment the two drift apart.
    //
    // Wails model constructors assign every declared field (to undefined
    // when the source omits it, so JSON.stringify shows {}), which is why
    // Object.keys(new backend.CompanyProfile()) is the full field list.
    // Each model field gets a distinct sentinel, so a reader that dropped
    // or swapped a field fails here naming exactly which one was lost.
    //
    // HasLogo is the one deliberate exception: it is not read here because
    // it is not part of the profile load at all. CompanyProfileCard's load
    // ignores profile.HasLogo and takes hasLogo from GetCompanyLogo
    // instead (setHasLogo(!logo.BuiltIn)), and the update endpoints never
    // write it either — PUT /v1/admin/company binds only the fourteen
    // editable columns and the store recomputes HasLogo from logo_bytes
    // (len(LogoBytes) > 0). A reader that skipped HasLogo therefore cannot
    // wipe the column, so excluding it here is safe. The assertion below
    // pins that the readers genuinely do not consume it.
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
        `themeFromProfile must cover every model field except HasLogo, or ` +
        `the next whole-record save clears the column.`
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
