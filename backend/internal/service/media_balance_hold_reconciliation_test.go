package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type mediaHoldReconciliationRepoStub struct {
	mu      sync.Mutex
	results []*MediaBalanceHoldReconciliationResult
	errors  []error
	calls   int
	called  chan struct{}
	cursors []*MediaBalanceHoldReconciliationCursor
}

func (r *mediaHoldReconciliationRepoStub) ReconcileExpiredMediaBalanceHolds(_ context.Context, after *MediaBalanceHoldReconciliationCursor, _ int) (*MediaBalanceHoldReconciliationResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	index := r.calls
	r.calls++
	r.cursors = append(r.cursors, after)
	if r.called != nil {
		select {
		case r.called <- struct{}{}:
		default:
		}
	}
	var result *MediaBalanceHoldReconciliationResult
	if index < len(r.results) {
		result = r.results[index]
	} else {
		result = &MediaBalanceHoldReconciliationResult{}
	}
	var err error
	if index < len(r.errors) {
		err = r.errors[index]
	}
	return result, err
}

type mediaHoldBalanceCacheStub struct {
	mu      sync.Mutex
	userIDs []int64
}

type blockingMediaHoldReconciliationRepo struct {
	started chan struct{}
}

func (r *blockingMediaHoldReconciliationRepo) ReconcileExpiredMediaBalanceHolds(ctx context.Context, _ *MediaBalanceHoldReconciliationCursor, _ int) (*MediaBalanceHoldReconciliationResult, error) {
	select {
	case r.started <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func (c *mediaHoldBalanceCacheStub) InvalidateUserBalance(_ context.Context, userID int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.userIDs = append(c.userIDs, userID)
	return nil
}

func TestMediaBalanceHoldReconciliationService_DrainsBatchesAndInvalidatesCaches(t *testing.T) {
	firstCursor := &MediaBalanceHoldReconciliationCursor{ExpiresAt: time.Now().Add(-time.Hour), UserID: 22}
	repo := &mediaHoldReconciliationRepoStub{results: []*MediaBalanceHoldReconciliationResult{
		{ScannedUsers: 2, ReconciledUserIDs: []int64{11, 22}, NextCursor: firstCursor},
		{ScannedUsers: 1, ReconciledUserIDs: []int64{33}},
	}}
	cache := &mediaHoldBalanceCacheStub{}
	svc := NewMediaBalanceHoldReconciliationService(repo, cache, time.Minute, 2)

	svc.runOnce()

	require.Equal(t, 2, repo.calls)
	require.Equal(t, []*MediaBalanceHoldReconciliationCursor{nil, firstCursor}, repo.cursors)
	require.Equal(t, []int64{11, 22, 33}, cache.userIDs)
}

func TestMediaBalanceHoldReconciliationService_AdvancesPastFailedBatch(t *testing.T) {
	firstCursor := &MediaBalanceHoldReconciliationCursor{ExpiresAt: time.Now().Add(-time.Hour), UserID: 22}
	repo := &mediaHoldReconciliationRepoStub{results: []*MediaBalanceHoldReconciliationResult{
		{ScannedUsers: 2, NextCursor: firstCursor},
		{ScannedUsers: 1, ReconciledUserIDs: []int64{33}},
	}}
	svc := NewMediaBalanceHoldReconciliationService(repo, nil, time.Minute, 2)

	svc.runOnce()

	require.Equal(t, 2, repo.calls)
	require.Equal(t, firstCursor, repo.cursors[1])
}

func TestMediaBalanceHoldReconciliationService_NonLeaderSkipsScan(t *testing.T) {
	lockCache := &fakeLeaderLockCache{}
	_, err := lockCache.TryAcquireLeaderLock(context.Background(), mediaBalanceHoldReconciliationLeaderLockKey, "peer", time.Minute)
	require.NoError(t, err)

	repo := &mediaHoldReconciliationRepoStub{}
	svc := NewMediaBalanceHoldReconciliationService(repo, nil, time.Minute, 10)
	svc.SetLeaderLock(lockCache, nil)
	svc.runOnce()

	require.Zero(t, repo.calls)
}

func TestMediaBalanceHoldReconciliationService_StartRunsImmediately(t *testing.T) {
	repo := &mediaHoldReconciliationRepoStub{called: make(chan struct{}, 1)}
	svc := NewMediaBalanceHoldReconciliationService(repo, nil, time.Hour, 10)
	svc.Start()
	t.Cleanup(svc.Stop)

	select {
	case <-repo.called:
	case <-time.After(time.Second):
		t.Fatal("reconciliation did not run at startup")
	}
}

func TestMediaBalanceHoldReconciliationService_StopCancelsActiveRun(t *testing.T) {
	repo := &blockingMediaHoldReconciliationRepo{started: make(chan struct{}, 1)}
	svc := NewMediaBalanceHoldReconciliationService(repo, nil, time.Hour, 10)
	svc.Start()

	select {
	case <-repo.started:
	case <-time.After(time.Second):
		t.Fatal("reconciliation did not start")
	}

	stopped := make(chan struct{})
	go func() {
		svc.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("Stop did not cancel the active reconciliation run")
	}
}
