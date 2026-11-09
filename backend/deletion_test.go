package backend

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// Issue #480: the deletion binding keeps the refusals the modal must act on
// typed. A changed chain is a value, not an error string; a dead challenge is a
// named problem; a real refusal (403 for a non-admin) is still an error.

func TestDeletionBindingSurfacesServerForbidden(t *testing.T) {
	stubAPIClient(t, func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(writer).Encode(map[string]string{
			"error": "permanently deleting a record requires an administrator",
			"code":  "deletion_forbidden",
		})
	})

	q := newTestQuotation()
	if _, err := q.PreviewDeletion(DeletionPreviewInput{
		TargetType: "purchase_order", TargetID: 9,
	}); err == nil || !strings.Contains(err.Error(), "administrator") {
		t.Fatalf("preview error = %v; want the server's 403 text", err)
	}
	if _, err := q.ConfirmDeletion(DeletionConfirmInput{
		TargetType: "purchase_order", TargetID: 9, Scope: "record",
		Reason: "cleanup", TypedNumber: "PO-9", ExpectedFingerprint: "v1",
	}); err == nil {
		t.Fatal("confirm without admin succeeded; want the 403 error")
	}
}

func TestDeletionBindingTurnsAChangedChainIntoATypedResult(t *testing.T) {
	var requested string
	stubAPIClient(t, func(writer http.ResponseWriter, request *http.Request) {
		requested = request.URL.Path
		writer.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"error": "the document chain changed since it was confirmed",
			"code":  "deletion_chain_changed",
			"changes": []map[string]string{
				{"field": "purchase_order_7_version", "label": "Purchase order 7 version", "before": "2", "after": "3"},
			},
			"preview": map[string]any{
				"targetType": "quotation", "targetId": 4, "targetUuid": "uuid-4",
				"documentNumber": "1502260004", "documentNumberLabel": "Quotation number",
				"quoteNumber": "1502260004", "customerName": "PT. Changed",
				"state": "issued", "fingerprint": "v1;target=quotation;quotation_id=4",
				"scopes": []map[string]any{
					{"scope": "record", "available": true, "removes": []any{}, "preserves": []any{}},
					{"scope": "chain", "available": true, "requiresChallenge": true, "removes": []any{}, "preserves": []any{}},
				},
			},
		})
	})

	q := newTestQuotation()
	result, err := q.ConfirmDeletion(DeletionConfirmInput{
		TargetType: "quotation", TargetID: 4, Scope: "chain",
		Reason: "cleanup", TypedNumber: "1502260004",
		ExpectedFingerprint: "v1;old", ChallengeID: "c", ChallengeLetters: "ABCDEFGHJK",
	})
	if err != nil {
		t.Fatalf("changed chain returned an error: %v", err)
	}
	if requested != "/v1/deletions" {
		t.Errorf("requested path = %q; want /v1/deletions", requested)
	}
	if result.Changed == nil {
		t.Fatalf("result = %+v; want a typed changed-chain result", result)
	}
	if len(result.Changed.Changes) != 1 || result.Changed.Changes[0].Label != "Purchase order 7 version" {
		t.Errorf("changes = %+v", result.Changed.Changes)
	}
	if result.Changed.Preview == nil || result.Changed.Preview.Fingerprint == "" {
		t.Errorf("changed result has no fresh preview: %+v", result.Changed.Preview)
	}
}

func TestDeletionBindingNamesADeadChallenge(t *testing.T) {
	stubAPIClient(t, func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(writer).Encode(map[string]string{
			"error": "the deletion challenge has expired",
			"code":  "deletion_challenge_expired",
		})
	})

	q := newTestQuotation()
	result, err := q.ConfirmDeletion(DeletionConfirmInput{
		TargetType: "quotation", TargetID: 4, Scope: "chain",
		Reason: "cleanup", TypedNumber: "1502260004",
		ExpectedFingerprint: "v1", ChallengeID: "c", ChallengeLetters: "ABCDEFGHJK",
	})
	if err != nil {
		t.Fatalf("expired challenge returned an error: %v", err)
	}
	if result.ChallengeProblem != "expired" {
		t.Errorf("challenge problem = %q; want expired", result.ChallengeProblem)
	}
}
