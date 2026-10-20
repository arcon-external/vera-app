package store

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"vera-app/backend/apicontract"
)

// Issue #474: the optional purchase-order confirmation number. These tests
// drive the real store against a disposable schema: the format and its
// customer/year scopes, the counter's warning, widening and hard bound, atomic
// allocation under a real race, idempotent assignment, the immutability the
// schema enforces, the promise that an unassigned record consumes nothing, and
// the rule that a deleted record's number is never reissued.

// poCounterNext reads a counter's next value, or 0 when the row does not
// exist. It is the test's only window into the counter; allocation goes through
// the store.
func poCounterNext(t *testing.T, database *Store, key string) int64 {
	t.Helper()
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	var next int64
	if err := pool.QueryRow(context.Background(), fmt.Sprintf(
		`SELECT COALESCE((SELECT next_value FROM %s.sequence_counters WHERE name = $1), 0)`,
		database.configSchema()), key).Scan(&next); err != nil {
		t.Fatalf("read counter %q: %v", key, err)
	}
	return next
}

// seedPOCounter forces one customer-year counter to a chosen next value. Real
// assignment never needs it; the warning threshold, the 5-digit widening and
// the exhaustion bound are unreachable by ordinary allocation.
func seedPOCounter(t *testing.T, database *Store, key string, next int64) {
	t.Helper()
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	if _, err := pool.Exec(context.Background(), fmt.Sprintf(`
		INSERT INTO %s.sequence_counters (name, next_value) VALUES ($1, $2)
		ON CONFLICT (name) DO UPDATE SET next_value = EXCLUDED.next_value`,
		database.configSchema()), key, next); err != nil {
		t.Fatalf("seed counter %q: %v", key, err)
	}
}

// allocatePOCounter runs one counter allocation in its own committed
// transaction, so the warning/widening/bound behaviour can be exercised
// without inventing a Purchase Order for every sequence value.
func allocatePOCounter(t *testing.T, database *Store, customerCode, yy int) (int64, bool, error) {
	t.Helper()
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin counter transaction: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	sequence, warn, err := nextPOConfirmationSequenceTx(
		ctx, tx, database.configSchema(), customerCode, yy)
	if err != nil {
		return 0, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit counter transaction: %v", err)
	}
	return sequence, warn, nil
}

// finalizeWithConfirmation saves a draft and asks for a confirmation number.
func finalizeWithConfirmation(t *testing.T, database *Store, ctx context.Context,
	draft PurchaseOrder, user AuthUser, number string, assign bool) (PurchaseOrder, error) {
	t.Helper()
	saved, err := database.FinalizePurchaseOrder(ctx, PurchaseOrderFinalize{
		PurchaseOrderUpdate:      finalizeInput(draft, user, number).PurchaseOrderUpdate,
		DocumentConfirmed:        true,
		AssignConfirmationNumber: assign,
	})
	return saved, err
}

func TestPOConfirmationNumberFormatAndWIBYear(t *testing.T) {
	cases := []struct {
		name   string
		code   int
		yy     int
		seq    int64
		number string
	}{
		{"customer 1 starts at 0001", 1, 26, 1, "2301260001"},
		{"customer 2 keeps its code", 2, 26, 42, "2302260042"},
		{"customer 99 keeps its code", 99, 26, 9999, "2399269999"},
		{"year 00 is its own cycle", 2, 0, 1, "2302000001"},
		{"year 99 is its own cycle", 2, 99, 1, "2302990001"},
		{"past 9999 widens to five digits", 2, 26, 10000, "23022610000"},
		{"the hard bound keeps the width", 2, 26, 99999, "23022699999"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := apicontract.FormatPOConfirmationNumber(
				testCase.code, testCase.yy, testCase.seq); got != testCase.number {
				t.Errorf("number = %q; want %q", got, testCase.number)
			}
		})
	}

	// The assignment year is the year in WIB, not the server's clock. The
	// company context is fixed until a real setting exists (company_date.go).
	yearCases := []struct {
		name string
		now  time.Time
		want int
	}{
		{"WIB new year begins at 17:00 UTC", time.Date(2026, 12, 31, 16, 59, 59, 0, time.UTC), 26},
		{"first WIB instant of 2027", time.Date(2026, 12, 31, 17, 0, 0, 0, time.UTC), 27},
		{"a UTC January instant is already the new WIB year", time.Date(2027, 1, 1, 3, 0, 0, 0, time.UTC), 27},
	}
	for _, testCase := range yearCases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := poAssignmentYY(testCase.now); got != testCase.want {
				t.Errorf("assignment year = %d; want %d", got, testCase.want)
			}
		})
	}
}

