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
	staticErr  error
	store      *store.Store
	limiter    *RateLimiter
	now        func() time.Time
	staticUser *store.AuthUser
	mu         sync.Mutex
	hasStatic  bool
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
// generic error that does not reveal whether the email exists. A remembered
// session gets the thirty-day idle lifetime under a ninety-day cap; every
// other one gets a working day under a seven-day cap, which is what a client
// that omits the flag gets. Renewal slides the idle deadline; the cap never
// moves.
func (s *Service) Login(ctx context.Context, email, password string, remember bool) (string, store.AuthUser, error) {
	now := s.now()
	key := strings.ToLower(strings.TrimSpace(email))
	if s.limiter.Blocked(key, now) {
		return "", store.AuthUser{}, fmt.Errorf("too many failed attempts; try again later")
	}
	fail := func() (string, store.AuthUser, error) {
		s.limiter.RecordFailure(key, now)
		return "", store.AuthUser{}, fmt.Errorf("invalid email or password")
	}

	user, err := s.store.GetUserByEmail(ctx, strings.TrimSpace(email))
	if err != nil {
		return fail()
	}
	if !user.IsActive || user.PasswordHash == "" {
		return fail()
	}
	if err := VerifyPassword(user.PasswordHash, password); err != nil {
		return fail()
	}
	if NeedsRehash(user.PasswordHash) {
		if hash, err := HashPassword(password); err == nil {
			// Best effort: a failed update must not deny a valid login; the next
			// login will retry. A concurrent reset is protected by the old hash.
			if updated, err := s.store.RehashPassword(ctx, user.ID, user.PasswordHash, hash); err == nil && updated {
				user.PasswordHash = hash
			}
		}
	}

	token, tokenHash, err := NewSessionToken()
	if err != nil {
		return "", store.AuthUser{}, err
	}
	// The idle lifetime slides on renewal; the absolute one is stamped once
	// and never extended, so a session cannot outlive its cap however often
	// the user clicks "Stay signed in".
	idle, absolute := SessionLifetimeStandard, SessionAbsoluteStandard
	if remember {
		idle, absolute = SessionLifetimeRemembered, SessionAbsoluteRemembered
	}
	if err := s.store.CreateSession(ctx, user.ID, tokenHash, now.Add(idle), now.Add(absolute), remember); err != nil {
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
	user, _, err := s.ValidateSession(ctx, token)
	return user, err
}

// ValidateSession resolves a session token to its active user and the
// session's expiry state, which the API reports so the desktop knows when the
// idle deadline is and whether a renewal can still extend it.
func (s *Service) ValidateSession(ctx context.Context, token string) (store.AuthUser, store.SessionInfo, error) {
	if strings.TrimSpace(token) == "" {
		return store.AuthUser{}, store.SessionInfo{}, fmt.Errorf("missing session")
	}
	return s.store.GetSession(ctx, HashSessionToken(token), s.now())
}

// RenewSession slides a live session's idle deadline by its own lifetime:
// a working day for a standard session, thirty days for a remembered one.
// The absolute cap is never moved, and an ended session returns the same
// typed *store.SessionError the session check uses.
func (s *Service) RenewSession(ctx context.Context, token string) (store.SessionInfo, error) {
	if strings.TrimSpace(token) == "" {
		return store.SessionInfo{}, &store.SessionError{Code: store.SessionCodeInvalid, Message: "invalid session"}
	}
	now := s.now()
	tokenHash := HashSessionToken(token)
	// The class is fixed at issuance, so this read only chooses the idle
	// lifetime; the store re-checks the session state when it writes.
	_, current, err := s.store.GetSession(ctx, tokenHash, now)
	if err != nil {
		return store.SessionInfo{}, err
	}
	idle := SessionLifetimeStandard
	if current.Remembered {
		idle = SessionLifetimeRemembered
	}
	return s.store.RenewSession(ctx, tokenHash, now, idle)
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

// currentUser resolves the injected static user, when one is set. A real
// session is never resolved here: the desktop resolves it through
// backend/session.go, and the API server through the request token. So a
// Service only ever carries the test identity injected by setStaticForTest,
// and without one the caller is simply not signed in.
func (s *Service) currentUser(ctx context.Context) (store.AuthUser, error) {
	s.mu.Lock()
	staticUser, staticErr, hasStatic := s.staticUser, s.staticErr, s.hasStatic
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
	return store.AuthUser{}, fmt.Errorf("not signed in")
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
