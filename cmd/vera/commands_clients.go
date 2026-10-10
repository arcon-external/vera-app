package main

import (
	"flag"
	"fmt"
	"strings"
)

// Client is one row of the client directory as the API sends it.
type Client struct {
	ID             int64  `json:"id"`
	Code           int16  `json:"code"`
	Name           string `json:"name"`
	PrimaryAddress string `json:"primaryAddress"`
	QuotationCount int64  `json:"quotationCount,omitempty"`
}

// clientsCommands cover the client directory.
//
// Clients are database rows, never code: the editor's dropdown, Prepare's
// resolution and the verification peek all read this directory, so adding one
// here is what makes it usable everywhere. Codes are immutable once created,
// which is why `edit` cannot change one.
func clientsCommands() *command {
	return commandGroup("clients", "The client directory.",
		clientsListCommand(),
		clientsAddCommand(),
		clientsEditCommand(),
		clientsRemoveCommand(),
	)
}

func clientsListCommand() *command {
	return &command{
		name:    "list",
		summary: "List the clients you can quote.",
		usage:   "vera clients list",
		run: func(a *app) error {
			if err := a.requireArgs(0, "no arguments"); err != nil {
				return err
			}
			clients, err := a.fetchClients()
			if err != nil {
				return err
			}
			if err := a.emitJSON(map[string]any{"clients": clients}); err != nil {
				return err
			}
			if len(clients) == 0 {
				a.line("No clients yet.")
				return nil
			}
			rows := make([][]string, 0, len(clients))
			for _, c := range clients {
				rows = append(rows, []string{
					fmt.Sprintf("%d", c.Code),
					fmt.Sprintf("%d", c.ID),
					c.Name,
					truncate(c.PrimaryAddress, 54),
				})
			}
			a.table([]string{"CODE", "ID", "NAME", "ADDRESS"}, rows)
			return nil
		},
	}
}

// fetchClients is shared by the list command and by anything that has to turn a
// code into a name -- the quotation commands use it to fill in the customer on
// a locally-rendered document without a database.
func (a *app) fetchClients() ([]Client, error) {
	client, err := a.client()
	if err != nil {
		return nil, err
	}
	var response struct {
		Clients []Client `json:"clients"`
	}
	if err := client.Get("/v1/clients", &response); err != nil {
		return nil, err
	}
	return response.Clients, nil
}

func clientsAddCommand() *command {
	var code int
	var name, address string
	return &command{
		name:    "add",
		summary: "Add a client to the directory (admin).",
		usage:   `vera clients add --code 7 --name "PT Example" [--address "..."]`,
		flags: func(fs *flag.FlagSet) {
			fs.IntVar(&code, "code", 0, "two-digit client code, unique and permanent (required)")
			fs.StringVar(&name, "name", "", "legal name as it should print (required)")
			fs.StringVar(&address, "address", "", "address as it should print (required)")
		},
		run: func(a *app) error {
			if err := a.requireArgs(0, "no arguments"); err != nil {
				return err
			}
			if code < 1 || code > 99 {
				return usagef(a.cmd, "--code must be between 1 and 99; it is printed inside every quotation number")
			}
			if name == "" {
				return usagef(a.cmd, "--name is required")
			}
			if strings.TrimSpace(address) == "" {
				return usagef(a.cmd, "--address is required: it prints under the name on every quotation")
			}
			client, err := a.client()
			if err != nil {
				return err
			}
			var created Client
			// The route reads "address" and answers with "primaryAddress":
			// the request and response names differ, so only one of them can
			// be guessed right.
			if err := client.Post("/v1/admin/clients", map[string]any{
				"code": code, "name": name, "address": address,
			}, &created); err != nil {
				return err
			}
			if err := a.emitJSON(created); err != nil {
				return err
			}
			a.line("Added client %d: %s", created.Code, created.Name)
			return nil
		},
	}
}

func clientsEditCommand() *command {
	var name, address string
	return &command{
		name:    "edit",
		summary: "Change a client's name or address (admin).",
		usage:   `vera clients edit <id> [--name "..."] [--address "..."]`,
		flags: func(fs *flag.FlagSet) {
			fs.StringVar(&name, "name", "", "new legal name")
			fs.StringVar(&address, "address", "", "new address")
		},
		run: func(a *app) error {
			if err := a.requireArgs(1, "a client id"); err != nil {
				return err
			}
			id, err := a.parseID("client id", a.args[0])
			if err != nil {
				return err
			}
			if name == "" && address == "" {
				return usagef(a.cmd, "nothing to change; pass --name and/or --address")
			}
			// The code is not editable on purpose: it is part of every quote
			// number already issued to this client.
			patch := map[string]any{}
			if name != "" {
				patch["name"] = name
			}
			if address != "" {
				patch["address"] = address
			}
			client, err := a.client()
			if err != nil {
				return err
			}
			var updated Client
			if err := client.Patch(fmt.Sprintf("/v1/admin/clients/%d", id), patch, &updated); err != nil {
				return err
			}
			if err := a.emitJSON(updated); err != nil {
				return err
			}
			a.line("Updated client %d: %s", updated.Code, updated.Name)
			return nil
		},
	}
}

func clientsRemoveCommand() *command {
	return &command{
		name:    "rm",
		summary: "Remove a client from the directory (admin).",
		usage:   "vera clients rm <id>",
		run: func(a *app) error {
			if err := a.requireArgs(1, "a client id"); err != nil {
				return err
			}
			id, err := a.parseID("client id", a.args[0])
			if err != nil {
				return err
			}
			if err := a.confirm("Remove client %d? A client with quotations cannot be removed.", id); err != nil {
				return err
			}
			client, err := a.client()
			if err != nil {
				return err
			}
			if err := client.Delete(fmt.Sprintf("/v1/admin/clients/%d", id), nil); err != nil {
				return err
			}
			if err := a.emitJSON(map[string]any{"removed": id}); err != nil {
				return err
			}
			a.line("Removed client %d.", id)
			return nil
		},
	}
}
