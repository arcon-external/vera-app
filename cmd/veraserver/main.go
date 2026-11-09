package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"vera-app/backend"
	"vera-app/backend/api"
	"vera-app/backend/auth"
	"vera-app/backend/store"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "veraserver: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	return runWith(args, os.Getenv, store.Open, 10, 3*time.Second)
}

// defaultAPIAddr is Cloud Run's contract: the bare production entrypoint binds
// every interface on :8080. scripts/dev-api.sh overrides it to loopback.
const defaultAPIAddr = ":8080"

func serverAddr(getenv func(string) string) string {
	if addr := getenv("VERA_API_ADDR"); addr != "" {
		return addr
	}
	return defaultAPIAddr
}

// parseServerFlags accepts only the flags the server owns. The local guard is
// opt-in so Cloud Run's bare invocation keeps its existing behaviour.
func parseServerFlags(args []string) (bool, error) {
	flags := flag.NewFlagSet("veraserver", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	localDevelopment := flags.Bool("local-development", false,
		"refuse non-local Postgres targets before opening the store")
	if err := flags.Parse(args); err != nil {
		return false, fmt.Errorf("parse veraserver flags: %w", err)
	}
	if flags.NArg() > 0 {
		return false, fmt.Errorf("unexpected veraserver argument %q", flags.Arg(0))
	}
	return *localDevelopment, nil
}

// runWith keeps the startup sequence testable without a database. Only
// --local-development gates the local target policy; a bare production launch
// behaves exactly as before.
func runWith(args []string, getenv func(string) string,
	open func(context.Context, store.Config) (*store.Store, error),
	maxAttempts int, retryDelay time.Duration) error {
	localDevelopment, err := parseServerFlags(args)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	addr := serverAddr(getenv)

	config, err := store.LoadConfig(getenv)
	if err != nil {
		return err
	}
	if localDevelopment {
		// Before any network, migration, or bootstrap: a copied production
		// environment must be refused here, not discovered by opening it.
		if err := store.RequireLocalDevelopmentTarget(config); err != nil {
			return err
		}
	}
	// Neon sleeps after 5 idle minutes; the first connection attempt of a
	// cold deploy may catch it waking. Retry briefly so sleepy databases
	// don't fail rollouts, then fail fast for real outages.
	database, err := openStoreWithRetry(ctx, config, maxAttempts, retryDelay, open)
	if err != nil {
		return err
	}
	defer database.Close()
	// Session rows remain for 30 days after their expiry or revocation for audit.
	// Cleanup is best-effort: an outage must not prevent the API from starting.
	cleanStaleSessions(ctx, database)
	go cleanupSessionsDaily(ctx, database)

	bootstrapName := getenv("VERA_BOOTSTRAP_ADMIN_NAME")
	bootstrapEmail := getenv("VERA_BOOTSTRAP_ADMIN_EMAIL")
	if bootstrapName == "" {
		bootstrapName = bootstrapEmail
	}
	if err := ensureBootstrap(ctx, database, bootstrapEmail, bootstrapName,
		getenv("VERA_BOOTSTRAP_ADMIN_PASSWORD")); err != nil {
		return fmt.Errorf("auth bootstrap failed: %w", err)
	}

	templates := &backend.Quotation{}
	if err := backend.Startup(templates, ctx); err != nil {
		return fmt.Errorf("load quotation templates: %w", err)
	}

	server := api.NewServer(database, auth.NewService(database), templates)
	server.SetDSN(config.DSN)

	// Signing is required: without it documents cannot be verified, so a
	// misconfigured deploy must fail here rather than print unverifiable
	// quotations. The key arrives through the environment only.
	signer, err := backend.LoadSignerFromEnvironment(ctx, database)
	if err != nil {
		return fmt.Errorf("signing setup failed: %w", err)
	}
	server.SetSigner(signer)

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           server,
		ReadHeaderTimeout: 10 * time.Second,
	}

	stopped := make(chan error, 1)
	go func() {
		fmt.Printf("veraserver: listening on %s\n", addr)
		stopped <- httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdown)
	case err := <-stopped:
		return err
	}
}

func cleanStaleSessions(ctx context.Context, database *store.Store) {
	cleanupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := database.DeleteStaleSessions(cleanupCtx, time.Now().Add(-30*24*time.Hour)); err != nil {
		fmt.Fprintf(os.Stderr, "veraserver: session cleanup failed: %v\n", err)
	}
}

func cleanupSessionsDaily(ctx context.Context, database *store.Store) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cleanStaleSessions(ctx, database)
		}
	}
}

// ensureBootstrap creates the first admin from environment configuration when
// no users exist. It is a no-op once any user exists.
func ensureBootstrap(ctx context.Context, database *store.Store, email, displayName, password string) error {
	count, err := database.CountUsers(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	if email == "" || password == "" {
		return fmt.Errorf("no users exist: set VERA_BOOTSTRAP_ADMIN_EMAIL and VERA_BOOTSTRAP_ADMIN_PASSWORD to create the first admin")
	}
	service := auth.NewService(database)
	if _, err := service.Bootstrap(ctx, email, displayName, password); err != nil {
		return err
	}
	return nil
}
