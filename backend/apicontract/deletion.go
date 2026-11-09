package apicontract

// Issue #480: the authenticated deletion surface. The preview is the only place
// the connected chain is described; the confirmation carries the fingerprint it
// was bound to, and the server refuses a chain that moved. Fields are stable
// API, mirrored by the desktop DTOs and the generated bindings.

// DeletionRecordRow is one document a deletion scope would remove (or keep).
type DeletionRecordRow struct {
	Type             string  `json:"type"`
	ID               int64   `json:"id"`
	UUID             string  `json:"uuid"`
	DocumentNumber   string  `json:"documentNumber"`
	SourceReference  string  `json:"sourceReference,omitempty"`
	Label            string  `json:"label"`
	State            string  `json:"state,omitempty"`
	Version          int64   `json:"version,omitempty"`
	Revisions        []int16 `json:"revisions,omitempty"`
	DocumentVersions int     `json:"documentVersions,omitempty"`
	HistoryEntries   int     `json:"historyEntries,omitempty"`
}

// DeletionScopeRow is one selectable scope with its actual consequences.
type DeletionScopeRow struct {
	Scope             string              `json:"scope"`
	Available         bool                `json:"available"`
	UnavailableReason string              `json:"unavailableReason,omitempty"`
	RequiresChallenge bool                `json:"requiresChallenge"`
	Removes           []DeletionRecordRow `json:"removes"`
	Preserves         []DeletionRecordRow `json:"preserves"`
}

// DeletionPreviewResponse is the exact chain an admin is asked to confirm.
type DeletionPreviewResponse struct {
	TargetType          string             `json:"targetType"`
	TargetID            int64              `json:"targetId"`
	TargetUUID          string             `json:"targetUuid"`
	DocumentNumber      string             `json:"documentNumber"`
	DocumentNumberLabel string             `json:"documentNumberLabel"`
	QuoteNumber         string             `json:"quoteNumber"`
	PONumber            string             `json:"poNumber,omitempty"`
	CustomerName        string             `json:"customerName"`
	State               string             `json:"state"`
	SourceMissing       bool               `json:"sourceMissing"`
	Version             int64              `json:"version,omitempty"`
	Fingerprint         string             `json:"fingerprint"`
	Scopes              []DeletionScopeRow `json:"scopes"`
}

// DeletionPreviewRequest names one record.
type DeletionPreviewRequest struct {
	TargetType string `json:"targetType"`
	TargetID   int64  `json:"targetId"`
}

// DeletionChallengeRequest asks for the whole-chain second confirmation. The
// expected fingerprint is the preview the admin read; a changed chain is
// refused before any letters are issued.
type DeletionChallengeRequest struct {
	TargetType          string `json:"targetType"`
	TargetID            int64  `json:"targetId"`
	Scope               string `json:"scope"`
	ExpectedFingerprint string `json:"expectedFingerprint"`
}

// DeletionChallengeResponse returns the ten letters exactly once. The server
// keeps only their hash, so losing this response means asking for a new one.
type DeletionChallengeResponse struct {
	ChallengeID string `json:"challengeId"`
	Letters     string `json:"letters"`
	ExpiresAt   string `json:"expiresAt"`
	Fingerprint string `json:"fingerprint"`
}

// DeletionConfirmRequest is the deliberate confirmation. ExpectedFingerprint is
// the preview's fingerprint for both scopes; a record deletion is bound to the
// record version exactly as a chain deletion is bound to the chain.
type DeletionConfirmRequest struct {
	TargetType          string `json:"targetType"`
	TargetID            int64  `json:"targetId"`
	Scope               string `json:"scope"`
	Reason              string `json:"reason"`
	TypedNumber         string `json:"typedNumber"`
	ExpectedFingerprint string `json:"expectedFingerprint"`
	ChallengeID         string `json:"challengeId,omitempty"`
	ChallengeLetters    string `json:"challengeLetters,omitempty"`
}

// DeletionChangeRow is one named difference between the confirmed chain and the
// chain as it stands. Code-carrying responses use it so the modal explains what
// changed instead of showing a generic refusal.
type DeletionChangeRow struct {
	Field  string `json:"field"`
	Label  string `json:"label"`
	Before string `json:"before"`
	After  string `json:"after"`
}

// DeletionConfirmResponse is what one committed deletion removed, plus the
// audit rows it left behind.
type DeletionConfirmResponse struct {
	Removed  []DeletionRecordRow `json:"removed"`
	AuditIDs []int64             `json:"auditIds"`
}

// DeletionChangedResponse is the 409 body for a stale confirmation. The fresh
// preview lets the modal show the current chain and restart the confirmation
// without guessing which request to repeat.
type DeletionChangedResponse struct {
	Error   string                   `json:"error"`
	Code    string                   `json:"code"`
	Changes []DeletionChangeRow      `json:"changes"`
	Preview *DeletionPreviewResponse `json:"preview,omitempty"`
}
