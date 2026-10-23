package localseed

import (
	"strings"
	"testing"

	"vera-app/backend/store"
)

func TestValidateTargetAcceptsOneConfirmedLoopbackDatabase(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		dsn    string
		schema string
	}{
		{"loopback", "postgres://vera:fake-password@127.0.0.1:55432/vera_local?sslmode=disable", "vera"},
		{"localhost name", "postgres://vera@localhost/vera_local", "vera"},
		{"ipv6 loopback", "postgres://vera@[::1]/vera_local", "vera"},
		{"unix socket", "postgres:///vera_local?host=/tmp", "vera_local_seed"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			target, err := ValidateTarget(store.Config{DSN: testCase.dsn, Schema: testCase.schema}, "vera_local")
			if err != nil {
				t.Fatalf("confirmed local target refused: %v", err)
			}
			if target.Database != "vera_local" {
				t.Fatalf("database = %q; want vera_local", target.Database)
			}
			if target.Schema != testCase.schema {
				t.Fatalf("schema = %q; want %q", target.Schema, testCase.schema)
			}
		})
	}
}

func TestValidateTargetDefaultsTheSchema(t *testing.T) {
	target, err := ValidateTarget(store.Config{
		DSN: "postgres://vera@127.0.0.1:55432/vera_local",
	}, "vera_local")
	if err != nil {
		t.Fatalf("ValidateTarget failed: %v", err)
	}
	if target.Schema != "vera" {
		t.Fatalf("schema = %q; want the documented default vera", target.Schema)
	}
}

// TestValidateTargetRefusesBeforeAnyWrite is the regression guard for the
// safety rule: a remote, multi-host, unparseable or unconfirmed target must be
// refused by the dry check, which runs before the store is opened (and before
// migrations or fixtures). The integration test additionally proves no schema
// is created for these refusals.
func TestValidateTargetRefusesBeforeAnyWrite(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		dsn       string
		confirmed string
		want      string
	}{
		{
			name:      "remote host",
			dsn:       "postgres://vera:fake-password@remote.invalid/vera_local",
			confirmed: "vera_local",
			want:      "not local",
		},
		{
			name:      "private network host",
			dsn:       "postgres://vera:fake-password@10.0.0.5/vera_local",
			confirmed: "vera_local",
			want:      "not local",
		},
		{
			name:      "multi host",
			dsn:       "postgres://vera@127.0.0.1:55432,remote.invalid:5432/vera_local",
			confirmed: "vera_local",
			want:      "multiple hosts",
		},
		{
			name:      "unparseable",
			dsn:       "not-a-dsn",
			confirmed: "vera_local",
			want:      "cannot parse",
		},
		{
			name:      "no confirmation",
			dsn:       "postgres://vera@127.0.0.1:55432/vera_local",
			confirmed: "",
			want:      "was not confirmed",
		},
		{
			name:      "wrong confirmation",
			dsn:       "postgres://vera@127.0.0.1:55432/vera_local",
			confirmed: "vera_production",
			want:      `resolved database is "vera_local"`,
		},
		{
			name:      "no database named",
			dsn:       "postgres://vera@127.0.0.1:55432/",
			confirmed: "vera_local",
			want:      "names no database",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := ValidateTarget(store.Config{DSN: testCase.dsn, Schema: "vera"}, testCase.confirmed)
			if err == nil {
				t.Fatal("unsafe target accepted; the seed would have proceeded")
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("refusal %q does not name %q", err, testCase.want)
			}
			if strings.Contains(err.Error(), "fake-password") {
				t.Fatalf("refusal leaks the DSN password: %v", err)
			}
			if strings.Contains(err.Error(), "sslmode") {
				t.Fatalf("refusal leaks DSN query values: %v", err)
			}
		})
	}
}

func TestTargetStringIsCredentialFree(t *testing.T) {
	target := Target{Host: "127.0.0.1", Database: "vera_local", Schema: "vera"}
	text := target.String()
	for _, forbidden := range []string{"vera:", "fake-password", "postgres://", "sslmode"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("target string %q contains %q", text, forbidden)
		}
	}
	if !strings.Contains(text, "vera_local") || !strings.Contains(text, "schema vera") {
		t.Fatalf("target string %q does not name the database and schema", text)
	}
}

func TestSanitizeConnectionErrorRedactsThePassword(t *testing.T) {
	got := sanitizeConnectionError(errString("failed to connect with password fake-password"), "fake-password")
	if strings.Contains(got, "fake-password") {
		t.Fatalf("sanitized error leaks the password: %q", got)
	}
	if !strings.Contains(got, "<redacted>") {
		t.Fatalf("sanitized error does not mark the redaction: %q", got)
	}
}

type errString string

func (e errString) Error() string { return string(e) }