func TestFinalizeAssignsOptionalConfirmationNumbersPerCustomerAndYear(t *testing.T) {
	database := openTestSchema(t, "vera_po_confirmation_scope")
	firstQuote := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "First PO")
	secondQuote := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Second PO")
	otherQuote := seedFinalizeQuotation(t, database, 3, "PT. Other Buyer", "Other PO")
	user := poTestUser(t, database)
	ctx := poContext(t, database)

	first := createFinalizeDraft(t, database, ctx, user, firstQuote)
	second := createFinalizeDraft(t, database, ctx, user, secondQuote)
	other := createFinalizeDraft(t, database, ctx, user, otherQuote)
	yy := poAssignmentYY(time.Now())
	quoteCounterForCustomer2 := poCounterNext(t, database, SequenceKey(2, yy))
	firstSaved, err := finalizeWithConfirmation(t, database, ctx, first, user, "PO-474-1", true)
	if err != nil {
		t.Fatalf("finalize first: %v", err)
	}
	secondSaved, err := finalizeWithConfirmation(t, database, ctx, second, user, "PO-474-2", true)
	if err != nil {
		t.Fatalf("finalize second: %v", err)
	}
	otherSaved, err := finalizeWithConfirmation(t, database, ctx, other, user, "PO-474-3", true)
	if err != nil {
		t.Fatalf("finalize other customer: %v", err)
	}

	if firstSaved.ConfirmationNumber == nil ||
		*firstSaved.ConfirmationNumber != apicontract.FormatPOConfirmationNumber(2, yy, 1) {
		t.Errorf("first number = %v; want customer 2 year %d sequence 1", firstSaved.ConfirmationNumber, yy)
	}
	if secondSaved.ConfirmationNumber == nil ||
		*secondSaved.ConfirmationNumber != apicontract.FormatPOConfirmationNumber(2, yy, 2) {
		t.Errorf("second number = %v; want customer 2 year %d sequence 2", secondSaved.ConfirmationNumber, yy)
	}
	// A different customer starts its own cycle. Quotation issuance (which
	// uses its own counter namespace) cannot consume either of these.
	if otherSaved.ConfirmationNumber == nil ||
		*otherSaved.ConfirmationNumber != apicontract.FormatPOConfirmationNumber(3, yy, 1) {
		t.Errorf("other customer number = %v; want customer 3 sequence 1", otherSaved.ConfirmationNumber)
	}
	if firstSaved.ConfirmationWarn || secondSaved.ConfirmationWarn || otherSaved.ConfirmationWarn {
		t.Errorf("fresh cycles warned: %v/%v/%v", firstSaved.ConfirmationWarn,
			secondSaved.ConfirmationWarn, otherSaved.ConfirmationWarn)
	}

	// The number is a customer-year cycle, not a global one.
	if next := poCounterNext(t, database, POConfirmationSequenceKey(2, yy)); next != 3 {
		t.Errorf("customer 2 counter = %d; want 3", next)
	}
	if next := poCounterNext(t, database, POConfirmationSequenceKey(3, yy)); next != 2 {
		t.Errorf("customer 3 counter = %d; want 2", next)
	}
	// Assigning confirmation numbers must not consume quotation numbers: the
	// quotation counter is exactly where issuing the three quotations left it.
	if next := poCounterNext(t, database, SequenceKey(2, yy)); next != quoteCounterForCustomer2 {
		t.Errorf("quotation counter for the same pair = %d; want untouched at %d",
			next, quoteCounterForCustomer2)
	}
}

