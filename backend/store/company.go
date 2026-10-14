package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"vera-app/backend/currency"
)

// CompanyProfile is the editable company identity shown in the admin-only
// Company section of Settings. It is a singleton row (id = 1). The logo
// artwork lives here too; empty bytes mean "use the baked asset".
type CompanyProfile struct {
	ThemeQInk    string
	ThemeQDeep   string
	Phone        string
	Email        string
	TaxID        string
	LogoPath     string
	ThemePInk    string
	LogoMime     string
	Address      string
	ThemeQAccent string
	ThemePSoft   string
	ThemeQSoft   string
	LegalName    string
	ThemePDeep   string
	ThemePAccent string
	// Currency is the company's ISO 4217 code from the shipped catalogue.
	// Normalized to upper-case on write; empty writes as IDR.
	Currency  string
	LogoBytes []byte
	HasLogo   bool
	// CanonicalUoM is the unit a quotation's package lines are quoted in
	// (it replaces the hardcoded "Lot"). It must name a member of the
	// company's unit list exactly; the FK on the column enforces that at the
	// database too (migration 026). Empty reads as the stored value on
	// update -- see UpdateCompanyProfile.
	CanonicalUoM string
}

// companyProfileColumns is the single declaration of the company_profile
// columns the store reads and writes, shared between the SELECT and every
// RETURNING below. The list used to be written out in full in each statement;
// a column dropped from one copy and not the others is a field that stops
// persisting, and nothing says so.
const companyProfileColumns = `legal_name, address, phone, email, tax_id, logo_path,
		logo_bytes, logo_mime, currency, canonical_uom,
		theme_q_deep, theme_q_accent, theme_q_soft, theme_q_ink,
		theme_p_deep, theme_p_accent, theme_p_soft, theme_p_ink`

// scanCompanyProfile scans one companyProfileColumns row into a profile. The
// caller wraps the error in the operation that failed, so the message still
// names the statement. HasLogo is recomputed from the artwork bytes here
// rather than read: it is derived, and a column for it would be a second
// truth.
func scanCompanyProfile(row pgx.Row) (CompanyProfile, error) {
	var profile CompanyProfile
	err := row.Scan(
		&profile.LegalName, &profile.Address, &profile.Phone,
		&profile.Email, &profile.TaxID, &profile.LogoPath,
		&profile.LogoBytes, &profile.LogoMime, &profile.Currency, &profile.CanonicalUoM,
		&profile.ThemeQDeep, &profile.ThemeQAccent, &profile.ThemeQSoft, &profile.ThemeQInk,
		&profile.ThemePDeep, &profile.ThemePAccent, &profile.ThemePSoft, &profile.ThemePInk)
	if err != nil {
		return CompanyProfile{}, err
	}
	profile.HasLogo = len(profile.LogoBytes) > 0
	return profile, nil
}

// maxCompanyLogoBytes caps uploaded artwork at 512KB, mirroring the avatar
// rules so admins meet one consistent limit everywhere.
const maxCompanyLogoBytes = 512 * 1024

// allowedCompanyLogoMimes mirrors the avatar allowlist.
func allowedCompanyLogoMime(mime string) bool {
	switch mime {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return true
	default:
		return false
	}
}

// isHexColor validates #RRGGBB input. Theme values travel into CSS, so
// anything but a strict hex color is rejected at the store boundary.
func isHexColor(value string) bool {
	if len(value) != 7 || value[0] != '#' {
		return false
	}
	for _, r := range value[1:] {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F') {
			return false
		}
	}
	return true
}

// checkThemeColors is the per-field rule behind every theme write, shared by
// the full-replace endpoint's checkTheme and the narrow per-tab endpoints:
// empty means "use the stylesheet default", anything else must be a strict
// #RRGGBB. One rule, one place, so a save from the Quotation tab and a save
// from the whole-record form cannot disagree about what a colour is.
func checkThemeColors(fields [][2]string) error {
	for _, field := range fields {
		if field[1] != "" && !isHexColor(field[1]) {
			return fmt.Errorf("company theme %s must be a #RRGGBB color", field[0])
		}
	}
	return nil
}

