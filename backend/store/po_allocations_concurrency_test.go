package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// Issue #650: finalize and correction take the global lock order (quotation,
// then purchase order). A PO deletion is the other writer of both rows, so it
// must take the same pair in the same order. These tests start both operations
// on one record at the same time; a lock-order inversion shows up as a
// PostgreSQL deadlock (40P01) or as a hung transaction, and the child context
// turns the hang into a failure instead of a stuck suite.

// runDeletionRace starts both operations together under one timeout and fails
// the test on a deadlock or a context timeout. Any other outcome is a clean
// refusal (not found, version conflict, changed chain, source missing) and is
// acceptable: this test proves order, not the winner.
func runDeletionRace(
	t *testing.T,
	label string,
	ctx context.Context,
	left, right func(context.Context) error,
) {
	t.Helper()
	raceCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	start := make(chan struct{})
	errs := make([]error, 2)
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		<-start
		errs[0] = left(raceCtx)
	}()
	go func() {
		defer wait.Done()
		<-start
		errs[1] = right(raceCtx)
	}()
	close(start)
	wait.Wait()

	for index, err := range errs {
		if err == nil {
			continue
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			t.Fatalf("%s: operation %d did not finish before the timeout (deadlock): %v",
				label, index, err)
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && (pgErr.Code == "40P01" || pgErr.Code == "55P03") {
			t.Fatalf("%s: operation %d hit SQLSTATE %s: %v", label, index, pgErr.Code, err)
		}
		if strings.Contains(strings.ToLower(err.Error()), "deadlock detected") {
			t.Fatalf("%s: operation %d reported a deadlock: %v", label, index, err)
		}
	}
}

func TestConcurrentPODeletionAndFinalizeNoDeadlock(t *testing.T) {
	database := openTestSchema(t, "vera_po_alloc_race_po_finalize")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	admin := deletionUser(t, database, "admin")

	for iteration := 0; iteration < 4; iteration++ {
		label := fmt.Sprintf("finalize vs PO deletion iteration %d", iteration)
		quotation := seedFinalizeQuotation(t, database, 2,
			"PT. Race Finalize", fmt.Sprintf("Race finalize %d", iteration))
		draft := createFinalizeDraft(t, database, ctx, user, quotation)
		preview := previewFor(t, database, admin, DeletionTargetPurchaseOrder, draft.ID)
		finalize := finalizeInput(draft, user, fmt.Sprintf("PO-RACE-FIN-%d", iteration))
		confirm := DeletionConfirmInput{
			TargetType: DeletionTargetPurchaseOrder, TargetID: draft.ID,
			Scope: DeletionScopeRecord, Reason: "race with finalize",
			TypedNumber: preview.DocumentNumber, ExpectedFingerprint: preview.Fingerprint,
			ActorID: admin.ID,
		}
		runDeletionRace(t, label, ctx,
			func(raceCtx context.Context) error {
				_, err := database.FinalizePurchaseOrder(raceCtx, finalize)
				return err
			},
			func(raceCtx context.Context) error {
				_, err := database.ConfirmDeletion(raceCtx, confirm)
				return err
			},
		)
	}
}

func TestConcurrentPODeletionAndCorrectionNoDeadlock(t *testing.T) {
	database := openTestSchema(t, "vera_po_alloc_race_po_correct")
	admin := deletionUser(t, database, "admin")

	for iteration := 0; iteration < 4; iteration++ {
		label := fmt.Sprintf("correction vs PO deletion iteration %d", iteration)
		_, saved, user, ctx := savedDeletionPO(t, database, "PT. Race Correct",
			fmt.Sprintf("PO-RACE-COR-%d", iteration))
		preview := previewFor(t, database, admin, DeletionTargetPurchaseOrder, saved.ID)
		correction := correctionFromSavedOrder(saved, user, "race with correction")
		correction.PaymentTerms = "30 days after delivery"
		confirm := DeletionConfirmInput{
			TargetType: DeletionTargetPurchaseOrder, TargetID: saved.ID,
			Scope: DeletionScopeRecord, Reason: "race with correction",
			TypedNumber: preview.DocumentNumber, ExpectedFingerprint: preview.Fingerprint,
			ActorID: admin.ID,
		}
		runDeletionRace(t, label, ctx,
			func(raceCtx context.Context) error {
				_, err := database.CorrectPurchaseOrder(raceCtx, correction)
				return err
			},
			func(raceCtx context.Context) error {
				_, err := database.ConfirmDeletion(raceCtx, confirm)
				return err
			},
		)
	}
}

