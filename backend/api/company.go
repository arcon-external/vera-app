package api

import (
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"vera-app/backend/store"
)

// companyDTO renders the editable profile, theme included. Artwork bytes
// travel through the dedicated logo endpoints, never this DTO.
func companyDTO(profile store.CompanyProfile) map[string]any {
	return map[string]any{
		"legalName":    profile.LegalName,
		"address":      profile.Address,
		"phone":        profile.Phone,
		"email":        profile.Email,
		"taxId":        profile.TaxID,
		"hasLogo":      profile.HasLogo,
		"themeQDeep":   profile.ThemeQDeep,
		"themeQAccent": profile.ThemeQAccent,
		"themeQSoft":   profile.ThemeQSoft,
		"themeQInk":    profile.ThemeQInk,
		"themePDeep":   profile.ThemePDeep,
		"themePAccent": profile.ThemePAccent,
		"themePSoft":   profile.ThemePSoft,
		"themePInk":    profile.ThemePInk,
	}
}

func (s *Server) getCompanyProfile(context echo.Context) error {
	profile, err := s.database.GetCompanyProfile(context.Request().Context())
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "load company profile"})
	}
	return context.JSON(http.StatusOK, companyDTO(profile))
}

// getCompanyIdentity serves the company identity to every signed-in role
// so UI surfaces (shell header) track the stored profile. No audit: reads
// must stay cheap and silent.
func (s *Server) getCompanyIdentity(context echo.Context) error {
	profile, err := s.database.GetCompanyProfile(context.Request().Context())
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "load company profile"})
	}
	return context.JSON(http.StatusOK, companyDTO(profile))
}

func (s *Server) updateCompanyProfile(context echo.Context) error {
	var request struct {
		LegalName    string `json:"legalName"`
		Address      string `json:"address"`
		Phone        string `json:"phone"`
		Email        string `json:"email"`
		TaxID        string `json:"taxId"`
		ThemeQDeep   string `json:"themeQDeep"`
		ThemeQAccent string `json:"themeQAccent"`
		ThemeQSoft   string `json:"themeQSoft"`
		ThemeQInk    string `json:"themeQInk"`
		ThemePDeep   string `json:"themePDeep"`
		ThemePAccent string `json:"themePAccent"`
		ThemePSoft   string `json:"themePSoft"`
		ThemePInk    string `json:"themePInk"`
	}
	if err := context.Bind(&request); err != nil {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	updated, err := s.database.UpdateCompanyProfile(context.Request().Context(), store.CompanyProfile{
		LegalName:    request.LegalName,
		Address:      request.Address,
		Phone:        request.Phone,
		Email:        request.Email,
		TaxID:        request.TaxID,
		ThemeQDeep:   request.ThemeQDeep,
		ThemeQAccent: request.ThemeQAccent,
		ThemeQSoft:   request.ThemeQSoft,
		ThemeQInk:    request.ThemeQInk,
		ThemePDeep:   request.ThemePDeep,
		ThemePAccent: request.ThemePAccent,
		ThemePSoft:   request.ThemePSoft,
		ThemePInk:    request.ThemePInk,
	})
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "required"):
			return context.JSON(http.StatusUnprocessableEntity, map[string]string{"error": "company legal name is required"})
		case strings.Contains(err.Error(), "invalid company email"),
			strings.Contains(err.Error(), "too long"),
			strings.Contains(err.Error(), "must be a #RRGGBB"):
			return context.JSON(http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		default:
			return context.JSON(http.StatusInternalServerError, map[string]string{"error": "update company profile"})
		}
	}
	me := sessionUser(context)
	actorID := me.ID
	_ = s.database.Audit(context.Request().Context(), &actorID, "company.update", nil, updated.LegalName)
	return context.JSON(http.StatusOK, companyDTO(updated))
}

// setCompanyLogo stores uploaded logo artwork. The payload mirrors the
// avatar shape ({logoBase64, logoMime}) so the frontend reuses one picker
// pattern; validation happens server-side regardless of what the client
// claims.
func (s *Server) setCompanyLogo(context echo.Context) error {
	var request struct {
		LogoBase64 string `json:"logoBase64"`
		LogoMime   string `json:"logoMime"`
	}
	if err := context.Bind(&request); err != nil {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	data, err := base64.StdEncoding.DecodeString(request.LogoBase64)
	if err != nil || len(data) == 0 {
		return context.JSON(http.StatusUnprocessableEntity, map[string]string{"error": "logo image data is required"})
	}
	profile, err := s.database.SetCompanyLogo(context.Request().Context(), data, request.LogoMime)
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "must be PNG"),
			strings.Contains(err.Error(), "over 512KB"):
			return context.JSON(http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		default:
			return context.JSON(http.StatusInternalServerError, map[string]string{"error": "save company logo"})
		}
	}
	me := sessionUser(context)
	actorID := me.ID
	_ = s.database.Audit(context.Request().Context(), &actorID, "company.logo.update", nil, profile.LogoMime)
	return context.JSON(http.StatusOK, map[string]any{"hasLogo": profile.HasLogo})
}

// clearCompanyLogo removes the uploaded artwork, restoring the baked asset.
func (s *Server) clearCompanyLogo(context echo.Context) error {
	profile, err := s.database.SetCompanyLogo(context.Request().Context(), nil, "")
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "clear company logo"})
	}
	me := sessionUser(context)
	actorID := me.ID
	_ = s.database.Audit(context.Request().Context(), &actorID, "company.logo.clear", nil, "")
	return context.JSON(http.StatusOK, map[string]any{"hasLogo": profile.HasLogo})
}

// getCompanyLogo serves the stored artwork for previews. Every signed-in
// role may read it: the logo prints on customer documents, so hiding it
// from staff would be security theater that breaks their previews.
func (s *Server) getCompanyLogo(context echo.Context) error {
	profile, err := s.database.GetCompanyProfile(context.Request().Context())
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "load company logo"})
	}
	if !profile.HasLogo {
		return context.JSON(http.StatusNotFound, map[string]string{"error": "no uploaded logo"})
	}
	return context.JSON(http.StatusOK, map[string]string{
		"logoBase64": base64.StdEncoding.EncodeToString(profile.LogoBytes),
		"logoMime":   profile.LogoMime,
	})
}
