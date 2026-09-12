package main

import (
	"context"
	"embed"
	"fmt"
	"os"

	"vera-app/backend"
	"vera-app/backend/store"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"

	// Register BMP decoder required for Windows application icon
	_ "golang.org/x/image/bmp"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	quotationApp := &backend.Quotation{}
	database, err := store.OpenFromEnvironment(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "database startup failed: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()
	quotationApp.SetStore(database)

	err = wails.Run(&options.App{
		Title:            "Vera",
		Width:            1024,
		Height:           768,
		Assets:           assets,
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup: func(ctx context.Context) {
			if err := backend.Startup(quotationApp, ctx); err != nil {
				fmt.Fprintf(os.Stderr, "startup failed: %v\n", err)
				os.Exit(1)
			}
		},
		Bind: []interface{}{
			quotationApp,
		},
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
