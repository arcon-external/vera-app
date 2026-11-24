package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/labstack/echo/v4"

	"vera-app/backend/apicontract"
	"vera-app/backend/store"
)

// Issue #656: explicit quotation closure. Closing moves the unbought remainder
// from Unsold (pipeline) to Closed Loss (realized), so it is an admin action
// with a mandatory reason. Both routes answer with the quotation detail the
// desktop renders, after the change, so the dialog never guesses the new state.

// closeQuotation closes one quotation. The route's admin middleware is the
// first gate; the store re-reads the actor's stored role inside the write
// transaction, so a direct caller cannot skip it.
func (s *Server) closeQuotation(context echo.Context) error {
	id, err := strconv.ParseInt(context.Param("id"), 10, 64)
	if err != nil || id < 1 {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid quotation id"})
	}
	var request apicontract.QuotationCloseRequest
	if err := context.Bind(&request); err != nil {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	reason := strings.TrimSpace(request.Reason)
	if reason == "" {
		return quotationClosureError(context, store.ErrQuotationCloseReasonRequired)
	}
	if utf8.RuneCountInString(reason) > store.MaxQuotationCloseReasonLength {
		return quotationClosureError(context, store.ErrQuotationCloseReasonTooLong)
	}
	if _, err := s.database.CloseQuotation(
		context.Request().Context(), id, reason, requestUser(context).ID); err != nil {
		return quotationClosureError(context, err)
	}
	return s.quotationAfterClosure(context, id)
}

// reopenQuotation restores a closed quotation. Reopening an open one is a
// conflict rather than a silent success: the caller's state was stale.
func (s *Server) reopenQuotation(context echo.Context) error {
	id, err := strconv.ParseInt(context.Param("id"), 10, 64)
	if err != nil || id < 1 {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid quotation id"})
	}
	if _, err := s.database.ReopenQuotation(
		context.Request().Context(), id, requestUser(context).ID); err != nil {
		return quotationClosureError(context, err)
	}
	return s.quotationAfterClosure(context, id)
}

// quotationAfterClosure answers with the quotation detail after the write: the
// new closure state, the expiry signal and the document the page already binds.
func (s *Server) quotationAfterClosure(context echo.Context, id int64) error {
	record, err := s.database.GetQuotation(context.Request().Context(), id, nil)
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{
			"error": "read the quotation after closing it",
		})
	}
	return context.JSON(http.StatusOK, mapDetail(record, record.HasDocument))
}

// quotationClosureError maps the store's typed refusals onto the wire. The
// codes are stable because the desktop and the CLI branch on them; the message
// stays human.
func quotationClosureError(context echo.Context, err error) error {
	switch {
	case errors.Is(err, store.ErrPOSourceNotFound):
		return context.JSON(http.StatusNotFound, map[string]string{
			"error": "quotation not found",
			"code":  "quotation_not_found",
		})
	case errors.Is(err, store.ErrQuotationCloseForbidden):
		return context.JSON(http.StatusForbidden, map[string]string{
			"error": err.Error(),
			"code":  "quotation_close_forbidden",
		})
	case errors.Is(err, store.ErrQuotationCloseReasonRequired):
		return context.JSON(http.StatusUnprocessableEntity, map[string]string{
			"error": "closing a quotation needs a reason",
			"code":  "quotation_close_reason_required",
		})
	case errors.Is(err, store.ErrQuotationCloseReasonTooLong):
		return context.JSON(http.StatusUnprocessableEntity, map[string]string{
			"error": err.Error(),
			"code":  "quotation_close_reason_too_long",
		})
	case errors.Is(err, store.ErrQuotationClosed):
		return context.JSON(http.StatusConflict, map[string]string{
			"error": err.Error(),
			"code":  "quotation_closed",
		})
	case errors.Is(err, store.ErrQuotationAlreadyClosed):
		return context.JSON(http.StatusConflict, map[string]string{
			"error": err.Error(),
			"code":  "quotation_already_closed",
		})
	case errors.Is(err, store.ErrQuotationNotClosed):
		return context.JSON(http.StatusConflict, map[string]string{
			"error": err.Error(),
			"code":  "quotation_not_closed",
		})
	default:
		return context.JSON(http.StatusInternalServerError, map[string]string{
			"error": "the quotation closure could not be saved",
		})
	}
}

// optionalRFC3339 formats a nullable instant for the wire. Nil stays nil, so
// "open" is JSON null rather than an empty string a client could mistake for
// an instant it failed to parse.
func optionalRFC3339(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := value.Format(time.RFC3339)
	return &formatted
}

// optionalDate formats a nullable calendar day for the wire.
func optionalDate(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := value.Format("2006-01-02")
	return &formatted
}
