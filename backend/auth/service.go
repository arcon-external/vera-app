package auth

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"vera-app/backend/store"
)

// Service orchestrates authentication over the Postgres store.
type Service struct {
	store   *store.Store
	limiter *RateLimiter
	now     func() time.Time

	mu           sync.Mutex
	currentToken string
	hasStatic    bool
	staticUser   *store.AuthUser
	staticErr    error
}

func NewService(database *store.Store) *Service {
	return &Service{
		store:   database,
		limiter: NewRateLimiter(5, time.Minute),
		now:     time.Now,
	}
}

// Bootstrap creates the very first admin account. It refuses to run when any
// user already exists. The caller displays the returned password once.
func (s *Service) Bootstrap(ctx context.Context, email, displayName, password string) (store.AuthUser, error) {
	count, err := s.store.CountUsers(ctx)
	if err != nil {
		return store.AuthUser{}, err
	}
	if count > 0 {
		return store.AuthUser{}, fmt.Errorf("bootstrap is disabled: users already exist")
	}
	email = strings.TrimSpace(email)
	if email == "" || displayName == "" || password == "" {
		return store.AuthUser{}, fmt.Errorf("email, display name, and password are required")
	}
	hash, err := HashPassword(password)
	if err != nil {
		return store.AuthUser{}, err
	}
	return s.store.CreateUser(ctx, email, displayName, hash, "admin")
}

// Login verifies credentials and issues a session token. Failures return a
// generic error that does not reveal whether the email exists.
func (s *Service) Login(ctx context.Context, email, password string) (string, store.AuthUser, error) {
	now := s.now()
	key := strings.ToLower(strings.TrimSpace(email))
	if s.limiter.Blocked(key, now) {
		return "", store.AuthUser{}, fmt.Errorf("too many failed attempts; try again later")
	}
	fail := func() (string, store.AuthUser, error) {
		s.limiter.RecordFailure(key, now)
		return "", store.AuthUser{}, fmt.Errorf("invalid email or password")
	}

	user, err := s.store.GetUserByEmail(ctx, email)
	if err != nil {
		return fail()
	}
	if !user.IsActive || user.PasswordHash == "" {
		return fail()
	}
	if err := VerifyPassword(user.PasswordHash, password); err != nil {
		return fail()
	}

	token, tokenHash, err := NewSessionToken()
	if err != nil {
		return "", store.AuthUser{}, err
	}
	if err := s.store.CreateSession(ctx, user.ID, tokenHash, now.Add(IdleTimeout)); err != nil {
		return "", store.AuthUser{}, err
	}
	if err := s.store.TouchLogin(ctx, user.ID, now); err != nil {
		return "", store.AuthUser{}, err
	}
	s.limiter.RecordSuccess(key)
	user.LastLoginAt = now
	return token, user, nil
}

// Validate resolves a session token to its active user.
func (s *Service) Validate(ctx context.Context, token string) (store.AuthUser, error) {
	if strings.TrimSpace(token) == "" {
		return store.AuthUser{}, fmt.Errorf("missing session")
	}
	return s.store.GetSessionUser(ctx, HashSessionToken(token), s.now())
}

// Logout revokes a single session token.
func (s *Service) Logout(ctx context.Context, token string) error {
	if strings.TrimSpace(token) == "" {
		return nil
	}
	return s.store.RevokeSession(ctx, HashSessionToken(token))
}

// UserCount returns the number of users, used by bootstrap.
func (s *Service) UserCount(ctx context.Context) (int, error) {
	return s.store.CountUsers(ctx)
}

// RequireRole denies users without the given role.
func RequireRole(user store.AuthUser, role string) error {
	if user.Role != role {
		return fmt.Errorf("admin access required")
	}
	return nil
}

// staticUser, when set, makes currentUser return a fixed result. It exists
// so unit tests can exercise quotation logic without a live session.
func (s *Service) setStaticForTest(user store.AuthUser, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.staticUser = &user
	s.staticErr = err
	s.hasStatic = true
}

// LoginAndRemember logs in and makes the session the active one for this
// single-user desktop instance, persisting the token in the OS keychain.
func (s *Service) LoginAndRemember(ctx context.Context, email, password string) (store.AuthUser, error) {
	token, user, err := s.Login(ctx, email, password)
	if err != nil {
		return store.AuthUser{}, err
	}
	s.mu.Lock()
	s.currentToken = token
	s.mu.Unlock()
	if err := SaveSessionToken(token); err != nil {
		return store.AuthUser{}, err
	}
	return user, nil
}

// Restore validates the keychain-stored session, if any, and makes it active.
func (s *Service) Restore(ctx context.Context) (store.AuthUser, error) {
	token, err := LoadSessionToken()
	if err != nil {
		return store.AuthUser{}, err
	}
	if strings.TrimSpace(token) == "" {
		return store.AuthUser{}, fmt.Errorf("no saved session")
	}
	user, err := s.Validate(ctx, token)
	if err != nil {
		_ = DeleteSessionToken()
		return store.AuthUser{}, err
	}
	s.mu.Lock()
	s.currentToken = token
	s.mu.Unlock()
	return user, nil
}

// LogoutCurrent revokes the active session and clears the keychain.
func (s *Service) LogoutCurrent(ctx context.Context) error {
	s.mu.Lock()
	token := s.currentToken
	s.currentToken = ""
	s.mu.Unlock()
	if strings.TrimSpace(token) != "" {
		if err := s.Logout(ctx, token); err != nil {
			return err
		}
	}
	return DeleteSessionToken()
}

// currentUser resolves the active session to its user.
func (s *Service) currentUser(ctx context.Context) (store.AuthUser, error) {
	s.mu.Lock()
	hasStatic, staticUser, staticErr, token := s.hasStatic, s.staticUser, s.staticErr, s.currentToken
	s.mu.Unlock()
	if hasStatic {
		if staticErr != nil {
			return store.AuthUser{}, staticErr
		}
		if staticUser == nil {
			return store.AuthUser{}, fmt.Errorf("not signed in")
		}
		return *staticUser, nil
	}
	if strings.TrimSpace(token) == "" {
		return store.AuthUser{}, fmt.Errorf("not signed in")
	}
	return s.Validate(ctx, token)
}

// RequireUser returns the signed-in user or an error.
func (s *Service) RequireUser(ctx context.Context) (store.AuthUser, error) {
	return s.currentUser(ctx)
}

// RequireAdmin returns the signed-in user if they are an admin.
func (s *Service) RequireAdmin(ctx context.Context) (store.AuthUser, error) {
	user, err := s.currentUser(ctx)
	if err != nil {
		return store.AuthUser{}, err
	}
	if err := RequireRole(user, "admin"); err != nil {
		return store.AuthUser{}, err
	}
	return user, nil
}
