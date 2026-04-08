package main

import (
	"context"
	"embed"
	"vera-app/backend"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var icon []byte

func main() {
	quotationHandler := &backend.Quotation{}

	// Create application with options
	err := wails.Run(&options.App{
		Title:            "wails-events",
		Width:            1024,
		Height:           768,
		Assets:           assets,
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup: func(ctx context.Context) {
			backend.Startup(quotationHandler, ctx)
		},
		Bind: []interface{}{
			quotationHandler,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
