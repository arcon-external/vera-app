package store

import (
	"context"
	"fmt"
	"strings"
)

// CompanyProfile is the editable company identity shown in the admin-only
// Company section of Settings. It is a singleton row (id = 1); the PDF
// header still uses baked-in assets until a follow-up wires this through.
type CompanyProfile struct {
	LegalName string
	Address   string
	Phone     string
	Email     string
	TaxID     string
	LogoPath  string
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
		SELECT legal_name, address, phone, email, tax_id, logo_path
		FROM %s.company_profile WHERE id = 1`, s.configSchema())).Scan(
		&profile.LegalName, &profile.Address, &profile.Phone,
		&profile.Email, &profile.TaxID, &profile.LogoPath)
	if err != nil {
		return CompanyProfile{}, fmt.Errorf("get company profile: %w", err)
	}
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
		RETURNING legal_name, address, phone, email, tax_id, logo_path`, s.configSchema()),
		profile.LegalName, profile.Address, profile.Phone, profile.Email, profile.TaxID).Scan(
		&updated.LegalName, &updated.Address, &updated.Phone,
		&updated.Email, &updated.TaxID, &updated.LogoPath)
	if err != nil {
		return CompanyProfile{}, fmt.Errorf("update company profile: %w", err)
	}
	return updated, nil
}
