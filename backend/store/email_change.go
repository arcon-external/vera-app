package store

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// EmailChangeRequest is a pending or decided email-change request. Staff
// file one with a proposed address; an admin approves or rejects it.
// Admins change emails directly via SetUserEmail and never need a row here.
type EmailChangeRequest struct {
	RequestedAt   time.Time
	Email         string
	DisplayName   string
	ProposedEmail string
	Status        string
	ID            int64
	UserID        int64
}

// validEmailFormat is deliberately light: trimmed, non-empty, with address
// parts on both sides of a single @. Uniqueness (case-insensitive) and
// admin approval are the real guards; this system has no SMTP loop.
func validEmailFormat(email string) bool {
	trimmed := strings.TrimSpace(email)
	if trimmed == "" || strings.Contains(trimmed, " ") {
		return false
	}
	local, domain, found := strings.Cut(trimmed, "@")
	return found && local != "" && domain != "" && !strings.Contains(domain, "@")
}

// SetUserEmail changes a user's login email immediately and revokes every
// session, including the current one: email is a credential, so a flip
// signs the account out everywhere like a password change does.
func (s *Store) SetUserEmail(ctx context.Context, userID int64, email string) (oldEmail, newEmail string, err error) {
	pool, err := s.poolOrError()
	if err != nil {
		return "", "", err
	}
	trimmed := strings.ToLower(strings.TrimSpace(email))
	if !validEmailFormat(trimmed) {
		return "", "", fmt.Errorf("invalid email address")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return "", "", fmt.Errorf("begin email change: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := tx.QueryRow(ctx, fmt.Sprintf(
		"SELECT email FROM %s.users WHERE id = $1", s.configSchema()),
		userID).Scan(&oldEmail); err != nil {
		return "", "", fmt.Errorf("user %d not found", userID)
	}
	var taken bool
	if err := tx.QueryRow(ctx, fmt.Sprintf(
		`SELECT EXISTS (SELECT 1 FROM %s.users WHERE LOWER(email) = LOWER($1) AND id <> $2)`,
		s.configSchema()), trimmed, userID).Scan(&taken); err != nil {
		return "", "", fmt.Errorf("check email collision: %w", err)
	}
	if taken {
		return "", "", fmt.Errorf("email %q is already in use", trimmed)
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf(
		"UPDATE %s.users SET email = $2 WHERE id = $1", s.configSchema()),
		userID, trimmed); err != nil {
		return "", "", fmt.Errorf("set user email: %w", err)
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.sessions SET revoked_at = NOW()
		WHERE user_id = $1 AND revoked_at IS NULL`, s.configSchema()),
		userID); err != nil {
		return "", "", fmt.Errorf("revoke sessions: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", "", fmt.Errorf("commit email change: %w", err)
	}
	return oldEmail, trimmed, nil
}

// RequestEmailChange opens a pending request for a proposed address. A
// second request while one is pending returns the existing row.
func (s *Store) RequestEmailChange(ctx context.Context, userID int64, proposedEmail string) (EmailChangeRequest, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return EmailChangeRequest{}, err
	}
	trimmed := strings.ToLower(strings.TrimSpace(proposedEmail))
	if !validEmailFormat(trimmed) {
		return EmailChangeRequest{}, fmt.Errorf("invalid email address")
	}
	user, err := s.GetUserByID(ctx, userID)
	if err != nil {
		return EmailChangeRequest{}, err
	}
	if strings.EqualFold(user.Email, trimmed) {
		return EmailChangeRequest{}, fmt.Errorf("already using email %q", trimmed)
	}
	var taken bool
	if err := pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT EXISTS (SELECT 1 FROM %s.users WHERE LOWER(email) = LOWER($1) AND id <> $2)`,
		s.configSchema()), trimmed, userID).Scan(&taken); err != nil {
		return EmailChangeRequest{}, fmt.Errorf("check email collision: %w", err)
	}
	if taken {
		return EmailChangeRequest{}, fmt.Errorf("email %q is already in use", trimmed)
	}

	var request EmailChangeRequest
	err = pool.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO %s.email_change_requests (user_id, proposed_email)
		VALUES ($1, $2)
		ON CONFLICT (user_id) WHERE status = 'pending'
		DO NOTHING
		RETURNING id, user_id, proposed_email, status, requested_at`, s.configSchema()),
		userID, trimmed).Scan(&request.ID, &request.UserID, &request.ProposedEmail, &request.Status, &request.RequestedAt)
	if err != nil {
		// Conflict hit an existing pending row: return it instead.
		err2 := pool.QueryRow(ctx, fmt.Sprintf(`
			SELECT id, user_id, proposed_email, status, requested_at
			FROM %s.email_change_requests
			WHERE user_id = $1 AND status = 'pending'`, s.configSchema()),
			userID).Scan(&request.ID, &request.UserID, &request.ProposedEmail, &request.Status, &request.RequestedAt)
		if err2 != nil {
			return EmailChangeRequest{}, fmt.Errorf("request email change: %w", err)
		}
	}
	request.Email = user.Email
	request.DisplayName = user.DisplayName
	return request, nil
}

// ListUserEmailChangeRequests returns a user's own requests, newest first,
// including decided ones so staff can see outcomes.
func (s *Store) ListUserEmailChangeRequests(ctx context.Context, userID int64) ([]EmailChangeRequest, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return nil, err
	}
	rows, err := pool.Query(ctx, fmt.Sprintf(`
		SELECT id, user_id, proposed_email, status, requested_at
		FROM %s.email_change_requests
		WHERE user_id = $1
		ORDER BY requested_at DESC`, s.configSchema()),
		userID)
	if err != nil {
		return nil, fmt.Errorf("list user email change requests: %w", err)
	}
	defer rows.Close()

	requests := make([]EmailChangeRequest, 0)
	for rows.Next() {
		var request EmailChangeRequest
		if err := rows.Scan(&request.ID, &request.UserID, &request.ProposedEmail, &request.Status, &request.RequestedAt); err != nil {
			return nil, fmt.Errorf("scan user email change request: %w", err)
		}
		requests = append(requests, request)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user email change requests: %w", err)
	}
	return requests, nil
}

// ListEmailChangeRequests returns pending requests newest first with user info.
func (s *Store) ListEmailChangeRequests(ctx context.Context) ([]EmailChangeRequest, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return nil, err
	}
	rows, err := pool.Query(ctx, fmt.Sprintf(`
		SELECT r.id, r.user_id, u.email, u.display_name, r.proposed_email, r.status, r.requested_at
		FROM %s.email_change_requests r
		JOIN %s.users u ON u.id = r.user_id
		WHERE r.status = 'pending'
		ORDER BY r.requested_at DESC`, s.configSchema(), s.configSchema()))
	if err != nil {
		return nil, fmt.Errorf("list email change requests: %w", err)
	}
	defer rows.Close()

	requests := make([]EmailChangeRequest, 0)
	for rows.Next() {
		var request EmailChangeRequest
		if err := rows.Scan(&request.ID, &request.UserID, &request.Email,
			&request.DisplayName, &request.ProposedEmail, &request.Status, &request.RequestedAt); err != nil {
			return nil, fmt.Errorf("scan email change request: %w", err)
		}
		requests = append(requests, request)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate email change requests: %w", err)
	}
	return requests, nil
}

// DecideEmailChange approves (flip email + revoke sessions) or rejects a
// pending request in one transaction. The collision check reruns inside
// the transaction: the proposed address may have been taken since filing.
func (s *Store) DecideEmailChange(ctx context.Context, requestID int64, deciderID int64, approve bool) (EmailChangeRequest, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return EmailChangeRequest{}, err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return EmailChangeRequest{}, fmt.Errorf("begin email change decision: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var request EmailChangeRequest
	err = tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT d.id, d.user_id, d.proposed_email, d.status, d.requested_at
		FROM %s.email_change_requests d
		WHERE d.id = $1`, s.configSchema()),
		requestID).Scan(&request.ID, &request.UserID, &request.ProposedEmail, &request.Status, &request.RequestedAt)
	if err != nil {
		return EmailChangeRequest{}, fmt.Errorf("email change request %d not found", requestID)
	}
	if request.Status != "pending" {
		return EmailChangeRequest{}, fmt.Errorf("email change request %d already decided", requestID)
	}

	status := "rejected"
	if approve {
		status = "approved"
	}
	if approve {
		request.ProposedEmail = strings.ToLower(strings.TrimSpace(request.ProposedEmail))
		var taken bool
		if err := tx.QueryRow(ctx, fmt.Sprintf(
			`SELECT EXISTS (SELECT 1 FROM %s.users WHERE LOWER(email) = LOWER($1) AND id <> $2)`,
			s.configSchema()), request.ProposedEmail, request.UserID).Scan(&taken); err != nil {
			return EmailChangeRequest{}, fmt.Errorf("check email collision: %w", err)
		}
		if taken {
			return EmailChangeRequest{}, fmt.Errorf("email %q is already in use", request.ProposedEmail)
		}
		if _, err := tx.Exec(ctx, fmt.Sprintf(`
			UPDATE %s.users SET email = $2 WHERE id = $1`, s.configSchema()),
			request.UserID, request.ProposedEmail); err != nil {
			return EmailChangeRequest{}, fmt.Errorf("flip user email: %w", err)
		}
		if _, err := tx.Exec(ctx, fmt.Sprintf(`
			UPDATE %s.sessions SET revoked_at = NOW()
			WHERE user_id = $1 AND revoked_at IS NULL`, s.configSchema()),
			request.UserID); err != nil {
			return EmailChangeRequest{}, fmt.Errorf("revoke sessions: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.email_change_requests
		SET status = $2, decided_at = NOW(), decided_by = $3
		WHERE id = $1`, s.configSchema()), requestID, status, deciderID); err != nil {
		return EmailChangeRequest{}, fmt.Errorf("decide email change: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return EmailChangeRequest{}, fmt.Errorf("commit email change decision: %w", err)
	}
	request.Status = status
	return request, nil
}
