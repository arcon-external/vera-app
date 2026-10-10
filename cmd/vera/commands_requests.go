package main

import (
	"fmt"
	"strings"
)

// requestsCommands are the two queues that need an admin's decision.
//
// They exist because the two things a person can ask for about their own account
// -- delete it, or move it to a new email -- cannot take effect on the person's
// own word. A lost laptop should not be able to delete a year of quotations, and
// a stolen session should not be able to walk an account onto an address the
// real owner does not control.
//
// The two queues are separate sub-groups rather than one command with a --type
// flag: they are decided by different people for different reasons, and typing
// the noun twice is cheaper than remembering which flag you are in.
func requestsCommands() *command {
	return commandGroup("requests", "Account requests waiting for an admin's decision (admin).",
		requestsQueue("deletions",
			"Accounts asking to be deleted.",
			"/v1/admin/deletion-requests",
			"/v1/admin/deletion-requests/%d/approve",
			"/v1/admin/deletion-requests/%d/reject",
			"Permanently delete %s and everything that belongs to them?"),
		requestsQueue("emails",
			"Email address changes waiting to be approved.",
			"/v1/admin/email-change-requests",
			"/v1/admin/email-change-requests/%d/approve",
			"/v1/admin/email-change-requests/%d/reject",
			"Move the account to %s?"),
	)
}

// requestsQueue builds the list/approve/reject trio the two queues share.
//
// They differ only in their paths and in how a row reads, so writing them once
// means a fix to the decision path cannot land in one queue and miss the other.
func requestsQueue(noun, summary, listPath, approvePath, rejectPath, question string) *command {
	return commandGroup(noun, summary,
		&command{
			name:    "list",
			summary: "Show what is waiting.",
			usage:   fmt.Sprintf("vera requests %s list", noun),
			run: func(a *app) error {
				if err := a.requireArgs(0, "no arguments"); err != nil {
					return err
				}
				client, err := a.client()
				if err != nil {
					return err
				}
				// Both queues answer with the same envelope and the same row
				// fields; when they do not, the extra field simply comes back
				// empty rather than failing the decode.
				var response struct {
					Requests []struct {
						ID            int64  `json:"id"`
						UserID        int64  `json:"userId"`
						Email         string `json:"email"`
						DisplayName   string `json:"displayName"`
						Status        string `json:"status"`
						ProposedEmail string `json:"proposedEmail"`
						RequestedAt   string `json:"requestedAt"`
					} `json:"requests"`
				}
				if err := client.Get(listPath, &response); err != nil {
					return err
				}
				if err := a.emitJSON(map[string]any{"requests": response.Requests, "count": len(response.Requests)}); err != nil {
					return err
				}
				if len(response.Requests) == 0 {
					a.line("Nothing waiting.")
					return nil
				}
				rows := make([][]string, 0, len(response.Requests))
				for _, row := range response.Requests {
					who := row.Email
					if who == "" {
						who = fmt.Sprintf("user %d", row.UserID)
					}
					detail := row.DisplayName
					if row.ProposedEmail != "" {
						detail = "wants " + row.ProposedEmail
					}
					rows = append(rows, []string{
						fmt.Sprintf("%d", row.ID), who, detail, row.Status, row.RequestedAt,
					})
				}
				a.table([]string{"ID", "ACCOUNT", "DETAIL", "STATUS", "REQUESTED"}, rows)
				if noun == "deletions" {
					a.note("Approving deletes the account for good, so nothing that belongs to them can be attributed any more.")
				}
				return nil
			},
		},
		&command{
			name:    "approve",
			summary: "Carry the request out.",
			usage:   fmt.Sprintf("vera requests %s approve <id>", noun),
			run: func(a *app) error {
				return a.decideRequest(noun, "approve", approvePath, question)
			},
		},
		&command{
			name:    "reject",
			summary: "Turn the request down, leaving things as they are.",
			usage:   fmt.Sprintf("vera requests %s reject <id>", noun),
			run: func(a *app) error {
				return a.decideRequest(noun, "reject", rejectPath, "")
			},
		},
	)
}

// decideRequest runs one decision.
//
// The confirmation names the account rather than the request id, because the
// caller is about to delete a person's history and \"request 7\" does not carry
// enough meaning to be worth confirming.
func (a *app) decideRequest(noun, decision, path string, question string) error {
	if err := a.requireArgs(1, "a request id"); err != nil {
		return err
	}
	id, err := a.parseID("request id", a.args[0])
	if err != nil {
		return err
	}

	if question != "" {
		subject := fmt.Sprintf("request %d", id)
		if client, err := a.client(); err == nil {
			if account, err := a.requestSubject(client, noun, id); err == nil && account != "" {
				subject = account
			}
		}
		if err := a.confirm(question, subject); err != nil {
			return err
		}
	}

	client, err := a.client()
	if err != nil {
		return err
	}
	var response struct {
		Status string `json:"status"`
	}
	if err := client.Post(fmt.Sprintf(path, id), nil, &response); err != nil {
		return err
	}
	result := map[string]any{"id": id, "decision": decision, "status": response.Status}
	if err := a.emitJSON(result); err != nil {
		return err
	}
	if response.Status != "" {
		a.line("Request %d: %s.", id, response.Status)
	} else {
		a.line("%s request %d.", titleCase(decision), id)
	}
	return nil
}

// requestSubject looks up who a request is about, for the confirmation prompt.
// A failure here is not fatal: the decision still has to be possible when the
// listing is unavailable.
func (a *app) requestSubject(client interface {
	Get(string, any) error
}, noun string, id int64) (string, error) {
	path := "/v1/admin/deletion-requests"
	if noun == "emails" {
		path = "/v1/admin/email-change-requests"
	}
	var response struct {
		Requests []struct {
			ID            int64  `json:"id"`
			Email         string `json:"email"`
			ProposedEmail string `json:"proposedEmail"`
		} `json:"requests"`
	}
	if err := client.Get(path, &response); err != nil {
		return "", err
	}
	for _, row := range response.Requests {
		if row.ID != id {
			continue
		}
		if row.ProposedEmail != "" {
			return fmt.Sprintf("%s (now %s)", row.Email, row.ProposedEmail), nil
		}
		return row.Email, nil
	}
	return "", nil
}

func titleCase(word string) string {
	if word == "" {
		return word
	}
	return strings.ToUpper(word[:1]) + word[1:]
}
