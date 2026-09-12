package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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
	database, err := store.Open(ctx, config)
	if err != nil {
		return err
	}
	defer database.Close()

	server := api.NewServer(database, auth.NewService(database))
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
