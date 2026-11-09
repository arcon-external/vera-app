package store

import (
	"fmt"
	"net"
	"strings"

	"github.com/jackc/pgx/v5"
)

// RequireLocalDevelopmentTarget refuses a database configuration that does not
// resolve to exactly one local endpoint. It is called by the API server only
// when it was started with --local-development, before the store is opened, so
// a copied production environment (or a stray remote PG* variable) can never
// ping, migrate, or bootstrap anything.
//
// The policy is deliberately transport-level: one loopback TCP host or one
// local Unix socket. "localhost" is trusted by name and never resolved; any
// other hostname is refused rather than looked up, so DNS or /etc/hosts cannot
// quietly redirect development. This is not #500's stronger seed allowlist: it
// answers "is this endpoint local?", while #500 must still refuse a local
// database that is not the recognised development one before writing fixtures.
func RequireLocalDevelopmentTarget(config Config) error {
	parsed, err := pgx.ParseConfig(config.DSN)
	if err != nil {
		return fmt.Errorf("local development database target: cannot parse the resolved Postgres configuration")
	}
	// pgx reports the primary host as a single fallback when there is one, so
	// more than one fallback means a multi-host DSN. A multi-host DSN with
	// explicit ports and sslmode can still compress to one fallback, so a
	// fallback that is not the same endpoint is the second signal. Refuse both
	// cases even when every host is loopback: this guard must see every target.
	if len(parsed.Fallbacks) > 1 || (len(parsed.Fallbacks) == 1 &&
		(!strings.EqualFold(parsed.Fallbacks[0].Host, parsed.Host) ||
			parsed.Fallbacks[0].Port != parsed.Port)) {
		return fmt.Errorf("local development database target: multiple hosts are not allowed; use one loopback host or a local Unix socket")
	}
	if err := requireLocalHost(parsed.Host); err != nil {
		return err
	}
	for _, fallback := range parsed.Fallbacks {
		if err := requireLocalHost(fallback.Host); err != nil {
			return err
		}
	}
	return nil
}

func requireLocalHost(host string) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return fmt.Errorf("local development database target: host is empty; set an explicit loopback host or a local Unix socket")
	}
	// pgx reports a Unix socket as the socket directory itself.
	if strings.HasPrefix(host, "/") {
		return nil
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil && ip.IsLoopback() {
		return nil
	}
	// Echo only the host. Userinfo and query values never reach this message.
	return fmt.Errorf("local development database target: host %q is not local; use localhost, a loopback address, or a local Unix socket", host)
}
