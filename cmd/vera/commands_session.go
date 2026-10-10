package main

import (
	"flag"
	"fmt"
	"time"
)

// sessionCommands are the commands about being signed in: who you are, and the
// account operations that belong to you rather than to an admin.
func sessionCommands() []*command {
	return []*command{
		loginCommand(),
		logoutCommand(),
		whoamiCommand(),
		redeemCommand(),
		passwordCommand(),
		profileCommand(),
		accountCommand(),
	}
}

// loginCommand signs in and stores the token.
//
// `--password-stdin` exists because it is the only form that is both
// non-interactive and safe: a --password flag would put the password in the
// process list and the shell history, and a prompt cannot be scripted.
func loginCommand() *command {
	var email string
	var passwordStdin bool
	return &command{
		name:    "login",
		summary: "Sign in and store the session.",
		usage:   "vera login [--email you@example.com] [--password-stdin]",
		flags: func(fs *flag.FlagSet) {
			fs.StringVar(&email, "email", "", "account email (prompted if omitted)")
			fs.BoolVar(&passwordStdin, "password-stdin", false, "read the password from stdin instead of prompting")
		},
		run: func(a *app) error {
			if err := a.requireArgs(0, "no arguments"); err != nil {
				return err
			}
			if email == "" {
				return usagef(a.cmd, "--email is required (there is no way to guess it)")
			}

			var password string
			if passwordStdin {
				read, err := readPassword()
				if err != nil {
					return err
				}
				password = read
			} else {
				if !isTerminal(stdin()) {
					return usagef(a.cmd, "no terminal to prompt on; pipe the password with --password-stdin")
				}
				prompted, err := promptPassword("Password: ")
				if err != nil {
					return err
				}
				password = prompted
			}

			base := a.apiBase()
			if err := checkTransport(base, a.insecure); err != nil {
				return err
			}
			// Signing in is the one thing that cannot require a session.
			client, err := a.anonClient()
			if err != nil {
				return err
			}

			var response struct {
				Token string `json:"token"`
				User  struct {
					ID          int64  `json:"id"`
					Email       string `json:"email"`
					DisplayName string `json:"displayName"`
					Role        string `json:"role"`
				} `json:"user"`
			}
			if err := client.Post("/v1/auth/login", map[string]string{
				"email": email, "password": password,
			}, &response); err != nil {
				return err
			}
			if response.Token == "" {
				return fmt.Errorf("the server accepted the sign-in but returned no token")
			}

			session := &Session{
				API:         base,
				Token:       response.Token,
				Email:       response.User.Email,
				DisplayName: response.User.DisplayName,
				Role:        response.User.Role,
				SignedInAt:  time.Now().UTC(),
			}
			if err := SaveSession(session); err != nil {
				return err
			}

			if err := a.emitJSON(map[string]any{
				"api":         session.API,
				"email":       session.Email,
				"displayName": session.DisplayName,
				"role":        session.Role,
			}); err != nil {
				return err
			}
			a.field("Signed in as", session.Email)
			a.field("Name", session.DisplayName)
			a.field("Role", session.Role)
			a.field("Server", session.API)
			a.note("Session stored. `vera logout` forgets it.")
			return nil
		},
	}
}

func logoutCommand() *command {
	var everywhere bool
	return &command{
		name:    "logout",
		summary: "Forget the stored session (and optionally end it on the server).",
		usage:   "vera logout [--everywhere]",
		flags: func(fs *flag.FlagSet) {
			fs.BoolVar(&everywhere, "everywhere", false, "also end every other session for this account")
		},
		run: func(a *app) error {
			if err := a.requireArgs(0, "no arguments"); err != nil {
				return err
			}
			// Best effort on the server: a token the server has already
			// forgotten should still leave the local file gone, or the next
			// command fails with a session nobody can clear.
			if session := a.loadSession(); session != nil && session.Token != "" {
				if client, err := a.client(); err == nil {
					if everywhere {
						if err := client.Post("/v1/auth/logout-everywhere", nil, nil); err != nil {
							a.warn("could not end every session on the server: %v", err)
						}
					} else if err := client.Post("/v1/auth/logout", nil, nil); err != nil {
						a.warn("could not end the session on the server: %v", err)
					}
				}
			}
			if err := ClearSession(); err != nil {
				return err
			}
			if err := a.emitJSON(map[string]any{"signedOut": true, "everywhere": everywhere}); err != nil {
				return err
			}
			a.line("Signed out.")
			return nil
		},
	}
}

