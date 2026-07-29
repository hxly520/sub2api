package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hxly520/sub2api/points-system/internal/domain"
	"github.com/hxly520/sub2api/points-system/internal/store"
	"github.com/hxly520/sub2api/points-system/internal/sub2client"
)

type balanceGrantStoreStub struct {
	claim   store.ClaimedBalanceGrant
	err     error
	attempt store.AttemptResult
}

type adjusterStub struct {
	adjustment sub2client.Adjustment
	err        error
}

func (s *adjusterStub) AdjustBalance(_ context.Context, adjustment sub2client.Adjustment) (sub2client.Result, error) {
	s.adjustment = adjustment
	return sub2client.Result{}, s.err
}

func (s *balanceGrantStoreStub) ClaimBalanceGrant(context.Context, time.Time, time.Duration) (store.ClaimedBalanceGrant, error) {
	return s.claim, s.err
}

func (s *balanceGrantStoreStub) CompleteBalanceGrantAttempt(_ context.Context, _ store.ClaimedBalanceGrant, result store.AttemptResult, _ time.Time) error {
	s.attempt = result
	return nil
}

func TestBalanceGrantWorkerRetainsFailedGrantForRetry(t *testing.T) {
	storeStub := &balanceGrantStoreStub{claim: store.ClaimedBalanceGrant{
		Grant: store.BalanceGrant{ID: "00000000-0000-4000-8000-000000000001", UserID: 9,
			AmountMicroUSD: 250_000, Kind: "checkin", Reason: "Daily check-in"},
		Operation: "credit", LeaseToken: "lease",
	}}
	client := &adjusterStub{err: errors.New("temporary outage")}
	now := time.Unix(1_700_000_000, 0)
	worker := &BalanceGrantWorker{Store: storeStub, Client: client, Lease: time.Minute, Now: func() time.Time { return now }}
	processed, err := worker.RunOnce(context.Background())
	if err != nil || !processed {
		t.Fatalf("processed=%v err=%v", processed, err)
	}
	if storeStub.attempt.Outcome != "retryable_failure" {
		t.Fatalf("unexpected outcome: %s", storeStub.attempt.Outcome)
	}
	if client.adjustment.AmountMicroUSD != 250_000 || client.adjustment.Kind != "checkin" {
		t.Fatalf("unexpected adjustment: %#v", client.adjustment)
	}
}

func TestBalanceGrantWorkerNoWork(t *testing.T) {
	worker := &BalanceGrantWorker{Store: &balanceGrantStoreStub{err: domain.ErrNotFound},
		Client: &adjusterStub{}, Lease: time.Minute, Now: time.Now}
	processed, err := worker.RunOnce(context.Background())
	if err != nil || processed {
		t.Fatalf("processed=%v err=%v", processed, err)
	}
}

func TestBalanceGrantWorkerMarksDeterministicClientErrorPermanent(t *testing.T) {
	storeStub := &balanceGrantStoreStub{claim: store.ClaimedBalanceGrant{
		Grant: store.BalanceGrant{ID: "00000000-0000-4000-8000-000000000001", UserID: 9,
			AmountMicroUSD: 250_000, Kind: "checkin", Reason: "Daily check-in"},
		Operation: "credit", LeaseToken: "lease",
	}}
	client := &adjusterStub{err: &sub2client.HTTPError{StatusCode: 422, Code: 40001, Message: "invalid credit"}}
	worker := &BalanceGrantWorker{Store: storeStub, Client: client, Lease: time.Minute, Now: time.Now}
	processed, err := worker.RunOnce(context.Background())
	if err != nil || !processed {
		t.Fatalf("processed=%v err=%v", processed, err)
	}
	if storeStub.attempt.Outcome != "permanent_failure" {
		t.Fatalf("unexpected outcome: %s", storeStub.attempt.Outcome)
	}
}

var _ BalanceAdjuster = (*adjusterStub)(nil)
