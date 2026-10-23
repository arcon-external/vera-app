// Package localseed builds the fictional Quotation and Purchase Order set used
// for manual acceptance testing on a developer's local database (#500).
//
// It is deliberately not an HTTP feature and not a migration. Nothing in this
// package is reachable from the deployed API: the only entry point is the
// standalone `veraseed` command, which refuses anything that is not an
// explicitly confirmed local Postgres target before it migrates or writes.
package localseed

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"vera-app/backend/store"
)

// Target is the resolved, confirmed local database. It carries no credentials
// and no DSN: it is what the command prints so a developer can see which
// database they are about to fill with mock records.
type Target struct {
	Host     string
	Database string
	Schema   string
}

// String renders the target without a user or a password.
func (t Target) String() string {
	host := t.Host
	if strings.HasPrefix(host, "/") {
		host = "unix:" + host
	}
	if host == "" {
		host = "<unknown>"
	}
	return fmt.Sprintf("%s/%s (schema %s)", host, t.Database, t.Schema)
}

// ValidateTarget is the dry, pre-connection half of the refusal policy. It
// answers two questions before anything opens a pool or runs a migration:
//
//   - is the resolved Postgres target exactly one local endpoint? That check
//     is store.RequireLocalDevelopmentTarget, the same transport guard
//     scripts/dev-api.sh relies on.
//   - did the operator explicitly confirm the resolved database name? A
//     loopback host alone is not enough: a local tunnel or a restored copy
//     can still be the wrong database, so --target must name the database
//     pgx resolved, exactly.
//
// Errors name the host or the two database names and never echo the DSN. The
// password, if any, cannot reach a message because this function never
// formats the DSN.
func ValidateTarget(config store.Config, confirmedDatabase string) (Target, error) {
	if err := store.RequireLocalDevelopmentTarget(config); err != nil {
		return Target{}, fmt.Errorf("%w; refusing before any migration or write", err)
	}
	parsed, err := pgx.ParseConfig(config.DSN)
	if err != nil {
		return Target{}, fmt.Errorf("local seed target: cannot parse the resolved Postgres configuration; refusing before any migration or write")
	}
	if strings.TrimSpace(parsed.Database) == "" {
		return Target{}, fmt.Errorf("local seed target: the resolved Postgres configuration names no database; refusing before any migration or write")
	}
	confirmed := strings.TrimSpace(confirmedDatabase)
	if confirmed == "" {
		return Target{}, fmt.Errorf("local seed target: %q was not confirmed; pass --target with the resolved database name; refusing before any migration or write", parsed.Database)
	}
	if confirmed != parsed.Database {
		return Target{}, fmt.Errorf("local seed target: the resolved database is %q, not the confirmed %q; refusing before any migration or write", parsed.Database, confirmed)
	}
	schema := strings.TrimSpace(config.Schema)
	if schema == "" {
		schema = "vera"
	}
	host := parsed.Host
	if host == "" && len(parsed.Fallbacks) > 0 {
		host = parsed.Fallbacks[0].Host
	}
	return Target{Host: host, Database: parsed.Database, Schema: schema}, nil
}

// verifyConnectedTarget is the second half: after a real connection exists but
// before the store migrates, ask the server which database it is. A DSN and
// the database a server actually opened can disagree (a service-file entry, a
// pooler, a copied environment), and the confirmation must bind to what the
// server reports, not to what the string said.
func verifyConnectedTarget(ctx context.Context, parsed *pgx.ConnConfig, confirmedDatabase string) error {
	conn, err := pgx.ConnectConfig(ctx, parsed)
	if err != nil {
		return fmt.Errorf("local seed target: cannot connect to the confirmed local database %q: %s",
			confirmedDatabase, sanitizeConnectionError(err, parsed.Password))
	}
	defer func() { _ = conn.Close(ctx) }()

	var database string
	if err := conn.QueryRow(ctx, "SELECT current_database()").Scan(&database); err != nil {
		return fmt.Errorf("local seed target: cannot read the connected database name; refusing before any migration or write")
	}
	if database != confirmedDatabase {
		return fmt.Errorf("local seed target: the server opened database %q, not the confirmed %q; refusing before any migration or write", database, confirmedDatabase)
	}
	return nil
}

// sanitizeConnectionError keeps a connection failure useful without ever
// repeating the password. pgx does not normally print the password, but the
// seed must not depend on that.
func sanitizeConnectionError(err error, password string) string {
	if err == nil {
		return "unknown error"
	}
	message := err.Error()
	if password != "" {
		message = strings.ReplaceAll(message, password, "<redacted>")
	}
	return message
}
