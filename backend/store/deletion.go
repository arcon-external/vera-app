package store

import (
	"context"
	"fmt"
	"time"
)

// Audit records an authentication-related event. Actor and target may be nil
// for system-initiated entries. Audit writes never fail the enclosing
// operation; callers log and continue on error.
func (s *Store) Audit(ctx context.Context, actorID *int64, action string, targetID *int64, details string) error {
	pool, err := s.poolOrError()
	if err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.auth_audit (actor_id, action, target_user_id, details)
		VALUES ($1, $2, $3, $4)`, s.configSchema()),
		actorID, action, targetID, details); err != nil {
		return fmt.Errorf("write audit entry: %w", err)
	}
	return nil
}

// UpdatePassword replaces a user's hash and clears the must-change flag.
func (s *Store) UpdatePassword(ctx context.Context, userID int64, passwordHash string) error {
	pool, err := s.poolOrError()
	if err != nil {
		return err
	}
	if passwordHash == "" {
		return fmt.Errorf("password hash is required")
	}
	command, err := pool.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.users
		SET password_hash = $2, must_change_password = FALSE
		WHERE id = $1`, s.configSchema()), userID, passwordHash)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	if command.RowsAffected() == 0 {
		return fmt.Errorf("user %d not found", userID)
	}
	return nil
}

// DeletionRequest is a pending or decided account-deletion request.
type DeletionRequest struct {
	ID          int64
	UserID      int64
	Email       string
	DisplayName string
	Status      string
	RequestedAt time.Time
}

// RequestDeletion opens a pending deletion request for a user. A second
// request while one is pending returns the existing row.
func (s *Store) RequestDeletion(ctx context.Context, userID int64) (DeletionRequest, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return DeletionRequest{}, err
	}
	var request DeletionRequest
	err = pool.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO %s.deletion_requests (user_id)
		VALUES ($1)
		ON CONFLICT (user_id) WHERE status = 'pending'
		DO NOTHING
		RETURNING id, user_id, status, requested_at`, s.configSchema()),
		userID).Scan(&request.ID, &request.UserID, &request.Status, &request.RequestedAt)
	if err != nil {
		// Conflict hit an existing pending row: return it instead.
		err2 := pool.QueryRow(ctx, fmt.Sprintf(`
			SELECT id, user_id, status, requested_at
			FROM %s.deletion_requests
			WHERE user_id = $1 AND status = 'pending'`, s.configSchema()),
			userID).Scan(&request.ID, &request.UserID, &request.Status, &request.RequestedAt)
		if err2 != nil {
			return DeletionRequest{}, fmt.Errorf("request deletion: %w", err)
		}
	}
	var user AuthUser
	if user, err = s.GetUserByID(ctx, userID); err != nil {
		return DeletionRequest{}, err
	}
	request.Email = user.Email
	request.DisplayName = user.DisplayName
	return request, nil
}

// ListDeletionRequests returns pending requests newest first with user info.
func (s *Store) ListDeletionRequests(ctx context.Context) ([]DeletionRequest, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return nil, err
	}
	rows, err := pool.Query(ctx, fmt.Sprintf(`
		SELECT r.id, r.user_id, u.email, u.display_name, r.status, r.requested_at
		FROM %s.deletion_requests r
		JOIN %s.users u ON u.id = r.user_id
		WHERE r.status = 'pending'
		ORDER BY r.requested_at DESC`, s.configSchema(), s.configSchema()))
	if err != nil {
		return nil, fmt.Errorf("list deletion requests: %w", err)
	}
	defer rows.Close()

	requests := make([]DeletionRequest, 0)
	for rows.Next() {
		var request DeletionRequest
		if err := rows.Scan(&request.ID, &request.UserID, &request.Email,
			&request.DisplayName, &request.Status, &request.RequestedAt); err != nil {
			return nil, fmt.Errorf("scan deletion request: %w", err)
		}
		requests = append(requests, request)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate deletion requests: %w", err)
	}
	return requests, nil
}

// DecideDeletion approves (soft-delete + revoke sessions) or rejects a
// pending request in one transaction.
func (s *Store) DecideDeletion(ctx context.Context, requestID int64, deciderID int64, approve bool) (DeletionRequest, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return DeletionRequest{}, err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return DeletionRequest{}, fmt.Errorf("begin deletion decision: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var request DeletionRequest
	var requestRole string
	var requestActive bool
	err = tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT d.id, d.user_id, d.status, d.requested_at, u.role, u.is_active
		FROM %s.deletion_requests d
		JOIN %s.users u ON u.id = d.user_id
		WHERE d.id = $1`, s.configSchema(), s.configSchema()),
		requestID).Scan(&request.ID, &request.UserID, &request.Status, &request.RequestedAt, &requestRole, &requestActive)
	if err != nil {
		return DeletionRequest{}, fmt.Errorf("deletion request %d not found", requestID)
	}
	if request.Status != "pending" {
		return DeletionRequest{}, fmt.Errorf("deletion request %d already decided", requestID)
	}
	if approve && requestRole == "admin" && requestActive {
		var adminCount int
		if err := tx.QueryRow(ctx, fmt.Sprintf(
			"SELECT COUNT(*) FROM %s.users WHERE role = 'admin' AND is_active = TRUE",
			s.configSchema())).Scan(&adminCount); err != nil {
			return DeletionRequest{}, fmt.Errorf("count active admins: %w", err)
		}
		if adminCount <= 1 {
			return DeletionRequest{}, fmt.Errorf("cannot remove the last active admin")
		}
	}

	status := "rejected"
	if approve {
		status = "approved"
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.deletion_requests
		SET status = $2, decided_at = NOW(), decided_by = $3
		WHERE id = $1`, s.configSchema()), requestID, status, deciderID); err != nil {
		return DeletionRequest{}, fmt.Errorf("decide deletion: %w", err)
	}
	if approve {
		if _, err := tx.Exec(ctx, fmt.Sprintf(`
			UPDATE %s.users SET is_active = FALSE WHERE id = $1`, s.configSchema()),
			request.UserID); err != nil {
			return DeletionRequest{}, fmt.Errorf("deactivate user: %w", err)
		}
		if _, err := tx.Exec(ctx, fmt.Sprintf(`
			UPDATE %s.sessions SET revoked_at = NOW()
			WHERE user_id = $1 AND revoked_at IS NULL`, s.configSchema()),
			request.UserID); err != nil {
			return DeletionRequest{}, fmt.Errorf("revoke sessions: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return DeletionRequest{}, fmt.Errorf("commit deletion decision: %w", err)
	}
	request.Status = status
	return request, nil
}
