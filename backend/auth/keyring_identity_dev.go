//go:build dev && !production

package auth

// A development build keeps the service name so the OS still shows one Vera
// entry, but stores its remembered sign-in under a distinct account. A dev
// build therefore cannot present the installed app's production token to the
// loopback API, and signing in there cannot overwrite what the installed app
// reads on its next launch. There is no migration: a dev token remembered
// under the old shared name is orphaned, never read by either mode again.
const (
	keyringService = "vera-app"
	keyringAccount = "session.dev"
)
