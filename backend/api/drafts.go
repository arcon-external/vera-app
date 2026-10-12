package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	"vera-app/backend/store"
)

// draftRequest is one save. ID is zero to create, set to overwrite.
//
// The payload is the editor state verbatim. It is deliberately not validated
// here: a draft is a scratchpad, and half a quotation is the normal thing to
// save. Prepare validates properly on export, which is where validity matters.
type draftRequest struct {
	Label        string          `json:"label"`
	Subject      string          `json:"subject"`
	CustomerName string          `json:"customerName"`
	CustomerID   *int64          `json:"customerId"`
	Payload      json.RawMessage `json:"payload"`
	ID           int64           `json:"id"`
	ItemCount    int             `json:"itemCount"`
}

// draftDTO is a list row. The payload is left out deliberately: opening the
// drafts page should not pull every saved quotation's editor state over the
// wire to render four columns.
func draftDTO(draft store.DraftSummary) map[string]any {
	return map[string]any{
		"id":           draft.ID,
		"label":        draft.Label,
		"subject":      draft.Subject,
		"customerName": draft.CustomerName,
		"itemCount":    draft.ItemCount,
		"updatedAt":    draft.UpdatedAt.Format(time.RFC3339),
		"displayName":  store.DraftDisplayName(draft.Label, draft.Subject, draft.CustomerName),
	}
}

// listDrafts serves the caller's own drafts, newest first. The handler has no
// id to get wrong: ownership is decided in the SQL, so there is no way to
// reach another account's list.
func (s *Server) listDrafts(context echo.Context) error {
	user, _ := context.Get("user").(store.AuthUser)
	drafts, err := s.database.ListDrafts(context.Request().Context(), user.ID)
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "list drafts"})
	}
	result := make([]map[string]any, 0, len(drafts))
	for _, draft := range drafts {
		result = append(result, draftDTO(draft))
	}
	return context.JSON(http.StatusOK, map[string]any{"drafts": result})
}

// saveDraft creates or overwrites a draft. Both are the same endpoint because
// the client cannot know its draft id until the first save succeeds, and a
// split create/update pair would make "save again" two calls to get right.
func (s *Server) saveDraft(context echo.Context) error {
	user, _ := context.Get("user").(store.AuthUser)
	var request draftRequest
	if err := context.Bind(&request); err != nil {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if len(request.Payload) == 0 {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "nothing to save"})
	}
	if request.ItemCount < 0 {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid item count"})
	}
	// The client reference is a CODE, which the quote-number format fixes at two
	// digits, so anything outside that is not a client however it got here.
	// Checking it here answers with the message a user can act on; letting it
	// reach the store instead fails to encode a smallint and reports a 500.
	if request.CustomerID != nil && (*request.CustomerID < 1 || *request.CustomerID > 99) {
		return context.JSON(http.StatusBadRequest, map[string]string{
			"error": "That client is no longer in the directory. Pick another one and save again."})
	}

	ctx := context.Request().Context()
	var draft store.Draft
	var err error
	if request.ID > 0 {
		draft, err = s.database.UpdateDraft(ctx, request.ID, user.ID, request.Label,
			request.Subject, request.CustomerName, request.CustomerID,
			request.ItemCount, request.Payload)
	} else {
		// The currency is stamped server-side from the live company profile,
		// never trusted from the client: it is the denomination the editor's
		// integers are written in, and opening the draft later blocks when
		// the profile has moved on. An update keeps the original stamp (see
		// UpdateDraft), so a save under a new profile cannot relabel old
		// amounts.
		profile, profileErr := s.database.GetCompanyProfile(ctx)
		if profileErr != nil {
			return context.JSON(http.StatusInternalServerError, map[string]string{"error": "save draft"})
		}
		draft, err = s.database.CreateDraft(ctx, user.ID, request.Label, request.Subject,
			request.CustomerName, request.CustomerID, request.ItemCount,
			profile.Currency, request.Payload)
	}
	if err != nil {
		switch {
		// Someone else's draft, or one that never existed. Both are 404 for
		// the same reason: a 403 here would confirm that the id exists.
		case errors.Is(err, store.ErrDraftNotFound):
			return context.JSON(http.StatusNotFound, map[string]string{"error": "draft not found"})
		case errors.Is(err, store.ErrDraftCustomerUnknown):
			return context.JSON(http.StatusBadRequest, map[string]string{
				"error": "That client is no longer in the directory. Pick another one and save again."})
		default:
			return context.JSON(http.StatusInternalServerError, map[string]string{"error": "save draft"})
		}
	}

	return context.JSON(http.StatusOK, map[string]any{
		"id":        draft.ID,
		"updatedAt": draft.UpdatedAt.Format(time.RFC3339),
	})
}

// getDraft returns one draft with its payload, for resuming.
func (s *Server) getDraft(context echo.Context) error {
	user, _ := context.Get("user").(store.AuthUser)
	draftID, ok := parseDraftID(context)
	if !ok {
		return nil
	}
	draft, err := s.database.GetDraft(context.Request().Context(), draftID, user.ID)
	if errors.Is(err, store.ErrDraftNotFound) {
		return context.JSON(http.StatusNotFound, map[string]string{"error": "draft not found"})
	}
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "load draft"})
	}
	return context.JSON(http.StatusOK, map[string]any{
		"id":           draft.ID,
		"label":        draft.Label,
		"subject":      draft.Subject,
		"customerName": draft.CustomerName,
		"customerId":   draft.CustomerID,
		"itemCount":    draft.ItemCount,
		"currency":     draft.Currency,
		"payload":      draft.Payload,
		"createdAt":    draft.CreatedAt.Format(time.RFC3339),
		"updatedAt":    draft.UpdatedAt.Format(time.RFC3339),
	})
}

// deleteDraft removes one of the caller's drafts.
func (s *Server) deleteDraft(context echo.Context) error {
	user, _ := context.Get("user").(store.AuthUser)
	draftID, ok := parseDraftID(context)
	if !ok {
		return nil
	}
	err := s.database.DeleteDraft(context.Request().Context(), draftID, user.ID)
	if errors.Is(err, store.ErrDraftNotFound) {
		return context.JSON(http.StatusNotFound, map[string]string{"error": "draft not found"})
	}
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "delete draft"})
	}
	return context.NoContent(http.StatusNoContent)
}

func parseDraftID(context echo.Context) (int64, bool) {
	id, err := strconv.ParseInt(context.Param("id"), 10, 64)
	if err != nil || id < 1 {
		_ = context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid draft id"})
		return 0, false
	}
	return id, true
}
