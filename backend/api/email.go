package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
)

// setUserEmail changes a user's login email immediately. The flip revokes
// every session including the current one, so the caller warns the target
// to sign in again.
func (s *Server) setUserEmail(context echo.Context) error {
	id, ok := parseUserID(context)
	if !ok {
		return nil
	}
	var request struct {
		Email string `json:"email"`
	}
	if err := context.Bind(&request); err != nil || strings.TrimSpace(request.Email) == "" {
		return context.JSON(http.StatusUnprocessableEntity, map[string]string{"error": "a new email address is required"})
	}
	if !validLoginEmail(strings.TrimSpace(request.Email)) {
		return context.JSON(http.StatusUnprocessableEntity, map[string]string{"error": "invalid email address"})
	}
	oldEmail, newEmail, err := s.database.SetUserEmail(context.Request().Context(), id, request.Email)
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "not found"):
			return context.JSON(http.StatusNotFound, map[string]string{"error": "user not found"})
		case strings.Contains(err.Error(), "already in use"):
			return context.JSON(http.StatusConflict, map[string]string{"error": "email already in use"})
		case strings.Contains(err.Error(), "invalid email"):
			return context.JSON(http.StatusUnprocessableEntity, map[string]string{"error": "invalid email address"})
		default:
			return context.JSON(http.StatusInternalServerError, map[string]string{"error": "change email"})
		}
	}
	me := sessionUser(context)
	actorID := me.ID
	_ = s.database.Audit(context.Request().Context(), &actorID, "user.email.change", &id, oldEmail+" -> "+newEmail)
	return context.JSON(http.StatusOK, map[string]string{"email": newEmail, "previousEmail": oldEmail})
}

// requestEmailChange files a staff email-change request carrying a proposed
// address. An admin approves or rejects it; nothing flips automatically.
func (s *Server) requestEmailChange(context echo.Context) error {
	me := sessionUser(context)
	var request struct {
		ProposedEmail string `json:"proposedEmail"`
	}
	if err := context.Bind(&request); err != nil || strings.TrimSpace(request.ProposedEmail) == "" {
		return context.JSON(http.StatusUnprocessableEntity, map[string]string{"error": "a proposed email address is required"})
	}
	if !validLoginEmail(strings.TrimSpace(request.ProposedEmail)) {
		return context.JSON(http.StatusUnprocessableEntity, map[string]string{"error": "invalid email address"})
	}
	record, err := s.database.RequestEmailChange(context.Request().Context(), me.ID, request.ProposedEmail)
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "already in use"):
			return context.JSON(http.StatusConflict, map[string]string{"error": "email already in use"})
		case strings.Contains(err.Error(), "already using"):
			return context.JSON(http.StatusUnprocessableEntity, map[string]string{"error": "that is already your email address"})
		case strings.Contains(err.Error(), "invalid email"):
			return context.JSON(http.StatusUnprocessableEntity, map[string]string{"error": "invalid email address"})
		default:
			return context.JSON(http.StatusInternalServerError, map[string]string{"error": "request email change"})
		}
	}
	_ = s.database.Audit(context.Request().Context(), &me.ID, "email.change.request", &me.ID, record.ProposedEmail)
	return context.JSON(http.StatusAccepted, map[string]any{"id": record.ID, "status": record.Status})
}

// myEmailChangeRequests returns the signed-in user's own requests, newest
// first, including decided ones so staff can follow outcomes.
func (s *Server) myEmailChangeRequests(context echo.Context) error {
	me := sessionUser(context)
	requests, err := s.database.ListUserEmailChangeRequests(context.Request().Context(), me.ID)
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "list email change requests"})
	}
	result := make([]map[string]any, 0, len(requests))
	for _, request := range requests {
		result = append(result, map[string]any{
			"id":            request.ID,
			"proposedEmail": request.ProposedEmail,
			"status":        request.Status,
			"requestedAt":   request.RequestedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	return context.JSON(http.StatusOK, map[string]any{"requests": result})
}

func (s *Server) listEmailChangeRequests(context echo.Context) error {
	requests, err := s.database.ListEmailChangeRequests(context.Request().Context())
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "list email change requests"})
	}
	result := make([]map[string]any, 0, len(requests))
	for _, request := range requests {
		result = append(result, map[string]any{
			"id":            request.ID,
			"userId":        request.UserID,
			"email":         request.Email,
			"displayName":   request.DisplayName,
			"proposedEmail": request.ProposedEmail,
			"status":        request.Status,
			"requestedAt":   request.RequestedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	return context.JSON(http.StatusOK, map[string]any{"requests": result})
}

func (s *Server) decideEmailChange(approve bool) echo.HandlerFunc {
	return func(context echo.Context) error {
		id, err := strconv.ParseInt(context.Param("id"), 10, 64)
		if err != nil || id < 1 {
			return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request id"})
		}
		me := sessionUser(context)
		record, err := s.database.DecideEmailChange(context.Request().Context(), id, me.ID, approve)
		if err != nil {
			switch {
			case isNotFoundError(err):
				return context.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
			case strings.Contains(err.Error(), "already in use"):
				return context.JSON(http.StatusConflict, map[string]string{"error": "email already in use"})
			default:
				return context.JSON(http.StatusInternalServerError, map[string]string{"error": "decide email change"})
			}
		}
		action := "email.change.reject"
		if approve {
			action = "email.change.approve"
		}
		_ = s.database.Audit(context.Request().Context(), &me.ID, action, &record.UserID, record.ProposedEmail)
		return context.JSON(http.StatusOK, map[string]string{"status": record.Status})
	}
}
