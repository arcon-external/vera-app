package api

import (
	"context"
	"crypto/ed25519"
	"embed"
	"html/template"
	"io"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"

	"vera-app/backend"
	"vera-app/backend/store"
)

//go:embed verify_template.html
var verifyTemplateFS embed.FS

var verifyTemplate = template.Must(template.ParseFS(verifyTemplateFS, "verify_template.html"))

// shortIDPattern gates every lookup before a query is issued: lowercase hex,
// exactly the width the store writes. Anything else is rejected without
// touching the database.
var shortIDPattern = regexp.MustCompile(`^[0-9a-f]{16}$`)

// Verification verdicts. The zero state is deliberately "unavailable" so a
// misconfigured server can never render a document as valid by omission.
const (
	verdictValid       = "valid"
	verdictSuperseded  = "superseded"
	verdictMismatch    = "mismatch"
	verdictNotFound    = "notfound"
	verdictUnavailable = "unavailable"
)

// verifyFieldDiff is one paper-vs-record comparison row.
type verifyFieldDiff struct {
	Label   string
	Claimed string
	Record  string
	Matches bool
	Meaning string
}

// verifyView is the entire render surface. Fields absent from this struct
// cannot reach the page, which is the structural guarantee that internal
// economics never leak through verification.
type verifyView struct {
	Verdict   string
	Headline  string
	Detail    string
	LookupID  string
	QuoteNo   string
	Revision  string
	IssueDate string
	Recipient string
	Issuer    string
	Total     string
	Diffs     []verifyFieldDiff
	AnyClaim  bool
	// ThemeP* brand colors for this page, resolved per request from the
	// company profile. Empty means "use the stylesheet default".
	ThemePDeep   string
	ThemePAccent string
	ThemePSoft   string
	ThemePInk    string
}

func (v verifyView) IsVerified() bool   { return v.Verdict == verdictValid }
func (v verifyView) IsSuperseded() bool { return v.Verdict == verdictSuperseded }
func (v verifyView) IsMismatch() bool   { return v.Verdict == verdictMismatch }
func (v verifyView) IsNotFound() bool   { return v.Verdict == verdictNotFound }
func (v verifyView) IsUnavailable() bool {
	return v.Verdict == verdictUnavailable
}

// Input bounds. Every field a reader can type is length-bounded before it
// reaches a query or the page: an unbounded field on a public endpoint is a
// memory and reflection problem waiting to happen.
const (
	maxLookupInput = 256  // a pasted link plus slack
	maxClaimNumber = 32   // quotation numbers are 10-11 digits
	maxClaimAmount = 32   // amounts are digits, possibly with separators
	maxVerifyBody  = 4096 // the whole form, generous for three fields
)

// verificationCSP locks the page down: nothing loads, nothing frames it, and
// forms may only post back here. Inline styles are required by the single
// embedded stylesheet; no scripts exist on this page at all.
const verificationCSP = "default-src 'none'; style-src 'unsafe-inline'; " +
	"form-action 'self'; base-uri 'none'; frame-ancestors 'none'"

// sanitizeLookupInput bounds a lookup value before any parsing.
func sanitizeLookupInput(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) > maxLookupInput {
		return ""
	}
	return trimmed
}

// sanitizeClaim bounds a comparison field and normalizes the numeric ones to
// digits. Returning "" means "nothing to compare", never "compare empty".
func sanitizeClaim(field, raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	switch field {
	case "number":
		if len(trimmed) > maxClaimNumber {
			return ""
		}
		// Quotation numbers are digits; anything else cannot match a record.
		return onlyDigits(trimmed)
	case "amount":
		if len(trimmed) > maxClaimAmount {
			return ""
		}
		return onlyDigits(trimmed)
	default:
		return ""
	}
}

