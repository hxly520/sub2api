package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/hxly520/sub2api/points-system/internal/domain"
	"github.com/hxly520/sub2api/points-system/internal/store"
	"github.com/hxly520/sub2api/points-system/internal/sub2client"
)

type BalanceGrantStore interface {
	ClaimBalanceGrant(context.Context, time.Time, time.Duration) (store.ClaimedBalanceGrant, error)
	CompleteBalanceGrantAttempt(context.Context, store.ClaimedBalanceGrant, store.AttemptResult, time.Time) error
}

type BalanceAdjuster interface {
	AdjustBalance(context.Context, sub2client.Adjustment) (sub2client.Result, error)
}

type BalanceGrantWorker struct {
	Store    BalanceGrantStore
	Client   BalanceAdjuster
	Interval time.Duration
	Lease    time.Duration
	Logger   *slog.Logger
	Now      func() time.Time
}

func (w *BalanceGrantWorker) Run(ctx context.Context) error {
	if w.Store == nil || w.Client == nil {
		return errors.New("balance grant worker is not configured")
	}
	w.setBalanceGrantDefaults()
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
			processed, err := w.RunOnce(ctx)
			if err != nil && !errors.Is(err, context.Canceled) {
				w.Logger.Error("balance grant worker failed", "error", err)
			}
			delay := w.Interval
			if processed {
				delay = 0
			}
			timer.Reset(delay)
		}
	}
}

func (w *BalanceGrantWorker) RunOnce(ctx context.Context) (bool, error) {
	if w.Store == nil || w.Client == nil {
		return false, errors.New("balance grant worker is not configured")
	}
	w.setBalanceGrantDefaults()
	now := w.Now().UTC()
	claim, err := w.Store.ClaimBalanceGrant(ctx, now, w.Lease)
	if errors.Is(err, domain.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	adjustment := balanceAdjustmentFor(claim)
	_, settlementErr := w.Client.AdjustBalance(ctx, adjustment)
	attempt := store.AttemptResult{Outcome: "success"}
	if settlementErr != nil {
		attempt.Outcome = "retryable_failure"
		attempt.Error = settlementErr.Error()
		var httpErr *sub2client.HTTPError
		if errors.As(settlementErr, &httpErr) {
			attempt.HTTPStatus = httpErr.StatusCode
			attempt.ErrorCode = fmt.Sprint(httpErr.Code)
			if httpErr.StatusCode >= 400 && httpErr.StatusCode < 500 && httpErr.StatusCode != 408 && httpErr.StatusCode != 429 {
				attempt.Outcome = "permanent_failure"
			}
		}
	}
	if err := w.Store.CompleteBalanceGrantAttempt(ctx, claim, attempt, w.Now().UTC()); err != nil {
		return true, err
	}
	if settlementErr != nil {
		w.Logger.Warn("Sub2API balance grant deferred", "balance_grant_id", claim.Grant.ID,
			"operation", claim.Operation, "error", settlementErr)
	}
	return true, nil
}

func (w *BalanceGrantWorker) setBalanceGrantDefaults() {
	if w.Interval <= 0 {
		w.Interval = 5 * time.Second
	}
	if w.Lease <= 0 {
		w.Lease = 30 * time.Second
	}
	if w.Logger == nil {
		w.Logger = slog.Default()
	}
	if w.Now == nil {
		w.Now = time.Now
	}
}

func balanceAdjustmentFor(claim store.ClaimedBalanceGrant) sub2client.Adjustment {
	transactionID := claim.Grant.ID
	amount := claim.Grant.AmountMicroUSD
	kind := claim.Grant.Kind
	if claim.Operation == "debit" {
		transactionID = store.BalanceGrantReversalTransactionID(claim.Grant.ID)
		amount = -amount
		kind = "reversal"
	}
	return sub2client.Adjustment{
		TransactionID: transactionID, UserID: claim.Grant.UserID, AmountMicroUSD: amount,
		Kind: kind, SourceReference: "points-balance-grant:" + claim.Grant.ID, Reason: claim.Grant.Reason,
	}
}
