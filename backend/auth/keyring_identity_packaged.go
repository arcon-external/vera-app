//go:build !dev || production

package auth

// The installed desktop's remembered sign-in lives under these exact bytes in
// the OS keychain. They are a compatibility contract, not a preference: every
// staff member's existing item is stored under service "vera-app", account
// "session", so changing either one silently signs them out.
const (
	keyringService = "vera-app"
	keyringAccount = "session"
)
