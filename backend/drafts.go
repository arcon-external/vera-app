package backend

import (
	"encoding/json"
	"fmt"

	"vera-app/backend/apicontract"
)

// DraftSummaryDTO is one row of the drafts list. The payload is deliberately
// absent: opening the drafts page should not pull every saved quotation's
// editor state over the wire to render four columns.
type DraftSummaryDTO struct {
	DisplayName  string
	Label        string
	Subject      string
	CustomerName string
	UpdatedAt    string
	ID           int64
	ItemCount    int
}

// DraftDTO is one draft with its editor state, for resuming.
type DraftDTO struct {
	Payload      string
	Subject      string
	CustomerName string
	UpdatedAt    string
	Label        string
	ID           int64
	ItemCount    int
	CustomerID   int64
}

// SavedDraft is what the editor hands over on save. Payload is the editor
// state as JSON; the label fields are denormalised copies so the drafts list
// can be rendered without decoding every row.
type SavedDraft struct {
	Payload      string
	Subject      string
	CustomerName string
	Label        string
	ID           int64
	ItemCount    int
	CustomerID   int64
}

// ListDrafts returns the signed-in user's own saved quotations, most recently
// changed first.
func (q *Quotation) ListDrafts() ([]DraftSummaryDTO, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return nil, err
	}
	client, err := api()
	if err != nil {
		return nil, err
	}
	var response struct {
		Drafts []apicontract.DraftSummaryRow `json:"drafts"`
	}
	if err := client.Get("/v1/drafts", &response); err != nil {
		return nil, err
	}
	result := make([]DraftSummaryDTO, 0, len(response.Drafts))
	for _, row := range response.Drafts {
		result = append(result, DraftSummaryDTO{
			ID:           row.ID,
			Label:        row.Label,
			Subject:      row.Subject,
			CustomerName: row.CustomerName,
			ItemCount:    row.ItemCount,
			UpdatedAt:    row.UpdatedAt,
			DisplayName:  row.DisplayName,
		})
	}
	return result, nil
}

// GetDraft returns one saved draft with its editor state.
func (q *Quotation) GetDraft(id int64) (DraftDTO, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return DraftDTO{}, err
	}
	client, err := api()
	if err != nil {
		return DraftDTO{}, err
	}
	var row apicontract.DraftRow
	if err := client.Get("/v1/drafts/"+idParam(id), &row); err != nil {
		return DraftDTO{}, err
	}
	draft := DraftDTO{
		ID:           row.ID,
		Label:        row.Label,
		Subject:      row.Subject,
		CustomerName: row.CustomerName,
		ItemCount:    row.ItemCount,
		UpdatedAt:    row.UpdatedAt,
	}
	// The payload is raw JSON on the wire so the server never has to know the
	// editor's shape; here it stays a string for the same reason, and the
	// frontend parses it into its own types.
	if len(row.Payload) > 0 {
		draft.Payload = string(row.Payload)
	}
	if row.CustomerID != nil {
		draft.CustomerID = *row.CustomerID
	}
	return draft, nil
}

// SaveDraft stores the editor state, or overwrites the draft the editor is
// already holding (SavedDraft.ID set). It returns the draft id, which the
// editor keeps so the next save updates the same row rather than piling up
// copies.
func (q *Quotation) SaveDraft(draft SavedDraft) (int64, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return 0, err
	}
	if draft.Payload == "" {
		return 0, fmt.Errorf("there is nothing to save yet")
	}
	// The payload must be JSON the server can store at all. Checking here
	// turns a silent server rejection into an error the editor can show.
	if !json.Valid([]byte(draft.Payload)) {
		return 0, fmt.Errorf("the quotation state could not be saved: it is not valid data")
	}
	client, err := api()
	if err != nil {
		return 0, err
	}
	body := map[string]any{
		"label":        draft.Label,
		"subject":      draft.Subject,
		"customerName": draft.CustomerName,
		"itemCount":    draft.ItemCount,
		"payload":      json.RawMessage(draft.Payload),
	}
	if draft.CustomerID > 0 {
		body["customerId"] = draft.CustomerID
	}
	if draft.ID > 0 {
		body["id"] = draft.ID
	}
	var response struct {
		ID int64 `json:"id"`
	}
	if err := client.Post("/v1/drafts", body, &response); err != nil {
		return 0, err
	}
	return response.ID, nil
}

// DeleteDraft removes one of the signed-in user's saved quotations.
func (q *Quotation) DeleteDraft(id int64) error {
	if err := q.requireUser(q.ctx); err != nil {
		return err
	}
	client, err := api()
	if err != nil {
		return err
	}
	return client.Delete("/v1/drafts/"+idParam(id), nil)
}

func idParam(id int64) string {
	return fmt.Sprintf("%d", id)
}
