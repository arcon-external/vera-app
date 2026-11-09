package api

import (
	"context"

	"vera-app/backend/store"
)

// organizationContext attaches the single seeded organization to a request.
//
// The product is single-tenant today, but the #465 boundary is real: store
// operations call RequireOrg and every PO query filters by the resolved id.
// When a real org switcher arrives this helper becomes the one place that
// chooses the caller's organization instead of inventing a second bypass.
func (s *Server) organizationContext(requestContext context.Context) (context.Context, error) {
	org, err := s.database.DefaultOrganization(requestContext)
	if err != nil {
		return requestContext, err
	}
	return store.ContextWithOrg(requestContext, org.ID), nil
}
