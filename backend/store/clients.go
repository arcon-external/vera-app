package store

import (
	"context"
	"fmt"
	"strings"
)

// ManagedClient is the admin-facing client shape with its primary address
// and archive size. Codes are immutable identity (numbering cycles are
// keyed by code); names and addresses are editable.
type ManagedClient struct {
	Name           string
	PrimaryAddress string
	ID             int64
	QuotationCount int64
	Code           int16
}

// CustomerRef is the minimal identity Prepare needs: display name plus
// the default recipient address.
type CustomerRef struct {
	Name    string
	Address string
}

// ListClients returns all clients ordered by code with quotation counts.
func (s *Store) ListClients(ctx context.Context) ([]ManagedClient, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return nil, err
	}
	schema := s.configSchema()
	rows, err := pool.Query(ctx, fmt.Sprintf(`
		SELECT c.id, c.customer_code, c.legal_name,
		       COALESCE((SELECT address FROM %s.customer_addresses
		                 WHERE customer_id = c.id AND is_primary = TRUE LIMIT 1), ''),
		       (SELECT COUNT(*) FROM %s.quotations q WHERE q.customer_id = c.id)
		FROM %s.customers c
		ORDER BY c.customer_code`, schema, schema, schema))
	if err != nil {
		return nil, fmt.Errorf("list clients: %w", err)
	}
	defer rows.Close()

	clients := make([]ManagedClient, 0)
	for rows.Next() {
		var client ManagedClient
		if err := rows.Scan(&client.ID, &client.Code, &client.Name,
			&client.PrimaryAddress, &client.QuotationCount); err != nil {
			return nil, fmt.Errorf("scan client: %w", err)
		}
		clients = append(clients, client)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate clients: %w", err)
	}
	return clients, nil
}

// GetClientByID returns one client row with its quotation count.
func (s *Store) GetClientByID(ctx context.Context, id int64) (ManagedClient, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return ManagedClient{}, err
	}
	schema := s.configSchema()
	var client ManagedClient
	err = pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT c.id, c.customer_code, c.legal_name,
		       COALESCE((SELECT address FROM %s.customer_addresses
		                 WHERE customer_id = c.id AND is_primary = TRUE LIMIT 1), ''),
		       (SELECT COUNT(*) FROM %s.quotations q WHERE q.customer_id = c.id)
		FROM %s.customers c
		WHERE c.id = $1`, schema, schema, schema),
		id).Scan(&client.ID, &client.Code, &client.Name,
		&client.PrimaryAddress, &client.QuotationCount)
	if err != nil {
		return ManagedClient{}, fmt.Errorf("client %d not found", id)
	}
	return client, nil
}

// GetCustomerByCode resolves a client code to its display name and primary
// address. Unknown codes fail: numbering and Prepare must never invent a
// client.
func (s *Store) GetCustomerByCode(ctx context.Context, code int) (CustomerRef, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return CustomerRef{}, err
	}
	var ref CustomerRef
	err = pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT c.legal_name,
		       COALESCE((SELECT address FROM %s.customer_addresses
		                 WHERE customer_id = c.id AND is_primary = TRUE LIMIT 1), '')
		FROM %s.customers c
		WHERE c.customer_code = $1`, s.configSchema(), s.configSchema()),
		code).Scan(&ref.Name, &ref.Address)
	if err != nil {
		return CustomerRef{}, fmt.Errorf("customer code %d not found", code)
	}
	return ref, nil
}

func checkClientFields(code int, name, address string) error {
	if code < 0 || code > 99 {
		return fmt.Errorf("invalid client code %d: want 0-99", code)
	}
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("client name is required")
	}
	if strings.TrimSpace(address) == "" {
		return fmt.Errorf("client address is required")
	}
	return nil
}

