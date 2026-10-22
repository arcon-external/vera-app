//go:build dev && !production

package auth

// A development build keeps the service name so the OS still shows one Vera
// entry, but stores its remembered sign-in under a distinct account. New dev
// sessions therefore cannot read or overwrite the installed app's token.
//
// There is no migration, and the old shared slot is ambiguous: production
// still reads vera-app/session, so a token written there by an older
// development build cannot be distinguished from a production token. New
// development sign-ins use session.dev; the pre-existing slot cannot be
// retrospectively classified.
const (
	keyringService = "vera-app"
	keyringAccount = "session.dev"
)
