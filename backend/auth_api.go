package backend

import (
	"context"
	"fmt"
	"time"

	"vera-app/backend/auth"
	"vera-app/backend/store"
)

// AuthUserDTO is the user shape exposed to the frontend. It never contains
// password hashes or session tokens.
type AuthUserDTO struct {
	ID                 int64
	Email              string
	DisplayName        string
	Role               string
	IsActive           bool
	MustChangePassword bool
	LastLoginAt        string
}

// AuthAPI exposes authentication to the frontend. All quotation methods
// require a signed-in user; destructive/admin methods require an admin.
type AuthAPI struct {
	service *auth.Service
}

func NewAuthAPI(service *auth.Service) *AuthAPI {
	return &AuthAPI{service: service}
}

func toAuthUserDTO(user store.AuthUser) AuthUserDTO {
	lastLogin := ""
	if !user.LastLoginAt.IsZero() {
		lastLogin = user.LastLoginAt.Format(time.RFC3339)
	}
	return AuthUserDTO{
		ID:                 user.ID,
		Email:              user.Email,
		DisplayName:        user.DisplayName,
		Role:               user.Role,
		IsActive:           user.IsActive,
		MustChangePassword: user.MustChangePassword,
		LastLoginAt:        lastLogin,
	}
}

// Login signs in with email and password and remembers the session.
func (a *AuthAPI) Login(ctx context.Context, email, password string) (AuthUserDTO, error) {
	user, err := a.service.LoginAndRemember(ctx, email, password)
	if err != nil {
		return AuthUserDTO{}, err
	}
	return toAuthUserDTO(user), nil
}

// Logout signs out the active session.
func (a *AuthAPI) Logout(ctx context.Context) error {
	return a.service.LogoutCurrent(ctx)
}

// RestoreSession validates the saved session, if any.
func (a *AuthAPI) RestoreSession(ctx context.Context) (AuthUserDTO, error) {
	user, err := a.service.Restore(ctx)
	if err != nil {
		return AuthUserDTO{}, err
	}
	return toAuthUserDTO(user), nil
}

// CurrentUser returns the signed-in user.
func (a *AuthAPI) CurrentUser(ctx context.Context) (AuthUserDTO, error) {
	user, err := a.service.RequireUser(ctx)
	if err != nil {
		return AuthUserDTO{}, err
	}
	return toAuthUserDTO(user), nil
}

// EnsureBootstrap creates the first admin from environment configuration when
// no users exist. It is a no-op once any user exists.
func EnsureBootstrap(ctx context.Context, service *auth.Service, email, displayName, password string) error {
	count, err := service.UserCount(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	if email == "" || password == "" {
		return fmt.Errorf("no users exist: set VERA_BOOTSTRAP_ADMIN_EMAIL and VERA_BOOTSTRAP_ADMIN_PASSWORD to create the first admin")
	}
	if _, err := service.Bootstrap(ctx, email, displayName, password); err != nil {
		return err
	}
	return nil
}
