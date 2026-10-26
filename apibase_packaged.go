//go:build !dev || production

package main

// productionAPIURL is where staff actually work: veraserver on Cloud Run.
// Packaged and debug-packaged desktops always use it -- compile-time, not an
// environment lookup -- so a stray VERA_API_URL in a staff member's shell (or a
// project .env left over from development) cannot redirect an installed app.
const productionAPIURL = "https://api.arconpentapersada.co.id"

// apiBaseURL resolves the API endpoint for a packaged build. There is nothing
// to read and nothing to fail: the value is fixed by the build mode. The dev
// implementation lives behind //go:build dev && !production.
func apiBaseURL() (string, error) {
	return productionAPIURL, nil
}
