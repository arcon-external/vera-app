package api

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"vera-app/backend/auth"
	"vera-app/backend/store"
)

// Server exposes the Vera HTTP API. Authentication reuses backend/auth
// sessions: login issues a bearer token, and protected routes validate it
// (expiry + revocation + active user) on every request.
type Server struct {
	echo     *echo.Echo
	database *store.Store
	service  *auth.Service
}

// NewServer builds the router. Database and service may be nil only for
// serving /healthz in tests; auth routes require them.
func NewServer(database *store.Store, service *auth.Service) *Server {
	server := &Server{echo: echo.New(), database: database, service: service}
	server.echo.HideBanner = true
	server.echo.HidePort = true
	server.echo.GET("/healthz", server.healthz)
	server.echo.POST("/v1/auth/login", server.login)
	server.echo.POST("/v1/auth/logout", server.logout, server.requireSession)
	server.echo.GET("/v1/auth/me", server.me, server.requireSession)
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
	return context.JSON(http.StatusOK, map[string]any{
		"id":          user.ID,
		"email":       user.Email,
		"displayName": user.DisplayName,
		"role":        user.Role,
	})
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

func bearerToken(request *http.Request) (string, bool) {
	header := request.Header.Get("Authorization")
	token, found := strings.CutPrefix(header, "Bearer ")
	if !found || strings.TrimSpace(token) == "" {
		return "", false
	}
	return strings.TrimSpace(token), true
}
