package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"vera-app/backend"
	"vera-app/backend/store"
	"vera-app/backend/theme"
)

// previewHandler drives the preview handler directly, the way
// company_mirrors_test.go drives updateCompanyProfile: an unconfigured store,
// an admin user already on the context, and no middleware. That keeps these
// checks about the handler's own behaviour — surface, colours, headers,
// template — runnable everywhere, and leaves the route's gates to
// TestPreviewRouteIsAdminOnly, which needs a real session.
func previewHandler(t *testing.T, server *Server, path string) (int, string, http.Header) {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	recorder := httptest.NewRecorder()
	// The server's own echo instance: context.Render resolves the renderer
	// from the echo that owns the context, and a fresh one has none.
	context := server.echo.NewContext(request, recorder)
	context.Set("user", store.AuthUser{Role: "admin"})
	if err := server.portalPreview(context); err != nil {
		t.Fatalf("portalPreview(%s): %v", path, err)
	}
	return recorder.Code, recorder.Body.String(), recorder.Header()
}

// previewServer builds a server whose store is deliberately unconfigured: the
// handler's one database read (the issuer name) then fails, which is a path it
// must survive by falling back to the template's default issuer.
func previewServer() *Server {
	return NewServer(&store.Store{}, nil, nil)
}

// previewPathWith adds parameters to a client-built preview path.
func previewPathWith(verdict, extra string) string {
	path := backend.PortalPreviewPath(verdict, "#1A237E", "#284AF3", "#D6E4FF", "#1a1a1a")
	if extra == "" {
		return path
	}
	return path + "&" + extra
}

// verdictMarks are the template's per-state verdict labels. They are unique to
// a state, unlike the verdict-* class names, which the stylesheet defines for
// all five in every page.
var verdictMarks = map[string]string{
	verdictValid:       "Verified",
	verdictSuperseded:  "Superseded",
	verdictMismatch:    "Mismatch",
	verdictNotFound:    "Not found",
	verdictUnavailable: "Unavailable",
}

// goSources returns every non-test Go source in this package, for the marker
// test. Test files are excluded because this file carries the marker by
// necessity: what must not carry it is the production code, which is what
// would let a hand-built markup copy pass as the real template.
func goSources(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("list package sources: %v", err)
	}
	var sources []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		sources = append(sources, string(raw))
	}
	return sources
}

// TestPreviewAcceptsTheClientPath pins the two sides of one surface: the
// exported client builder and the endpoint's accepted parameters. A name that
// drifts on either side fails here as a 400 rather than silently falling back
// to the default palette.
func TestPreviewAcceptsTheClientPath(t *testing.T) {
	code, body, _ := previewHandler(t, previewServer(), previewPathWith(verdictValid, ""))
	if code != http.StatusOK {
		t.Fatalf("client-built path = %d; want 200 (%s)", code, body)
	}
}

// TestPreviewRefusesUnknownParameters keeps the surface closed. A company
// edits four colours and a verdict; anything else in the query is refused
// rather than ignored, so the endpoint cannot quietly accept a fifth editable
// thing about the portal.
func TestPreviewRefusesUnknownParameters(t *testing.T) {
	server := previewServer()
	for _, extra := range []string{
		"themePDeep=%23112233",
		"legalName=Other+Company",
		"label=Subject",
		"issuer=Someone+Else",
	} {
		code, _, _ := previewHandler(t, server, previewPathWith(verdictValid, extra))
		if code != http.StatusBadRequest {
			t.Errorf("preview with %q = %d; want 400", extra, code)
		}
	}
}

// TestPreviewRefusesUnknownVerdicts proves the toggle cannot name a state the
// template has no class for.
func TestPreviewRefusesUnknownVerdicts(t *testing.T) {
	server := previewServer()
	path := backend.PortalPreviewPath("tampered", "#1A237E", "#284AF3", "#D6E4FF", "#1a1a1a")
	code, _, _ := previewHandler(t, server, path)
	if code != http.StatusBadRequest {
		t.Errorf("preview with an unknown verdict = %d; want 400", code)
	}
}