// normalizeLookupID accepts a raw handle or any URL containing one, so a
// pasted link works as well as a typed id. It never returns partial input.
func normalizeLookupID(raw string) string {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	if trimmed == "" {
		return ""
	}
	if shortIDPattern.MatchString(trimmed) {
		return trimmed
	}
	// A URL or path may carry the handle as its last segment.
	for _, field := range strings.FieldsFunc(trimmed, func(r rune) bool {
		return r == '/' || r == '?' || r == '#' || r == '&' || r == '='
	}) {
		candidate := strings.TrimSpace(field)
		if shortIDPattern.MatchString(candidate) {
			return candidate
		}
	}
	return ""
}

// verifyTitles keeps copy in one place so every verdict names itself and
// what it means in plain language.
func verifyTitles(verdict string) (headline, detail string) {
	switch verdict {
	case verdictValid:
		return "Verified", "This document matches the record issued by the company."
	case verdictSuperseded:
		return "Superseded", "This revision is genuine, but a newer revision replaced it. Ask for the current one."
	case verdictMismatch:
		return "Does not match our records", "The details on this document differ from the issued record."
	case verdictUnavailable:
		return "Verification unavailable", "This server is not configured to verify documents right now."
	default:
		return "Not found", "No quotation matches this code. Check the code, or contact us to confirm."
	}
}

func (s *Server) renderVerify(context echo.Context, status int, view verifyView) error {
	defaultHeadline, defaultDetail := verifyTitles(view.Verdict)
	if view.Headline == "" {
		view.Headline = defaultHeadline
	}
	if view.Detail == "" {
		view.Detail = defaultDetail
	}
	context.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
	context.Response().Header().Set("X-Content-Type-Options", "nosniff")
	context.Response().Header().Set("X-Frame-Options", "DENY")
	context.Response().Header().Set("Referrer-Policy", "no-referrer")
	context.Response().Header().Set("Content-Security-Policy", verificationCSP)
	// Verification results are per-request facts and must never be cached by
	// a proxy or a shared device: a stale "Verified" is worse than none.
	context.Response().Header().Set("Cache-Control", "no-store, max-age=0")
	return context.Render(status, "verify_template.html", view)
}

// verifyRenderer wires the embedded verification template into Echo.
type verifyRenderer struct{}

func (r *verifyRenderer) Render(w io.Writer, name string, data interface{}, context echo.Context) error {
	return verifyTemplate.ExecuteTemplate(w, name, data)
}

