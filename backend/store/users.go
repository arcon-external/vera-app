package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
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
	Avatar             []byte
	AvatarMime         string
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
		       role, is_active, must_change_password, last_login_at, avatar, avatar_mime
		FROM %s.users WHERE LOWER(email) = LOWER($1)`, s.configSchema()),
		email).Scan(
		&user.ID, &user.Email, &user.DisplayName, &user.PasswordHash,
		&user.Role, &user.IsActive, &user.MustChangePassword,
		&lastLogin, &user.Avatar, &user.AvatarMime)
	if err != nil {
		return AuthUser{}, fmt.Errorf("get user by email: %w", err)
	}
	if lastLogin != nil {
		user.LastLoginAt = *lastLogin
	}
	return user, nil
}

// GetUserByID returns a single user by ID.
func (s *Store) GetUserByID(ctx context.Context, userID int64) (AuthUser, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return AuthUser{}, err
	}
	var user AuthUser
	var lastLogin *time.Time
	err = pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT id, email, display_name, password_hash,
		       role, is_active, must_change_password, last_login_at, avatar, avatar_mime
		FROM %s.users WHERE id = $1`, s.configSchema()),
		userID).Scan(
		&user.ID, &user.Email, &user.DisplayName, &user.PasswordHash,
		&user.Role, &user.IsActive, &user.MustChangePassword,
		&lastLogin, &user.Avatar, &user.AvatarMime)
	if err != nil {
		return AuthUser{}, fmt.Errorf("get user: %w", err)
	}
	if lastLogin != nil {
		user.LastLoginAt = *lastLogin
	}
	return user, nil
}

// SetMustChangePassword flags a user to change their password on next use.
func (s *Store) SetMustChangePassword(ctx context.Context, userID int64, mustChange bool) error {
	pool, err := s.poolOrError()
	if err != nil {
		return err
	}
	command, err := pool.Exec(ctx, fmt.Sprintf(
		"UPDATE %s.users SET must_change_password = $2 WHERE id = $1", s.configSchema()),
		userID, mustChange)
	if err != nil {
		return fmt.Errorf("set must-change flag: %w", err)
	}
	if command.RowsAffected() == 0 {
		return fmt.Errorf("user %d not found", userID)
	}
	return nil
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
		       u.role, u.is_active, u.must_change_password, u.last_login_at, u.avatar, u.avatar_mime
		FROM %s.sessions s
		JOIN %s.users u ON u.id = s.user_id
		WHERE s.token_hash = $1 AND s.revoked_at IS NULL
		  AND s.expires_at > $2 AND u.is_active = TRUE`, s.configSchema(), s.configSchema()),
		tokenHash, now).Scan(
		&user.ID, &user.Email, &user.DisplayName, &user.PasswordHash,
		&user.Role, &user.IsActive, &user.MustChangePassword,
		&lastLogin, &user.Avatar, &user.AvatarMime)
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

// ListUsers returns all users ordered by ID. Password hashes are included in
// the struct but must never be serialized to clients; handlers map to DTOs.
func (s *Store) ListUsers(ctx context.Context) ([]AuthUser, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return nil, err
	}
	rows, err := pool.Query(ctx, fmt.Sprintf(`
		SELECT id, email, display_name, password_hash,
		       role, is_active, must_change_password, last_login_at, avatar, avatar_mime
		FROM %s.users ORDER BY id`, s.configSchema()))
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	users := make([]AuthUser, 0)
	for rows.Next() {
		var user AuthUser
		var lastLogin *time.Time
		if err := rows.Scan(
			&user.ID, &user.Email, &user.DisplayName, &user.PasswordHash,
			&user.Role, &user.IsActive, &user.MustChangePassword,
			&lastLogin, &user.Avatar, &user.AvatarMime); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		if lastLogin != nil {
			user.LastLoginAt = *lastLogin
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate users: %w", err)
	}
	return users, nil
}

// SetUserRole changes a user's role.
func (s *Store) SetUserRole(ctx context.Context, userID int64, role string) error {
	pool, err := s.poolOrError()
	if err != nil {
		return err
	}
	if role != "staff" && role != "admin" {
		return fmt.Errorf("invalid role %q", role)
	}
	command, err := pool.Exec(ctx, fmt.Sprintf(
		"UPDATE %s.users SET role = $2 WHERE id = $1", s.configSchema()),
		userID, role)
	if err != nil {
		return fmt.Errorf("set user role: %w", err)
	}
	if command.RowsAffected() == 0 {
		return fmt.Errorf("user %d not found", userID)
	}
	return nil
}

// SetUserActive enables or disables a user account.
func (s *Store) SetUserActive(ctx context.Context, userID int64, active bool) error {
	pool, err := s.poolOrError()
	if err != nil {
		return err
	}
	command, err := pool.Exec(ctx, fmt.Sprintf(
		"UPDATE %s.users SET is_active = $2 WHERE id = $1", s.configSchema()),
		userID, active)
	if err != nil {
		return fmt.Errorf("set user active: %w", err)
	}
	if command.RowsAffected() == 0 {
		return fmt.Errorf("user %d not found", userID)
	}
	return nil
}

// CountActiveAdmins returns the number of enabled admin accounts.
func (s *Store) CountActiveAdmins(ctx context.Context) (int, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return 0, err
	}
	var count int
	if err := pool.QueryRow(ctx, fmt.Sprintf(
		"SELECT COUNT(*) FROM %s.users WHERE role = 'admin' AND is_active = TRUE",
		s.configSchema())).Scan(&count); err != nil {
		return 0, fmt.Errorf("count active admins: %w", err)
	}
	return count, nil
}

// SetProfile updates a user's display name and/or avatar. Empty display
// name leaves the name unchanged; clearAvatar removes the picture.
func (s *Store) SetProfile(ctx context.Context, userID int64, displayName string, avatar []byte, mime string, clearAvatar bool) error {
	pool, err := s.poolOrError()
	if err != nil {
		return err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin profile update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if displayName != "" {
		if _, err := tx.Exec(ctx, fmt.Sprintf(
			"UPDATE %s.users SET display_name = $2 WHERE id = $1", s.configSchema()),
			userID, displayName); err != nil {
			return fmt.Errorf("update display name: %w", err)
		}
	}
	if clearAvatar {
		if _, err := tx.Exec(ctx, fmt.Sprintf(
			"UPDATE %s.users SET avatar = NULL, avatar_mime = '' WHERE id = $1", s.configSchema()),
			userID); err != nil {
			return fmt.Errorf("clear avatar: %w", err)
		}
	} else if avatar != nil {
		if _, err := tx.Exec(ctx, fmt.Sprintf(
			"UPDATE %s.users SET avatar = $2, avatar_mime = $3 WHERE id = $1", s.configSchema()),
			userID, avatar, mime); err != nil {
			return fmt.Errorf("update avatar: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit profile update: %w", err)
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

// IssueInviteToken creates a fresh single-use invite/reset token, replacing
// any outstanding tokens for the user so only the newest one works.
func (s *Store) IssueInviteToken(ctx context.Context, userID int64, expiresAt time.Time) (string, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return "", err
	}
	var exists bool
	if err := pool.QueryRow(ctx, fmt.Sprintf(
		"SELECT EXISTS (SELECT 1 FROM %s.users WHERE id = $1)", s.configSchema()),
		userID).Scan(&exists); err != nil {
		return "", fmt.Errorf("check user: %w", err)
	}
	if !exists {
		return "", fmt.Errorf("user %d not found", userID)
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate invite token: %w", err)
	}
	token := hex.EncodeToString(raw)
	sum := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(sum[:])

	tx, err := pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin invite transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, fmt.Sprintf(
		"DELETE FROM %s.invite_tokens WHERE user_id = $1", s.configSchema()),
		userID); err != nil {
		return "", fmt.Errorf("clear old invite tokens: %w", err)
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.invite_tokens (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)`, s.configSchema()),
		userID, tokenHash, expiresAt); err != nil {
		return "", fmt.Errorf("insert invite token: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit invite token: %w", err)
	}
	return token, nil
}

