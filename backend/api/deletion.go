package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"vera-app/backend/apicontract"
	"vera-app/backend/store"
)

// Issue #480: admin-only permanent deletion of a Quotation/PO record or its
// chain. Every route is gated by requireAdmin; the store re-reads the actor's
// stored role inside the same transaction, so the gate is not the only one.
//
// The flow is three calls: preview (what would go), challenge (the ten-letter
// second confirmation for a chain), confirm (the irreversible write). The
// confirmation carries the preview's fingerprint, so a chain that changed in
// between is refused with a 409 that names the changes and includes a fresh
// preview. Nothing is deleted on a stale confirmation.

func (s *Server) previewDeletion(context echo.Context) error {
	var request apicontract.DeletionPreviewRequest
	if err := context.Bind(&request); err != nil {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	user := requestUser(context)
	requestContext, err := s.organizationContext(context.Request().Context())
	if err != nil {
		return s.deletionError(context, request.TargetType, request.TargetID, err)
	}
	preview, err := s.database.PreviewDeletion(requestContext, store.DeletionPreviewInput{
		TargetType: request.TargetType,
		TargetID:   request.TargetID,
		ActorID:    user.ID,
	})
	if err != nil {
		return s.deletionError(context, request.TargetType, request.TargetID, err)
	}
	return context.JSON(http.StatusOK, mapDeletionPreview(preview))
}

func (s *Server) createDeletionChallenge(context echo.Context) error {
	var request apicontract.DeletionChallengeRequest
	if err := context.Bind(&request); err != nil {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	user := requestUser(context)
	requestContext, err := s.organizationContext(context.Request().Context())
	if err != nil {
		return s.deletionError(context, request.TargetType, request.TargetID, err)
	}
	challenge, err := s.database.CreateDeletionChallenge(requestContext, store.DeletionChallengeInput{
		TargetType:          request.TargetType,
		TargetID:            request.TargetID,
		Scope:               request.Scope,
		ExpectedFingerprint: request.ExpectedFingerprint,
		ActorID:             user.ID,
	})
	if err != nil {
		return s.deletionError(context, request.TargetType, request.TargetID, err)
	}
	return context.JSON(http.StatusCreated, apicontract.DeletionChallengeResponse{
		ChallengeID: challenge.ID,
		Letters:     challenge.Letters,
		ExpiresAt:   challenge.ExpiresAt.Format(time.RFC3339),
		Fingerprint: challenge.Fingerprint,
	})
}

func (s *Server) confirmDeletion(context echo.Context) error {
	var request apicontract.DeletionConfirmRequest
	if err := context.Bind(&request); err != nil {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	user := requestUser(context)
	requestContext, err := s.organizationContext(context.Request().Context())
	if err != nil {
		return s.deletionError(context, request.TargetType, request.TargetID, err)
	}
	result, err := s.database.ConfirmDeletion(requestContext, store.DeletionConfirmInput{
		TargetType:          request.TargetType,
		TargetID:            request.TargetID,
		Scope:               request.Scope,
		Reason:              request.Reason,
		TypedNumber:         request.TypedNumber,
		ExpectedFingerprint: request.ExpectedFingerprint,
		ChallengeID:         request.ChallengeID,
		ChallengeLetters:    request.ChallengeLetters,
		ActorID:             user.ID,
	})
	if err != nil {
		return s.deletionError(context, request.TargetType, request.TargetID, err)
	}
	response := apicontract.DeletionConfirmResponse{
		Removed:  make([]apicontract.DeletionRecordRow, 0, len(result.Removed)),
		AuditIDs: result.AuditIDs,
	}
	for _, record := range result.Removed {
		response.Removed = append(response.Removed, mapDeletionRecord(record))
	}
	return context.JSON(http.StatusOK, response)
}

// deletionError maps the store's named refusals to stable response codes. The
// modal branches on the code, never on the message: a changed chain must
// restart the confirmation, an expired challenge must ask for a new one, and a
// typed number must stay typed.
func (s *Server) deletionError(context echo.Context, targetType string, targetID int64, err error) error {
	var changed *store.DeletionChainChangedError
	if errors.As(err, &changed) {
		response := apicontract.DeletionChangedResponse{
			Error:   err.Error(),
			Code:    "deletion_chain_changed",
			Changes: make([]apicontract.DeletionChangeRow, 0, len(changed.Changes)),
		}
		for _, change := range changed.Changes {
			response.Changes = append(response.Changes, apicontract.DeletionChangeRow{
				Field: change.Field, Label: change.Label,
				Before: change.Before, After: change.After,
			})
		}
		// A fresh preview is best-effort: the target may itself be gone now,
		// and a 409 with changes and no preview is still more useful than a
		// 500.
		previewContext, previewErr := s.organizationContext(context.Request().Context())
		if previewErr != nil {
			return context.JSON(http.StatusConflict, response)
		}
		if preview, previewErr := s.database.PreviewDeletion(
			previewContext, store.DeletionPreviewInput{
				TargetType: targetType, TargetID: targetID,
				ActorID: requestUser(context).ID,
			}); previewErr == nil {
			mapped := mapDeletionPreview(preview)
			response.Preview = &mapped
		}
		return context.JSON(http.StatusConflict, response)
	}
	switch {
	case errors.Is(err, store.ErrDeletionForbidden):
		return context.JSON(http.StatusForbidden, map[string]string{
			"error": err.Error(), "code": "deletion_forbidden",
		})
	case errors.Is(err, store.ErrDeletionTargetNotFound):
		return context.JSON(http.StatusNotFound, map[string]string{"error": "record not found"})
	case errors.Is(err, store.ErrDeletionReasonRequired):
		return context.JSON(http.StatusUnprocessableEntity, map[string]string{
			"error": err.Error(), "code": "deletion_reason_required", "field": "reason",
		})
	case errors.Is(err, store.ErrDeletionTypedNumberMismatch):
		return context.JSON(http.StatusUnprocessableEntity, map[string]string{
			"error": err.Error(), "code": "deletion_typed_number_mismatch", "field": "typedNumber",
		})
	case errors.Is(err, store.ErrDeletionChallengeRequired):
		return context.JSON(http.StatusConflict, map[string]string{
			"error": err.Error(), "code": "deletion_challenge_required",
		})
	case errors.Is(err, store.ErrDeletionChallengeInvalid):
		return context.JSON(http.StatusConflict, map[string]string{
			"error": err.Error(), "code": "deletion_challenge_invalid",
		})
	case errors.Is(err, store.ErrDeletionChallengeExpired):
		return context.JSON(http.StatusConflict, map[string]string{
			"error": err.Error(), "code": "deletion_challenge_expired",
		})
	case errors.Is(err, store.ErrDeletionScopeUnavailable):
		return context.JSON(http.StatusUnprocessableEntity, map[string]string{
			"error": err.Error(), "code": "deletion_scope_unavailable", "field": "scope",
		})
	case errors.Is(err, store.ErrDeletionInvalid):
		return context.JSON(http.StatusBadRequest, map[string]string{
			"error": err.Error(), "code": "deletion_invalid",
		})
	}
	return context.JSON(http.StatusInternalServerError, map[string]string{"error": "delete the record"})
}

func mapDeletionPreview(preview store.DeletionPreview) apicontract.DeletionPreviewResponse {
	response := apicontract.DeletionPreviewResponse{
		TargetType:          preview.TargetType,
		TargetID:            preview.TargetID,
		TargetUUID:          preview.TargetUUID,
		DocumentNumber:      preview.DocumentNumber,
		DocumentNumberLabel: preview.DocumentNumberLabel,
		QuoteNumber:         preview.QuoteNumber,
		PONumber:            preview.PONumber,
		CustomerName:        preview.CustomerName,
		State:               preview.State,
		SourceMissing:       preview.SourceMissing,
		Version:             preview.Version,
		Fingerprint:         preview.Fingerprint,
		Scopes:              make([]apicontract.DeletionScopeRow, 0, len(preview.Scopes)),
	}
	for _, plan := range preview.Scopes {
		row := apicontract.DeletionScopeRow{
			Scope:             plan.Scope,
			Available:         plan.Available,
			UnavailableReason: plan.UnavailableReason,
			RequiresChallenge: plan.RequiresChallenge,
			Removes:           mapDeletionRecords(plan.Removes),
			Preserves:         mapDeletionRecords(plan.Preserves),
		}
		response.Scopes = append(response.Scopes, row)
	}
	return response
}

func mapDeletionRecords(records []store.DeletionRecord) []apicontract.DeletionRecordRow {
	rows := make([]apicontract.DeletionRecordRow, 0, len(records))
	for _, record := range records {
		rows = append(rows, mapDeletionRecord(record))
	}
	return rows
}

func mapDeletionRecord(record store.DeletionRecord) apicontract.DeletionRecordRow {
	return apicontract.DeletionRecordRow{
		Type:             record.Type,
		ID:               record.ID,
		UUID:             record.UUID,
		DocumentNumber:   record.DocumentNumber,
		SourceReference:  record.SourceReference,
		Label:            record.Label,
		State:            record.State,
		Version:          record.Version,
		Revisions:        append(make([]int16, 0, len(record.Revisions)), record.Revisions...),
		DocumentVersions: record.DocumentVersions,
		HistoryEntries:   record.HistoryEntries,
	}
}
