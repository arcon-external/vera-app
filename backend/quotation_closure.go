package backend

import (
	"strconv"

	"vera-app/backend/apicontract"
)

// Issue #656: desktop bindings for quotation closure. The desktop has no HTTP
// layer of its own, so these are thin, typed wrappers over the two admin routes.
// Both answer with the quotation detail after the change, mapped by the same
// function the archive read uses: the dialog never guesses what it saved.

// CloseQuotation closes one quotation with a mandatory reason. The server
// trims and validates it, re-checks the stored admin role inside the write
// transaction, and refuses a second close with a typed 409.
func (q *Quotation) CloseQuotation(id int64, reason string) (ArchivedQuotation, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return ArchivedQuotation{}, err
	}
	client, err := api()
	if err != nil {
		return ArchivedQuotation{}, err
	}
	var detail apicontract.QuotationDetail
	if err := client.Post(
		"/v1/quotations/"+strconv.FormatInt(id, 10)+"/close",
		apicontract.QuotationCloseRequest{Reason: reason},
		&detail,
	); err != nil {
		return ArchivedQuotation{}, err
	}
	return archivedQuotation(detail), nil
}

// ReopenQuotation restores a closed quotation. Allocations are never rewritten:
// the units bought stay bought, and the unbought remainder returns to Unsold.
func (q *Quotation) ReopenQuotation(id int64) (ArchivedQuotation, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return ArchivedQuotation{}, err
	}
	client, err := api()
	if err != nil {
		return ArchivedQuotation{}, err
	}
	var detail apicontract.QuotationDetail
	if err := client.Post(
		"/v1/quotations/"+strconv.FormatInt(id, 10)+"/reopen",
		nil,
		&detail,
	); err != nil {
		return ArchivedQuotation{}, err
	}
	return archivedQuotation(detail), nil
}
