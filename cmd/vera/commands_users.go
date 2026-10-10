package main

import (
	"flag"
	"fmt"
	"strings"
)

// User is one account as the server lists it.
//
// A reset does not set a password: it issues a fresh invite token, which the
// user redeems themselves. That is deliberate -- an admin never chooses someone
// else's password, so no admin can sign in as staff.
type User struct {
	ID                 int64  `json:"id"`
	Email              string `json:"email"`
	DisplayName        string `json:"displayName"`
	Role               string `json:"role"`
	IsActive           bool   `json:"isActive"`
	MustChangePassword bool   `json:"mustChangePassword"`
	LastLoginAt        string `json:"lastLoginAt"`
}

func usersCommands() *command {
	return commandGroup("users", "Staff accounts (admin).",
		usersListCommand(),
		usersAddCommand(),
		usersRoleCommand(),
		usersActiveCommand(),
		usersResetCommand(),
		usersEmailCommand(),
		usersRemoveCommand(),
	)
}

func usersListCommand() *command {
	return &command{
		name:    "list",
		summary: "List every account.",
		usage:   "vera users list",
		run: func(a *app) error {
			if err := a.requireArgs(0, "no arguments"); err != nil {
				return err
			}
			client, err := a.client()
			if err != nil {
				return err
			}
			var response struct {
				Users []User `json:"users"`
			}
			if err := client.Get("/v1/admin/users", &response); err != nil {
				return err
			}
			if err := a.emitJSON(map[string]any{"users": response.Users, "count": len(response.Users)}); err != nil {
				return err
			}
			if len(response.Users) == 0 {
				a.line("No accounts.")
				return nil
			}
			rows := make([][]string, 0, len(response.Users))
			for _, user := range response.Users {
				state := "active"
				if !user.IsActive {
					state = "disabled"
				}
				if user.MustChangePassword {
					state += ", invite pending"
				}
				last := user.LastLoginAt
				if last == "" {
					last = "never"
				}
				rows = append(rows, []string{
					fmt.Sprintf("%d", user.ID), user.Email, user.DisplayName, user.Role, state, last,
				})
			}
			a.table([]string{"ID", "EMAIL", "NAME", "ROLE", "STATE", "LAST SIGN-IN"}, rows)
			return nil
		},
	}
}

func usersAddCommand() *command {
	var email, name, role string
	return &command{
		name:    "add",
		summary: "Create an account and print its invite token (admin).",
		usage:   `vera users add --email them@example.com --name "Their Name" [--role staff|admin]`,
		flags: func(fs *flag.FlagSet) {
			fs.StringVar(&email, "email", "", "their email address (required)")
			fs.StringVar(&name, "name", "", "the name shown beside their work (required)")
			fs.StringVar(&role, "role", "staff", "staff or admin")
		},
		run: func(a *app) error {
			if err := a.requireArgs(0, "no arguments"); err != nil {
				return err
			}
			if strings.TrimSpace(email) == "" || strings.TrimSpace(name) == "" {
				return usagef(a.cmd, "--email and --name are both required")
			}
			if role != "staff" && role != "admin" {
				return usagef(a.cmd, "--role must be staff or admin, got %q", role)
			}
			client, err := a.client()
			if err != nil {
				return err
			}
			var created User
			if err := client.Post("/v1/admin/users", map[string]string{
				"email": email, "displayName": name, "role": role,
			}, &created); err != nil {
				return err
			}
			// The create response carries no token, so ask for one right away.
			// Without it the account exists but nobody can sign in, and the
			// admin would have to run a second command to find out.
			var invite struct {
				Token string `json:"token"`
			}
			if err := client.Post(fmt.Sprintf("/v1/admin/users/%d/reset", created.ID), nil, &invite); err != nil {
				a.warn("the account was created but the invite token could not be issued: %v", err)
				a.warn("run 'vera users reset %d' to get one", created.ID)
			}

			result := map[string]any{
				"id": created.ID, "email": created.Email,
				"displayName": created.DisplayName, "role": created.Role,
				"inviteToken": invite.Token,
			}
			if err := a.emitJSON(result); err != nil {
				return err
			}
			a.field("Created", fmt.Sprintf("%s (%s, id %d)", created.Email, created.Role, created.ID))
			if invite.Token != "" {
				a.blank()
				a.line("Invite token (give it to them; it expires):")
				a.line("  %s", invite.Token)
				a.blank()
				a.line("They accept it with:")
				a.line("  vera redeem %s", invite.Token)
			}
			return nil
		},
	}
}

