package store

import (
	"strings"
	"testing"
)

func TestLoadConfigUsesDSNAndDefaultSchema(t *testing.T) {
	config, err := LoadConfig(func(key string) string {
		if key == "VERA_POSTGRES_DSN" {
			return "postgres://vera:secret@example.test:5432/quotes?sslmode=require"
		}
		return ""
	})
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if config.DSN == "" {
		t.Fatal("DSN is empty")
	}
	if config.Schema != "vera" {
		t.Errorf("schema = %q; want vera", config.Schema)
	}
	if config.RedactedDSN() == config.DSN || config.RedactedDSN() == "" {
		t.Errorf("RedactedDSN = %q; expected a redacted non-empty DSN", config.RedactedDSN())
	}
	if config.RedactedDSN() == "" || containsSecret(config.RedactedDSN(), "secret") {
		t.Errorf("RedactedDSN leaks password: %q", config.RedactedDSN())
	}
}

func TestLoadConfigBuildsDSNFromPGEnvironment(t *testing.T) {
	values := map[string]string{
		"PGHOST":     "localhost",
		"PGPORT":     "5432",
		"PGDATABASE": "quotes",
		"PGUSER":     "vera",
		"PGPASSWORD": "secret",
		"PGSSLMODE":  "disable",
	}
	config, err := LoadConfig(func(key string) string { return values[key] })
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if config.DSN == "" || containsSecret(config.DSN, "secret") == false {
		t.Error("fallback DSN did not include the configured password")
	}
	if config.RedactedDSN() == config.DSN || containsSecret(config.RedactedDSN(), "secret") {
		t.Errorf("RedactedDSN leaks fallback password: %q", config.RedactedDSN())
	}
}

func TestLoadConfigRequiresConnectionSettings(t *testing.T) {
	_, err := LoadConfig(func(string) string { return "" })
	if err == nil {
		t.Fatal("expected missing Postgres configuration error")
	}
}

func TestLoadConfigRejectsUnsafeSchema(t *testing.T) {
	_, err := LoadConfig(func(key string) string {
		if key == "VERA_POSTGRES_DSN" {
			return "postgres://localhost/quotes"
		}
		if key == "VERA_POSTGRES_SCHEMA" {
			return "vera;drop table quotations"
		}
		return ""
	})
	if err == nil {
		t.Fatal("expected unsafe schema error")
	}
}

func containsSecret(value, secret string) bool {
	return len(secret) > 0 && len(value) >= len(secret) && strings.Contains(value, secret)
}
