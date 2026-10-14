package api

import (
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	echoMiddleware "github.com/labstack/echo/v4/middleware"

	"vera-app/backend"
	"vera-app/backend/auth"
	"vera-app/backend/store"
)

// Server exposes the Vera HTTP API. Authentication reuses backend/auth
// sessions: login issues a bearer token, and protected routes validate it
// (expiry + revocation + active user) on every request.
type Server struct {
	backups       backupSource
	sink          backupSink
	echo          *echo.Echo
	database      *store.Store
	service       *auth.Service
	templates     *backend.Quotation
	signer        *backend.Signer
	verifyLimiter *ipRateLimiter
	// previewLimiter bounds the admin portal preview separately from
	// verifyLimiter. The preview is authenticated and admin-only, but it is
	// rendered per keystroke-commit rather than per customer scan, so a shared
	// bucket would let one admin's rebrand lock out every reader behind the
	// same office address.
	previewLimiter *ipRateLimiter
	// loginLimiter bounds sign-in attempts per address.
	//
	// It exists because the limiter in backend/auth keys on the email in the
	// request body -- a value the caller chooses. That protects a specific
	// account, but it cannot bound the endpoint: a flood of distinct addresses
	// passes straight through it, and each one costs a database round trip.
	loginLimiter *ipRateLimiter
	backupDir    string
	dsn          string
}

// SetSigner attaches the revision signer. Without it, exports store unsigned
// revisions and the portal reports verification unavailable — production
// wires it from VERA_SIGNING_KEY at boot.
func (s *Server) SetSigner(signer *backend.Signer) {
	s.signer = signer
}

// SetBackupDir overrides the backup directory (tests and operators).
func (s *Server) SetBackupDir(dir string) {
	s.backupDir = dir
}

// SetDSN configures the connection string used by backup subprocesses
// (pg_dump/pg_restore). It is separate from the pool so tests can point it
// at a disposable database.
func (s *Server) SetDSN(dsn string) {
	s.dsn = dsn
}

func (s *Server) dsnOrEmpty() string {
	if s.dsn != "" {
		return s.dsn
	}
	return s.database.DSN()
}

func (s *Server) backupDirOrDefault() string {
	if s.backupDir != "" {
		return s.backupDir
	}
	if dir := os.Getenv("VERA_BACKUP_DIR"); dir != "" {
		return dir
	}
	return "/home/tristan/backups/vera"
}

