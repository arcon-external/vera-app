package main

import (
	"context"
	"fmt"

	"vera-app/backend"
	"vera-app/backend/store"
)

// localQuotation builds a Quotation that can prepare pricing and render a PDF
// without a database.
//
// The desktop is wired the same way: the application's own code does the work,
// and four things come from the server rather than the disk --
//
//   - who is asking (the token's account),
//   - the client directory, so a code resolves to a name and address,
//   - the company profile, so the letterhead and brand colours are the real
//     ones rather than the baked-in fallback,
//   - and the embedded templates, which ship inside the binary.
//
// The store is deliberately nil. resolveCustomer falls back to the cached
// directory when there is no store, which is exactly the path the desktop takes.
func (a *app) localQuotation() (*backend.Quotation, error) {
	// client() refuses when there is no token at all, which is the real
	// requirement here. It deliberately does not insist on a session *file*:
	// VERA_TOKEN is the documented way to run this from CI, and those runs have
	// no file.
	client, err := a.client()
	if err != nil {
		return nil, err
	}

	// The signed-in account, so authorisation inside the application's code
	// answers the same way it would in the desktop.
	var me struct {
		ID          int64  `json:"id"`
		Email       string `json:"email"`
		DisplayName string `json:"displayName"`
		Role        string `json:"role"`
	}
	if err := client.Get("/v1/auth/me", &me); err != nil {
		return nil, err
	}
	user := store.AuthUser{
		ID:          me.ID,
		Email:       me.Email,
		DisplayName: me.DisplayName,
		Role:        me.Role,
		IsActive:    true,
	}

	// The application's own helper reads the API for the directory and the
	// profile, so the client has to be registered before they are called.
	backend.ConfigureAPI(client)

	templates := &backend.Quotation{}
	if err := backend.Startup(templates, context.Background()); err != nil {
		return nil, fmt.Errorf("load the embedded templates: %w", err)
	}

	quotation := backend.NewQuotationForRequest(nil, user, templates)

	// Fetch the directory up front: a client code in the input file has to
	// resolve, and doing it here turns "code 9" into "there is no client 9"
	// before any work is done.
	if _, err := quotation.ListCustomers(); err != nil {
		return nil, err
	}
	// The company profile carries the letterhead, the address and the brand
	// colours. A failure here is not fatal -- the baked defaults are a valid
	// document -- but it is worth saying so, because a CLI PDF that looks
	// different from the desktop's is confusing.
	if err := quotation.RefreshCompanyIdentity(); err != nil {
		a.warn("could not load the company profile, using the built-in letterhead: %v", err)
	}
	return quotation, nil
}
