package api

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"vera-app/backend"
	"vera-app/backend/labels"
	"vera-app/backend/store"
)

// companyDTO renders the editable profile, theme and currency included.
// Artwork bytes travel through the dedicated logo endpoints, never this DTO.
func companyDTO(profile store.CompanyProfile) map[string]any {
	return map[string]any{
		"legalName":    profile.LegalName,
		"address":      profile.Address,
		"phone":        profile.Phone,
		"email":        profile.Email,
		"taxId":        profile.TaxID,
		"hasLogo":      profile.HasLogo,
		"currency":     profile.Currency,
		"canonicalUoM": profile.CanonicalUoM,
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
		Currency     string `json:"currency"`
		CanonicalUoM string `json:"canonicalUoM"`
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
		Currency:     request.Currency,
		CanonicalUoM: request.CanonicalUoM,
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
			strings.Contains(err.Error(), "company currency"),
			strings.Contains(err.Error(), "canonical unit"),
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

// companyLabelsDTO renders the effective label set for the settings editor.
//
// Value and Default both travel: the editor shows what prints and what a
// cleared field falls back to, and the frontend never hard-codes the
// document's wording. MaxLength and Terms are the store's own rules, served so
// the client does not keep a second copy of them. The order is the declaration
// order in backend/labels, which is the order the document reads in.
func companyLabelsDTO(effective labels.DocumentLabels) map[string]any {
	declarations := labels.LabelDefaults()
	rows := make([]map[string]any, 0, len(declarations))
	for _, def := range declarations {
		rows = append(rows, map[string]any{
			"key":       string(def.Key),
			"value":     effective.Get(def.Key),
			"default":   def.Default,
			"maxLength": def.MaxLen,
			"terms":     def.Terms,
		})
	}
	return map[string]any{"labels": rows}
}

// getCompanyLabels serves the document's effective wording to every signed-in
// role: the labels print on customer documents, so the preview that renders
// them has to be able to read them. The admin editor reads the same route --
// being an admin decides who may write, not who may read what a document says.
func (s *Server) getCompanyLabels(context echo.Context) error {
	effective, err := s.database.GetCompanyLabels(context.Request().Context())
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "load company labels"})
	}
	return context.JSON(http.StatusOK, companyLabelsDTO(effective))
}

// updateCompanyLabels full-replaces the company's overrides. The store
// validates the whole set -- every key a real label, no key twice, every value
// plain text -- so this handler only crosses the wire and maps a refused save
// to the status the settings form shows.
func (s *Server) updateCompanyLabels(context echo.Context) error {
	var request struct {
		Labels []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		} `json:"labels"`
	}
	if err := context.Bind(&request); err != nil {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	rows := make([]labels.CompanyLabel, 0, len(request.Labels))
	for _, label := range request.Labels {
		rows = append(rows, labels.CompanyLabel{Key: label.Key, Value: label.Value})
	}
	effective, err := s.database.UpdateCompanyLabels(context.Request().Context(), rows)
	if err != nil {
		// Every validation error the store raises for the label set names the
		// label it objected to ("label \"docTitle\" is over 24 characters"),
		// which is what separates a refused save from a broken one. Anything
		// else is the database.
		if strings.Contains(err.Error(), "label ") {
			return context.JSON(http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		}
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "replace company labels"})
	}
	me := sessionUser(context)
	actorID := me.ID
	_ = s.database.Audit(context.Request().Context(), &actorID, "company.labels.update", nil,
		fmt.Sprintf("%d labels", len(rows)))
	return context.JSON(http.StatusOK, companyLabelsDTO(effective))
}

// companyUnitsDTO renders the unit list as the ordered array the wire
// contract promises. SortOrder is deliberately absent: the array order IS
// the order, and a redundant number on the wire could drift from it.
func companyUnitsDTO(units []store.CompanyUnit) map[string]any {
	rows := make([]map[string]any, 0, len(units))
	for _, unit := range units {
		rows = append(rows, map[string]any{"value": unit.Value, "label": unit.Label})
	}
	return map[string]any{"units": rows}
}

// getCompanyUnits serves the company's unit list to every signed-in role:
// units print on customer documents, so hiding them from staff would break
// their previews (the same reasoning the logo endpoints carry).
func (s *Server) getCompanyUnits(context echo.Context) error {
	units, err := s.database.GetCompanyUnits(context.Request().Context())
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "load company units"})
	}
	return context.JSON(http.StatusOK, companyUnitsDTO(units))
}

// updateCompanyUnits full-replaces the company's unit list. The store
// validates shape, uniqueness and the canonical unit's survival; this handler
// only crosses the wire.
func (s *Server) updateCompanyUnits(context echo.Context) error {
	var request struct {
		Units []struct {
			Value string `json:"value"`
			Label string `json:"label"`
		} `json:"units"`
	}
	if err := context.Bind(&request); err != nil {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	units := make([]store.CompanyUnit, 0, len(request.Units))
	for _, unit := range request.Units {
		units = append(units, store.CompanyUnit{Value: unit.Value, Label: unit.Label})
	}
	updated, err := s.database.UpdateCompanyUnits(context.Request().Context(), units)
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "at least one unit"),
			strings.Contains(err.Error(), "value is required"),
			strings.Contains(err.Error(), "is too long"),
			strings.Contains(err.Error(), "not shaped like a unit label"),
			strings.Contains(err.Error(), "label is required"),
			strings.Contains(err.Error(), "duplicates an earlier unit"),
			strings.Contains(err.Error(), "canonical unit"):
			return context.JSON(http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		default:
			return context.JSON(http.StatusInternalServerError, map[string]string{"error": "replace company units"})
		}
	}
	me := sessionUser(context)
	actorID := me.ID
	_ = s.database.Audit(context.Request().Context(), &actorID, "company.units.update", nil, fmt.Sprintf("%d units", len(updated)))
	return context.JSON(http.StatusOK, companyUnitsDTO(updated))
}

// getCompanyLogo serves the effective artwork for previews: the upload when
// present, otherwise the baked-in asset with builtIn set. Every signed-in
// role may read it: the logo prints on customer documents, so hiding it
// from staff would be security theater that breaks their previews.
func (s *Server) getCompanyLogo(context echo.Context) error {
	profile, err := s.database.GetCompanyProfile(context.Request().Context())
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "load company logo"})
	}
	if profile.HasLogo {
		return context.JSON(http.StatusOK, map[string]any{
			"logoBase64": base64.StdEncoding.EncodeToString(profile.LogoBytes),
			"logoMime":   profile.LogoMime,
			"builtIn":    false,
		})
	}
	bakedIn, err := backend.BakedInLogo()
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "load built-in logo"})
	}
	return context.JSON(http.StatusOK, map[string]any{
		"logoBase64": base64.StdEncoding.EncodeToString(bakedIn),
		"logoMime":   "image/png",
		"builtIn":    true,
	})
}