// NewServer builds the router. Templates carries preloaded PDF template
// assets used to render customer HTML snapshots; it may be nil only in
// tests that never touch snapshot rendering.
func NewServer(database *store.Store, service *auth.Service, templates *backend.Quotation) *Server {
	server := &Server{echo: echo.New(), database: database, service: service, templates: templates,
		verifyLimiter:  newIPRateLimiter(30, time.Minute),
		previewLimiter: newIPRateLimiter(previewsPerMinute, time.Minute),
		loginLimiter:   newIPRateLimiter(loginAttemptsPerMinute, time.Minute)}
	server.echo.HideBanner = true
	server.echo.HidePort = true
	server.echo.Renderer = &verifyRenderer{}
	server.echo.GET("/healthz", server.healthz)
	server.echo.HEAD("/healthz", server.healthz)
	// /health mirrors /healthz: some edges (Cloud Run's) swallow the exact
	// /healthz path, so monitors and deploys probe /health instead.
	server.echo.GET("/health", server.healthz)
	server.echo.HEAD("/health", server.healthz)
	// Public release state for the desktop updater. Unauthenticated like
	// /healthz: update checks run pre-login and must never alias a 401 into
	// a sign-out.
	server.echo.GET("/v1/app/latest", server.getAppLatest)
	server.echo.GET("/v1/app/changelog", server.getAppChangelog)
	server.echo.POST("/v1/auth/login", server.login)
	server.echo.POST("/v1/auth/logout", server.logout, server.requireSession)
	server.echo.GET("/v1/auth/me", server.me, server.requireSession)
	server.echo.POST("/v1/quotations/prepare", server.prepareEndpoint, server.requireSession)
	server.echo.POST("/v1/quotations/export", server.exportEndpoint, server.requireSession)
	server.echo.GET("/v1/quotations", server.listEndpoint, server.requireSession)
	server.echo.GET("/v1/quotations/next-sequence", server.nextSequence, server.requireSession)
	server.echo.GET("/v1/quotations/:id", server.detailEndpoint, server.requireSession)
	server.echo.GET("/v1/quotations/:id/revisions", server.revisionsEndpoint, server.requireSession)
	server.echo.DELETE("/v1/quotations/:id", server.deleteEndpoint, server.requireSession, server.requireAdmin)
	server.echo.PUT("/v1/quotations/:id/revisions/:rev/document", server.attachDocumentEndpoint, server.requireSession)
	server.echo.GET("/v1/admin/users", server.listUsers, server.requireSession, server.requireAdmin)
	server.echo.POST("/v1/admin/users", server.createUser, server.requireSession, server.requireAdmin)
	server.echo.PATCH("/v1/admin/users/:id/role", server.setUserRole, server.requireSession, server.requireAdmin)
	server.echo.PATCH("/v1/admin/users/:id/active", server.setUserActive, server.requireSession, server.requireAdmin)
	server.echo.POST("/v1/admin/users/:id/reset", server.resetUser, server.requireSession, server.requireAdmin)
	server.echo.DELETE("/v1/admin/users/:id", server.deleteUser, server.requireSession, server.requireAdmin)
	server.echo.POST("/v1/auth/redeem", server.redeemInvite)
	server.echo.PATCH("/v1/auth/profile", server.updateProfile, server.requireSession)
	server.echo.PATCH("/v1/auth/password", server.changePassword, server.requireSession)
	server.echo.POST("/v1/auth/logout-everywhere", server.logoutEverywhere, server.requireSession)
	server.echo.POST("/v1/auth/deletion-request", server.requestDeletion, server.requireSession)
	server.echo.GET("/v1/auth/deletion-requests/mine", server.myDeletionRequests, server.requireSession)
	server.echo.GET("/v1/admin/deletion-requests", server.listDeletionRequests, server.requireSession, server.requireAdmin)
	server.echo.GET("/v1/admin/backup/latest", server.downloadBackup, server.requireSession, server.requireAdmin)
	server.echo.POST("/v1/admin/backup/restore", server.restoreBackup, server.requireSession, server.requireAdmin)
	server.echo.POST("/v1/admin/deletion-requests/:id/approve", server.decideDeletion(true), server.requireSession, server.requireAdmin)
	server.echo.POST("/v1/admin/deletion-requests/:id/reject", server.decideDeletion(false), server.requireSession, server.requireAdmin)
	server.echo.PATCH("/v1/admin/users/:id/email", server.setUserEmail, server.requireSession, server.requireAdmin)
	server.echo.POST("/v1/auth/email-change-request", server.requestEmailChange, server.requireSession)
	server.echo.GET("/v1/auth/email-change-requests/mine", server.myEmailChangeRequests, server.requireSession)
	server.echo.GET("/v1/admin/email-change-requests", server.listEmailChangeRequests, server.requireSession, server.requireAdmin)
	server.echo.POST("/v1/admin/email-change-requests/:id/approve", server.decideEmailChange(true), server.requireSession, server.requireAdmin)
	server.echo.POST("/v1/admin/email-change-requests/:id/reject", server.decideEmailChange(false), server.requireSession, server.requireAdmin)
	server.echo.GET("/v1/admin/company", server.getCompanyProfile, server.requireSession, server.requireAdmin)
	server.echo.PUT("/v1/admin/company", server.updateCompanyProfile, server.requireSession, server.requireAdmin)
	// The narrow company writes (issue #432): one route per settings tab, each
	// writing only its own columns. The whole-record PUT above stays for the
	// clients that still send every editable field at once -- the CLI's
	// `vera company set` among them -- so a save from one tab can never clear
	// the columns another tab owns.
	server.echo.PUT("/v1/admin/company/general", server.updateCompanyGeneral, server.requireSession, server.requireAdmin)
	server.echo.PUT("/v1/admin/company/theme/quotation", server.updateQuotationTheme, server.requireSession, server.requireAdmin)
	server.echo.PUT("/v1/admin/company/theme/portal", server.updatePortalTheme, server.requireSession, server.requireAdmin)
	server.echo.GET("/v1/admin/company/logo", server.getCompanyLogo, server.requireSession)
	server.echo.PUT("/v1/admin/company/logo", server.setCompanyLogo, server.requireSession, server.requireAdmin)
	server.echo.DELETE("/v1/admin/company/logo", server.clearCompanyLogo, server.requireSession, server.requireAdmin)
	// Session-readable company identity for the app shell and other UI
	// surfaces. Writes stay admin-only above. The unit list rides a sibling
	// route: units print on customer documents, so every signed-in role reads
	// them, but only admins replace the list.
	server.echo.GET("/v1/company", server.getCompanyIdentity, server.requireSession)
	server.echo.GET("/v1/company/units", server.getCompanyUnits, server.requireSession)
	server.echo.PUT("/v1/admin/company/units", server.updateCompanyUnits, server.requireSession, server.requireAdmin)
	// The document's wording follows the same split as the units: every
	// signed-in role reads it, because it prints on customer documents and the
	// preview has to render it; only an admin replaces it.
	server.echo.GET("/v1/company/labels", server.getCompanyLabels, server.requireSession)
	server.echo.PUT("/v1/admin/company/labels", server.updateCompanyLabels, server.requireSession, server.requireAdmin)
	// The admin portal preview (issue #340): the verification page as it renders
	// right now, so a rebrand is judged on all five states before it is saved.
	// Admin-only, and its own rate-limit budget — sharing the public verifier's
	// would let a rebranding admin starve the customers scanning real
	// quotations from the same office address.
	server.echo.GET("/v1/admin/company/portal-preview", server.portalPreview, server.requireSession, server.requireAdmin)
	// Public verification surface. Deliberately registered without
	// requireSession and with no write path: see docs in verify.go. The POST
	// route carries a body limit so an oversized form is rejected before it
	// is parsed.
	server.echo.GET("/v/:id", server.verifyPage)
	server.echo.POST("/verify", server.verifySubmit, echoMiddleware.BodyLimit("4K"))
	// Drafts are per-user and decided in SQL: these handlers only ever pass
	// the caller's id, so there is no route by which one account reaches
	// another's drafts, admins included.
	server.echo.GET("/v1/drafts", server.listDrafts, server.requireSession)
	server.echo.POST("/v1/drafts", server.saveDraft, server.requireSession)
	server.echo.GET("/v1/drafts/:id", server.getDraft, server.requireSession)
	server.echo.DELETE("/v1/drafts/:id", server.deleteDraft, server.requireSession)
	server.echo.GET("/v1/clients", server.listClients, server.requireSession)
	server.echo.POST("/v1/admin/clients", server.createClient, server.requireSession, server.requireAdmin)
	server.echo.PATCH("/v1/admin/clients/:id", server.updateClient, server.requireSession, server.requireAdmin)
	server.echo.DELETE("/v1/admin/clients/:id", server.deleteClient, server.requireSession, server.requireAdmin)
	return server
}

