package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRequireLocalDevelopmentTargetAcceptsLocalTargets covers the policy:
// exactly one loopback TCP host, or a local Unix socket. "localhost" is
// accepted by name; every other hostname is not consulted at all.
func TestRequireLocalDevelopmentTargetAcceptsLocalTargets(t *testing.T) {
	for _, dsn := range []string{
		"postgres://vera:secret@127.0.0.1:5432/quotes",
		"postgres://vera@localhost:5432/quotes",
		"postgres://vera@LOCALHOST:5432/quotes",
		"postgres://vera@[::1]:5432/quotes",
		"postgres://vera@127.0.0.2:5432/quotes", // the whole 127/8 range is loopback
		"postgres://vera@localhost:5432/quotes?host=127.0.0.1",
		"postgres:///quotes?host=/var/run/postgresql",
		"postgres:///quotes?host=/tmp",
		"host=/var/run/postgresql dbname=quotes user=vera",
	} {
		t.Run(dsn, func(t *testing.T) {
			if err := RequireLocalDevelopmentTarget(Config{DSN: dsn}); err != nil {
				t.Fatalf("local target refused: %v", err)
			}
		})
	}
}

// TestRequireLocalDevelopmentTargetRefusesRemoteAndAmbiguous is the regression
// guard for a copied production environment: a remote, multi-host, or
// unparseable target must be refused before anything opens the store.
func TestRequireLocalDevelopmentTargetRefusesRemoteAndAmbiguous(t *testing.T) {
	for _, dsn := range []string{
		"postgres://vera:fake-password@remote.invalid/dev",
		"postgres://vera@100.93.103.83:5432/quotes",
		"postgres://vera@10.0.0.5:5432/quotes",
		"postgres://vera@db.internal:5432/quotes",
		"postgres://vera@127.0.0.1:5432,remote.invalid:5432/quotes",
		"postgres://vera@localhost,127.0.0.1:5432/quotes",
		"postgres://vera@127.0.0.1:5432,127.0.0.1:5433/quotes?sslmode=disable",
		"postgres://vera:secret@localhost:5432/quotes?host=remote.invalid",
		"host=remote.invalid dbname=dev user=vera password=fake-password",
		"not-a-dsn",
	} {
		t.Run(dsn, func(t *testing.T) {
			if err := RequireLocalDevelopmentTarget(Config{DSN: dsn}); err == nil {
				t.Fatalf("non-local target accepted: %s", dsn)
			}
		})
	}
}

// TestRequireLocalDevelopmentTargetUsesResolvedPGFallback proves the guard
// sees the same configuration the store would open, including the PG* fallback
// and VERA_POSTGRES_DSN precedence.
func TestRequireLocalDevelopmentTargetUsesResolvedPGFallback(t *testing.T) {
	remote := map[string]string{
		"PGHOST":     "remote.invalid",
		"PGPORT":     "5432",
		"PGDATABASE": "dev",
		"PGUSER":     "vera",
		"PGPASSWORD": "fake-password",
		"PGSSLMODE":  "disable",
	}
	config, err := LoadConfig(func(key string) string { return remote[key] })
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if err := RequireLocalDevelopmentTarget(config); err == nil {
		t.Fatal("remote PG* fallback accepted")
	}

	multiHost := map[string]string{
		"PGHOST":     "127.0.0.1,remote.invalid",
		"PGDATABASE": "dev",
		"PGUSER":     "vera",
	}
	config, err = LoadConfig(func(key string) string { return multiHost[key] })
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if err := RequireLocalDevelopmentTarget(config); err == nil {
		t.Fatal("multi-host PG* fallback accepted")
	}

	local := map[string]string{
		"PGHOST":     "127.0.0.1",
		"PGDATABASE": "dev",
		"PGUSER":     "vera",
	}
	config, err = LoadConfig(func(key string) string { return local[key] })
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if err := RequireLocalDevelopmentTarget(config); err != nil {
		t.Fatalf("loopback PG* fallback refused: %v", err)
	}

	precedence := map[string]string{
		"VERA_POSTGRES_DSN": "postgres://vera@127.0.0.1:5432/quotes",
		"PGHOST":            "remote.invalid",
		"PGDATABASE":        "dev",
		"PGUSER":            "vera",
	}
	config, err = LoadConfig(func(key string) string { return precedence[key] })
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if err := RequireLocalDevelopmentTarget(config); err != nil {
		t.Fatalf("explicit loopback DSN should win over remote PG*: %v", err)
	}
}

// TestRequireLocalDevelopmentTargetResolvesServiceFiles covers pgx's service /
// servicefile inputs: a remote service must be refused, a local one accepted,
// and an explicit loopback host must win over a remote service.
func TestRequireLocalDevelopmentTargetResolvesServiceFiles(t *testing.T) {
	dir := t.TempDir()
	serviceFile := filepath.Join(dir, "pg_service.conf")
	body := "[remote]\n" +
		"host=remote.invalid\nport=5432\ndbname=dev\nuser=vera\npassword=fake-password\n\n" +
		"[local]\nhost=127.0.0.1\nport=5432\ndbname=quotes\nuser=vera\n"
	if err := os.WriteFile(serviceFile, []byte(body), 0o600); err != nil {
		t.Fatalf("write service file: %v", err)
	}
	t.Setenv("PGSERVICEFILE", serviceFile)

	if err := RequireLocalDevelopmentTarget(Config{DSN: "service=local"}); err != nil {
		t.Fatalf("local service refused: %v", err)
	}
	err := RequireLocalDevelopmentTarget(Config{DSN: "service=remote"})
	if err == nil {
		t.Fatal("remote service accepted")
	}
	if strings.Contains(err.Error(), "fake-password") {
		t.Fatalf("service refusal leaks the service password: %v", err)
	}
	if err := RequireLocalDevelopmentTarget(Config{DSN: "service=remote servicefile=" + serviceFile}); err == nil {
		t.Fatal("remote servicefile accepted")
	}
	if err := RequireLocalDevelopmentTarget(Config{DSN: "service=remote host=127.0.0.1"}); err != nil {
		t.Fatalf("explicit loopback host should win over the remote service: %v", err)
	}
}

// TestRequireLocalDevelopmentTargetErrorsDoNotLeakCredentials keeps the
// refusal usable in a terminal: no password, userinfo, or raw DSN.
func TestRequireLocalDevelopmentTargetErrorsDoNotLeakCredentials(t *testing.T) {
	for _, dsn := range []string{
		"postgres://vera:fake-password@remote.invalid/dev",
		"host=remote.invalid dbname=dev user=vera password=fake-password",
	} {
		err := RequireLocalDevelopmentTarget(Config{DSN: dsn})
		if err == nil {
			t.Fatalf("expected refusal for %q", dsn)
		}
		message := err.Error()
		for _, leaked := range []string{"fake-password", "vera:", "@remote.invalid", dsn} {
			if strings.Contains(message, leaked) {
				t.Fatalf("error %q leaks %q", message, leaked)
			}
		}
	}
}
