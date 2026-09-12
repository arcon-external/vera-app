package store

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
)

const defaultSchema = "vera"

var schemaPattern = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

// Config contains the non-secret database settings used by the persistence
// layer. DSN is read from the process environment and is never logged.
type Config struct {
	DSN    string
	Schema string
}

// LoadConfig reads the primary VERA_POSTGRES_DSN setting, falling back to the
// standard PG* environment variables supported by libpq and Postgres tools.
// getenv is injectable so configuration can be tested without mutating the
// process environment.
func LoadConfig(getenv func(string) string) (Config, error) {
	dsn := strings.TrimSpace(getenv("VERA_POSTGRES_DSN"))
	if dsn == "" {
		dsn = buildDSNFromEnvironment(getenv)
	}
	if dsn == "" {
		return Config{}, fmt.Errorf("Postgres configuration is missing: set VERA_POSTGRES_DSN or PGHOST/PGDATABASE/PGUSER")
	}

	schema := strings.TrimSpace(getenv("VERA_POSTGRES_SCHEMA"))
	if schema == "" {
		schema = defaultSchema
	}
	if !schemaPattern.MatchString(schema) {
		return Config{}, fmt.Errorf("invalid Postgres schema %q: use lowercase letters, digits, and underscores only", schema)
	}

	return Config{DSN: dsn, Schema: schema}, nil
}

func LoadConfigFromEnvironment() (Config, error) {
	return LoadConfig(os.Getenv)
}

func buildDSNFromEnvironment(getenv func(string) string) string {
	host := strings.TrimSpace(getenv("PGHOST"))
	database := strings.TrimSpace(getenv("PGDATABASE"))
	user := strings.TrimSpace(getenv("PGUSER"))
	if host == "" || database == "" || user == "" {
		return ""
	}

	port := strings.TrimSpace(getenv("PGPORT"))
	if port == "" {
		port = "5432"
	}
	sslMode := strings.TrimSpace(getenv("PGSSLMODE"))
	if sslMode == "" {
		sslMode = "require"
	}

	values := url.Values{}
	values.Set("sslmode", sslMode)

	connectionURL := &url.URL{
		Scheme:   "postgres",
		Host:     host + ":" + port,
		Path:     "/" + database,
		RawQuery: values.Encode(),
	}
	if password := getenv("PGPASSWORD"); password != "" {
		connectionURL.User = url.UserPassword(user, password)
	} else {
		connectionURL.User = url.User(user)
	}

	return connectionURL.String()
}

// RedactedDSN is safe for diagnostics because the password is removed before
// the value is returned. Callers must never log Config.DSN directly.
func (c Config) RedactedDSN() string {
	parsed, err := url.Parse(c.DSN)
	if err != nil {
		return "<invalid Postgres DSN>"
	}
	if parsed.User != nil {
		username := parsed.User.Username()
		parsed.User = url.User(username)
	}
	return parsed.String()
}
