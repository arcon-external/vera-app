package main

import (
	"encoding/json"
	"flag"
	"fmt"
)

// Drafts are private to their owner -- admins included -- which is why they are
// their own noun rather than part of the quotation archive. A draft is the state
// of an unfinished editor, saved unvalidated so half-typed work survives.
func draftsCommands() *command {
	return commandGroup("drafts", "Unfinished quotations, private to you.",
		draftsListCommand(),
		draftsSaveCommand(),
		draftsShowCommand(),
		draftsRemoveCommand(),
	)
}

// DraftSummary is one row of the drafts list.
type DraftSummary struct {
	ID           int64  `json:"id"`
	Label        string `json:"label"`
	Subject      string `json:"subject"`
	CustomerName string `json:"customerName"`
	CustomerID   *int64 `json:"customerId"`
	ItemCount    int    `json:"itemCount"`
	UpdatedAt    string `json:"updatedAt"`
}

func draftsListCommand() *command {
	return &command{
		name:    "list",
		summary: "List your drafts.",
		usage:   "vera drafts list",
		run: func(a *app) error {
			if err := a.requireArgs(0, "no arguments"); err != nil {
				return err
			}
			client, err := a.client()
			if err != nil {
				return err
			}
			var response struct {
				Drafts []DraftSummary `json:"drafts"`
			}
			if err := client.Get("/v1/drafts", &response); err != nil {
				return err
			}
			if err := a.emitJSON(map[string]any{"drafts": response.Drafts, "count": len(response.Drafts)}); err != nil {
				return err
			}
			if len(response.Drafts) == 0 {
				a.line("No drafts.")
				return nil
			}
			rows := make([][]string, 0, len(response.Drafts))
			for _, draft := range response.Drafts {
				name := draft.Label
				if name == "" {
					name = draft.Subject
				}
				if name == "" {
					name = draft.CustomerName
				}
				if name == "" {
					name = "(untitled)"
				}
				rows = append(rows, []string{
					fmt.Sprintf("%d", draft.ID),
					truncate(name, 44),
					draft.CustomerName,
					fmt.Sprintf("%d", draft.ItemCount),
					draft.UpdatedAt,
				})
			}
			a.table([]string{"ID", "NAME", "CLIENT", "ITEMS", "SAVED"}, rows)
			return nil
		},
	}
}

// draftsSaveCommand stores an input file as a draft.
//
// The payload carries the same JSON the export commands read, wrapped so the
// editor could resume it: a draft is editor state, not a filed document.
func draftsSaveCommand() *command {
	var input, label string
	var draftID int64
	return &command{
		name:    "save",
		summary: "Save an input file as a draft, or overwrite one.",
		usage:   `vera drafts save --input quotation.json [--label "..." ] [--draft N]`,
		flags: func(fs *flag.FlagSet) {
			fs.StringVar(&input, "input", "", "the quotation JSON file, or - for stdin")
			fs.StringVar(&label, "label", "", "a name to find it by later (defaults to the subject)")
			fs.Int64Var(&draftID, "draft", 0, "overwrite this draft instead of creating one")
		},
		run: func(a *app) error {
			if err := a.requireArgs(0, "no arguments"); err != nil {
				return err
			}
			doc, err := a.loadInputDoc(input)
			if err != nil {
				return err
			}
			raw, err := readInputFile(input)
			if err != nil {
				return err
			}
			name := label
			if name == "" {
				name = doc.Subject
			}
			client, err := a.client()
			if err != nil {
				return err
			}
			var saved struct {
				ID        int64  `json:"id"`
				UpdatedAt string `json:"updatedAt"`
			}
			if err := client.Post("/v1/drafts", map[string]any{
				"id":           draftID,
				"label":        name,
				"subject":      doc.Subject,
				"customerName": "",
				"customerId":   doc.CustomerCode,
				"itemCount":    len(doc.Items),
				"payload":      json.RawMessage(raw),
			}, &saved); err != nil {
				return err
			}
			if err := a.emitJSON(map[string]any{"id": saved.ID, "updatedAt": saved.UpdatedAt, "label": name}); err != nil {
				return err
			}
			a.line("Saved draft %d.", saved.ID)
			return nil
		},
	}
}

func draftsShowCommand() *command {
	return &command{
		name:    "show",
		summary: "Print one draft, payload included.",
		usage:   "vera drafts show <id>",
		run: func(a *app) error {
			if err := a.requireArgs(1, "a draft id"); err != nil {
				return err
			}
			id, err := a.parseID("draft id", a.args[0])
			if err != nil {
				return err
			}
			client, err := a.client()
			if err != nil {
				return err
			}
			var draft map[string]any
			if err := client.Get(fmt.Sprintf("/v1/drafts/%d", id), &draft); err != nil {
				return err
			}
			return a.emitJSON(draft)
		},
	}
}

func draftsRemoveCommand() *command {
	return &command{
		name:    "rm",
		summary: "Delete one of your drafts.",
		usage:   "vera drafts rm <id>",
		run: func(a *app) error {
			if err := a.requireArgs(1, "a draft id"); err != nil {
				return err
			}
			id, err := a.parseID("draft id", a.args[0])
			if err != nil {
				return err
			}
			if err := a.confirm("Delete draft %d?", id); err != nil {
				return err
			}
			client, err := a.client()
			if err != nil {
				return err
			}
			if err := client.Delete(fmt.Sprintf("/v1/drafts/%d", id), nil); err != nil {
				return err
			}
			if err := a.emitJSON(map[string]any{"removed": id}); err != nil {
				return err
			}
			a.line("Deleted draft %d.", id)
			return nil
		},
	}
}
