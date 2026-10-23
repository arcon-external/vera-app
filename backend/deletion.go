package backend

import (
	"encoding/json"

	"vera-app/backend/apiclient"
	"vera-app/backend/apicontract"
)

// Issue #480: desktop bindings for permanent deletion. The desktop has no HTTP
// layer of its own -- it calls the same authenticated API as the CLI -- so these
// methods are thin, typed wrappers over /v1/deletions.
//
// ConfirmDeletion returns a typed result rather than relying on an error shape:
// a changed chain, a dead challenge and a completed deletion are three outcomes
// the modal must tell apart, and Wails hands the frontend an error as prose.

// DeletionRecordDTO is one document a deletion scope removes or keeps.
type DeletionRecordDTO struct {
	Type             string
	UUID             string
	DocumentNumber   string
	SourceReference  string
	Label            string
	State            string
	Revisions        []int16
	ID               int64
	Version          int64
	DocumentVersions int
	HistoryEntries   int
}

// DeletionScopeDTO is one selectable scope with its actual consequences.
type DeletionScopeDTO struct {
	Scope             string
	UnavailableReason string
	Removes           []DeletionRecordDTO
	Preserves         []DeletionRecordDTO
	Available         bool
	RequiresChallenge bool
}

// DeletionPreviewDTO is the exact chain an admin is asked to confirm.
type DeletionPreviewDTO struct {
	TargetType          string
	TargetUUID          string
	DocumentNumber      string
	DocumentNumberLabel string
	QuoteNumber         string
	PONumber            string
	CustomerName        string
	State               string
	Fingerprint         string
	Scopes              []DeletionScopeDTO
	TargetID            int64
	Version             int64
	SourceMissing       bool
}

// DeletionPreviewInput names one record for the preview.
type DeletionPreviewInput struct {
	TargetType string
	TargetID   int64
}

// DeletionChallengeInput asks for the whole-chain second confirmation. The
// expected fingerprint is the preview the admin read, so a challenge is never
// issued against a chain that changed unseen.
type DeletionChallengeInput struct {
	TargetType          string
	TargetID            int64
	Scope               string
	ExpectedFingerprint string
}

// DeletionChallengeResult is the typed answer to CreateDeletionChallenge:
// Challenge when letters were issued, Changed when the chain moved since the
// preview (the modal shows the changes and renews the confirmation).
type DeletionChallengeResult struct {
	Challenge *DeletionChallengeDTO
	Changed   *DeletionChangedDTO
}

// DeletionChallengeDTO is the ten letters, returned once. Losing it means
// asking for a new challenge; the server keeps only the hash.
type DeletionChallengeDTO struct {
	ChallengeID string
	Letters     string
	ExpiresAt   string
	Fingerprint string
}

// DeletionConfirmInput is the deliberate confirmation. ExpectedFingerprint is
// mandatory: record and chain deletions are both bound to what was previewed.
type DeletionConfirmInput struct {
	TargetType          string
	TargetID            int64
	Scope               string
	Reason              string
	TypedNumber         string
	ExpectedFingerprint string
	ChallengeID         string
	ChallengeLetters    string
}

// DeletionChangeDTO is one named difference the modal explains.
type DeletionChangeDTO struct {
	Field  string
	Label  string
	Before string
	After  string
}

// DeletionChangedDTO is a refused confirmation: the chain moved, and the fresh
// preview is what the renewed confirmation must be based on.
type DeletionChangedDTO struct {
	Preview *DeletionPreviewDTO
	Changes []DeletionChangeDTO
}

// DeletionOutcomeDTO is one committed deletion.
type DeletionOutcomeDTO struct {
	Removed  []DeletionRecordDTO
	AuditIDs []int64
}

// DeletionConfirmResult is the typed answer to ConfirmDeletion. Exactly one side
// is set: Outcome when the deletion committed, Changed when the chain moved (the
// modal explains the changes and renews the confirmation), or ChallengeProblem
// ("required", "invalid" or "expired") when the second confirmation is dead and
// a fresh challenge is needed. A real failure is still the second return value.
type DeletionConfirmResult struct {
	Outcome          *DeletionOutcomeDTO
	Changed          *DeletionChangedDTO
	ChallengeProblem string
}

// PreviewDeletion describes one record and the scopes deletion can take.
func (q *Quotation) PreviewDeletion(input DeletionPreviewInput) (DeletionPreviewDTO, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return DeletionPreviewDTO{}, err
	}
	client, err := api()
	if err != nil {
		return DeletionPreviewDTO{}, err
	}
	var response apicontract.DeletionPreviewResponse
	if err := client.Post("/v1/deletions/preview", apicontract.DeletionPreviewRequest{
		TargetType: input.TargetType,
		TargetID:   input.TargetID,
	}, &response); err != nil {
		return DeletionPreviewDTO{}, err
	}
	return mapDeletionPreviewDTO(response), nil
}

