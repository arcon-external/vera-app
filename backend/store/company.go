package store

import (
	"context"
	"fmt"
	"strings"
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
	LogoBytes    []byte
	HasLogo      bool
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

// checkTheme validates the eight theme fields, all optional but strict.
func checkTheme(profile CompanyProfile) error {
	for _, field := range [][2]string{
		{"quotation deep", profile.ThemeQDeep},
		{"quotation accent", profile.ThemeQAccent},
		{"quotation soft", profile.ThemeQSoft},
		{"quotation ink", profile.ThemeQInk},
		{"portal deep", profile.ThemePDeep},
		{"portal accent", profile.ThemePAccent},
		{"portal soft", profile.ThemePSoft},
		{"portal ink", profile.ThemePInk},
	} {
		if field[1] != "" && !isHexColor(field[1]) {
			return fmt.Errorf("company theme %s must be a #RRGGBB color", field[0])
		}
	}
	return nil
}

// GetCompanyProfile returns the singleton profile, seeding the default
// identity if the row is somehow missing.
func (s *Store) GetCompanyProfile(ctx context.Context) (CompanyProfile, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return CompanyProfile{}, err
	}
	var profile CompanyProfile
	err = pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT legal_name, address, phone, email, tax_id, logo_path,
		       logo_bytes, logo_mime,
		       theme_q_deep, theme_q_accent, theme_q_soft, theme_q_ink,
		       theme_p_deep, theme_p_accent, theme_p_soft, theme_p_ink
		FROM %s.company_profile WHERE id = 1`, s.configSchema())).Scan(
		&profile.LegalName, &profile.Address, &profile.Phone,
		&profile.Email, &profile.TaxID, &profile.LogoPath,
		&profile.LogoBytes, &profile.LogoMime,
		&profile.ThemeQDeep, &profile.ThemeQAccent, &profile.ThemeQSoft, &profile.ThemeQInk,
		&profile.ThemePDeep, &profile.ThemePAccent, &profile.ThemePSoft, &profile.ThemePInk)
	if err != nil {
		return CompanyProfile{}, fmt.Errorf("get company profile: %w", err)
	}
	profile.HasLogo = len(profile.LogoBytes) > 0
	return profile, nil
}

// UpdateCompanyProfile full-replaces the editable fields. Empty strings
// clear a field, except legal name which is always required.
func (s *Store) UpdateCompanyProfile(ctx context.Context, profile CompanyProfile) (CompanyProfile, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return CompanyProfile{}, err
	}
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
	if err := checkTheme(profile); err != nil {
		return CompanyProfile{}, err
	}
	var updated CompanyProfile
	err = pool.QueryRow(ctx, fmt.Sprintf(`
		UPDATE %s.company_profile
		SET legal_name = $1, address = $2, phone = $3, email = $4, tax_id = $5,
		    theme_q_deep = $6, theme_q_accent = $7, theme_q_soft = $8, theme_q_ink = $9,
		    theme_p_deep = $10, theme_p_accent = $11, theme_p_soft = $12, theme_p_ink = $13,
		    updated_at = NOW()
		WHERE id = 1
		RETURNING legal_name, address, phone, email, tax_id, logo_path,
		    logo_bytes, logo_mime,
		    theme_q_deep, theme_q_accent, theme_q_soft, theme_q_ink,
		    theme_p_deep, theme_p_accent, theme_p_soft, theme_p_ink`, s.configSchema()),
		profile.LegalName, profile.Address, profile.Phone, profile.Email, profile.TaxID,
		profile.ThemeQDeep, profile.ThemeQAccent, profile.ThemeQSoft, profile.ThemeQInk,
		profile.ThemePDeep, profile.ThemePAccent, profile.ThemePSoft, profile.ThemePInk).Scan(
		&updated.LegalName, &updated.Address, &updated.Phone,
		&updated.Email, &updated.TaxID, &updated.LogoPath,
		&updated.LogoBytes, &updated.LogoMime,
		&updated.ThemeQDeep, &updated.ThemeQAccent, &updated.ThemeQSoft, &updated.ThemeQInk,
		&updated.ThemePDeep, &updated.ThemePAccent, &updated.ThemePSoft, &updated.ThemePInk)
	if err != nil {
		return CompanyProfile{}, fmt.Errorf("update company profile: %w", err)
	}
	updated.HasLogo = len(updated.LogoBytes) > 0
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
		    logo_bytes, logo_mime`, s.configSchema()),
		nilIfEmptyBytes(data), &mime).Scan(
		&profile.LegalName, &profile.Address, &profile.Phone,
		&profile.Email, &profile.TaxID, &profile.LogoPath,
		&profile.LogoBytes, &profile.LogoMime)
	if err != nil {
		return CompanyProfile{}, fmt.Errorf("set company logo: %w", err)
	}
	profile.HasLogo = len(profile.LogoBytes) > 0
	return profile, nil
}