// TestPreviewRendersEveryState proves the five states render, that each
// carries its own verdict class and headline, and that the document shown is
// identical across them — the toggle switches the verdict, never the data,
// which is what makes the five states comparable.
func TestPreviewRendersEveryState(t *testing.T) {
	server := previewServer()
	wantClass := map[string]string{
		verdictValid:       "verdict-valid",
		verdictSuperseded:  "verdict-superseded",
		verdictMismatch:    "verdict-mismatch",
		verdictNotFound:    "verdict-notfound",
		verdictUnavailable: "verdict-unavailable",
	}
	seenHeadline := map[string]bool{}
	constant := []string{"15CC260001", "Rev 00", "14 October 2026", "PT Contoh Nusantara", "Rp"}
	for _, verdict := range portalPreviewVerdicts {
		code, body, _ := previewHandler(t, server, previewPathWith(verdict, ""))
		if code != http.StatusOK {
			t.Fatalf("preview(%s) = %d; want 200", verdict, code)
		}
		if !strings.Contains(body, wantClass[verdict]) {
			t.Errorf("preview(%s) carries no %s class", verdict, wantClass[verdict])
		}
		// The card shows this state's mark and no other state's. The stylesheet
		// defines every verdict class in every page, so the class names cannot
		// prove which state was chosen — the mark can.
		if !strings.Contains(body, verdictMarks[verdict]) {
			t.Errorf("preview(%s) does not show the %q mark", verdict, verdictMarks[verdict])
		}
		for _, other := range portalPreviewVerdicts {
			if other != verdict && strings.Contains(body, verdictMarks[other]) {
				t.Errorf("preview(%s) also shows the %q mark", verdict, verdictMarks[other])
			}
		}
		headline, _ := verifyTitles(verdict)
		if !strings.Contains(body, headline) {
			t.Errorf("preview(%s) is missing its headline %q", verdict, headline)
		}
		if seenHeadline[headline] {
			t.Errorf("two verdicts share the headline %q", headline)
		}
		seenHeadline[headline] = true
		// The three states that carry a record show the same document; the two
		// that do not (not found, unavailable) must show none of it — a page
		// with no document behind it has no fields to print.
		hasRecord := verdict == verdictValid || verdict == verdictSuperseded || verdict == verdictMismatch
		for _, want := range constant {
			if hasRecord && !strings.Contains(body, want) {
				t.Errorf("preview(%s) missing the constant document value %q", verdict, want)
			}
			if !hasRecord && strings.Contains(body, want) {
				t.Errorf("preview(%s) shows the document value %q with no record behind it", verdict, want)
			}
		}
	}
}

// TestPreviewBrandColoursMoveEveryState proves a colour change is visible in
// every state, including the negative ones — a rebrand that only showed on the
// valid page would hide exactly the states a customer reads in a hurry.
func TestPreviewBrandColoursMoveEveryState(t *testing.T) {
	server := previewServer()
	const brand = "#0B3D91"
	for _, verdict := range portalPreviewVerdicts {
		path := backend.PortalPreviewPath(verdict, brand, brand, brand, brand)
		_, body, _ := previewHandler(t, server, path)
		if !strings.Contains(body, "--deep-blue: "+brand+";") {
			t.Errorf("preview(%s) did not take the typed deep colour", verdict)
		}
		if !strings.Contains(body, "--accent-blue: "+brand+";") {
			t.Errorf("preview(%s) did not take the typed accent colour", verdict)
		}
	}
}

// TestPreviewStatusColoursNeverMove is the criterion that matters most: a
// brand colour must not be able to restyle a verdict, because a page that
// cannot be mistaken for a verdict it is not is the whole product. The status
// set is literal in the template, so it must survive every brand colour.
func TestPreviewStatusColoursNeverMove(t *testing.T) {
	server := previewServer()
	// A brand set chosen to sit close to the status hues.
	for _, brand := range []string{"#1B7F3B", "#B3261E", "#9A6100", "#5b6478"} {
		_, body, _ := previewHandler(t, server, backend.PortalPreviewPath(verdictValid, brand, brand, brand, brand))
		for _, want := range []string{
			"--ok: #1B7F3B;", "--bad: #B3261E;", "--warn: #9A6100;", "--neutral: #5b6478;",
			".verdict-valid { border-color: #bfe0c8; background: var(--ok-bg); }",
			".verdict-mismatch { border-color: #f0c2be; background: var(--bad-bg); }",
		} {
			if !strings.Contains(body, want) {
				t.Errorf("brand %s restyled a verdict status: %q missing", brand, want)
			}
		}
	}
}