func TestAssignConfirmationNumberUsesItsOwnCustomerYearCycle(t *testing.T) {
	database := openTestSchema(t, "vera_po_confirmation_year")
	firstQuote := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Year A")
	secondQuote := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Year B")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	first := createFinalizeDraft(t, database, ctx, user, firstQuote)
	second := createFinalizeDraft(t, database, ctx, user, secondQuote)

	firstSaved, err := finalizeWithConfirmation(t, database, ctx, first, user, "PO-YEAR-A", false)
	if err != nil {
		t.Fatalf("finalize first: %v", err)
	}
	secondSaved, err := finalizeWithConfirmation(t, database, ctx, second, user, "PO-YEAR-B", false)
	if err != nil {
		t.Fatalf("finalize second: %v", err)
	}
	if firstSaved.ConfirmationNumber != nil || secondSaved.ConfirmationNumber != nil {
		t.Fatalf("an unrequested save assigned a number: %v/%v",
			firstSaved.ConfirmationNumber, secondSaved.ConfirmationNumber)
	}

	// Two different assignment years, same customer: each starts at 0001.
	inYear26, err := database.assignPurchaseOrderConfirmation(
		ctx, firstSaved.ID, user.ID, time.Date(2026, 12, 30, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("assign 2026: %v", err)
	}
	inYear27, err := database.assignPurchaseOrderConfirmation(
		ctx, secondSaved.ID, user.ID, time.Date(2027, 1, 2, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("assign 2027: %v", err)
	}
	if inYear26.ConfirmationNumber == nil || *inYear26.ConfirmationNumber != "2302260001" {
		t.Errorf("2026 number = %v; want 2302260001", inYear26.ConfirmationNumber)
	}
	if inYear27.ConfirmationNumber == nil || *inYear27.ConfirmationNumber != "2302270001" {
		t.Errorf("2027 number = %v; want 2302270001", inYear27.ConfirmationNumber)
	}

	// Reassigning the 2026 record in 2027 must not mint a second, 2027 number.
	again, err := database.assignPurchaseOrderConfirmation(
		ctx, firstSaved.ID, user.ID, time.Date(2027, 3, 1, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("reassign 2026 record: %v", err)
	}
	if again.ConfirmationNumber == nil || *again.ConfirmationNumber != "2302260001" {
		t.Errorf("reassigned number = %v; want the original 2302260001", again.ConfirmationNumber)
	}
	if next := poCounterNext(t, database, POConfirmationSequenceKey(2, 26)); next != 2 {
		t.Errorf("2026 counter = %d; want 2", next)
	}
	if next := poCounterNext(t, database, POConfirmationSequenceKey(2, 27)); next != 2 {
		t.Errorf("2027 counter = %d; want 2", next)
	}
}

func TestAssigningTwiceReturnsTheExistingNumber(t *testing.T) {
	database := openTestSchema(t, "vera_po_confirmation_idempotent")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Idempotent")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	saved, err := finalizeWithConfirmation(t, database, ctx, draft, user, "PO-IDEMPOTENT", false)
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}

	now := time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)
	first, err := database.assignPurchaseOrderConfirmation(ctx, saved.ID, user.ID, now)
	if err != nil {
		t.Fatalf("first assign: %v", err)
	}
	second, err := database.assignPurchaseOrderConfirmation(ctx, saved.ID, user.ID, now)
	if err != nil {
		t.Fatalf("second assign: %v", err)
	}
	if first.ConfirmationNumber == nil || second.ConfirmationNumber == nil ||
		*first.ConfirmationNumber != *second.ConfirmationNumber {
		t.Errorf("numbers = %v then %v; want the same number", first.ConfirmationNumber, second.ConfirmationNumber)
	}
	if first.ConfirmationNumber == nil || *first.ConfirmationNumber != "2302260001" {
		t.Errorf("number = %v; want 2302260001", first.ConfirmationNumber)
	}
	if first.ConfirmationSequence == nil || *first.ConfirmationSequence != 1 {
		t.Errorf("sequence = %v; want 1", first.ConfirmationSequence)
	}
	// One assignment consumed exactly one number.
	if next := poCounterNext(t, database, POConfirmationSequenceKey(2, 26)); next != 2 {
		t.Errorf("counter = %d after assigning twice; want 2", next)
	}
}

func TestConfirmationAllocationIsAtomicUnderConcurrency(t *testing.T) {
	database := openTestSchema(t, "vera_po_confirmation_race")
	user := poTestUser(t, database)
	ctx := poContext(t, database)

	const workers = 6
	drafts := make([]PurchaseOrder, 0, workers)
	for index := 0; index < workers; index++ {
		quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer",
			fmt.Sprintf("Race %d", index))
		drafts = append(drafts, createFinalizeDraft(t, database, ctx, user, quotation))
	}

	numbers := make([]string, workers)
	errs := make([]error, workers)
	var wait sync.WaitGroup
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			saved, err := finalizeWithConfirmation(t, database, ctx, drafts[index], user,
				fmt.Sprintf("PO-RACE-%d", index), true)
			errs[index] = err
			if saved.ConfirmationNumber != nil {
				numbers[index] = *saved.ConfirmationNumber
			}
		}(index)
	}
	wait.Wait()

	for index, err := range errs {
		if err != nil {
			t.Fatalf("worker %d: %v", index, err)
		}
	}
	sort.Strings(numbers)
	yy := poAssignmentYY(time.Now())
	for index, number := range numbers {
		want := apicontract.FormatPOConfirmationNumber(2, yy, int64(index+1))
		if number != want {
			t.Errorf("number %d = %q; want %q (unique and sequential, no gaps)", index, number, want)
		}
	}
	if next := poCounterNext(t, database, POConfirmationSequenceKey(2, yy)); next != workers+1 {
		t.Errorf("counter = %d; want %d", next, workers+1)
	}
}