func usersRoleCommand() *command {
	return &command{
		name:    "role",
		summary: "Change an account's role (admin).",
		usage:   "vera users role <id> staff|admin",
		run: func(a *app) error {
			if err := a.requireArgs(2, "a user id and a role (staff or admin)"); err != nil {
				return err
			}
			id, err := a.parseID("user id", a.args[0])
			if err != nil {
				return err
			}
			role := a.args[1]
			if role != "staff" && role != "admin" {
				return usagef(a.cmd, "the role must be staff or admin, got %q", role)
			}
			client, err := a.client()
			if err != nil {
				return err
			}
			if err := client.Patch(fmt.Sprintf("/v1/admin/users/%d/role", id),
				map[string]string{"role": role}, nil); err != nil {
				return err
			}
			if err := a.emitJSON(map[string]any{"id": id, "role": role}); err != nil {
				return err
			}
			a.line("User %d is now %s.", id, role)
			return nil
		},
	}
}

func usersActiveCommand() *command {
	var enable, disable bool
	return &command{
		name:    "active",
		summary: "Enable or disable an account (admin).",
		usage:   "vera users active <id> --enable|--disable",
		flags: func(fs *flag.FlagSet) {
			fs.BoolVar(&enable, "enable", false, "let them sign in again")
			fs.BoolVar(&disable, "disable", false, "block them from signing in, keeping their account")
		},
		run: func(a *app) error {
			if err := a.requireArgs(1, "a user id"); err != nil {
				return err
			}
			if enable == disable {
				return usagef(a.cmd, "pass exactly one of --enable or --disable")
			}
			id, err := a.parseID("user id", a.args[0])
			if err != nil {
				return err
			}
			client, err := a.client()
			if err != nil {
				return err
			}
			if err := client.Patch(fmt.Sprintf("/v1/admin/users/%d/active", id),
				map[string]bool{"active": enable}, nil); err != nil {
				return err
			}
			state := "disabled"
			if enable {
				state = "enabled"
			}
			if err := a.emitJSON(map[string]any{"id": id, "isActive": enable}); err != nil {
				return err
			}
			a.line("User %d is %s.", id, state)
			return nil
		},
	}
}

func usersResetCommand() *command {
	return &command{
		name:    "reset",
		summary: "Issue a fresh invite token, so an account can set its own password (admin).",
		usage:   "vera users reset <id>",
		run: func(a *app) error {
			if err := a.requireArgs(1, "a user id"); err != nil {
				return err
			}
			id, err := a.parseID("user id", a.args[0])
			if err != nil {
				return err
			}
			client, err := a.client()
			if err != nil {
				return err
			}
			var invite struct {
				Token string `json:"token"`
			}
			if err := client.Post(fmt.Sprintf("/v1/admin/users/%d/reset", id), nil, &invite); err != nil {
				return err
			}
			if err := a.emitJSON(map[string]any{"id": id, "inviteToken": invite.Token}); err != nil {
				return err
			}
			a.line("Invite token for user %d (expires):", id)
			a.line("  %s", invite.Token)
			a.note("They accept it with `vera redeem <token>`. You never choose their password.")
			return nil
		},
	}
}

func usersEmailCommand() *command {
	var email string
	return &command{
		name:    "email",
		summary: "Change an account's email address outright (admin).",
		usage:   "vera users email <id> --email new@example.com",
		flags: func(fs *flag.FlagSet) {
			fs.StringVar(&email, "email", "", "the new address (required)")
		},
		run: func(a *app) error {
			if err := a.requireArgs(1, "a user id"); err != nil {
				return err
			}
			if strings.TrimSpace(email) == "" {
				return usagef(a.cmd, "--email is required")
			}
			id, err := a.parseID("user id", a.args[0])
			if err != nil {
				return err
			}
			client, err := a.client()
			if err != nil {
				return err
			}
			var response struct {
				Email         string `json:"email"`
				PreviousEmail string `json:"previousEmail"`
			}
			if err := client.Patch(fmt.Sprintf("/v1/admin/users/%d/email", id),
				map[string]string{"email": email}, &response); err != nil {
				return err
			}
			if err := a.emitJSON(response); err != nil {
				return err
			}
			a.line("%s is now %s.", response.PreviousEmail, response.Email)
			a.note("This is the admin path: it applies immediately, without the queue an email change request goes through.")
			return nil
		},
	}
}

func usersRemoveCommand() *command {
	return &command{
		name:    "rm",
		summary: "Permanently delete an account (admin).",
		usage:   "vera users rm <id>",
		run: func(a *app) error {
			if err := a.requireArgs(1, "a user id"); err != nil {
				return err
			}
			id, err := a.parseID("user id", a.args[0])
			if err != nil {
				return err
			}
			if err := a.confirm("Permanently delete user %d? Their account cannot be restored; disable it instead if this is temporary.", id); err != nil {
				return err
			}
			client, err := a.client()
			if err != nil {
				return err
			}
			if err := client.Delete(fmt.Sprintf("/v1/admin/users/%d", id), nil); err != nil {
				return err
			}
			if err := a.emitJSON(map[string]any{"removed": id}); err != nil {
				return err
			}
			a.line("Deleted user %d.", id)
			return nil
		},
	}
}
