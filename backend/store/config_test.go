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

// Issue #658: VERA_PO_ALLOCATIONS_STRICT is the one switch that flips the
// allocation cutover. Its zero value must stay compatibility mode, so every
// store a test builds with Config{DSN, Schema} behaves like today's server.
func TestLoadConfigParsesStrictPOAllocations(t *testing.T) {
	for _, testCase := range []struct {
		value string
		want  bool
	}{
		{"", false},
		{"false", false},
		{"0", false},
		{"true", true},
		{"1", true},
		{"TRUE", true},
		{" True ", true},
	} {
		t.Run("value="+testCase.value, func(t *testing.T) {
			config, err := LoadConfig(func(key string) string {
				switch key {
				case "VERA_POSTGRES_DSN":
					return "postgres://vera:secret@example.test:5432/quotes?sslmode=require"
				case "VERA_PO_ALLOCATIONS_STRICT":
					return testCase.value
				}
				return ""
			})
			if err != nil {
				t.Fatalf("LoadConfig failed for %q: %v", testCase.value, err)
			}
			if config.StrictPOAllocations != testCase.want {
				t.Errorf("StrictPOAllocations = %v for %q; want %v",
					config.StrictPOAllocations, testCase.value, testCase.want)
			}
		})
	}
}

// A typo in the cutover flag must stop the server, not silently leave it in
// compatibility mode while the operator believes strict is on.
func TestLoadConfigRefusesAnUnknownStrictPOAllocationsValue(t *testing.T) {
	_, err := LoadConfig(func(key string) string {
		switch key {
		case "VERA_POSTGRES_DSN":
			return "postgres://localhost/quotes"
		case "VERA_PO_ALLOCATIONS_STRICT":
			return "yes"
		}
		return ""
	})
	if err == nil {
		t.Fatal("LoadConfig accepted an unknown VERA_PO_ALLOCATIONS_STRICT value")
	}
	if !strings.Contains(err.Error(), "VERA_PO_ALLOCATIONS_STRICT") {
		t.Errorf("error = %q; want it to name the variable", err)
	}
	if !strings.Contains(err.Error(), `"yes"`) {
		t.Errorf("error = %q; want it to name the refused value", err)
	}
}

func containsSecret(value, secret string) bool {
	return len(secret) > 0 && len(value) >= len(secret) && strings.Contains(value, secret)
}
