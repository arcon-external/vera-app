package backend

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// ManagedUser is the admin-facing user shape. It never contains password
// hashes, session tokens, or invite tokens.
type ManagedUser struct {
	ID                 int64
	Email              string
	DisplayName        string
	Role               string
	IsActive           bool
	MustChangePassword bool
	LastLoginAt        string
}

// AdminAPI exposes user management to the frontend. Every method requires an
// admin session; the server enforces this independently.
type AdminAPI struct {
	ctx context.Context
}

// NewAdminAPI builds the frontend admin binding. The shared API client must
// be configured first via ConfigureAPI.
func NewAdminAPI() *AdminAPI {
	return &AdminAPI{}
}

// SetContext stores the Wails runtime context for native dialogs.
func (a *AdminAPI) SetContext(ctx context.Context) {
	a.ctx = ctx
}

func (a *AdminAPI) ListUsers() ([]ManagedUser, error) {
	client, err := api()
	if err != nil {
		return nil, err
	}
	var response struct {
		Users []ManagedUser `json:"users"`
	}
	if err := client.Get("/v1/admin/users", &response); err != nil {
		return nil, err
	}
	if response.Users == nil {
		response.Users = []ManagedUser{}
	}
	return response.Users, nil
}

func (a *AdminAPI) CreateUser(email, displayName, role string) (ManagedUser, error) {
	client, err := api()
	if err != nil {
		return ManagedUser{}, err
	}
	var user ManagedUser
	if err := client.Post("/v1/admin/users", map[string]string{
		"email":       email,
		"displayName": displayName,
		"role":        role,
	}, &user); err != nil {
		return ManagedUser{}, err
	}
	return user, nil
}

func (a *AdminAPI) SetUserRole(id int64, role string) error {
	client, err := api()
	if err != nil {
		return err
	}
	return client.Patch("/v1/admin/users/"+itoa(id)+"/role", map[string]string{
		"role": role,
	}, nil)
}

func (a *AdminAPI) SetUserActive(id int64, active bool) error {
	client, err := api()
	if err != nil {
		return err
	}
	return client.Patch("/v1/admin/users/"+itoa(id)+"/active", map[string]bool{
		"active": active,
	}, nil)
}

// DeleteUser permanently removes an inactive user. The server refuses
// active targets and self-deletion; the caller confirms explicitly first.
func (a *AdminAPI) DeleteUser(id int64) (string, error) {
	client, err := api()
	if err != nil {
		return "", err
	}
	var response struct {
		Email string `json:"email"`
	}
	if err := client.Delete("/v1/admin/users/"+itoa(id), &response); err != nil {
		return "", err
	}
	return response.Email, nil
}

func (a *AdminAPI) IssuePasswordReset(id int64) (string, error) {
	client, err := api()
	if err != nil {
		return "", err
	}
	var response struct {
		Token string `json:"token"`
	}
	if err := client.Post("/v1/admin/users/"+itoa(id)+"/reset", nil, &response); err != nil {
		return "", err
	}
	return response.Token, nil
}

// EmailChangeResult reports a completed direct email change.
type EmailChangeResult struct {
	Email         string
	PreviousEmail string
}

// SetUserEmail changes a user's login email immediately. Sessions die
// with the flip, so the caller warns the target to sign in again.
func (a *AdminAPI) SetUserEmail(id int64, email string) (EmailChangeResult, error) {
	client, err := api()
	if err != nil {
		return EmailChangeResult{}, err
	}
	var response struct {
		Email         string `json:"email"`
		PreviousEmail string `json:"previousEmail"`
	}
	if err := client.Patch("/v1/admin/users/"+itoa(id)+"/email", map[string]string{
		"email": email,
	}, &response); err != nil {
		return EmailChangeResult{}, err
	}
	return EmailChangeResult{Email: response.Email, PreviousEmail: response.PreviousEmail}, nil
}

// EmailChangeRow is one pending email-change request for the admin queue.
type EmailChangeRow struct {
	ID            int64
	UserID        int64
	Email         string
	DisplayName   string
	ProposedEmail string
	Status        string
	RequestedAt   string
}

// ListEmailChangeRequests returns pending email-change requests.
func (a *AdminAPI) ListEmailChangeRequests() ([]EmailChangeRow, error) {
	client, err := api()
	if err != nil {
		return nil, err
	}
	var response struct {
		Requests []EmailChangeRow `json:"requests"`
	}
	if err := client.Get("/v1/admin/email-change-requests", &response); err != nil {
		return nil, err
	}
	if response.Requests == nil {
		response.Requests = []EmailChangeRow{}
	}
	return response.Requests, nil
}

// DecideEmailChange approves (flip + sign-out) or rejects a request.
func (a *AdminAPI) DecideEmailChange(id int64, approve bool) error {
	client, err := api()
	if err != nil {
		return err
	}
	action := "reject"
	if approve {
		action = "approve"
	}
	return client.Post("/v1/admin/email-change-requests/"+itoa(id)+"/"+action, nil, nil)
}

