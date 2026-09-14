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
	AvatarURL          string
}

// serverUser mirrors the veraserver user payload.
type serverUser struct {
	ID                 int64  `json:"id"`
	Email              string `json:"email"`
	DisplayName        string `json:"displayName"`
	Role               string `json:"role"`
	MustChangePassword bool   `json:"mustChangePassword"`
	AvatarURL          string `json:"avatarUrl"`
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
		AvatarURL:          user.AvatarURL,
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

// RedeemInvite activates an account with an invite or password-reset code,
// setting its password. It signs no one in; the user signs in afterwards.
// Valid codes are single-use and expire after an hour.
func (a *AuthAPI) RedeemInvite(token, newPassword string) error {
	client, err := api()
	if err != nil {
		return err
	}
	var response struct {
		Status string `json:"status"`
	}
	if err := client.Post("/v1/auth/redeem", map[string]string{
		"token":       token,
		"newPassword": newPassword,
	}, &response); err != nil {
		return err
	}
	if response.Status != "ok" {
		return fmt.Errorf("invite code was not accepted")
	}
	return nil
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

// ChangePassword sets a new password. The current password is required
// unless the account is flagged for first setup.
func (a *AuthAPI) ChangePassword(currentPassword, newPassword string) error {
	client, err := api()
	if err != nil {
		return err
	}
	return client.Patch("/v1/auth/password", map[string]string{
		"currentPassword": currentPassword,
		"newPassword":     newPassword,
	}, nil)
}

// LogoutEverywhere revokes all sessions for the signed-in user, including
// the current one.
func (a *AuthAPI) LogoutEverywhere() error {
	client, err := api()
	if err != nil {
		return err
	}
	if err := client.Post("/v1/auth/logout-everywhere", nil, nil); err != nil {
		return err
	}
	return ClearSessionToken()
}

// DeletionRequest is a pending account-deletion request.
type DeletionRequest struct {
	ID          int64
	UserID      int64
	Email       string
	DisplayName string
	Status      string
	RequestedAt string
}

// RequestDeletion opens a deletion request for the signed-in user. The
// confirmed flag is the explicit second confirmation.
func (a *AuthAPI) RequestDeletion(confirmed bool) (DeletionRequest, error) {
	client, err := api()
	if err != nil {
		return DeletionRequest{}, err
	}
	var response struct {
		ID     int64  `json:"id"`
		Status string `json:"status"`
	}
	if err := client.Post("/v1/auth/deletion-request", map[string]bool{
		"confirm": confirmed,
	}, &response); err != nil {
		return DeletionRequest{}, err
	}
	return DeletionRequest{ID: response.ID, Status: response.Status}, nil
}

// UpdateProfile changes the signed-in user's display name and/or avatar.
// Empty display name leaves the name unchanged; removeAvatar clears the
// picture; avatarBase64/avatarMime upload a replacement.
func (a *AuthAPI) UpdateProfile(displayName, avatarBase64, avatarMime string, removeAvatar bool) (AuthUserDTO, error) {
	client, err := api()
	if err != nil {
		return AuthUserDTO{}, err
	}
	var response struct {
		DisplayName string `json:"displayName"`
		AvatarURL   string `json:"avatarUrl"`
	}
	if err := client.Patch("/v1/auth/profile", map[string]any{
		"displayName":  displayName,
		"avatarBase64": avatarBase64,
		"avatarMime":   avatarMime,
		"removeAvatar": removeAvatar,
	}, &response); err != nil {
		return AuthUserDTO{}, err
	}
	return a.CurrentUser()
}

// EnsureBootstrap is a no-op on the desktop: the first admin is created by
// the server at startup from its own environment. It remains so existing
// callers keep compiling.
func EnsureBootstrap(ctx context.Context, email, displayName, password string) error {
	return nil
}
