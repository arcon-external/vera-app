package backend

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"vera-app/backend/currency"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// ManagedUser is the admin-facing user shape. It never contains password
// hashes, session tokens, or invite tokens.
type ManagedUser struct {
	Email              string
	DisplayName        string
	Role               string
	LastLoginAt        string
	ID                 int64
	IsActive           bool
	MustChangePassword bool
}

// ManagedClient is the admin-facing client shape with its primary address
// and archive size.
type ManagedClient struct {
	Name           string
	PrimaryAddress string
	ID             int64
	QuotationCount int64
	Code           int16
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
	return NonNilList(response.Users), nil
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
	Email         string
	DisplayName   string
	ProposedEmail string
	Status        string
	RequestedAt   string
	ID            int64
	UserID        int64
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
	ThemeQDeep   string
	ThemePSoft   string
	Phone        string
	Email        string
	TaxID        string
	ThemePInk    string
	ThemeQSoft   string
	LegalName    string
	Address      string
	ThemeQInk    string
	ThemePDeep   string
	ThemePAccent string
	ThemeQAccent string
	// Currency is the company's ISO 4217 code from the shipped catalogue.
	// It round-trips verbatim like the theme colours; empty reads as IDR.
	Currency string
	// CanonicalUoM is the unit a quotation's package lines are quoted in,
	// replacing the hardcoded "Lot". Empty keeps the stored value.
	CanonicalUoM string
	HasLogo      bool
}

// CompanyUnit is one entry of the company's unit-of-measure list as the
// desktop sees it: the value the editor stores and the label that prints. The
// list is ordered; the wire carries it as an array (a SortOrder number is
// deliberately absent -- the array position IS the order).
type CompanyUnit struct {
	Value string
	Label string
}

// CompanyLabelEntry is one editable label of the quotation document as the
// settings editor sees it.
//
// Value is what the document prints today: the company's override, or the
// built-in default when there is none. Default is the built-in, so the editor
// can show what a cleared field falls back to -- and so the frontend never
// hard-codes the document's wording. MaxLength and Terms are the server's own
// rules, served rather than duplicated in the client: the cap the store
// enforces, and whether the value may carry the four fixed placeholder tokens.
type CompanyLabelEntry struct {
	Key       string
	Value     string
	Default   string
	MaxLength int
	Terms     bool
}

// GetCompanyLabels loads the document's effective wording in editor order.
// The settings editor reads it, and the quotation's identity cache reads the
// same route so previews print the company's own words.
func (a *AdminAPI) GetCompanyLabels() ([]CompanyLabelEntry, error) {
	client, err := api()
	if err != nil {
		return nil, err
	}
	var response struct {
		Labels []CompanyLabelEntry `json:"labels"`
	}
	if err := client.Get("/v1/company/labels", &response); err != nil {
		return nil, err
	}
	if response.Labels == nil {
		response.Labels = []CompanyLabelEntry{}
	}
	return NonNilList(response.Labels), nil
}

// UpdateCompanyLabels full-replaces the company's overrides. An empty value
// clears that label back to the built-in default; the server validates every
// value and refuses the whole set naming the field it objects to.
func (a *AdminAPI) UpdateCompanyLabels(entries []CompanyLabelEntry) ([]CompanyLabelEntry, error) {
	client, err := api()
	if err != nil {
		return nil, err
	}
	if entries == nil {
		entries = []CompanyLabelEntry{}
	}
	var response struct {
		Labels []CompanyLabelEntry `json:"labels"`
	}
	if err := client.Put("/v1/admin/company/labels", map[string]any{
		"labels": entries,
	}, &response); err != nil {
		return nil, err
	}
	if response.Labels == nil {
		response.Labels = []CompanyLabelEntry{}
	}
	return NonNilList(response.Labels), nil
}

// GetPortalPreviewHTML renders the verification portal as it looks right now,
// for the settings editor: one of the five verdicts, with the four portal
// colours typed but not yet saved. It writes nothing, and no revision is
// reachable through it — the document it verifies is fabricated server-side.
func (a *AdminAPI) GetPortalPreviewHTML(verdict, deep, accent, soft, ink string) (string, error) {
	client, err := api()
	if err != nil {
		return "", err
	}
	return client.GetBody(PortalPreviewPath(verdict, deep, accent, soft, ink))
}

// PortalPreviewPath builds the preview request path. Exported so the server's
// accepted-parameter test asserts against the exact query the client builds:
// a name that drifts on either side fails there rather than silently
// returning the default palette.
func PortalPreviewPath(verdict, deep, accent, soft, ink string) string {
	values := url.Values{}
	values.Set("verdict", verdict)
	for _, pair := range [][2]string{{"deep", deep}, {"accent", accent}, {"soft", soft}, {"ink", ink}} {
		values.Set(pair[0], pair[1])
	}
	return "/v1/admin/company/portal-preview?" + values.Encode()
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
		"legalName":    profile.LegalName,
		"address":      profile.Address,
		"phone":        profile.Phone,
		"email":        profile.Email,
		"taxId":        profile.TaxID,
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
	}, &updated); err != nil {
		return CompanyProfile{}, err
	}
	return updated, nil
}

