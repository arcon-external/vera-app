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
	LegalName string
	Address   string
	Phone     string
	Email     string
	TaxID     string
	LogoPath  string
	LogoBytes []byte
	LogoMime  string
	HasLogo   bool
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
		       logo_bytes, logo_mime
		FROM %s.company_profile WHERE id = 1`, s.configSchema())).Scan(
		&profile.LegalName, &profile.Address, &profile.Phone,
		&profile.Email, &profile.TaxID, &profile.LogoPath,
		&profile.LogoBytes, &profile.LogoMime)
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
	var updated CompanyProfile
	err = pool.QueryRow(ctx, fmt.Sprintf(`
		UPDATE %s.company_profile
		SET legal_name = $1, address = $2, phone = $3, email = $4, tax_id = $5,
		    updated_at = NOW()
		WHERE id = 1
		RETURNING legal_name, address, phone, email, tax_id, logo_path,
		    logo_bytes, logo_mime`, s.configSchema()),
		profile.LegalName, profile.Address, profile.Phone, profile.Email, profile.TaxID).Scan(
		&updated.LegalName, &updated.Address, &updated.Phone,
		&updated.Email, &updated.TaxID, &updated.LogoPath,
		&updated.LogoBytes, &updated.LogoMime)
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