func TestConcurrentQuotationDeletionAndFinalizeNoDeadlock(t *testing.T) {
	database := openTestSchema(t, "vera_po_alloc_race_quote_finalize")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	admin := deletionUser(t, database, "admin")

	for iteration := 0; iteration < 4; iteration++ {
		label := fmt.Sprintf("finalize vs quotation chain deletion iteration %d", iteration)
		quotation := seedFinalizeQuotation(t, database, 2,
			"PT. Race Chain Finalize", fmt.Sprintf("Race chain finalize %d", iteration))
		draft := createFinalizeDraft(t, database, ctx, user, quotation)
		preview := previewFor(t, database, admin, DeletionTargetQuotation, quotation.ID)
		challenge, err := database.CreateDeletionChallenge(ctx, DeletionChallengeInput{
			TargetType: DeletionTargetQuotation, TargetID: quotation.ID,
			Scope: DeletionScopeChain, ExpectedFingerprint: preview.Fingerprint,
			ActorID: admin.ID,
		})
		if err != nil {
			t.Fatalf("%s: create chain challenge: %v", label, err)
		}
		finalize := finalizeInput(draft, user, fmt.Sprintf("PO-RACE-CHAIN-FIN-%d", iteration))
		confirm := DeletionConfirmInput{
			TargetType: DeletionTargetQuotation, TargetID: quotation.ID,
			Scope: DeletionScopeChain, Reason: "race with finalize",
			TypedNumber: quotation.QuoteNumber, ExpectedFingerprint: preview.Fingerprint,
			ChallengeID: challenge.ID, ChallengeLetters: challenge.Letters,
			ActorID: admin.ID,
		}
		runDeletionRace(t, label, ctx,
			func(raceCtx context.Context) error {
				_, err := database.FinalizePurchaseOrder(raceCtx, finalize)
				return err
			},
			func(raceCtx context.Context) error {
				_, err := database.ConfirmDeletion(raceCtx, confirm)
				return err
			},
		)
	}
}

func TestConcurrentQuotationDeletionAndCorrectionNoDeadlock(t *testing.T) {
	database := openTestSchema(t, "vera_po_alloc_race_quote_correct")
	admin := deletionUser(t, database, "admin")

	for iteration := 0; iteration < 4; iteration++ {
		label := fmt.Sprintf("correction vs quotation chain deletion iteration %d", iteration)
		quotation, saved, user, ctx := savedDeletionPO(t, database,
			"PT. Race Chain Correct", fmt.Sprintf("PO-RACE-CHAIN-COR-%d", iteration))
		preview := previewFor(t, database, admin, DeletionTargetQuotation, quotation.ID)
		challenge, err := database.CreateDeletionChallenge(ctx, DeletionChallengeInput{
			TargetType: DeletionTargetQuotation, TargetID: quotation.ID,
			Scope: DeletionScopeChain, ExpectedFingerprint: preview.Fingerprint,
			ActorID: admin.ID,
		})
		if err != nil {
			t.Fatalf("%s: create chain challenge: %v", label, err)
		}
		correction := correctionFromSavedOrder(saved, user, "race with chain deletion")
		correction.PaymentTerms = "30 days after delivery"
		confirm := DeletionConfirmInput{
			TargetType: DeletionTargetQuotation, TargetID: quotation.ID,
			Scope: DeletionScopeChain, Reason: "race with correction",
			TypedNumber: quotation.QuoteNumber, ExpectedFingerprint: preview.Fingerprint,
			ChallengeID: challenge.ID, ChallengeLetters: challenge.Letters,
			ActorID: admin.ID,
		}
		runDeletionRace(t, label, ctx,
			func(raceCtx context.Context) error {
				_, err := database.CorrectPurchaseOrder(raceCtx, correction)
				return err
			},
			func(raceCtx context.Context) error {
				_, err := database.ConfirmDeletion(raceCtx, confirm)
				return err
			},
		)
	}
}
