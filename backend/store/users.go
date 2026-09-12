package store

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// AuthUser is the user record used by the authentication service.
type AuthUser struct {
	ID                 int64
	Email              string
	DisplayName        string
	PasswordHash       string
	Role               string
	IsActive           bool
	MustChangePassword bool
	LastLoginAt        time.Time
}

// CreateUser inserts a user. Email/password is the only login method, so a
// password hash is mandatory; the database enforces this.
func (s *Store) CreateUser(ctx context.Context, email, displayName, passwordHash, role string) (AuthUser, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return AuthUser{}, err
	}
	if role != "staff" && role != "admin" {
		return AuthUser{}, fmt.Errorf("invalid role %q", role)
	}
	if strings.TrimSpace(passwordHash) == "" {
		return AuthUser{}, fmt.Errorf("password hash is required: email/password is the only login method")
	}
	var user AuthUser
	err = pool.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO %s.users (email, display_name, password_hash, role)
		VALUES ($1, $2, $3, $4)
		RETURNING id, email, display_name, password_hash,
		          role, is_active, must_change_password`,
		s.configSchema()),
		email, displayName, passwordHash, role).Scan(
		&user.ID, &user.Email, &user.DisplayName, &user.PasswordHash,
		&user.Role, &user.IsActive, &user.MustChangePassword)
	if err != nil {
		return AuthUser{}, fmt.Errorf("create user: %w", err)
	}
	return user, nil
}

// GetUserByEmail looks a user up case-insensitively.
func (s *Store) GetUserByEmail(ctx context.Context, email string) (AuthUser, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return AuthUser{}, err
	}
	var user AuthUser
	var lastLogin *time.Time
	err = pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT id, email, display_name, password_hash,
		       role, is_active, must_change_password, last_login_at
		FROM %s.users WHERE LOWER(email) = LOWER($1)`, s.configSchema()),
		email).Scan(
		&user.ID, &user.Email, &user.DisplayName, &user.PasswordHash,
		&user.Role, &user.IsActive, &user.MustChangePassword,
		&lastLogin)
	if err != nil {
		return AuthUser{}, fmt.Errorf("get user by email: %w", err)
	}
	if lastLogin != nil {
		user.LastLoginAt = *lastLogin
	}
	return user, nil
}

// CountUsers returns the number of user rows, used by the one-time bootstrap.
func (s *Store) CountUsers(ctx context.Context) (int, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return 0, err
	}
	var count int
	if err := pool.QueryRow(ctx, fmt.Sprintf(
		"SELECT COUNT(*) FROM %s.users", s.configSchema())).Scan(&count); err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return count, nil
}

// CreateSession stores a session token hash with its expiry.
func (s *Store) CreateSession(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) error {
	pool, err := s.poolOrError()
	if err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.sessions (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)`, s.configSchema()),
		userID, tokenHash, expiresAt); err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

// GetSessionUser returns the active user for a live session token hash.
func (s *Store) GetSessionUser(ctx context.Context, tokenHash string, now time.Time) (AuthUser, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return AuthUser{}, err
	}
	var user AuthUser
	var lastLogin *time.Time
	err = pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT u.id, u.email, u.display_name, u.password_hash,
		       u.role, u.is_active, u.must_change_password, u.last_login_at
		FROM %s.sessions s
		JOIN %s.users u ON u.id = s.user_id
		WHERE s.token_hash = $1 AND s.revoked_at IS NULL
		  AND s.expires_at > $2 AND u.is_active = TRUE`, s.configSchema(), s.configSchema()),
		tokenHash, now).Scan(
		&user.ID, &user.Email, &user.DisplayName, &user.PasswordHash,
		&user.Role, &user.IsActive, &user.MustChangePassword,
		&lastLogin)
	if err != nil {
		return AuthUser{}, fmt.Errorf("get session user: %w", err)
	}
	if lastLogin != nil {
		user.LastLoginAt = *lastLogin
	}
	return user, nil
}

// RevokeSession marks one session revoked.
func (s *Store) RevokeSession(ctx context.Context, tokenHash string) error {
	pool, err := s.poolOrError()
	if err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.sessions SET revoked_at = NOW()
		WHERE token_hash = $1 AND revoked_at IS NULL`, s.configSchema()),
		tokenHash); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

// RevokeUserSessions marks every live session of a user revoked.
func (s *Store) RevokeUserSessions(ctx context.Context, userID int64) error {
	pool, err := s.poolOrError()
	if err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.sessions SET revoked_at = NOW()
		WHERE user_id = $1 AND revoked_at IS NULL`, s.configSchema()),
		userID); err != nil {
		return fmt.Errorf("revoke user sessions: %w", err)
	}
	return nil
}

// TouchLogin updates the user's last-login timestamp.
func (s *Store) TouchLogin(ctx context.Context, userID int64, now time.Time) error {
	pool, err := s.poolOrError()
	if err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.users SET last_login_at = $2 WHERE id = $1`, s.configSchema()),
		userID, now); err != nil {
		return fmt.Errorf("touch login: %w", err)
	}
	return nil
}