// TestPreviewBadColourFallsBackRatherThanBreaking proves a non-hex value
// cannot reach the template as an empty colour, which would drop the page's
// borders. The rule mirrors theme.fallback on the write path.
func TestPreviewBadColourFallsBackRatherThanBreaking(t *testing.T) {
	server := previewServer()
	for _, bad := range []string{"", "red", "#12345", "#12345g", "javascript:alert(1)", "%23zzz"} {
		_, body, _ := previewHandler(t, server, backend.PortalPreviewPath(verdictValid, bad, bad, bad, bad))
		if !strings.Contains(body, "--deep-blue: "+theme.DefaultDeep+";") {
			t.Errorf("preview with deep=%q did not fall back to the default", bad)
		}
		if !strings.Contains(body, "--accent-blue: "+theme.DefaultAccent+";") {
			t.Errorf("preview with accent=%q did not fall back to the default", bad)
		}
		if strings.Contains(body, "--deep-blue: ;") {
			t.Errorf("preview with deep=%q emitted an empty colour", bad)
		}
	}
}

// TestPreviewIsFrameableAndTheLivePortalIsNot is the header rule, asserted in
// both directions. The preview has to load inside the settings iframe; a
// customer-facing verification page must never be framed by anyone.
func TestPreviewIsFrameableAndTheLivePortalIsNot(t *testing.T) {
	server := previewServer()
	_, _, previewHeaders := previewHandler(t, server, previewPathWith(verdictValid, ""))
	if got := previewHeaders.Get("X-Frame-Options"); got != "SAMEORIGIN" {
		t.Errorf("preview X-Frame-Options = %q; want SAMEORIGIN", got)
	}
	if csp := previewHeaders.Get("Content-Security-Policy"); !strings.Contains(csp, "frame-ancestors 'self'") {
		t.Errorf("preview CSP = %q; want frame-ancestors 'self'", csp)
	}
	if strings.Contains(previewHeaders.Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Error("preview CSP still carries the live page's frame-ancestors 'none'")
	}

	// The live page, rendered through the shared helper with no database.
	request := httptest.NewRequest(http.MethodGet, "/v/0123456789abcdef", nil)
	recorder := httptest.NewRecorder()
	liveContext := server.echo.NewContext(request, recorder)
	if err := server.renderVerify(liveContext, http.StatusOK, verifyView{Verdict: verdictValid}); err != nil {
		t.Fatalf("render live page: %v", err)
	}
	if got := recorder.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Errorf("live X-Frame-Options = %q; want DENY", got)
	}
	if csp := recorder.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Errorf("live CSP = %q; want frame-ancestors 'none'", csp)
	}
}

// TestPreviewIsNotCached: a preview is a judgement aid, and a cached one
// would show a colour the company has already moved past.
func TestPreviewIsNotCached(t *testing.T) {
	_, _, headers := previewHandler(t, previewServer(), previewPathWith(verdictValid, ""))
	if got := headers.Get("Cache-Control"); !strings.Contains(got, "no-store") {
		t.Errorf("preview Cache-Control = %q; want no-store", got)
	}
}

// TestPreviewRendersTheRealTemplate is the "verified by changing the template,
// not by inspection" criterion made permanent. The marker is read from
// verify_template.html itself and asserted to be present in the response and
// absent from every Go source: a preview that stopped executing the template
// — a hand-built markup copy, a React port — loses the marker and fails here,
// and so does anyone who moves it into Go.
func TestPreviewRendersTheRealTemplate(t *testing.T) {
	templateSource, err := os.ReadFile("verify_template.html")
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	const marker = "Verification checks the signature issued with the document."
	if !strings.Contains(string(templateSource), marker) {
		t.Fatalf("marker %q is no longer in the template; pick the new one", marker)
	}
	for _, source := range goSources(t) {
		if strings.Contains(source, marker) {
			t.Fatalf("marker %q exists in Go source; it can no longer prove the template ran", marker)
		}
	}
	_, body, _ := previewHandler(t, previewServer(), previewPathWith(verdictValid, ""))
	if !strings.Contains(body, marker) {
		t.Error("preview response does not contain the template's own marker; it is not rendering the real template")
	}
}