func whoamiCommand() *command {
	return &command{
		name:    "whoami",
		summary: "Show who the stored session belongs to, per the server.",
		usage:   "vera whoami",
		run: func(a *app) error {
			if err := a.requireArgs(0, "no arguments"); err != nil {
				return err
			}
			client, err := a.client()
			if err != nil {
				return err
			}
			var me struct {
				ID          int64  `json:"id"`
				Email       string `json:"email"`
				DisplayName string `json:"displayName"`
				Role        string `json:"role"`
			}
			if err := client.Get("/v1/auth/me", &me); err != nil {
				return err
			}
			if err := a.emitJSON(map[string]any{
				"id": me.ID, "email": me.Email, "displayName": me.DisplayName,
				"role": me.Role, "api": a.apiBase(),
			}); err != nil {
				return err
			}
			a.field("Email", me.Email)
			a.field("Name", me.DisplayName)
			a.field("Role", me.Role)
			a.field("Server", a.apiBase())
			return nil
		},
	}
}

// redeemCommand accepts an invite token, which is how an account created by an
// admin becomes usable.
func redeemCommand() *command {
	var passwordStdin bool
	return &command{
		name:    "redeem",
		summary: "Accept an invite: set the password for a new account.",
		usage:   "vera redeem <invite-token> [--password-stdin]",
		flags: func(fs *flag.FlagSet) {
			fs.BoolVar(&passwordStdin, "password-stdin", false, "read the new password from stdin instead of prompting")
		},
		run: func(a *app) error {
			if err := a.requireArgs(1, "an invite token"); err != nil {
				return err
			}
			password, err := a.readNewPassword(passwordStdin)
			if err != nil {
				return err
			}
			if len(password) < 10 {
				return usagef(a.cmd, "the password must be at least 10 characters")
			}
			// Redeeming is how an account becomes usable: no session exists yet.
			client, err := a.anonClient()
			if err != nil {
				return err
			}
			// The server calls this field newPassword; there is no "password"
			// on this route.
			if err := client.Post("/v1/auth/redeem", map[string]string{
				"token": a.args[0], "newPassword": password,
			}, nil); err != nil {
				return err
			}
			if err := a.emitJSON(map[string]any{"redeemed": true}); err != nil {
				return err
			}
			a.line("Invite accepted. Sign in with `vera login --email <you>`.")
			return nil
		},
	}
}

// readNewPassword centralises the password input rule the three password-taking
// commands share.
func (a *app) readNewPassword(fromStdin bool) (string, error) {
	if fromStdin {
		return readPassword()
	}
	if !isTerminal(stdin()) {
		return "", usagef(a.cmd, "no terminal to prompt on; pipe the password with --password-stdin")
	}
	first, err := promptPassword("New password: ")
	if err != nil {
		return "", err
	}
	again, err := promptPassword("Repeat password: ")
	if err != nil {
		return "", err
	}
	if first != again {
		return "", fmt.Errorf("the passwords do not match")
	}
	return first, nil
}

func passwordCommand() *command {
	var passwordStdin bool
	return commandGroup("password", "Your own password.",
		&command{
			name:    "change",
			summary: "Change your password.",
			usage:   "vera password change [--password-stdin]",
			flags: func(fs *flag.FlagSet) {
				fs.BoolVar(&passwordStdin, "password-stdin", false, "read the new password from stdin instead of prompting")
			},
			run: func(a *app) error {
				if err := a.requireArgs(0, "no arguments"); err != nil {
					return err
				}
				current := ""
				if isTerminal(stdin()) && !passwordStdin {
					var err error
					if current, err = promptPassword("Current password: "); err != nil {
						return err
					}
				} else {
					return usagef(a.cmd, "changing a password needs the current one; run it in a terminal, or use `vera password reset` if you cannot sign in")
				}
				next, err := a.readNewPassword(false)
				if err != nil {
					return err
				}
				client, err := a.client()
				if err != nil {
					return err
				}
				if err := client.Patch("/v1/auth/password", map[string]string{
					"currentPassword": current, "newPassword": next,
				}, nil); err != nil {
					return err
				}
				if err := a.emitJSON(map[string]any{"changed": true}); err != nil {
					return err
				}
				a.line("Password changed.")
				return nil
			},
		},
	)
}

