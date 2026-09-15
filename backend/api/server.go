package api

import (
	"net/http"
	"os"
	"strings"

	"github.com/labstack/echo/v4"

	"vera-app/backend"
	"vera-app/backend/auth"
	"vera-app/backend/store"
)

// Server exposes the Vera HTTP API. Authentication reuses backend/auth
// sessions: login issues a bearer token, and protected routes validate it
// (expiry + revocation + active user) on every request.
type Server struct {
	echo      *echo.Echo
	database  *store.Store
	service   *auth.Service
	templates *backend.Quotation
	backupDir string
	dsn       string
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
	server := &Server{echo: echo.New(), database: database, service: service, templates: templates}
	server.echo.HideBanner = true
	server.echo.HidePort = true
	server.echo.GET("/healthz", server.healthz)
	server.echo.HEAD("/healthz", server.healthz)
	// /health mirrors /healthz: some edges (Cloud Run's) swallow the exact
	// /healthz path, so monitors and deploys probe /health instead.
	server.echo.GET("/health", server.healthz)
	server.echo.HEAD("/health", server.healthz)
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
	server.echo.GET("/v1/admin/deletion-requests", server.listDeletionRequests, server.requireSession, server.requireAdmin)
	server.echo.GET("/v1/admin/backup/latest", server.downloadBackup, server.requireSession, server.requireAdmin)
	server.echo.POST("/v1/admin/backup/restore", server.restoreBackup, server.requireSession, server.requireAdmin)
	server.echo.POST("/v1/admin/deletion-requests/:id/approve", server.decideDeletion(true), server.requireSession, server.requireAdmin)
	server.echo.POST("/v1/admin/deletion-requests/:id/reject", server.decideDeletion(false), server.requireSession, server.requireAdmin)
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

func (s *Server) login(context echo.Context) error {
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