// verifyDocument is the shared verdict engine for both entry points.
//
// claims carries paper-side values the requester typed (empty when unknown).
// A nil claims map means "no comparison requested".
func (s *Server) verifyDocument(httpContext echo.Context, lookupID string, claims map[string]string) verifyView {
	view := verifyView{Verdict: verdictNotFound, LookupID: strings.TrimSpace(lookupID), AnyClaim: len(claims) > 0}

	normalized := normalizeLookupID(lookupID)
	if normalized == "" {
		return view
	}
	view.LookupID = normalized

	requestContext := httpContext.Request().Context()
	evidence, err := s.database.GetRevisionEvidenceByShortID(requestContext, normalized)
	if err != nil {
		return view // unknown and revoked share this path and this page
	}

	// Populate the display fields first: a mismatch still tells the reader
	// what we hold on record, which is the whole point of the page.
	view.QuoteNo = evidence.QuoteNumber
	view.Revision = "Rev " + pad2(evidence.RevisionNumber)
	view.IssueDate = evidence.CreatedAt.Format("2 January 2006")
	view.Recipient = evidence.RecipientCompany
	view.Issuer = evidence.IssuerName
	view.Total = "Rp" + formatAmount(evidence.GrandTotal)

	// Theme and issuer display follow the live company profile; a stored
	// empty issuer (legacy rows predate it) falls back to the profile name.
	// Either way the values shown are program data, never hardcoded text.
	if profile, err := s.database.GetCompanyProfile(requestContext); err == nil {
		view.ThemePDeep = profile.ThemePDeep
		view.ThemePAccent = profile.ThemePAccent
		view.ThemePSoft = profile.ThemePSoft
		view.ThemePInk = profile.ThemePInk
		if view.Issuer == "" {
			view.Issuer = profile.LegalName
		}
	}

	// Verify the stored signature. Legacy rows (signed before this feature
	// existed) self-heal once, using the current key and issuer.
	if evidence.Signature == "" {
		if s.signer == nil {
			view.Verdict = verdictUnavailable
			return view
		}
		signature, digest := s.signer.SignRevision(backend.SignableFromEvidence(evidence))
		if err := s.database.SaveRevisionSignature(requestContext, evidence.ShortID, signature,
			s.signer.KeyID(), digest); err != nil {
			view.Verdict = verdictUnavailable
			return view
		}
		evidence.Signature = signature
		evidence.SigKeyID = s.signer.KeyID()
	}

	publicKey, err := s.resolveSigningKey(requestContext, evidence.SigKeyID)
	if err != nil {
		view.Verdict = verdictUnavailable
		return view
	}
	if !backend.VerifyRevision(publicKey, backend.SignableFromEvidence(evidence), evidence.Signature) {
		// The stored record no longer matches what was signed: the database
		// itself was altered. Per-field attribution is impossible by design
		// (the signature covers the set), so say so plainly.
		view.Verdict = verdictMismatch
		view.Detail = "The stored record does not match its issued signature, which means it was altered after issue."
		return view
	}

	// Compare paper-side claims when the requester supplied them. This is
	// what catches a physically altered document. Only real mismatches are
	// listed: showing matching rows under a "differs" heading contradicted
	// the verdict, so all-match comparisons leave Diffs empty and the page
	// renders no section at all.
	if len(claims) > 0 {
		for _, diff := range compareClaims(claims, evidence) {
			if diff.Matches {
				continue
			}
			view.Diffs = append(view.Diffs, diff)
		}
		if len(view.Diffs) > 0 {
			view.Verdict = verdictMismatch
			view.Detail = "The details below differ from the issued record."
			return view
		}
	}

	latest, err := s.database.LatestRevisionNumber(requestContext, evidence.QuotationID)
	if err == nil && latest > evidence.RevisionNumber {
		view.Verdict = verdictSuperseded
		view.Detail = "This revision is genuine, but " + "Rev " + pad2(latest) + " replaced it. Ask for the current revision."
		return view
	}

	view.Verdict = verdictValid
	return view
}

// resolveSigningKey finds the public key a signature was made with. Retired
// keys stay resolvable so historical codes keep verifying.
func (s *Server) resolveSigningKey(requestContext context.Context, keyID string) (ed25519.PublicKey, error) {
	record, err := s.database.GetSigningKey(requestContext, keyID)
	if err != nil {
		return nil, err
	}
	return backend.PublicKeyFromHex(record.PublicKey)
}

// compareClaims diffs the values a reader sees on paper against the record.
// Only fields a human can read off the document are comparable.
func compareClaims(claims map[string]string, evidence store.RevisionEvidence) []verifyFieldDiff {
	diffs := make([]verifyFieldDiff, 0, 2)
	if claimed, ok := claims["number"]; ok && strings.TrimSpace(claimed) != "" {
		normalized := strings.TrimSpace(claimed)
		diffs = append(diffs, verifyFieldDiff{
			Label: "Quotation number", Claimed: normalized, Record: evidence.QuoteNumber,
			Matches: normalized == evidence.QuoteNumber,
			Meaning: "A different number means this document is not the one it claims to be.",
		})
	}
	if claimed, ok := claims["amount"]; ok && strings.TrimSpace(claimed) != "" {
		digits := onlyDigits(claimed)
		recordNumber := strconv.FormatInt(evidence.GrandTotal, 10)
		matches := digits != "" && digits == recordNumber
		diffs = append(diffs, verifyFieldDiff{
			Label: "Grand total", Claimed: formatClaimAmount(claimed), Record: "Rp" + formatAmount(evidence.GrandTotal),
			Matches: matches,
			Meaning: "A changed amount is the most common sign of an altered document.",
		})
	}
	return diffs
}