func TestLaterAssignmentIsIdempotentUnderConcurrency(t *testing.T) {
	database := openTestSchema(t, "vera_po_confirmation_later_race")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Later race")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	saved, err := finalizeWithConfirmation(t, database, ctx, draft, user, "PO-LATER-RACE", false)
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}

	const workers = 6
	numbers := make([]string, workers)
	errs := make([]error, workers)
	var wait sync.WaitGroup
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			assigned, err := database.AssignPurchaseOrderConfirmationNumber(ctx, saved.ID, user.ID)
			errs[index] = err
			if assigned.ConfirmationNumber != nil {
				numbers[index] = *assigned.ConfirmationNumber
			}
		}(index)
	}
	wait.Wait()

	for index, err := range errs {
		if err != nil {
			t.Fatalf("worker %d: %v", index, err)
		}
	}
	for index, number := range numbers {
		if number == "" || number != numbers[0] {
			t.Errorf("worker %d number = %q; want every worker to see %q", index, number, numbers[0])
		}
	}
	yy := poAssignmentYY(time.Now())
	if numbers[0] != apicontract.FormatPOConfirmationNumber(2, yy, 1) {
		t.Errorf("number = %q; want the first number", numbers[0])
	}
	// Six concurrent calls, one assignment.
	if next := poCounterNext(t, database, POConfirmationSequenceKey(2, yy)); next != 2 {
		t.Errorf("counter = %d; want 2", next)
	}
}

func TestConfirmationNumberCannotBeChangedClearedOrRegenerated(t *testing.T) {
	database := openTestSchema(t, "vera_po_confirmation_immutable")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Immutable")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	saved, err := finalizeWithConfirmation(t, database, ctx, draft, user, "PO-IMMUTABLE", true)
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if saved.ConfirmationNumber == nil {
		t.Fatal("no confirmation number was assigned")
	}
	number := *saved.ConfirmationNumber

	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	schema := database.configSchema()
	mutations := []struct {
		name      string
		statement string
	}{
		{"clear the number", `SET confirmation_number = NULL`},
		{"replace the number", `SET confirmation_number = '2302260099'`},
		{"move the customer code", `SET confirmation_customer_code = 3`},
		{"move the year", `SET confirmation_year = 27`},
		{"move the sequence", `SET confirmation_sequence = 99`},
		{"clear the actor", `SET confirmation_assigned_by = NULL`},
		{"move the time", `SET confirmation_assigned_at = NOW() + interval '1 day'`},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			if _, err := pool.Exec(ctx, fmt.Sprintf(
				`UPDATE %s.purchase_orders %s WHERE id = $1`, schema, mutation.statement),
				saved.ID); err == nil {
				t.Fatalf("%s was accepted; an issued number must be immutable", mutation.name)
			}
		})
	}

	// An unrelated edit is still allowed: the guard is about the identifier,
	// not about freezing the whole row.
	if _, err := pool.Exec(ctx, fmt.Sprintf(
		`UPDATE %s.purchase_orders SET contact_email = 'edited@example.com' WHERE id = $1`,
		schema), saved.ID); err != nil {
		t.Fatalf("unrelated update: %v", err)
	}
	reloaded, err := database.GetPurchaseOrder(ctx, saved.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.ConfirmationNumber == nil || *reloaded.ConfirmationNumber != number {
		t.Errorf("number after mutations = %v; want %q", reloaded.ConfirmationNumber, number)
	}

	// The store offers no regeneration or clearing path: a second assign
	// returns the original, and a second final save is refused.
	again, err := database.AssignPurchaseOrderConfirmationNumber(ctx, saved.ID, user.ID)
	if err != nil {
		t.Fatalf("reassign: %v", err)
	}
	if again.ConfirmationNumber == nil || *again.ConfirmationNumber != number {
		t.Errorf("reassign = %v; want %q", again.ConfirmationNumber, number)
	}
}

