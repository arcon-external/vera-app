package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Organization is the thin tenancy identity new PO tables reference (issue
// #465). The fleet stays single-tenant: exactly one row exists, seeded by
// migration 029 from the company profile. Existing tables and queries are
// untouched -- no org_id is retrofitted, no RLS, no memberships. Later
// tickets carry this identity on new tenant-owned rows.
type Organization struct {
	ID   string
	Name string
}

// orgContextKey carries the organization identity for new tenant-owned
// operations. It is unexported so only ContextWithOrg can set it; readers
// go through OrgIDFromContext or RequireOrg.
type orgContextKey struct{}

// ContextWithOrg attaches an organization identity to the context. New
// tenant-owned store operations require it via RequireOrg.
func ContextWithOrg(ctx context.Context, orgID string) context.Context {
	return context.WithValue(ctx, orgContextKey{}, orgID)
}

// OrgIDFromContext returns the organization identity on the context, or an
// error when the caller never attached one. A blank identity is the same as
// none: it cannot name a tenant.
func OrgIDFromContext(ctx context.Context) (string, error) {
	orgID, _ := ctx.Value(orgContextKey{}).(string)
	if strings.TrimSpace(orgID) == "" {
		return "", fmt.Errorf("organization context is required for this operation")
	}
	return orgID, nil
}

func scanOrganization(row pgx.Row) (Organization, error) {
	var org Organization
	if err := row.Scan(&org.ID, &org.Name); err != nil {
		return Organization{}, err
	}
	return org, nil
}

// DefaultOrganization returns the single seeded organization. It errors
// unless exactly one row exists, so a missing or duplicated seed is loud
// rather than silently picked.
func (s *Store) DefaultOrganization(ctx context.Context) (Organization, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return Organization{}, err
	}
	org, err := scanOrganization(pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT id, name FROM %s.organizations`, s.configSchema())))
	if err != nil {
		return Organization{}, fmt.Errorf("default organization: %w", err)
	}
	var count int
	if err := pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT COUNT(*) FROM %s.organizations`, s.configSchema())).Scan(&count); err != nil {
		return Organization{}, fmt.Errorf("count organizations: %w", err)
	}
	if count != 1 {
		return Organization{}, fmt.Errorf("want exactly 1 organization, found %d", count)
	}
	return org, nil
}

// RequireOrg resolves the context's organization identity to its row,
// refusing the operation when the context carries none, names an unknown
// organization, or names a suspended one. New tenant-owned operations call
// this first; existing quotation operations never do.
func (s *Store) RequireOrg(ctx context.Context) (Organization, error) {
	orgID, err := OrgIDFromContext(ctx)
	if err != nil {
		return Organization{}, err
	}
	pool, err := s.poolOrError()
	if err != nil {
		return Organization{}, err
	}
	var org Organization
	var suspended bool
	err = pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT id, name, suspended_at IS NOT NULL FROM %s.organizations WHERE id = $1`,
		s.configSchema()), orgID).Scan(&org.ID, &org.Name, &suspended)
	if err != nil {
		return Organization{}, fmt.Errorf("unknown organization: %w", err)
	}
	if suspended {
		return Organization{}, fmt.Errorf("organization is suspended")
	}
	return org, nil
}