func onlyDigits(value string) string {
	var b strings.Builder
	for _, r := range value {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func formatClaimAmount(value string) string {
	digits := onlyDigits(value)
	if digits == "" {
		return strings.TrimSpace(value)
	}
	number, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return strings.TrimSpace(value)
	}
	return "Rp" + formatAmount(number)
}

func pad2(n int16) string {
	if n < 10 {
		return "0" + strconv.Itoa(int(n))
	}
	return strconv.Itoa(int(n))
}

// verifyPage serves the QR path: GET /v/<shortid>.
func (s *Server) verifyPage(context echo.Context) error {
	if !s.verifyLimiter.allow(clientIP(context)) {
		return s.renderVerify(context, http.StatusTooManyRequests,
			verifyView{Verdict: verdictUnavailable, LookupID: "", Detail: "Too many checks from this device. Please wait a moment and try again."})
	}
	view := s.verifyDocument(context, sanitizeLookupInput(context.Param("id")), nil)
	status := http.StatusOK
	if view.IsNotFound() {
		status = http.StatusNotFound
	}
	return s.renderVerify(context, status, view)
}

// verifySubmit serves the paste path: POST /verify with an id plus optional
// paper-side claims. Comparison is what catches a physically altered page.
func (s *Server) verifySubmit(context echo.Context) error {
	if !s.verifyLimiter.allow(clientIP(context)) {
		return s.renderVerify(context, http.StatusTooManyRequests,
			verifyView{Verdict: verdictUnavailable, Detail: "Too many checks from this device. Please wait a moment and try again."})
	}
	id := sanitizeLookupInput(context.FormValue("id"))
	claims := map[string]string{}
	for _, field := range []string{"number", "amount"} {
		if value := sanitizeClaim(field, context.FormValue(field)); value != "" {
			claims[field] = value
		}
	}
	view := s.verifyDocument(context, id, claims)
	status := http.StatusOK
	if view.IsNotFound() {
		status = http.StatusNotFound
	}
	return s.renderVerify(context, status, view)
}

// clientIP returns the caller's address for rate-limit bucketing.
//
// Only the *last* X-Forwarded-For entry is trusted: a client can send its own
// header, and the platform appends the real peer address after it. Reading
// the first entry (which echo's RealIP does) lets an attacker rotate a fake
// value and bypass the limiter entirely. Anything that is not a parseable IP
// is discarded rather than used as a key.
func clientIP(context echo.Context) string {
	if forwarded := context.Request().Header.Get("X-Forwarded-For"); forwarded != "" && len(forwarded) <= 512 {
		parts := strings.Split(forwarded, ",")
		for i := len(parts) - 1; i >= 0; i-- {
			candidate := strings.TrimSpace(parts[i])
			if net.ParseIP(candidate) != nil {
				return candidate
			}
		}
	}
	if host, _, err := net.SplitHostPort(context.Request().RemoteAddr); err == nil {
		if net.ParseIP(host) != nil {
			return host
		}
	}
	return "unknown"
}

// ipRateLimiter is a small fixed-window limiter. Verification is a
// read-only public surface, so the limit exists to make automated probing
// expensive and noisy — not as an access control. The tracked set is capped
// so a flood from many distinct addresses cannot grow it without bound.
type ipRateLimiter struct {
	mu        sync.Mutex
	window    time.Duration
	limit     int
	maxKeys   int
	seen      map[string]int
	windowEnd time.Time
}

// maxTrackedIPsPerWindow bounds limiter memory. Beyond it the window resets,
// which fails open for one window rather than letting the map grow.
const maxTrackedIPsPerWindow = 20000

func newIPRateLimiter(limit int, window time.Duration) *ipRateLimiter {
	return &ipRateLimiter{
		limit:     limit,
		window:    window,
		maxKeys:   maxTrackedIPsPerWindow,
		seen:      map[string]int{},
		windowEnd: time.Now().Add(window),
	}
}

func (l *ipRateLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if now.After(l.windowEnd) || len(l.seen) >= l.maxKeys {
		l.seen = map[string]int{}
		l.windowEnd = now.Add(l.window)
	}
	l.seen[ip]++
	return l.seen[ip] <= l.limit
}