func TestDeletedConfirmationNumberIsNeverReissued(t *testing.T) {
	database := openTestSchema(t, "vera_po_confirmation_delete")
	firstQuote := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Delete first")
	secondQuote := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Delete second")
	user := poTestUser(t, database)
	ctx := poContext(t, database)

	first := createFinalizeDraft(t, database, ctx, user, firstQuote)
	firstSaved, err := finalizeWithConfirmation(t, database, ctx, first, user, "PO-DELETE-1", true)
	if err != nil {
		t.Fatalf("finalize first: %v", err)
	}
	if firstSaved.ConfirmationNumber == nil || *firstSaved.ConfirmationNumber !=
		apicontract.FormatPOConfirmationNumber(2, poAssignmentYY(time.Now()), 1) {
		t.Fatalf("first number = %v; want the first sequence", firstSaved.ConfirmationNumber)
	}
	deletedNumber := *firstSaved.ConfirmationNumber

	// Deletion is #480's path. At the counter level it must not release the
	// number, so removing the row directly is the strongest proof available
	// here: nothing in the schema decrements the sequence.
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(
		`DELETE FROM %s.purchase_orders WHERE id = $1`, database.configSchema()),
		firstSaved.ID); err != nil {
		t.Fatalf("delete purchase order: %v", err)
	}

	second := createFinalizeDraft(t, database, ctx, user, secondQuote)
	secondSaved, err := finalizeWithConfirmation(t, database, ctx, second, user, "PO-DELETE-2", true)
	if err != nil {
		t.Fatalf("finalize second: %v", err)
	}
	if secondSaved.ConfirmationNumber == nil {
		t.Fatal("the second record was not assigned a number")
	}
	if *secondSaved.ConfirmationNumber == deletedNumber {
		t.Fatalf("the deleted record's number %q was reissued", deletedNumber)
	}
	yy := poAssignmentYY(time.Now())
	if *secondSaved.ConfirmationNumber != apicontract.FormatPOConfirmationNumber(2, yy, 2) {
		t.Errorf("second number = %q; want sequence 2", *secondSaved.ConfirmationNumber)
	}
}

func TestUnrequestedOrFailedSavesConsumeNoConfirmationNumber(t *testing.T) {
	database := openTestSchema(t, "vera_po_confirmation_noconsume")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "No consume")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createFinalizeDraft(t, database, ctx, user, quotation)

	// A save that does not ask for a number consumes none.
	saved, err := finalizeWithConfirmation(t, database, ctx, draft, user, "PO-NO-NUMBER", false)
	if err != nil {
		t.Fatalf("finalize unassigned: %v", err)
	}
	if saved.ConfirmationNumber != nil {
		t.Fatalf("unrequested number = %v; want null", saved.ConfirmationNumber)
	}
	yy := poAssignmentYY(time.Now())
	if next := poCounterNext(t, database, POConfirmationSequenceKey(2, yy)); next != 0 {
		t.Errorf("unrequested save wrote counter = %d; want no row", next)
	}

	// A refused final save that asked for a number consumes none either.
	secondQuote := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Failed save")
	second := createFinalizeDraft(t, database, ctx, user, secondQuote)
	if _, err := finalizeWithConfirmation(t, database, ctx, second, user, "   ", true); !errors.Is(err, ErrPurchaseOrderIncomplete) {
		t.Fatalf("blank-number save = %v; want ErrPurchaseOrderIncomplete", err)
	}
	if next := poCounterNext(t, database, POConfirmationSequenceKey(2, yy)); next != 0 {
		t.Errorf("failed save wrote counter = %d; want no row", next)
	}
	reloaded, err := database.GetPurchaseOrder(ctx, second.ID)
	if err != nil {
		t.Fatalf("reload refused draft: %v", err)
	}
	if reloaded.State != PurchaseOrderStateDraft || reloaded.ConfirmationNumber != nil {
		t.Errorf("refused save changed the record: %+v", reloaded)
	}

	// The next genuine assignment starts at 0001: the failure left no gap.
	assigned, err := database.assignPurchaseOrderConfirmation(ctx, saved.ID, user.ID, time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("assign after refusals: %v", err)
	}
	if assigned.ConfirmationNumber == nil || *assigned.ConfirmationNumber != "2302260001" {
		t.Errorf("number = %v; want 2302260001", assigned.ConfirmationNumber)
	}
}