// CompanyProfile is the editable company identity for the admin-only
// Company section of Settings.
type CompanyProfile struct {
	LegalName string
	Address   string
	Phone     string
	Email     string
	TaxID     string
}

// GetCompanyProfile loads the company profile.
func (a *AdminAPI) GetCompanyProfile() (CompanyProfile, error) {
	client, err := api()
	if err != nil {
		return CompanyProfile{}, err
	}
	var profile CompanyProfile
	if err := client.Get("/v1/admin/company", &profile); err != nil {
		return CompanyProfile{}, err
	}
	return profile, nil
}

// UpdateCompanyProfile full-replaces the editable company fields. Empty
// strings clear a field, except legal name which is always required.
func (a *AdminAPI) UpdateCompanyProfile(profile CompanyProfile) (CompanyProfile, error) {
	client, err := api()
	if err != nil {
		return CompanyProfile{}, err
	}
	var updated CompanyProfile
	if err := client.Put("/v1/admin/company", map[string]string{
		"legalName": profile.LegalName,
		"address":   profile.Address,
		"phone":     profile.Phone,
		"email":     profile.Email,
		"taxId":     profile.TaxID,
	}, &updated); err != nil {
		return CompanyProfile{}, err
	}
	return updated, nil
}

// ListDeletionRequests returns pending account-deletion requests.
func (a *AdminAPI) ListDeletionRequests() ([]DeletionRequest, error) {
	client, err := api()
	if err != nil {
		return nil, err
	}
	var response struct {
		Requests []DeletionRequest `json:"requests"`
	}
	if err := client.Get("/v1/admin/deletion-requests", &response); err != nil {
		return nil, err
	}
	if response.Requests == nil {
		response.Requests = []DeletionRequest{}
	}
	return response.Requests, nil
}

// DecideDeletion approves (soft-delete) or rejects a deletion request.
func (a *AdminAPI) DecideDeletion(id int64, approve bool) error {
	client, err := api()
	if err != nil {
		return err
	}
	action := "reject"
	if approve {
		action = "approve"
	}
	return client.Post("/v1/admin/deletion-requests/"+itoa(id)+"/"+action, nil, nil)
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	negative := n < 0
	if negative {
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if negative {
		return "-" + string(digits)
	}
	return string(digits)
}

// DownloadBackup fetches the newest database dump through a save dialog and
// writes it to the chosen path. It returns the written path for confirmation.
func (a *AdminAPI) DownloadBackup() (string, error) {
	client, err := api()
	if err != nil {
		return "", err
	}
	filename, data, err := client.Download("/v1/admin/backup/latest", "vera-backup.dump")
	if err != nil {
		return "", err
	}
	filePath, err := saveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Save Database Backup",
		DefaultFilename: filename,
		Filters:         []runtime.FileFilter{{DisplayName: "Postgres Dump (*.dump)", Pattern: "*.dump"}},
	})
	if err != nil {
		return "", fmt.Errorf("open save dialog: %w", err)
	}
	if filePath == "" {
		return "", fmt.Errorf("user cancelled")
	}
	if err := os.WriteFile(filePath, data, 0o600); err != nil {
		return "", fmt.Errorf("write backup: %w", err)
	}
	return filePath, nil
}

// openFileDialog opens the native file picker. It is a variable so tests can
// substitute a stub; production uses the Wails runtime dialog.
var openFileDialog = runtime.OpenFileDialog

// RestoreResult reports what a backup restore did.
type RestoreResult struct {
	SafetySnapshot string
	Quotations     int64
	Revisions      int64
}

// RestoreBackup opens a file dialog for a dump, validates the PGDMP magic
// client-side, and uploads it for a server-side restore. The caller must
// have completed the typed-confirm step; the native dialog is the second
// confirmation. Everyone is signed out by a successful restore.
func (a *AdminAPI) RestoreBackup() (RestoreResult, error) {
	client, err := api()
	if err != nil {
		return RestoreResult{}, err
	}
	filePath, err := openFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   "Select Database Backup to Restore",
		Filters: []runtime.FileFilter{{DisplayName: "Postgres Dump (*.dump)", Pattern: "*.dump"}},
	})
	if err != nil {
		return RestoreResult{}, fmt.Errorf("open file dialog: %w", err)
	}
	if filePath == "" {
		return RestoreResult{}, fmt.Errorf("user cancelled")
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return RestoreResult{}, fmt.Errorf("read backup file: %w", err)
	}
	if len(data) < 5 || len(data) > 512<<20 || string(data[:5]) != "PGDMP" {
		return RestoreResult{}, fmt.Errorf("not a Postgres custom-format dump (missing PGDMP header)")
	}
	var response struct {
		SafetySnapshot string `json:"safetySnapshot"`
		Quotations     int64  `json:"quotations"`
		Revisions      int64  `json:"revisions"`
	}
	if err := client.Upload("/v1/admin/backup/restore", "dump", filepath.Base(filePath), data, &response); err != nil {
		return RestoreResult{}, err
	}
	return RestoreResult{
		SafetySnapshot: response.SafetySnapshot,
		Quotations:     response.Quotations,
		Revisions:      response.Revisions,
	}, nil
}
