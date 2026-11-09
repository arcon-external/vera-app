package backend

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"vera-app/backend/apiclient"
)

// AuthUserDTO is the user shape exposed to the frontend. It never contains
// password hashes or session tokens.
type AuthUserDTO struct {
	Email              string
	DisplayName        string
	Role               string
	LastLoginAt        string
	AvatarURL          string
	ID                 int64
	IsActive           bool
	MustChangePassword bool
}

// serverUser mirrors the veraserver user payload.
type serverUser struct {
	Email              string `json:"email"`
	DisplayName        string `json:"displayName"`
	Role               string `json:"role"`
	AvatarURL          string `json:"avatarUrl"`
	ID                 int64  `json:"id"`
	MustChangePassword bool   `json:"mustChangePassword"`
}

// serverMe is the /v1/auth/me and /v1/auth/renew payload: the user plus the
// live session's expiry state, so the desktop can offer renewal at T-10
// minutes without a second request.
type serverMe struct {
	serverUser
	ExpiresAt         string `json:"expiresAt"`
	AbsoluteExpiresAt string `json:"absoluteExpiresAt"`
	CanRenew          bool   `json:"canRenew"`
	Remembered        bool   `json:"remembered"`
}

// Session states returned by CheckSession. Only the first four are a real
// session end; the rest are connection or server states and must never sign
// the desktop out.
const (
	SessionStateActive       = "active"
	SessionStateExpired      = "expired"
	SessionStateRevoked      = "revoked"
	SessionStateInactive     = "inactive"
	SessionStateInvalid      = "invalid"
	SessionStateUnreachable  = "unreachable"
	SessionStateServerError  = "server_error"
	SessionStateRateLimited  = "rate_limited"
	SessionStateIncompatible = "incompatible"
)