func profileCommand() *command {
	var displayName, email string
	return commandGroup("profile", "Your own profile.",
		&command{
			name:    "set",
			summary: "Change your display name, and request an email change.",
			usage:   "vera profile set [--display-name \"Name\"] [--email new@example.com]",
			flags: func(fs *flag.FlagSet) {
				fs.StringVar(&displayName, "display-name", "", "the name shown beside your work")
				fs.StringVar(&email, "email", "", "a new email address; an admin has to approve it")
			},
			run: func(a *app) error {
				if err := a.requireArgs(0, "no arguments"); err != nil {
					return err
				}
				if displayName == "" && email == "" {
					return usagef(a.cmd, "nothing to change; pass --display-name and/or --email")
				}
				client, err := a.client()
				if err != nil {
					return err
				}
				result := map[string]any{}
				if displayName != "" {
					if err := client.Patch("/v1/auth/profile",
						map[string]string{"displayName": displayName}, nil); err != nil {
						return err
					}
					result["displayName"] = displayName
				}
				if email != "" {
					// A change of address is a change of identity, so the server
					// queues it for an admin rather than applying it.
					if err := client.Post("/v1/auth/email-change-request",
						map[string]string{"email": email}, nil); err != nil {
						return err
					}
					result["emailChangeRequested"] = email
				}
				if err := a.emitJSON(result); err != nil {
					return err
				}
				if displayName != "" {
					a.line("Display name changed to %q.", displayName)
				}
				if email != "" {
					a.line("Email change to %s requested; an admin has to approve it.", email)
					a.line("Check it with `vera account email-requests`.")
				}
				return nil
			},
		},
	)
}

func accountCommand() *command {
	return commandGroup("account", "Your own account: deletion and email-change requests.",
		&command{
			name:    "delete-request",
			summary: "Ask an admin to delete your account.",
			usage:   "vera account delete-request",
			run: func(a *app) error {
				if err := a.requireArgs(0, "no arguments"); err != nil {
					return err
				}
				// The route insists on an explicit confirm, and there is no
				// reason field to send: asking for deletion is the whole
				// message.
				client, err := a.client()
				if err != nil {
					return err
				}
				if err := client.Post("/v1/auth/deletion-request",
					map[string]bool{"confirm": true}, nil); err != nil {
					return err
				}
				if err := a.emitJSON(map[string]any{"requested": true}); err != nil {
					return err
				}
				a.line("Deletion requested. An admin has to approve it.")
				a.line("Check it with `vera account requests`.")
				return nil
			},
		},
		&command{
			name:    "requests",
			summary: "Show your own deletion requests.",
			usage:   "vera account requests",
			run: func(a *app) error {
				if err := a.requireArgs(0, "no arguments"); err != nil {
					return err
				}
				// This one answers with a bare array; the email one below wraps
				// its rows. The two shapes are the server's, not ours to unify.
				client, err := a.client()
				if err != nil {
					return err
				}
				var rows []struct {
					ID          int64  `json:"id"`
					Status      string `json:"status"`
					RequestedAt string `json:"requestedAt"`
				}
				if err := client.Get("/v1/auth/deletion-requests/mine", &rows); err != nil {
					return err
				}
				if err := a.emitJSON(map[string]any{"requests": rows, "count": len(rows)}); err != nil {
					return err
				}
				if len(rows) == 0 {
					a.line("No deletion requests.")
					return nil
				}
				table := make([][]string, 0, len(rows))
				for _, row := range rows {
					table = append(table, []string{
						fmt.Sprintf("%d", row.ID), row.Status, row.RequestedAt,
					})
				}
				a.table([]string{"ID", "STATUS", "REQUESTED"}, table)
				return nil
			},
		},
		&command{
			name:    "email-requests",
			summary: "Show your own email-change requests.",
			usage:   "vera account email-requests",
			run: func(a *app) error {
				if err := a.requireArgs(0, "no arguments"); err != nil {
					return err
				}
				client, err := a.client()
				if err != nil {
					return err
				}
				var response struct {
					Requests []struct {
						ID            int64  `json:"id"`
						Status        string `json:"status"`
						ProposedEmail string `json:"proposedEmail"`
						RequestedAt   string `json:"requestedAt"`
					} `json:"requests"`
				}
				if err := client.Get("/v1/auth/email-change-requests/mine", &response); err != nil {
					return err
				}
				if err := a.emitJSON(map[string]any{"requests": response.Requests, "count": len(response.Requests)}); err != nil {
					return err
				}
				if len(response.Requests) == 0 {
					a.line("No email-change requests.")
					return nil
				}
				table := make([][]string, 0, len(response.Requests))
				for _, row := range response.Requests {
					table = append(table, []string{
						fmt.Sprintf("%d", row.ID), row.Status, row.ProposedEmail, row.RequestedAt,
					})
				}
				a.table([]string{"ID", "STATUS", "PROPOSED", "REQUESTED"}, table)
				return nil
			},
		},
	)
}

// promptPassword reads a password without echoing it.
func promptPassword(prompt string) (string, error) {
	return readPasswordMasked(prompt)
}
