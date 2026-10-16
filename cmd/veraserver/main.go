package main

import (
	"context"
	"fmt"
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
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "veraserver: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	addr := os.Getenv("VERA_API_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	config, err := store.LoadConfigFromEnvironment()
	if err != nil {
		return err
	}
	// Neon sleeps after 5 idle minutes; the first connection attempt of a
	// cold deploy may catch it waking. Retry briefly so sleepy databases
	// don't fail rollouts, then fail fast for real outages.
	database, err := openStoreWithRetry(ctx, config, 10, 3*time.Second, store.Open)
	if err != nil {
		return err
	}
	defer database.Close()
	// Session rows remain for 30 days after their expiry or revocation for audit.
	// Cleanup is best-effort: an outage must not prevent the API from starting.
	cleanStaleSessions(ctx, database)
	go cleanupSessionsDaily(ctx, database)

	bootstrapName := os.Getenv("VERA_BOOTSTRAP_ADMIN_NAME")
	bootstrapEmail := os.Getenv("VERA_BOOTSTRAP_ADMIN_EMAIL")
	if bootstrapName == "" {
		bootstrapName = bootstrapEmail
	}
	if err := ensureBootstrap(ctx, database, bootstrapEmail, bootstrapName,
		os.Getenv("VERA_BOOTSTRAP_ADMIN_PASSWORD")); err != nil {
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