func TestPOConfirmationAllocationRollsBackWithItsTransaction(t *testing.T) {
	database := openTestSchema(t, "vera_po_confirmation_rollback")
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	ctx := context.Background()
	schema := database.configSchema()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	sequence, warn, err := nextPOConfirmationSequenceTx(ctx, tx, schema, 2, 26)
	if err != nil || sequence != 1 || warn {
		t.Fatalf("allocate in rollback transaction = %d/%v/%v; want 1/false/nil",
			sequence, warn, err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if next := poCounterNext(t, database, POConfirmationSequenceKey(2, 26)); next != 0 {
		t.Fatalf("rolled-back allocation left counter = %d; want no row", next)
	}

	sequence, warn, err = allocatePOCounter(t, database, 2, 26)
	if err != nil || sequence != 1 || warn {
		t.Fatalf("allocate after rollback = %d/%v/%v; want 1/false/nil", sequence, warn, err)
	}
}

func TestPOConfirmationWarnWidenAndExhaust(t *testing.T) {
	database := openTestSchema(t, "vera_po_confirmation_bounds")
	key := POConfirmationSequenceKey(2, 26)

	seedPOCounter(t, database, key, 7999)
	if sequence, warn, err := allocatePOCounter(t, database, 2, 26); err != nil || sequence != 7999 || warn {
		t.Fatalf("allocate 7999 = %d/%v/%v; want 7999/false/nil", sequence, warn, err)
	}
	if sequence, warn, err := allocatePOCounter(t, database, 2, 26); err != nil || sequence != 8000 || !warn {
		t.Fatalf("allocate 8000 = %d/%v/%v; want 8000/true/nil", sequence, warn, err)
	}

	// 9999 keeps the four-digit width; 10000 widens instead of wrapping.
	seedPOCounter(t, database, key, 9999)
	if sequence, warn, err := allocatePOCounter(t, database, 2, 26); err != nil || sequence != 9999 || !warn {
		t.Fatalf("allocate 9999 = %d/%v/%v; want 9999/true/nil", sequence, warn, err)
	}
	if sequence, _, err := allocatePOCounter(t, database, 2, 26); err != nil || sequence != 10000 {
		t.Fatalf("allocate 10000 = %d/%v; want 10000/nil", sequence, err)
	}
	if number := apicontract.FormatPOConfirmationNumber(2, 26, 10000); number != "23022610000" {
		t.Errorf("widened number = %q; want 23022610000", number)
	}

	// The hard bound refuses instead of wrapping.
	seedPOCounter(t, database, key, 99999)
	if sequence, _, err := allocatePOCounter(t, database, 2, 26); err != nil || sequence != 99999 {
		t.Fatalf("allocate 99999 = %d/%v; want 99999/nil", sequence, err)
	}
	if _, _, err := allocatePOCounter(t, database, 2, 26); !errors.Is(err, ErrPOConfirmationExhausted) {
		t.Fatalf("allocate past the bound = %v; want ErrPOConfirmationExhausted", err)
	}
	if next := poCounterNext(t, database, key); next != 100000 {
		t.Errorf("counter after exhaustion = %d; want 100000 (no wrap)", next)
	}
	// Another customer is unaffected by one cycle's exhaustion.
	if sequence, _, err := allocatePOCounter(t, database, 3, 26); err != nil || sequence != 1 {
		t.Fatalf("other customer after exhaustion = %d/%v; want 1/nil", sequence, err)
	}
}

func TestAssignConfirmationNumberRefusesADraft(t *testing.T) {
	database := openTestSchema(t, "vera_po_confirmation_draft")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Draft refused")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createFinalizeDraft(t, database, ctx, user, quotation)

	if _, err := database.AssignPurchaseOrderConfirmationNumber(ctx, draft.ID, user.ID); !errors.Is(err, ErrPurchaseOrderNotSaved) {
		t.Fatalf("assign to draft = %v; want ErrPurchaseOrderNotSaved", err)
	}
	yy := poAssignmentYY(time.Now())
	if next := poCounterNext(t, database, POConfirmationSequenceKey(2, yy)); next != 0 {
		t.Errorf("refused assignment wrote counter = %d; want no row", next)
	}
	reloaded, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reload draft: %v", err)
	}
	if reloaded.ConfirmationNumber != nil {
		t.Errorf("a draft carries a confirmation number: %v", reloaded.ConfirmationNumber)
	}
}
