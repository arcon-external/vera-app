package main

import (
	"context"
	"embed"
	"fmt"
	"os"

	"vera-app/backend"
	"vera-app/backend/apiclient"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"

	// Register BMP decoder required for Windows application icon
	_ "golang.org/x/image/bmp"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	quotationApp := &backend.Quotation{}

	// The desktop never touches Postgres directly. All data flows through
	// veraserver; VERA_API_URL points at it (Cloud Run primary by default,
	// override for dev, tailnet, or a future custom domain).
	apiBase := os.Getenv("VERA_API_URL")
	if apiBase == "" {
		apiBase = "https://api.arconpentapersada.co.id"
	}
	client := apiclient.New(apiBase, backend.SessionToken)
	client.OnUnauthorized = func() {
		_ = backend.ClearSessionToken()
	}
	backend.ConfigureAPI(client)
	authAPI := backend.NewAuthAPI()
	adminAPI := backend.NewAdminAPI()

	err := wails.Run(&options.App{
		Title:            "Vera",
		Width:            1024,
		Height:           768,
		Assets:           assets,
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup: func(ctx context.Context) {
			adminAPI.SetContext(ctx)
			if err := backend.Startup(quotationApp, ctx); err != nil {
				fmt.Fprintf(os.Stderr, "startup failed: %v\n", err)
				os.Exit(1)
			}
		},
		Bind: []interface{}{
			quotationApp,
			authAPI,
			adminAPI,
		},
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
