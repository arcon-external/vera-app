package extract

import (
	"context"
	"errors"
	"testing"

	"vera-app/backend/apicontract"
	"vera-app/backend/store"
)

// Issue #498: the desktop needs a typed answer for "can this deployment
// extract at all?", not prose matched from a failed request. The service
// answers through the same Prober preflight Start uses, so the banner and the
// 503 can never disagree about the deployment.

type availabilityEngine struct {
	err error
}

func (engine availabilityEngine) Extract(context.Context, []byte) (apicontract.ExtractionOutput, error) {
	return apicontract.ExtractionOutput{}, engine.err
}

func (engine availabilityEngine) Available(context.Context) error {
	return engine.err
}

type nonProberEngine struct{}

func (nonProberEngine) Extract(context.Context, []byte) (apicontract.ExtractionOutput, error) {
	return apicontract.ExtractionOutput{}, nil
}

func TestExtractionAvailabilityUsesThePreflightProbe(t *testing.T) {
	if (&Service{}).Available(context.Background()) {
		t.Error("a service with no store or engine reported available")
	}

	unavailable := errors.New("no isolated extractor")
	if NewService(&store.Store{}, availabilityEngine{err: unavailable}).Available(context.Background()) {
		t.Error("a failing preflight reported available")
	}
	if !NewService(&store.Store{}, availabilityEngine{}).Available(context.Background()) {
		t.Error("a passing preflight reported unavailable")
	}
	// An engine without the optional Prober is assumed available, exactly as
	// Start treats it. A test stub must not silently disable the feature.
	if !NewService(&store.Store{}, nonProberEngine{}).Available(context.Background()) {
		t.Error("an engine without a preflight reported unavailable")
	}
	if NewService(&store.Store{}, nil).Available(context.Background()) {
		t.Error("a service with no engine reported available")
	}
}