func (s *Server) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	s.echo.ServeHTTP(writer, request)
}

func (s *Server) healthz(context echo.Context) error {
	return context.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// loginAttemptsPerMinute is deliberately generous.
//
// An office signs in from one address, and a tight limit here would lock out
// colleagues because one of them mistyped a password twice. Sixty a minute is
// far above what a real office does and far below what a script needs, so the
// trade lands where it should: no legitimate traffic is ever refused, and the
// unthrottled flood is gone.
//
// It is an attempt limit rather than a failure limit because that is what bounds
// the database work, which is the thing worth bounding -- every attempt costs a
// GetUserByEmail round trip. It is checked before the body is read, so a flood
// is refused as cheaply as possible.
const loginAttemptsPerMinute = 60

// previewsPerMinute bounds the admin portal preview. One render per colour
// commit, and a judgement session moves four sliders — so a limit far above
// what a person does, and far below what a loop does.
const previewsPerMinute = 120

func (s *Server) login(context echo.Context) error {
	if !s.loginLimiter.allow(clientIP(context)) {
		return context.JSON(http.StatusTooManyRequests, map[string]string{
			"error": "too many sign-in attempts; try again shortly",
		})
	}

	var request loginRequest
	if err := context.Bind(&request); err != nil {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	token, user, err := s.service.Login(context.Request().Context(), request.Email, request.Password)
	if err != nil {
		return context.JSON(http.StatusUnauthorized, map[string]string{"error": err.Error()})
	}
	return context.JSON(http.StatusOK, map[string]any{
		"token": token,
		"user": map[string]any{
			"id":                 user.ID,
			"email":              user.Email,
			"displayName":        user.DisplayName,
			"role":               user.Role,
			"mustChangePassword": user.MustChangePassword,
		},
	})
}

func (s *Server) logout(context echo.Context) error {
	token, _ := bearerToken(context.Request())
	if err := s.service.Logout(context.Request().Context(), token); err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "sign out failed"})
	}
	return context.NoContent(http.StatusNoContent)
}

func (s *Server) me(context echo.Context) error {
	user, _ := context.Get("user").(store.AuthUser)
	response := map[string]any{
		"id":          user.ID,
		"email":       user.Email,
		"displayName": user.DisplayName,
		"role":        user.Role,
	}
	if url := avatarURL(user); url != "" {
		response["avatarUrl"] = url
	}
	return context.JSON(http.StatusOK, response)
}

// requireSession validates the bearer token and stores the user for handlers.
func (s *Server) requireSession(next echo.HandlerFunc) echo.HandlerFunc {
	return func(context echo.Context) error {
		token, ok := bearerToken(context.Request())
		if !ok {
			return context.JSON(http.StatusUnauthorized, map[string]string{"error": "missing session"})
		}
		user, err := s.service.Validate(context.Request().Context(), token)
		if err != nil {
			return context.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid or expired session"})
		}
		context.Set("user", user)
		return next(context)
	}
}

// requireAdmin denies non-admin callers. It must run after requireSession.
func (s *Server) requireAdmin(next echo.HandlerFunc) echo.HandlerFunc {
	return func(context echo.Context) error {
		user, _ := context.Get("user").(store.AuthUser)
		if err := auth.RequireRole(user, "admin"); err != nil {
			return context.JSON(http.StatusForbidden, map[string]string{"error": err.Error()})
		}
		return next(context)
	}
}

func bearerToken(request *http.Request) (string, bool) {
	header := request.Header.Get("Authorization")
	token, found := strings.CutPrefix(header, "Bearer ")
	if !found || strings.TrimSpace(token) == "" {
		return "", false
	}
	return strings.TrimSpace(token), true
}
