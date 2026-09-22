package api

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"vera-app/backend/store"
)

// companyDTO renders the editable profile. LogoPath stays internal until
// the object-storage follow-up wires logo bytes.
func companyDTO(legalName, address, phone, email, taxID string) map[string]string {
	return map[string]string{
		"legalName": legalName,
		"address":   address,
		"phone":     phone,
		"email":     email,
		"taxId":     taxID,
	}
}

func (s *Server) getCompanyProfile(context echo.Context) error {
	profile, err := s.database.GetCompanyProfile(context.Request().Context())
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "load company profile"})
	}
	return context.JSON(http.StatusOK, companyDTO(
		profile.LegalName, profile.Address, profile.Phone, profile.Email, profile.TaxID))
}

func (s *Server) updateCompanyProfile(context echo.Context) error {
	var request struct {
		LegalName string `json:"legalName"`
		Address   string `json:"address"`
		Phone     string `json:"phone"`
		Email     string `json:"email"`
		TaxID     string `json:"taxId"`
	}
	if err := context.Bind(&request); err != nil {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	updated, err := s.database.UpdateCompanyProfile(context.Request().Context(), store.CompanyProfile{
		LegalName: request.LegalName,
		Address:   request.Address,
		Phone:     request.Phone,
		Email:     request.Email,
		TaxID:     request.TaxID,
	})
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "required"):
			return context.JSON(http.StatusUnprocessableEntity, map[string]string{"error": "company legal name is required"})
		case strings.Contains(err.Error(), "invalid company email"),
			strings.Contains(err.Error(), "too long"):
			return context.JSON(http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		default:
			return context.JSON(http.StatusInternalServerError, map[string]string{"error": "update company profile"})
		}
	}
	me := sessionUser(context)
	actorID := me.ID
	_ = s.database.Audit(context.Request().Context(), &actorID, "company.update", nil, updated.LegalName)
	return context.JSON(http.StatusOK, companyDTO(
		updated.LegalName, updated.Address, updated.Phone, updated.Email, updated.TaxID))
}
