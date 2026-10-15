package main

import (
	"flag"

	"vera-app/backend"
	"vera-app/backend/render"
)

// appCommands are about the installation itself rather than the data: which
// version this is, and what the server says the current release is.
func appCommands() []*command {
	return []*command{
		versionCommand(),
		updatesCommand(),
		changelogCommand(),
	}
}

// versionCommand prints the build's version.
//
// backend.AppVersion is the same value the desktop stamps into its installer and
// the updater compares against, so this answers "what am I running" with the
// number the release pipeline uses rather than a private one.
func versionCommand() *command {
	return &command{
		name:    "version",
		summary: "Print the CLI's version.",
		usage:   "vera version",
		run: func(a *app) error {
			if err := a.requireArgs(0, "no arguments"); err != nil {
				return err
			}
			// The renderer is reported alongside the version because the two
			// move independently: the same version number can be built with
			// either engine, and "which one is in this binary?" is the first
			// question when a document comes out wrong.
			if err := a.emitJSON(map[string]any{
				"version":  backend.AppVersion,
				"renderer": render.Active.Name(),
			}); err != nil {
				return err
			}
			a.line("%s (%s)", backend.AppVersion, render.Active.Name())
			return nil
		},
	}
}

// updatesCommand asks the public release endpoint, the same one the desktop's
// update banner reads. It needs no session: update checks run before sign-in.
func updatesCommand() *command {
	return commandGroup("updates", "What the server says the current release is.",
		&command{
			name:    "check",
			summary: "Compare this build with the published release.",
			usage:   "vera updates check",
			run: func(a *app) error {
				if err := a.requireArgs(0, "no arguments"); err != nil {
					return err
				}
				// The desktop checks for updates before anyone signs in, so
				// this route takes no session.
				client, err := a.anonClient()
				if err != nil {
					return err
				}
				// The release contract is backend's: this decodes the same
				// struct the desktop does, so a renamed key cannot break one
				// reader and leave the other parsing a release that no longer
				// exists (the #437 shape pin covers the announce script, not a
				// second private copy of these keys in here).
				//
				// The transport stays the CLI's own. Its StatusError is what
				// exitCodeFor turns into the documented exit codes, and the
				// shared decode flattens a status to prose: a 429 routed
				// through it would lose both its exit code and the server's
				// message.
				//
				// The static manifest the desktop falls back to is
				// deliberately absent: this command answers "what does the
				// server I was pointed at say", and a bucket is not that
				// server. An operator who cannot reach the API retries, or
				// reads the desktop's notice, which has the fallback for
				// exactly that outage.
				var latest backend.ServerRelease
				if err := client.Get("/v1/app/latest", &latest); err != nil {
					return err
				}
				current := backend.AppVersion
				if err := a.emitJSON(map[string]any{
					"current":      current,
					"latest":       latest.Latest,
					"minSupported": latest.MinSupported,
					"notesUrl":     latest.NotesURL,
					"downloads":    latest.Downloads,
					"upToDate":     current == latest.Latest,
				}); err != nil {
					return err
				}
				a.field("This build", current)
				a.field("Latest", latest.Latest)
				a.field("Minimum supported", latest.MinSupported)
				a.field("Notes", latest.NotesURL)
				if current == latest.Latest {
					a.line("Up to date.")
				} else {
					a.line("An update is available.")
				}
				return nil
			},
		},
	)
}

func changelogCommand() *command {
	return &command{
		name:    "changelog",
		summary: "Print the release notes the server serves.",
		usage:   "vera changelog",
		run: func(a *app) error {
			if err := a.requireArgs(0, "no arguments"); err != nil {
				return err
			}
			client, err := a.anonClient()
			if err != nil {
				return err
			}
			var response struct {
				Changelog string `json:"changelog"`
			}
			if err := client.Get("/v1/app/changelog", &response); err != nil {
				return err
			}
			if err := a.emitJSON(map[string]any{"changelog": response.Changelog}); err != nil {
				return err
			}
			a.line("%s", response.Changelog)
			return nil
		},
	}
}

// configCommands read and write the settings file. The session file is separate
// on purpose -- one is shareable, the other is a credential.
func configCommands() []*command {
	var setJSON bool
	var setAPI string
	config := commandGroup("config", "Local settings for this machine.",
		&command{
			name:    "show",
			summary: "Show the settings, the session, and which server they point at.",
			usage:   "vera config show",
			run: func(a *app) error {
				if err := a.requireArgs(0, "no arguments"); err != nil {
					return err
				}
				cfg, err := LoadConfig()
				if err != nil {
					return err
				}
				e := map[string]any{
					"api":       a.apiBase(),
					"configApi": cfg.API,
					// Named for what it is: the saved preference, not the
					// --json flag this very invocation may be using.
					"defaultJsonOutput": cfg.JSON,
					"hasSession":        false,
					"sessionFile":       "",
				}
				if session := a.loadSession(); session != nil {
					// The token itself never appears here: it is a credential,
					// and `config show` is the kind of command someone pastes
					// into a bug report.
					e["hasSession"] = session.Token != ""
					e["sessionEmail"] = session.Email
					e["sessionAPI"] = session.API
					e["sessionSavedAt"] = session.SignedInAt
				}
				if path, err := sessionPath(); err == nil {
					e["sessionFile"] = path
				}
				if err := a.emitJSON(e); err != nil {
					return err
				}
				a.field("API", a.apiBase())
				if cfg.API != "" {
					a.field("Config API", cfg.API)
				}
				if session := a.loadSession(); session != nil && session.Token != "" {
					a.field("Signed in as", session.Email)
				} else {
					a.line("Not signed in.")
				}
				if path, err := sessionPath(); err == nil {
					a.field("Session file", path)
				}
				return nil
			},
		},
		&command{
			name:    "set",
			summary: "Set the default API address, or the default output mode.",
			usage:   "vera config set [--default-api URL] [--json-output]",
			flags: func(fs *flag.FlagSet) {
				// Not --api: that global flag already means "talk to this
				// server now", and a command cannot redefine it.
				fs.StringVar(&setAPI, "default-api", "", "API base URL to use when none is given")
				fs.BoolVar(&setJSON, "json-output", false, "make --json the default for this machine")
			},
			run: func(a *app) error {
				if err := a.requireArgs(0, "no arguments"); err != nil {
					return err
				}
				if setAPI == "" && !setJSON {
					return usagef(a.cmd, "nothing to set; pass --default-api and/or --json-output")
				}
				cfg, err := LoadConfig()
				if err != nil {
					return err
				}
				if setAPI != "" {
					cfg.API = setAPI
				}
				if setJSON {
					cfg.JSON = true
				}
				if err := SaveConfig(cfg); err != nil {
					return err
				}
				path, _ := configPath()
				if err := a.emitJSON(map[string]any{"api": cfg.API, "json": cfg.JSON, "path": path}); err != nil {
					return err
				}
				a.line("Saved to %s", path)
				return nil
			},
		},
		&command{
			name:    "clear",
			summary: "Remove the saved default API address.",
			usage:   "vera config clear",
			run: func(a *app) error {
				if err := a.requireArgs(0, "no arguments"); err != nil {
					return err
				}
				cfg, err := LoadConfig()
				if err != nil {
					return err
				}
				cfg.API = ""
				if err := SaveConfig(cfg); err != nil {
					return err
				}
				if err := a.emitJSON(map[string]any{"api": ""}); err != nil {
					return err
				}
				a.line("Cleared the saved API address.")
				return nil
			},
		},
	)
	return []*command{config}
}