// CreateClient inserts a client with its primary address. Code 0 allocates
// the next free code (max+1); explicit codes must be unique.
func (s *Store) CreateClient(ctx context.Context, code int, name, address string) (ManagedClient, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return ManagedClient{}, err
	}
	if err := checkClientFields(code, name, address); err != nil {
		return ManagedClient{}, err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return ManagedClient{}, fmt.Errorf("begin create client: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	schema := s.configSchema()

	if code == 0 {
		var max int
		if err := tx.QueryRow(ctx, fmt.Sprintf(
			`SELECT COALESCE(MAX(customer_code), 0) FROM %s.customers`, schema)).Scan(&max); err != nil {
			return ManagedClient{}, fmt.Errorf("allocate client code: %w", err)
		}
		code = max + 1
		if code > 99 {
			return ManagedClient{}, fmt.Errorf("client codes exhausted")
		}
	}
	var exists bool
	if err := tx.QueryRow(ctx, fmt.Sprintf(
		`SELECT EXISTS (SELECT 1 FROM %s.customers WHERE customer_code = $1)`, schema),
		code).Scan(&exists); err != nil {
		return ManagedClient{}, fmt.Errorf("check client code: %w", err)
	}
	if exists {
		return ManagedClient{}, fmt.Errorf("client code %d is already in use", code)
	}
	if err := tx.QueryRow(ctx, fmt.Sprintf(
		`SELECT EXISTS (SELECT 1 FROM %s.customers WHERE LOWER(legal_name) = LOWER($1))`, schema),
		strings.TrimSpace(name)).Scan(&exists); err != nil {
		return ManagedClient{}, fmt.Errorf("check client name: %w", err)
	}
	if exists {
		return ManagedClient{}, fmt.Errorf("client name %q is already in use", strings.TrimSpace(name))
	}

	var client ManagedClient
	err = tx.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO %s.customers (customer_code, legal_name)
		VALUES ($1, $2)
		RETURNING id, customer_code, legal_name`, schema),
		code, strings.TrimSpace(name)).Scan(&client.ID, &client.Code, &client.Name)
	if err != nil {
		return ManagedClient{}, fmt.Errorf("insert client: %w", err)
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.customer_addresses (customer_id, address, is_primary)
		VALUES ($1, $2, TRUE)`, schema),
		client.ID, strings.TrimSpace(address)); err != nil {
		return ManagedClient{}, fmt.Errorf("insert client address: %w", err)
	}
	client.PrimaryAddress = strings.TrimSpace(address)
	if err := tx.Commit(ctx); err != nil {
		return ManagedClient{}, fmt.Errorf("commit create client: %w", err)
	}
	return client, nil
}

// UpdateClient renames a client and replaces its primary address. The code
// is immutable: numbering cycles are keyed by code, so a code change would
// fork the client's sequence.
func (s *Store) UpdateClient(ctx context.Context, id int64, name, address string) (ManagedClient, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return ManagedClient{}, err
	}
	if err := checkClientFields(1, name, address); err != nil {
		return ManagedClient{}, err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return ManagedClient{}, fmt.Errorf("begin update client: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	schema := s.configSchema()

	var client ManagedClient
	err = tx.QueryRow(ctx, fmt.Sprintf(
		`SELECT id, customer_code FROM %s.customers WHERE id = $1`, schema),
		id).Scan(&client.ID, &client.Code)
	if err != nil {
		return ManagedClient{}, fmt.Errorf("client %d not found", id)
	}
	var clash bool
	if err := tx.QueryRow(ctx, fmt.Sprintf(
		`SELECT EXISTS (SELECT 1 FROM %s.customers WHERE LOWER(legal_name) = LOWER($1) AND id <> $2)`, schema),
		strings.TrimSpace(name), id).Scan(&clash); err != nil {
		return ManagedClient{}, fmt.Errorf("check client name: %w", err)
	}
	if clash {
		return ManagedClient{}, fmt.Errorf("client name %q is already in use", strings.TrimSpace(name))
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf(
		`UPDATE %s.customers SET legal_name = $2, updated_at = NOW() WHERE id = $1`, schema),
		id, strings.TrimSpace(name)); err != nil {
		return ManagedClient{}, fmt.Errorf("update client: %w", err)
	}
	command, err := tx.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.customer_addresses SET address = $2
		WHERE customer_id = $1 AND is_primary = TRUE`, schema),
		id, strings.TrimSpace(address))
	if err != nil {
		return ManagedClient{}, fmt.Errorf("update client address: %w", err)
	}
	if command.RowsAffected() == 0 {
		if _, err := tx.Exec(ctx, fmt.Sprintf(`
			INSERT INTO %s.customer_addresses (customer_id, address, is_primary)
			VALUES ($1, $2, TRUE)`, schema),
			id, strings.TrimSpace(address)); err != nil {
			return ManagedClient{}, fmt.Errorf("insert client address: %w", err)
		}
	}
	client.Name = strings.TrimSpace(name)
	client.PrimaryAddress = strings.TrimSpace(address)
	if err := tx.Commit(ctx); err != nil {
		return ManagedClient{}, fmt.Errorf("commit update client: %w", err)
	}
	if err := pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT COUNT(*) FROM %s.quotations WHERE customer_id = $1`, schema),
		id).Scan(&client.QuotationCount); err != nil {
		return ManagedClient{}, fmt.Errorf("count client quotations: %w", err)
	}
	return client, nil
}

// DeleteClient removes a client and its addresses. Referenced clients are
// refused: archive history must keep pointing at a real client row.
func (s *Store) DeleteClient(ctx context.Context, id int64) (string, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return "", err
	}
	schema := s.configSchema()
	var name string
	if err := pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT legal_name FROM %s.customers WHERE id = $1`, schema),
		id).Scan(&name); err != nil {
		return "", fmt.Errorf("client %d not found", id)
	}
	var count int64
	if err := pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT COUNT(*) FROM %s.quotations WHERE customer_id = $1`, schema),
		id).Scan(&count); err != nil {
		return "", fmt.Errorf("count client quotations: %w", err)
	}
	if count > 0 {
		return "", fmt.Errorf("cannot delete client %q: %d quotations reference it", name, count)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(
		`DELETE FROM %s.customers WHERE id = $1`, schema), id); err != nil {
		return "", fmt.Errorf("delete client: %w", err)
	}
	return name, nil
}
