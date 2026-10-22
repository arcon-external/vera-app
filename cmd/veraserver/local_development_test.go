package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"vera-app/backend/store"
)

func envLookup(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func storeSpy(called *bool, err error) func(context.Context, store.Config) (*store.Store, error) {
	return func(context.Context, store.Config) (*store.Store, error) {
		*called = true
		return nil, err
	}
}

// TestRunWithLocalDevelopmentRefusesRemoteDatabaseBeforeOpen is the executable
// guard for the reported repro: a copied remote DSN must exit before store
// open, so no ping, migration, or bootstrap can run.
func TestRunWithLocalDevelopmentRefusesRemoteDatabaseBeforeOpen(t *testing.T) {
	for name, env := range map[string]map[string]string{
		"VERA_POSTGRES_DSN": {
			"VERA_POSTGRES_DSN": "postgres://fake:fake-password@remote.invalid/dev",
			"VERA_API_ADDR":     "127.0.0.1:8081",
		},
		"PG fallback": {
			"PGHOST":        "remote.invalid",
			"PGPORT":        "5432",
			"PGDATABASE":    "dev",
			"PGUSER":        "fake",
			"PGPASSWORD":    "fake-password",
			"VERA_API_ADDR": "127.0.0.1:8081",
		},
	} {
		t.Run(name, func(t *testing.T) {
			openCalled := false
			err := runWith([]string{"--local-development"}, envLookup(env),
				storeSpy(&openCalled, errors.New("open must not be reached")), 1, 0)
			if err == nil {
				t.Fatal("remote database target was accepted")
			}
			if openCalled {
				t.Fatal("store open was reached before the local target was refused")
			}
			if strings.Contains(err.Error(), "fake-password") ||
				strings.Contains(err.Error(), "fake:fake-password") {
				t.Fatalf("refusal leaks credentials: %v", err)
			}
			if !strings.Contains(err.Error(), "local development") {
				t.Fatalf("refusal does not name the policy: %v", err)
			}
		})
	}
}

// TestRunWithLocalDevelopmentOpensLoopback proves the flag does not block the
// supported local workflow: validation passes and the store open is reached.
func TestRunWithLocalDevelopmentOpensLoopback(t *testing.T) {
	openCalled := false
	sentinel := errors.New("open reached")
	err := runWith([]string{"--local-development"},
		envLookup(map[string]string{
			"VERA_POSTGRES_DSN": "postgres://vera:secret@127.0.0.1:5432/quotes",
			"VERA_API_ADDR":     "127.0.0.1:8081",
		}),
		storeSpy(&openCalled, sentinel), 1, 0)
	if !openCalled {
		t.Fatal("loopback target did not reach store open")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("error = %v; want the store open sentinel", err)
	}
}

// TestRunWithoutLocalDevelopmentKeepsProductionStartup pins that the guard is
// opt-in: the production path still hands its resolved config straight to the
// store opener, remote or not.
func TestRunWithoutLocalDevelopmentKeepsProductionStartup(t *testing.T) {
	openCalled := false
	sentinel := errors.New("open reached")
	err := runWith(nil,
		envLookup(map[string]string{
			"VERA_POSTGRES_DSN": "postgres://vera@remote.invalid:5432/prod",
		}),
		storeSpy(&openCalled, sentinel), 1, 0)
	if !openCalled {
		t.Fatal("production startup was diverted before store open")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("error = %v; want the store open sentinel", err)
	}
}

// TestServerAddrDefaultsToProductionPort pins Cloud Run's bare :8080 default
// and the explicit override the launcher uses.
func TestServerAddrDefaultsToProductionPort(t *testing.T) {
	if got := serverAddr(envLookup(nil)); got != ":8080" {
		t.Fatalf("serverAddr() = %q; want the production default :8080", got)
	}
	if got := serverAddr(envLookup(map[string]string{"VERA_API_ADDR": "127.0.0.1:8081"})); got != "127.0.0.1:8081" {
		t.Fatalf("serverAddr() = %q; want the explicit override", got)
	}
}

func TestParseServerFlags(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    []string
		want    bool
		wantErr bool
	}{
		{name: "default", args: nil, want: false},
		{name: "long flag", args: []string{"--local-development"}, want: true},
		{name: "short flag", args: []string{"-local-development"}, want: true},
		{name: "unknown flag", args: []string{"--remote"}, wantErr: true},
		{name: "positional argument", args: []string{"serve"}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseServerFlags(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected a flag error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseServerFlags error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("localDevelopment = %v; want %v", got, tc.want)
			}
		})
	}
}

// TestRetryBudgetIsBounded keeps the test seam honest: runWith must use the
// attempts/delay it is given, so a production-path test cannot sleep for 30s.
func TestRetryBudgetIsBounded(t *testing.T) {
	openCalled := false
	calls := 0
	err := runWith(nil,
		envLookup(map[string]string{"VERA_POSTGRES_DSN": "postgres://vera@remote.invalid/db"}),
		func(context.Context, store.Config) (*store.Store, error) {
			openCalled = true
			calls++
			return nil, errors.New("connection refused")
		}, 2, time.Millisecond)
	if err == nil {
		t.Fatal("expected connect failure")
	}
	if !openCalled || calls != 2 {
		t.Fatalf("open calls = %d; want 2", calls)
	}
}