// TestPreviewSharesTheViewBuilderWithTheLivePortal pins the other half of "the
// real template": both consumers build their view through verifyDisplay, so a
// field the portal shows is a field the preview shows. The rendered preview
// must therefore carry exactly the document fields the live page carries for
// the same evidence shape.
func TestPreviewSharesTheViewBuilderWithTheLivePortal(t *testing.T) {
	server := previewServer()
	_, body, _ := previewHandler(t, server, previewPathWith(verdictValid, ""))

	live := verifyView{Verdict: verdictValid}
	verifyDisplay(&live, previewEvidence())
	live.Portal = theme.DefaultTheme().ResolvePortal()
	for _, want := range []string{live.QuoteNo, live.Revision, live.IssueDate, live.Recipient} {
		if !strings.Contains(body, want) {
			t.Errorf("preview is missing the shared field %q the live page renders", want)
		}
	}
	if !strings.Contains(body, live.Total) {
		t.Errorf("preview is missing the total %q the live page renders", live.Total)
	}
	if !strings.Contains(body, live.PPN) {
		t.Errorf("preview is missing the PPN row %q the live page renders", live.PPN)
	}
}

// previewRouteRequest goes through the router with a bearer token, so the
// middleware runs. It reads only the status: the body is a whole HTML
// document, which doJSON would try to decode as JSON.
func previewRouteRequest(t *testing.T, server *Server, token, path string) int {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	return recorder.Code
}

// TestPreviewRouteIsAdminOnly gates the route the way the other company
// routes are gated: no session at all is a 401, a signed-in non-admin is a
// 403, and the admin gets the page. The preview shows a company's own brand
// colours on a customer-facing document, so it is not a public surface.
func TestPreviewRouteIsAdminOnly(t *testing.T) {
	server, _, token := adminTestSetup(t, "vera_api_portal_preview_auth")
	path := backend.PortalPreviewPath(verdictValid, "#1A237E", "#284AF3", "#D6E4FF", "#1a1a1a")

	staffToken := staffLogin(t, server, "vera_api_portal_preview_auth")
	if code := previewRouteRequest(t, server, staffToken, path); code != http.StatusForbidden {
		t.Errorf("staff preview = %d; want 403", code)
	}
	if code := previewRouteRequest(t, server, "", path); code != http.StatusUnauthorized {
		t.Errorf("anon preview = %d; want 401", code)
	}
	if code := previewRouteRequest(t, server, token, path); code != http.StatusOK {
		t.Errorf("admin preview = %d; want 200", code)
	}
}

// TestPreviewOwnsItsRateLimitBudget proves the preview's limiter is not the
// public verifier's: a rebranding admin must not be able to starve the
// customers scanning real quotations from the same office address.
func TestPreviewOwnsItsRateLimitBudget(t *testing.T) {
	server := previewServer()
	if server.previewLimiter == server.verifyLimiter {
		t.Fatal("the preview shares the public verifier's limiter")
	}
	server.previewLimiter = newIPRateLimiter(1, time.Minute)
	_, _, _ = previewHandler(t, server, previewPathWith(verdictValid, ""))
	code, _, _ := previewHandler(t, server, previewPathWith(verdictValid, ""))
	if code != http.StatusTooManyRequests {
		t.Errorf("second preview over the budget = %d; want 429", code)
	}
	// The public budget is untouched by it.
	request := httptest.NewRequest(http.MethodGet, "/v/0123456789abcdef", nil)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code == http.StatusTooManyRequests {
		t.Error("preview traffic consumed the public verifier's budget")
	}
}
