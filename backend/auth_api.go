package backend

import (
	"context"
	"fmt"
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

// serverUser mirrors the veraserver user payload.
type serverUser struct {
	ID                 int64  `json:"id"`
	Email              string `json:"email"`
	DisplayName        string `json:"displayName"`
	Role               string `json:"role"`
	MustChangePassword bool   `json:"mustChangePassword"`
}

// AuthAPI exposes authentication to the frontend. It talks to veraserver;
// session tokens live in memory mirrored to the OS keychain.
type AuthAPI struct{}

// NewAuthAPI builds the frontend auth binding. The shared API client must be
// configured first via ConfigureAPI.
func NewAuthAPI() *AuthAPI {
	return &AuthAPI{}
}

func toAuthUserDTO(user serverUser) AuthUserDTO {
	return AuthUserDTO{
		ID:                 user.ID,
		Email:              user.Email,
		DisplayName:        user.DisplayName,
		Role:               user.Role,
		IsActive:           true,
		MustChangePassword: user.MustChangePassword,
	}
}

// Login signs in with email and password and remembers the session.
func (a *AuthAPI) Login(email, password string) (AuthUserDTO, error) {
	client, err := api()
	if err != nil {
		return AuthUserDTO{}, err
	}
	var response struct {
		Token string     `json:"token"`
		User  serverUser `json:"user"`
	}
	if err := client.Post("/v1/auth/login", map[string]string{
		"email":    email,
		"password": password,
	}, &response); err != nil {
		return AuthUserDTO{}, err
	}
	if response.Token == "" {
		return AuthUserDTO{}, fmt.Errorf("sign in failed")
	}
	if err := SetSessionToken(response.Token); err != nil {
		return AuthUserDTO{}, err
	}
	return toAuthUserDTO(response.User), nil
}

// Logout signs out the active session.
func (a *AuthAPI) Logout() error {
	if client, err := api(); err == nil {
		_ = client.Post("/v1/auth/logout", nil, nil)
	}
	return ClearSessionToken()
}

// RestoreSession validates the saved session, if any.
func (a *AuthAPI) RestoreSession() (AuthUserDTO, error) {
	if SessionToken() == "" {
		return AuthUserDTO{}, fmt.Errorf("no saved session")
	}
	return a.CurrentUser()
}

// CurrentUser returns the signed-in user.
func (a *AuthAPI) CurrentUser() (AuthUserDTO, error) {
	client, err := api()
	if err != nil {
		return AuthUserDTO{}, err
	}
	var user serverUser
	if err := client.Get("/v1/auth/me", &user); err != nil {
		return AuthUserDTO{}, err
	}
	return toAuthUserDTO(user), nil
}

// EnsureBootstrap is a no-op on the desktop: the first admin is created by
// the server at startup from its own environment. It remains so existing
// callers keep compiling.
func EnsureBootstrap(ctx context.Context, email, displayName, password string) error {
	return nil
}