// GetCompanyUnits loads the company's unit list in display order. Every
// signed-in role reads it -- units print on customer documents, so the
// editor, the review panel and the admin list all draw from the same source.
func (a *AdminAPI) GetCompanyUnits() ([]CompanyUnit, error) {
	client, err := api()
	if err != nil {
		return nil, err
	}
	var response struct {
		Units []CompanyUnit `json:"units"`
	}
	if err := client.Get("/v1/company/units", &response); err != nil {
		return nil, err
	}
	if response.Units == nil {
		response.Units = []CompanyUnit{}
	}
	return NonNilList(response.Units), nil
}

// UpdateCompanyUnits full-replaces the company's unit list, in the given
// order. The server refuses a replacement that drops the canonical unit or
// violates the shape rules.
func (a *AdminAPI) UpdateCompanyUnits(units []CompanyUnit) ([]CompanyUnit, error) {
	client, err := api()
	if err != nil {
		return nil, err
	}
	var updated struct {
		Units []CompanyUnit `json:"units"`
	}
	if err := client.Put("/v1/admin/company/units", map[string]any{
		"units": units,
	}, &updated); err != nil {
		return nil, err
	}
	if updated.Units == nil {
		updated.Units = []CompanyUnit{}
	}
	return NonNilList(updated.Units), nil
}

// GetCurrencyCatalogue returns the shipped currency catalogue for the
// settings currency picker. It reads the compiled-in catalogue directly
// instead of proxying veraserver like its neighbours: the catalogue is
// identical on both sides (the server validates against the same package),
// so a network round-trip would only add failure modes to static data. It
// lives on AdminAPI because the Company settings section that owns the
// picker already talks to this binding, and no new bound object is needed.
func (a *AdminAPI) GetCurrencyCatalogue() []currency.CurrencyInfo {
	return currency.Catalogue()
}

// CompanyLogo is the effective artwork with its MIME type for preview
// rendering: the upload when present, otherwise the baked-in asset
// (BuiltIn set) so previews never show a placeholder.
type CompanyLogo struct {
	LogoBase64 string
	LogoMime   string
	BuiltIn    bool
}

// GetCompanyLogo loads the stored artwork for the settings preview.
func (a *AdminAPI) GetCompanyLogo() (CompanyLogo, error) {
	client, err := api()
	if err != nil {
		return CompanyLogo{}, err
	}
	var logo CompanyLogo
	if err := client.Get("/v1/admin/company/logo", &logo); err != nil {
		return CompanyLogo{}, err
	}
	return logo, nil
}

// SetCompanyLogo stores uploaded artwork, validated server-side.
func (a *AdminAPI) SetCompanyLogo(logoBase64, logoMime string) error {
	client, err := api()
	if err != nil {
		return err
	}
	return client.Put("/v1/admin/company/logo", map[string]string{
		"logoBase64": logoBase64,
		"logoMime":   logoMime,
	}, nil)
}

// ClearCompanyLogo removes uploaded artwork, restoring the baked asset.
func (a *AdminAPI) ClearCompanyLogo() error {
	client, err := api()
	if err != nil {
		return err
	}
	return client.Delete("/v1/admin/company/logo", nil)
}

// ListClients returns all clients ordered by code with quote counts.
// Reads go through the shared session endpoint: every signed-in role may
// list, only admins may mutate.
func (a *AdminAPI) ListClients() ([]ManagedClient, error) {
	client, err := api()
	if err != nil {
		return nil, err
	}
	var response struct {
		Clients []ManagedClient `json:"clients"`
	}
	if err := client.Get("/v1/clients", &response); err != nil {
		return nil, err
	}
	if response.Clients == nil {
		response.Clients = []ManagedClient{}
	}
	return NonNilList(response.Clients), nil
}

// CreateClient adds a client. Code 0 allocates the next free code.
func (a *AdminAPI) CreateClient(code int64, name, address string) (ManagedClient, error) {
	client, err := api()
	if err != nil {
		return ManagedClient{}, err
	}
	var created ManagedClient
	if err := client.Post("/v1/admin/clients", map[string]any{
		"code": code, "name": name, "address": address,
	}, &created); err != nil {
		return ManagedClient{}, err
	}
	return created, nil
}

// UpdateClient renames a client and replaces its primary address. The code
// is immutable.
func (a *AdminAPI) UpdateClient(id int64, name, address string) (ManagedClient, error) {
	client, err := api()
	if err != nil {
		return ManagedClient{}, err
	}
	var updated ManagedClient
	if err := client.Patch("/v1/admin/clients/"+itoa(id), map[string]string{
		"name": name, "address": address,
	}, &updated); err != nil {
		return ManagedClient{}, err
	}
	return updated, nil
}

// DeleteClient removes a client with no quotations. Referenced clients are
// refused server-side; the caller surfaces the count first.
func (a *AdminAPI) DeleteClient(id int64) (string, error) {
	client, err := api()
	if err != nil {
		return "", err
	}
	var response struct {
		Name string `json:"name"`
	}
	if err := client.Delete("/v1/admin/clients/"+itoa(id), &response); err != nil {
		return "", err
	}
	return response.Name, nil
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
