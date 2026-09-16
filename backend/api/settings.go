package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"vera-app/backend/auth"
	"vera-app/backend/store"
)

// ErrLastAdmin is returned when an operation would remove the final admin.
const ErrLastAdmin = "cannot remove the last active admin"

func sessionUser(context echo.Context) store.AuthUser {
	user, _ := context.Get("user").(store.AuthUser)
	return user
}

func (s *Server) changePassword(context echo.Context) error {
	me := sessionUser(context)
	var request struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := context.Bind(&request); err != nil || len(request.NewPassword) < 10 {
		return context.JSON(http.StatusUnprocessableEntity, map[string]string{"error": "a new password of at least 10 characters is required"})
	}

	stored, err := s.database.GetUserByID(context.Request().Context(), me.ID)
	if err != nil {
		return context.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid or expired session"})
	}
	// First-setup accounts (must-change flag) prove identity with the invite;
	// everyone else must supply the current password.
	if !stored.MustChangePassword {
		if err := auth.VerifyPassword(stored.PasswordHash, request.CurrentPassword); err != nil {
			return context.JSON(http.StatusUnauthorized, map[string]string{"error": "current password is incorrect"})
		}
	}
	hash, err := auth.HashPassword(request.NewPassword)
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "change password"})
	}
	if err := s.database.UpdatePassword(context.Request().Context(), me.ID, hash); err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "change password"})
	}
	if err := s.database.RevokeUserSessions(context.Request().Context(), me.ID); err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "revoke sessions"})
	}
	_ = s.database.Audit(context.Request().Context(), &me.ID, "password.change", &me.ID, "")
	return context.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) logoutEverywhere(context echo.Context) error {
	me := sessionUser(context)
	if err := s.database.RevokeUserSessions(context.Request().Context(), me.ID); err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "sign out everywhere"})
	}
	_ = s.database.Audit(context.Request().Context(), &me.ID, "session.revoke_all", &me.ID, "")
	return context.NoContent(http.StatusNoContent)
}

func (s *Server) requestDeletion(context echo.Context) error {
	me := sessionUser(context)
	var request struct {
		Confirm bool `json:"confirm"`
	}
	if err := context.Bind(&request); err != nil || !request.Confirm {
		return context.JSON(http.StatusUnprocessableEntity, map[string]string{"error": "explicit confirmation is required"})
	}
	record, err := s.database.RequestDeletion(context.Request().Context(), me.ID)
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "request deletion"})
	}
	_ = s.database.Audit(context.Request().Context(), &me.ID, "deletion.request", &me.ID, "")
	return context.JSON(http.StatusAccepted, map[string]any{"id": record.ID, "status": record.Status})
}

// myDeletionRequests returns the signed-in user's own deletion requests,
// newest first, including decided ones so staff can follow outcomes.
func (s *Server) myDeletionRequests(context echo.Context) error {
	me := sessionUser(context)
	requests, err := s.database.ListUserDeletionRequests(context.Request().Context(), me.ID)
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "list deletion requests"})
	}
	result := make([]map[string]any, 0, len(requests))
	for _, request := range requests {
		result = append(result, map[string]any{
			"id":          request.ID,
			"status":      request.Status,
			"requestedAt": request.RequestedAt,
		})
	}
	return context.JSON(http.StatusOK, result)
}

func (s *Server) listDeletionRequests(context echo.Context) error {
	requests, err := s.database.ListDeletionRequests(context.Request().Context())
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "list deletion requests"})
	}
	result := make([]map[string]any, 0, len(requests))
	for _, request := range requests {
		result = append(result, map[string]any{
			"id":          request.ID,
			"userId":      request.UserID,
			"email":       request.Email,
			"displayName": request.DisplayName,
			"status":      request.Status,
			"requestedAt": request.RequestedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	return context.JSON(http.StatusOK, map[string]any{"requests": result})
}

func (s *Server) decideDeletion(approve bool) echo.HandlerFunc {
	return func(context echo.Context) error {
		id, err := strconv.ParseInt(context.Param("id"), 10, 64)
		if err != nil || id < 1 {
			return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request id"})
		}
		me := sessionUser(context)
		record, err := s.database.DecideDeletion(context.Request().Context(), id, me.ID, approve)
		if err != nil {
			if isNotFoundError(err) {
				return context.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
			}
			if isLastAdminError(err) {
				return context.JSON(http.StatusConflict, map[string]string{"error": err.Error()})
			}
			return context.JSON(http.StatusInternalServerError, map[string]string{"error": "decide deletion"})
		}
		action := "deletion.reject"
		if approve {
			action = "deletion.approve"
		}
		_ = s.database.Audit(context.Request().Context(), &me.ID, action, &record.UserID, "")
		return context.JSON(http.StatusOK, map[string]string{"status": record.Status})
	}
}

func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "not found") ||
		strings.Contains(message, "already decided") ||
		strings.Contains(message, "invalid or expired")
}

func isLastAdminError(err error) bool {
	return err != nil && err.Error() == ErrLastAdmin
}