// checkTheme validates the eight theme fields, all optional but strict.
func checkTheme(profile CompanyProfile) error {
	return checkThemeColors([][2]string{
		{"quotation deep", profile.ThemeQDeep},
		{"quotation accent", profile.ThemeQAccent},
		{"quotation soft", profile.ThemeQSoft},
		{"quotation ink", profile.ThemeQInk},
		{"portal deep", profile.ThemePDeep},
		{"portal accent", profile.ThemePAccent},
		{"portal soft", profile.ThemePSoft},
		{"portal ink", profile.ThemePInk},
	})
}

// checkCurrency validates the company currency against the shipped
// catalogue. The error names the field, like every other store-boundary
// validation, so a rejected save says what to fix.
func checkCurrency(profile CompanyProfile) error {
	if profile.Currency == "" {
		return nil
	}
	if _, ok := currency.Lookup(profile.Currency); !ok {
		return fmt.Errorf("company currency %q is not supported (want one of: %s)",
			profile.Currency, strings.Join(currency.Codes(), ", "))
	}
	return nil
}

// normalizeCompanyGeneral trims and validates the six fields the admin
// General tab owns: the legal name, address, phone, email, tax ID and
// currency. Both update paths run through it -- the full-replace
// UpdateCompanyProfile and the narrow UpdateCompanyGeneral -- so a value that
// is acceptable from one is acceptable from the other, and the store one
// spelling of the currency code either way.
//
// The legal name is the only required field; every other one clears on empty.
func normalizeCompanyGeneral(profile CompanyProfile) (CompanyProfile, error) {
	profile.LegalName = strings.TrimSpace(profile.LegalName)
	profile.Address = strings.TrimSpace(profile.Address)
	profile.Phone = strings.TrimSpace(profile.Phone)
	profile.Email = strings.TrimSpace(profile.Email)
	profile.TaxID = strings.TrimSpace(profile.TaxID)
	if profile.LegalName == "" {
		return CompanyProfile{}, fmt.Errorf("company legal name is required")
	}
	if len(profile.LegalName) > 160 {
		return CompanyProfile{}, fmt.Errorf("company legal name is too long")
	}
	if profile.Email != "" && !validEmailFormat(profile.Email) {
		return CompanyProfile{}, fmt.Errorf("invalid company email address")
	}
	if len(profile.Phone) > 40 {
		return CompanyProfile{}, fmt.Errorf("company phone is too long")
	}
	if len(profile.TaxID) > 40 {
		return CompanyProfile{}, fmt.Errorf("company tax ID is too long")
	}
	// Empty writes as the default; anything else must be a catalogue code,
	// upper-cased so "usd" and "USD" store identically.
	profile.Currency = strings.ToUpper(strings.TrimSpace(profile.Currency))
	if profile.Currency == "" {
		profile.Currency = currency.DefaultCode
	}
	if err := checkCurrency(profile); err != nil {
		return CompanyProfile{}, err
	}
	return profile, nil
}