// SessionStatus is the typed answer to "what is my session state?". It is
// data, not an error: Wails rejects an error as an object the old poll could
// not read, and every poll failure used to sign the user out (issue #531).
type SessionStatus struct {
	State     string
	User      AuthUserDTO
	Code      string
	Message   string
	ExpiresAt string
	CanRenew  bool
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

// Login signs in with email and password.
//
// remember asks the server for a 30-day session instead of the 8-hour one
// and persists the token in the OS keychain, so the session survives a
// restart. Without it the token lives in this process only — and any token
// an earlier remembered sign-in left in the keychain is deleted right here,
// or the next launch restores that stale 30-day session through
// SessionToken's keychain fallback: "keep me signed in" unchecked would
// still leave the machine signed in, on a token this sign-in never issued.
func (a *AuthAPI) Login(email, password string, remember bool) (AuthUserDTO, error) {
	client, err := api()
	if err != nil {
		return AuthUserDTO{}, err
	}
	var response struct {
		Token string     `json:"token"`
		User  serverUser `json:"user"`
	}
	if err := client.Post("/v1/auth/login", map[string]any{
		"email":    email,
		"password": password,
		"remember": remember,
	}, &response); err != nil {
		return AuthUserDTO{}, err
	}
	if response.Token == "" {
		return AuthUserDTO{}, fmt.Errorf("sign in failed")
	}
	if remember {
		if err := SetSessionToken(response.Token); err != nil {
			return AuthUserDTO{}, err
		}
		return toAuthUserDTO(response.User), nil
	}
	setSessionTokenMemory(response.Token)
	if err := forgetKeyringToken(); err != nil {
		return AuthUserDTO{}, err
	}
	return toAuthUserDTO(response.User), nil
}

// RedeemInvite activates an account with an invite or password-reset code,
// setting its password. It returns the activated email address, or "" when
// talking to an older server that only reports status (generic success).
// It signs no one in; the user signs in afterwards. Valid codes are
// single-use and expire after an hour.
func (a *AuthAPI) RedeemInvite(token, newPassword string) (string, error) {
	client, err := api()
	if err != nil {
		return "", err
	}
	var response struct {
		Status string `json:"status"`
		Email  string `json:"email"`
	}
	if err := client.Post("/v1/auth/redeem", map[string]string{
		"token":       token,
		"newPassword": newPassword,
	}, &response); err != nil {
		return "", err
	}
	if response.Status != "ok" {
		return "", fmt.Errorf("invite code was not accepted")
	}
	return response.Email, nil
}

// Logout signs out the active session.
func (a *AuthAPI) Logout() error {
	if client, err := api(); err == nil {
		_ = client.Post("/v1/auth/logout", nil, nil)
	}
	return ClearSessionToken()
}

// RestoreSession validates the saved session, if any. It goes through
// CheckSession so a dead remembered token is cleared at startup, while a
// transient failure keeps it for the next launch.
func (a *AuthAPI) RestoreSession() (AuthUserDTO, error) {
	if SessionToken() == "" {
		return AuthUserDTO{}, fmt.Errorf("no saved session")
	}
	status := a.CheckSession()
	if status.State == SessionStateActive {
		return status.User, nil
	}
	if status.Message != "" {
		return AuthUserDTO{}, fmt.Errorf("%s", status.Message)
	}
	return AuthUserDTO{}, fmt.Errorf("no saved session")
}

// CheckSession asks the server what the current session state is and returns
// typed data for every expected outcome. It never returns an error, because a
// Wails rejection is exactly the shape the old poll mishandled: transport,
// 5xx and 429 outcomes are connection states, not a session end.
//
// Only a real session end clears the token; that is the "session-end path"
// the keychain lifecycle names. A manual Logout clears it too. No other call
// may.
func (a *AuthAPI) CheckSession() SessionStatus {
	if SessionToken() == "" {
		return SessionStatus{
			State:   SessionStateInvalid,
			Code:    "session_missing",
			Message: "no saved session",
		}
	}
	client, err := api()
	if err != nil {
		return SessionStatus{
			State:   SessionStateIncompatible,
			Code:    "session_check_unavailable",
			Message: err.Error(),
		}
	}

	var me serverMe
	if err := client.Get("/v1/auth/me", &me); err != nil {
		return classifySessionError(err)
	}
	if me.ID == 0 || strings.TrimSpace(me.Email) == "" {
		return SessionStatus{
			State:   SessionStateIncompatible,
			Code:    "session_incompatible",
			Message: "the server's session response was not understood",
		}
	}
	return SessionStatus{
		State:     SessionStateActive,
		User:      toAuthUserDTO(me.serverUser),
		Code:      "session_active",
		ExpiresAt: me.ExpiresAt,
		CanRenew:  me.CanRenew,
	}
}

// RenewSession asks the server to slide the current session's idle deadline.
//
// Like CheckSession it returns typed data for every expected outcome, never an
// error: the "Stay signed in" button must distinguish "the session really
// ended" from "the laptop is offline" without parsing prose. A real end is
// the only outcome that clears the token (issue #531's rule).
func (a *AuthAPI) RenewSession() SessionStatus {
	if SessionToken() == "" {
		return SessionStatus{
			State:   SessionStateInvalid,
			Code:    "session_missing",
			Message: "no saved session",
		}
	}
	client, err := api()
	if err != nil {
		return SessionStatus{
			State:   SessionStateIncompatible,
			Code:    "session_check_unavailable",
			Message: err.Error(),
		}
	}

	var me serverMe
	if err := client.Post("/v1/auth/renew", nil, &me); err != nil {
		return classifySessionError(err)
	}
	if me.ID == 0 || strings.TrimSpace(me.Email) == "" {
		return SessionStatus{
			State:   SessionStateIncompatible,
			Code:    "session_incompatible",
			Message: "the server's session response was not understood",
		}
	}
	return SessionStatus{
		State:     SessionStateActive,
		User:      toAuthUserDTO(me.serverUser),
		Code:      "session_active",
		ExpiresAt: me.ExpiresAt,
		CanRenew:  me.CanRenew,
	}
}

// classifySessionError turns a failed session request into a typed status. A
// 401 is a real session end (and is the only branch that clears the token);
// everything else is a connection or server state.
func classifySessionError(err error) SessionStatus {
	status := apiclient.StatusOf(err)
	code := apiclient.CodeOf(err)

	if status == http.StatusUnauthorized {
		state := SessionStateInvalid
		switch code {
		case "session_expired":
			state = SessionStateExpired
		case "session_revoked":
			state = SessionStateRevoked
		case "session_inactive":
			state = SessionStateInactive
		}
		if code == "" {
			code = "session_invalid"
		}
		// This is the session-end path: the token is dead, so the keychain
		// must not keep offering it on the next launch.
		_ = ClearSessionToken()
		return SessionStatus{State: state, Code: code, Message: err.Error()}
	}

	switch {
	case status == http.StatusTooManyRequests:
		return SessionStatus{State: SessionStateRateLimited, Code: "rate_limited", Message: err.Error()}
	case status >= http.StatusInternalServerError:
		return SessionStatus{State: SessionStateServerError, Code: "session_check_failed", Message: err.Error()}
	case status == http.StatusNotFound:
		return SessionStatus{State: SessionStateIncompatible, Code: "session_incompatible", Message: err.Error()}
	case status == 0 && apiclient.IsTransport(err):
		return SessionStatus{State: SessionStateUnreachable, Code: "session_unreachable", Message: err.Error()}
	case status == 0:
		return SessionStatus{State: SessionStateIncompatible, Code: "session_incompatible", Message: err.Error()}
	default:
		return SessionStatus{State: SessionStateServerError, Code: "session_check_failed", Message: err.Error()}
	}
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
	Email       string
	DisplayName string
	Status      string
	RequestedAt string
	ID          int64
	UserID      int64
}

// RequestDeletion opens a deletion request for the signed-in user. The
// confirmed flag is the explicit second confirmation.
func (a *AuthAPI) RequestDeletion(confirmed bool) (DeletionRequest, error) {
	client, err := api()
	if err != nil {
		return DeletionRequest{}, err
	}
	var response struct {
		Status string `json:"status"`
		ID     int64  `json:"id"`
	}
	if err := client.Post("/v1/auth/deletion-request", map[string]bool{
		"confirm": confirmed,
	}, &response); err != nil {
		return DeletionRequest{}, err
	}
	return DeletionRequest{ID: response.ID, Status: response.Status}, nil
}

// OwnDeletionRequest is a signed-in user's own deletion request status.
type OwnDeletionRequest struct {
	Status string
	ID     int64
}

// MyDeletionRequests lists the signed-in user's own deletion requests,
// newest first, for the notifications bell and the settings view.
func (a *AuthAPI) MyDeletionRequests() ([]OwnDeletionRequest, error) {
	client, err := api()
	if err != nil {
		return nil, err
	}
	var response []struct {
		Status string `json:"status"`
		ID     int64  `json:"id"`
	}
	if err := client.Get("/v1/auth/deletion-requests/mine", &response); err != nil {
		return nil, err
	}
	requests := make([]OwnDeletionRequest, 0, len(response))
	for _, item := range response {
		requests = append(requests, OwnDeletionRequest{ID: item.ID, Status: item.Status})
	}
	return requests, nil
}

// EmailChangeRequest is a staff email-change request with its outcome.
type EmailChangeRequest struct {
	ProposedEmail string
	Status        string
	RequestedAt   string
	ID            int64
}

// RequestEmailChange files an email-change request carrying a proposed
// address. Approval flips the login email and signs the account out
// everywhere; nothing changes until an admin approves.
func (a *AuthAPI) RequestEmailChange(proposedEmail string) (EmailChangeRequest, error) {
	client, err := api()
	if err != nil {
		return EmailChangeRequest{}, err
	}
	var response struct {
		ProposedEmail string `json:"proposedEmail"`
		Status        string `json:"status"`
		ID            int64  `json:"id"`
	}
	if err := client.Post("/v1/auth/email-change-request", map[string]string{
		"proposedEmail": proposedEmail,
	}, &response); err != nil {
		return EmailChangeRequest{}, err
	}
	return EmailChangeRequest{ID: response.ID, ProposedEmail: response.ProposedEmail, Status: response.Status}, nil
}

// MyEmailChangeRequests lists the signed-in user's own requests, newest
// first, for the settings view.
func (a *AuthAPI) MyEmailChangeRequests() ([]EmailChangeRequest, error) {
	client, err := api()
	if err != nil {
		return nil, err
	}
	var response struct {
		Requests []EmailChangeRequest `json:"requests"`
	}
	if err := client.Get("/v1/auth/email-change-requests/mine", &response); err != nil {
		return nil, err
	}
	if response.Requests == nil {
		response.Requests = []EmailChangeRequest{}
	}
	return response.Requests, nil
}

// GetCompany loads the company identity for UI surfaces (shell header).
// Every signed-in role may read; writes stay admin-only.
func (a *AuthAPI) GetCompany() (CompanyProfile, error) {
	client, err := api()
	if err != nil {
		return CompanyProfile{}, err
	}
	var profile CompanyProfile
	if err := client.Get("/v1/company", &profile); err != nil {
		return CompanyProfile{}, err
	}
	return profile, nil
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
