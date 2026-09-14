package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"vera-app/backend/auth"
	"vera-app/backend/store"
)

const inviteTTL = time.Hour

func userDTO(id int64, email, displayName, role string, active, mustChange bool, lastLogin string) map[string]any {
	return map[string]any{
		"id":                 id,
		"email":              email,
		"displayName":        displayName,
		"role":               role,
		"isActive":           active,
		"mustChangePassword": mustChange,
		"lastLoginAt":        lastLogin,
	}
}

func parseUserID(context echo.Context) (int64, bool) {
	id, err := strconv.ParseInt(context.Param("id"), 10, 64)
	if err != nil || id < 1 {
		_ = context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid user id"})
		return 0, false
	}
	return id, true
}

// lastAdminGuard refuses an operation that would leave zero active admins.
// It returns false (having already written the response) when the target is
// currently an active admin and it is the last one.
func (s *Server) lastAdminGuard(ctx echo.Context, userID int64) bool {
	target, err := s.database.GetUserByID(ctx.Request().Context(), userID)
	if err != nil {
		_ = ctx.JSON(http.StatusNotFound, map[string]string{"error": "user not found"})
		return false
	}
	if !target.IsActive || target.Role != "admin" {
		return true
	}
	count, err := s.database.CountActiveAdmins(ctx.Request().Context())
	if err != nil {
		_ = ctx.JSON(http.StatusInternalServerError, map[string]string{"error": "check admin count"})
		return false
	}
	if count <= 1 {
		_ = ctx.JSON(http.StatusConflict, map[string]string{"error": "cannot remove the last active admin"})
		return false
	}
	return true
}

func (s *Server) listUsers(context echo.Context) error {
	users, err := s.database.ListUsers(context.Request().Context())
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "list users"})
	}
	result := make([]map[string]any, 0, len(users))
	for _, user := range users {
		lastLogin := ""
		if !user.LastLoginAt.IsZero() {
			lastLogin = user.LastLoginAt.Format("2006-01-02T15:04:05Z07:00")
		}
		result = append(result, userDTO(user.ID, user.Email, user.DisplayName, user.Role, user.IsActive, user.MustChangePassword, lastLogin))
	}
	return context.JSON(http.StatusOK, map[string]any{"users": result})
}

func (s *Server) createUser(context echo.Context) error {
	var request struct {
		Email       string `json:"email"`
		DisplayName string `json:"displayName"`
		Role        string `json:"role"`
	}
	if err := context.Bind(&request); err != nil {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	request.Email = strings.TrimSpace(request.Email)
	request.DisplayName = strings.TrimSpace(request.DisplayName)
	if request.Email == "" || request.DisplayName == "" {
		return context.JSON(http.StatusUnprocessableEntity, map[string]string{"error": "email and display name are required"})
	}
	if request.Role != "staff" && request.Role != "admin" {
		return context.JSON(http.StatusUnprocessableEntity, map[string]string{"error": "role must be staff or admin"})
	}

	// The account starts with a random unusable password; the invite flow
	// sets the real one and clears the must-change flag.
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "create user"})
	}
	hash, err := auth.HashPassword(hex.EncodeToString(raw))
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "create user"})
	}
	user, err := s.database.CreateUser(context.Request().Context(), request.Email, request.DisplayName, hash, request.Role)
	if err != nil {
		return context.JSON(http.StatusConflict, map[string]string{"error": "email already in use"})
	}
	if err := s.database.SetMustChangePassword(context.Request().Context(), user.ID, true); err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "create user"})
	}
	return context.JSON(http.StatusCreated, userDTO(user.ID, user.Email, user.DisplayName, user.Role, user.IsActive, true, ""))
}

func (s *Server) setUserRole(context echo.Context) error {
	id, ok := parseUserID(context)
	if !ok {
		return nil
	}
	var request struct {
		Role string `json:"role"`
	}
	if err := context.Bind(&request); err != nil || (request.Role != "staff" && request.Role != "admin") {
		return context.JSON(http.StatusUnprocessableEntity, map[string]string{"error": "role must be staff or admin"})
	}
	if !s.lastAdminGuard(context, id) {
		return nil
	}
	if err := s.database.SetUserRole(context.Request().Context(), id, request.Role); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return context.JSON(http.StatusNotFound, map[string]string{"error": "user not found"})
		}
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "update role"})
	}
	return context.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) deleteUser(context echo.Context) error {
	id, ok := parseUserID(context)
	if !ok {
		return nil
	}
	me, _ := context.Get("user").(store.AuthUser)
	if id == me.ID {
		return context.JSON(http.StatusUnprocessableEntity, map[string]string{"error": "you cannot permanently delete your own account"})
	}
	email, err := s.database.DeleteUser(context.Request().Context(), id)
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "not found"):
			return context.JSON(http.StatusNotFound, map[string]string{"error": "user not found"})
		case strings.Contains(err.Error(), "still active"):
			return context.JSON(http.StatusUnprocessableEntity, map[string]string{"error": "deactivate the account first; permanent deletion is for inactive users only"})
		default:
			return context.JSON(http.StatusInternalServerError, map[string]string{"error": "delete user"})
		}
	}
	actorID := me.ID
	_ = s.database.Audit(context.Request().Context(), &actorID, "user.delete", nil, "permanently deleted "+email)
	return context.JSON(http.StatusOK, map[string]string{"email": email})
}

func (s *Server) setUserActive(context echo.Context) error {
	id, ok := parseUserID(context)
	if !ok {
		return nil
	}
	var request struct {
		Active *bool `json:"active"`
	}
	if err := context.Bind(&request); err != nil || request.Active == nil {
		return context.JSON(http.StatusUnprocessableEntity, map[string]string{"error": "active must be true or false"})
	}
	if !*request.Active && !s.lastAdminGuard(context, id) {
		return nil
	}
	if err := s.database.SetUserActive(context.Request().Context(), id, *request.Active); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return context.JSON(http.StatusNotFound, map[string]string{"error": "user not found"})
		}
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "update user"})
	}
	if !*request.Active {
		if err := s.database.RevokeUserSessions(context.Request().Context(), id); err != nil {
			return context.JSON(http.StatusInternalServerError, map[string]string{"error": "revoke sessions"})
		}
	}
	return context.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) resetUser(context echo.Context) error {
	id, ok := parseUserID(context)
	if !ok {
		return nil
	}
	token, err := s.database.IssueInviteToken(context.Request().Context(), id, time.Now().Add(inviteTTL))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return context.JSON(http.StatusNotFound, map[string]string{"error": "user not found"})
		}
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "issue invite token"})
	}
	return context.JSON(http.StatusOK, map[string]string{"token": token})
}

func (s *Server) redeemInvite(context echo.Context) error {
	var request struct {
		Token       string `json:"token"`
		NewPassword string `json:"newPassword"`
	}
	if err := context.Bind(&request); err != nil || request.Token == "" || len(request.NewPassword) < 10 {
		return context.JSON(http.StatusUnprocessableEntity, map[string]string{"error": "valid token and a password of at least 10 characters are required"})
	}
	hash, err := auth.HashPassword(request.NewPassword)
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "redeem invite"})
	}
	if _, err := s.database.RedeemInviteToken(context.Request().Context(), request.Token, hash, time.Now()); err != nil {
		return context.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid or expired invite token"})
	}
	return context.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