// CreateDeletionChallenge issues the ten-letter second confirmation.
func (q *Quotation) CreateDeletionChallenge(input DeletionChallengeInput) (DeletionChallengeResult, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return DeletionChallengeResult{}, err
	}
	client, err := api()
	if err != nil {
		return DeletionChallengeResult{}, err
	}
	var response apicontract.DeletionChallengeResponse
	err = client.Post("/v1/deletions/challenge", apicontract.DeletionChallengeRequest{
		TargetType:          input.TargetType,
		TargetID:            input.TargetID,
		Scope:               input.Scope,
		ExpectedFingerprint: input.ExpectedFingerprint,
	}, &response)
	if err != nil {
		if conflict := apiclient.DeletionOf(err); conflict != nil {
			changed := deletionChangedFrom(conflict)
			return DeletionChallengeResult{Changed: changed}, nil
		}
		return DeletionChallengeResult{}, err
	}
	return DeletionChallengeResult{Challenge: &DeletionChallengeDTO{
		ChallengeID: response.ChallengeID,
		Letters:     response.Letters,
		ExpiresAt:   response.ExpiresAt,
		Fingerprint: response.Fingerprint,
	}}, nil
}

// deletionChangedFrom maps the API's changed-chain body to the typed modal
// value shared by the challenge and confirmation calls.
func deletionChangedFrom(conflict *apiclient.DeletionConflictInfo) *DeletionChangedDTO {
	changed := &DeletionChangedDTO{
		Changes: make([]DeletionChangeDTO, 0, len(conflict.Changes)),
	}
	for _, change := range conflict.Changes {
		changed.Changes = append(changed.Changes, DeletionChangeDTO{
			Field: change.Field, Label: change.Label,
			Before: change.Before, After: change.After,
		})
	}
	if len(conflict.Preview) > 0 {
		var preview apicontract.DeletionPreviewResponse
		if json.Unmarshal(conflict.Preview, &preview) == nil {
			mapped := mapDeletionPreviewDTO(preview)
			changed.Preview = &mapped
		}
	}
	return changed
}

// ConfirmDeletion performs the irreversible write and keeps every refusal the
// modal can act on typed.
func (q *Quotation) ConfirmDeletion(input DeletionConfirmInput) (DeletionConfirmResult, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return DeletionConfirmResult{}, err
	}
	client, err := api()
	if err != nil {
		return DeletionConfirmResult{}, err
	}
	var response apicontract.DeletionConfirmResponse
	err = client.Post("/v1/deletions", apicontract.DeletionConfirmRequest{
		TargetType:          input.TargetType,
		TargetID:            input.TargetID,
		Scope:               input.Scope,
		Reason:              input.Reason,
		TypedNumber:         input.TypedNumber,
		ExpectedFingerprint: input.ExpectedFingerprint,
		ChallengeID:         input.ChallengeID,
		ChallengeLetters:    input.ChallengeLetters,
	}, &response)
	if err != nil {
		if conflict := apiclient.DeletionOf(err); conflict != nil {
			return DeletionConfirmResult{Changed: deletionChangedFrom(conflict)}, nil
		}
		switch apiclient.CodeOf(err) {
		case "deletion_challenge_required":
			return DeletionConfirmResult{ChallengeProblem: "required"}, nil
		case "deletion_challenge_invalid":
			return DeletionConfirmResult{ChallengeProblem: "invalid"}, nil
		case "deletion_challenge_expired":
			return DeletionConfirmResult{ChallengeProblem: "expired"}, nil
		}
		return DeletionConfirmResult{}, err
	}
	outcome := &DeletionOutcomeDTO{
		Removed:  make([]DeletionRecordDTO, 0, len(response.Removed)),
		AuditIDs: response.AuditIDs,
	}
	for _, record := range response.Removed {
		outcome.Removed = append(outcome.Removed, mapDeletionRecordDTO(record))
	}
	return DeletionConfirmResult{Outcome: outcome}, nil
}

func mapDeletionPreviewDTO(preview apicontract.DeletionPreviewResponse) DeletionPreviewDTO {
	mapped := DeletionPreviewDTO{
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
		Scopes:              make([]DeletionScopeDTO, 0, len(preview.Scopes)),
	}
	for _, plan := range preview.Scopes {
		mapped.Scopes = append(mapped.Scopes, DeletionScopeDTO{
			Scope:             plan.Scope,
			Available:         plan.Available,
			UnavailableReason: plan.UnavailableReason,
			RequiresChallenge: plan.RequiresChallenge,
			Removes:           mapDeletionRecordsDTO(plan.Removes),
			Preserves:         mapDeletionRecordsDTO(plan.Preserves),
		})
	}
	return mapped
}

func mapDeletionRecordsDTO(records []apicontract.DeletionRecordRow) []DeletionRecordDTO {
	mapped := make([]DeletionRecordDTO, 0, len(records))
	for _, record := range records {
		mapped = append(mapped, mapDeletionRecordDTO(record))
	}
	return mapped
}

func mapDeletionRecordDTO(record apicontract.DeletionRecordRow) DeletionRecordDTO {
	return DeletionRecordDTO{
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