// GetCompanyProfile returns the singleton profile, seeding the default
// identity if the row is somehow missing.
func (s *Store) GetCompanyProfile(ctx context.Context) (CompanyProfile, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return CompanyProfile{}, err
	}
	profile, err := scanCompanyProfile(pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT %s FROM %s.company_profile WHERE id = 1`,
		companyProfileColumns, s.configSchema())))
	if err != nil {
		return CompanyProfile{}, fmt.Errorf("get company profile: %w", err)
	}
	return profile, nil
}

// UpdateCompanyProfile full-replaces the editable fields. Empty strings
// clear a field, except legal name which is always required.
//
// This is the old-client path: a desktop that knows only this endpoint sends
// every editable column in one payload, so it still works untouched beside the
// narrow per-tab writes below. New clients save one tab at a time through
// UpdateCompanyGeneral, UpdateQuotationTheme, UpdatePortalTheme and
// UpdateCompanyUnits.
func (s *Store) UpdateCompanyProfile(ctx context.Context, profile CompanyProfile) (CompanyProfile, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return CompanyProfile{}, err
	}
	profile, err = normalizeCompanyGeneral(profile)
	if err != nil {
		return CompanyProfile{}, err
	}
	// Canonical unit: an empty value keeps whatever is stored -- an old
	// desktop client that does not know the field must not be able to move
	// it -- and anything else must name a live list member exactly: the FK
	// compares canonical_uom against company_uom.value byte for byte, so a
	// value that matches only case-insensitively would be refused by the
	// database at commit anyway, and the store refuses earlier with a
	// message naming the unit.
	if trimmed := strings.TrimSpace(profile.CanonicalUoM); trimmed != "" {
		profile.CanonicalUoM = trimmed
		units, err := s.GetCompanyUnits(ctx)
		if err != nil {
			return CompanyProfile{}, fmt.Errorf("validate canonical unit: %w", err)
		}
		if !unitListContains(units, profile.CanonicalUoM) {
			return CompanyProfile{}, fmt.Errorf("company canonical unit %q is not on the company's unit list", profile.CanonicalUoM)
		}
	}
	if err := checkTheme(profile); err != nil {
		return CompanyProfile{}, err
	}
	updated, err := scanCompanyProfile(pool.QueryRow(ctx, fmt.Sprintf(`
		UPDATE %s.company_profile
		SET legal_name = $1, address = $2, phone = $3, email = $4, tax_id = $5,
		    theme_q_deep = $6, theme_q_accent = $7, theme_q_soft = $8, theme_q_ink = $9,
		    theme_p_deep = $10, theme_p_accent = $11, theme_p_soft = $12, theme_p_ink = $13,
		    currency = $14,
		    canonical_uom = COALESCE(NULLIF($15, ''), canonical_uom),
		    updated_at = NOW()
		WHERE id = 1
		RETURNING %s`, s.configSchema(), companyProfileColumns),
		profile.LegalName, profile.Address, profile.Phone, profile.Email, profile.TaxID,
		profile.ThemeQDeep, profile.ThemeQAccent, profile.ThemeQSoft, profile.ThemeQInk,
		profile.ThemePDeep, profile.ThemePAccent, profile.ThemePSoft, profile.ThemePInk,
		profile.Currency, profile.CanonicalUoM))
	if err != nil {
		return CompanyProfile{}, fmt.Errorf("update company profile: %w", err)
	}
	return updated, nil
}

// UpdateCompanyGeneral writes the six General-tab columns and nothing else:
// the legal name, address, phone, email, tax ID and currency. The theme
// columns, the canonical unit and the artwork are not in this statement, so a
// save from the General tab cannot clear a colour or move the canonical unit
// the way the full-replace path requires its caller to resend everything.
//
// Validation is the same normalizeCompanyGeneral the full-replace runs, so the
// two paths cannot disagree about what a legal name is.
func (s *Store) UpdateCompanyGeneral(ctx context.Context, profile CompanyProfile) (CompanyProfile, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return CompanyProfile{}, err
	}
	profile, err = normalizeCompanyGeneral(profile)
	if err != nil {
		return CompanyProfile{}, err
	}
	updated, err := scanCompanyProfile(pool.QueryRow(ctx, fmt.Sprintf(`
		UPDATE %s.company_profile
		SET legal_name = $1, address = $2, phone = $3, email = $4, tax_id = $5,
		    currency = $6, updated_at = NOW()
		WHERE id = 1
		RETURNING %s`, s.configSchema(), companyProfileColumns),
		profile.LegalName, profile.Address, profile.Phone, profile.Email,
		profile.TaxID, profile.Currency))
	if err != nil {
		return CompanyProfile{}, fmt.Errorf("update company profile: %w", err)
	}
	return updated, nil
}

// UpdateQuotationTheme writes the four quotation-theme columns and nothing
// else, so a colour save from the Quotation tab cannot touch the portal's
// four. Each value is optional and strict: empty means "use the stylesheet
// default", and anything else must be a #RRGGBB, the same rule checkTheme
// applies to the full-replace path.
func (s *Store) UpdateQuotationTheme(ctx context.Context, deep, accent, soft, ink string) (CompanyProfile, error) {
	if err := checkThemeColors([][2]string{
		{"quotation deep", deep}, {"quotation accent", accent},
		{"quotation soft", soft}, {"quotation ink", ink},
	}); err != nil {
		return CompanyProfile{}, err
	}
	pool, err := s.poolOrError()
	if err != nil {
		return CompanyProfile{}, err
	}
	updated, err := scanCompanyProfile(pool.QueryRow(ctx, fmt.Sprintf(`
		UPDATE %s.company_profile
		SET theme_q_deep = $1, theme_q_accent = $2, theme_q_soft = $3, theme_q_ink = $4,
		    updated_at = NOW()
		WHERE id = 1
		RETURNING %s`, s.configSchema(), companyProfileColumns),
		deep, accent, soft, ink))
	if err != nil {
		return CompanyProfile{}, fmt.Errorf("update company quotation theme: %w", err)
	}
	return updated, nil
}

// UpdatePortalTheme writes the four portal-theme columns and nothing else. It
// is UpdateQuotationTheme with the other half of the brand, and it exists
// separately because the two halves are independent columns: rebranding the
// verification page must not repaint every quotation.
func (s *Store) UpdatePortalTheme(ctx context.Context, deep, accent, soft, ink string) (CompanyProfile, error) {
	if err := checkThemeColors([][2]string{
		{"portal deep", deep}, {"portal accent", accent},
		{"portal soft", soft}, {"portal ink", ink},
	}); err != nil {
		return CompanyProfile{}, err
	}
	pool, err := s.poolOrError()
	if err != nil {
		return CompanyProfile{}, err
	}
	updated, err := scanCompanyProfile(pool.QueryRow(ctx, fmt.Sprintf(`
		UPDATE %s.company_profile
		SET theme_p_deep = $1, theme_p_accent = $2, theme_p_soft = $3, theme_p_ink = $4,
		    updated_at = NOW()
		WHERE id = 1
		RETURNING %s`, s.configSchema(), companyProfileColumns),
		deep, accent, soft, ink))
	if err != nil {
		return CompanyProfile{}, fmt.Errorf("update company portal theme: %w", err)
	}
	return updated, nil
}

// SetCompanyLogo stores uploaded logo artwork after validating it
// server-side (never trust the client): allowlisted image type, 512KB cap.
// Clearing passes nil data, which restores the baked asset.
func (s *Store) SetCompanyLogo(ctx context.Context, data []byte, mime string) (CompanyProfile, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return CompanyProfile{}, err
	}
	if len(data) > 0 {
		if !allowedCompanyLogoMime(mime) {
			return CompanyProfile{}, fmt.Errorf("logo must be PNG, JPEG, GIF, or WebP")
		}
		if len(data) > maxCompanyLogoBytes {
			return CompanyProfile{}, fmt.Errorf("logo is over 512KB")
		}
	} else {
		mime = ""
	}
	var profile CompanyProfile
	err = pool.QueryRow(ctx, fmt.Sprintf(`
		UPDATE %s.company_profile
		SET logo_bytes = $1, logo_mime = $2, updated_at = NOW()
		WHERE id = 1
		RETURNING legal_name, address, phone, email, tax_id, logo_path,
		    logo_bytes, logo_mime, canonical_uom`, s.configSchema()),
		nilIfEmptyBytes(data), &mime).Scan(
		&profile.LegalName, &profile.Address, &profile.Phone,
		&profile.Email, &profile.TaxID, &profile.LogoPath,
		&profile.LogoBytes, &profile.LogoMime, &profile.CanonicalUoM)
	if err != nil {
		return CompanyProfile{}, fmt.Errorf("set company logo: %w", err)
	}
	profile.HasLogo = len(profile.LogoBytes) > 0
	return profile, nil
}