// RedeemInviteToken consumes a valid invite token: it marks the token used,
// sets the new password hash, and clears the must-change flag. Reuse,
// expiry, and unknown tokens all fail.
func (s *Store) RedeemInviteToken(ctx context.Context, token, passwordHash string, now time.Time) (int64, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return 0, err
	}
	if strings.TrimSpace(passwordHash) == "" {
		return 0, fmt.Errorf("password hash is required")
	}
	sum := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(sum[:])

	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin redeem transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var userID int64
	err = tx.QueryRow(ctx, fmt.Sprintf(`
		UPDATE %s.invite_tokens SET used_at = $1
		WHERE token_hash = $2 AND used_at IS NULL AND expires_at > $1
		RETURNING user_id`, s.configSchema()), now, tokenHash).Scan(&userID)
	if err != nil {
		return 0, fmt.Errorf("invalid or expired invite token")
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.users SET password_hash = $2, must_change_password = FALSE
		WHERE id = $1`, s.configSchema()), userID, passwordHash); err != nil {
		return 0, fmt.Errorf("set redeemed password: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit invite redeem: %w", err)
	}
	return userID, nil
}

// SetPassword replaces a user's password hash directly (admin reset path;
// the user is flagged to change it on next use).
func (s *Store) SetPassword(ctx context.Context, userID int64, passwordHash string) error {
	pool, err := s.poolOrError()
	if err != nil {
		return err
	}
	if strings.TrimSpace(passwordHash) == "" {
		return fmt.Errorf("password hash is required")
	}
	command, err := pool.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.users SET password_hash = $2, must_change_password = TRUE
		WHERE id = $1`, s.configSchema()), userID, passwordHash)
	if err != nil {
		return fmt.Errorf("set password: %w", err)
	}
	if command.RowsAffected() == 0 {
		return fmt.Errorf("user %d not found", userID)
	}
	return nil
}
