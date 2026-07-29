package service

import (
	"context"
	"database/sql"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	mediaBalanceHoldReconciliationInterval      = time.Minute
	mediaBalanceHoldReconciliationTimeout       = 50 * time.Second
	mediaBalanceHoldReconciliationBatchSize     = 100
	mediaBalanceHoldReconciliationLeaderLockKey = "billing:media-hold:reconciliation:leader"
	mediaBalanceHoldReconciliationLeaderLockTTL = 2 * time.Minute
)

type MediaBalanceHoldReconciliationResult struct {
	ScannedUsers      int
	ReconciledUserIDs []int64
	NextCursor        *MediaBalanceHoldReconciliationCursor
}

type MediaBalanceHoldReconciliationCursor struct {
	ExpiresAt time.Time
	UserID    int64
}

type MediaBalanceHoldReconciliationRepository interface {
	ReconcileExpiredMediaBalanceHolds(ctx context.Context, after *MediaBalanceHoldReconciliationCursor, limit int) (*MediaBalanceHoldReconciliationResult, error)
}

type mediaBalanceCacheInvalidator interface {
	InvalidateUserBalance(ctx context.Context, userID int64) error
}

// MediaBalanceHoldReconciliationService recovers expired media holds even when
// the affected user never sends another request.
type MediaBalanceHoldReconciliationService struct {
	repo         MediaBalanceHoldReconciliationRepository
	balanceCache mediaBalanceCacheInvalidator
	interval     time.Duration
	batchSize    int
	stopCh       chan struct{}
	startOnce    sync.Once
	stopOnce     sync.Once
	wg           sync.WaitGroup

	lockCache  LeaderLockCache
	db         *sql.DB
	instanceID string
}

func NewMediaBalanceHoldReconciliationService(
	repo MediaBalanceHoldReconciliationRepository,
	balanceCache mediaBalanceCacheInvalidator,
	interval time.Duration,
	batchSize int,
) *MediaBalanceHoldReconciliationService {
	return &MediaBalanceHoldReconciliationService{
		repo:         repo,
		balanceCache: balanceCache,
		interval:     interval,
		batchSize:    batchSize,
		stopCh:       make(chan struct{}),
		instanceID:   uuid.NewString(),
	}
}

func (s *MediaBalanceHoldReconciliationService) SetLeaderLock(lockCache LeaderLockCache, db *sql.DB) {
	if s == nil {
		return
	}
	s.lockCache = lockCache
	s.db = db
}

func (s *MediaBalanceHoldReconciliationService) Start() {
	if s == nil || s.repo == nil || s.interval <= 0 || s.batchSize <= 0 {
		return
	}
	s.startOnce.Do(func() {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			ticker := time.NewTicker(s.interval)
			defer ticker.Stop()

			s.runOnce()
			for {
				select {
				case <-ticker.C:
					s.runOnce()
				case <-s.stopCh:
					return
				}
			}
		}()
	})
}

func (s *MediaBalanceHoldReconciliationService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() { close(s.stopCh) })
	s.wg.Wait()
}

func (s *MediaBalanceHoldReconciliationService) runOnce() {
	lockCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	release, acquired := tryAcquireSingletonLeaderLock(
		lockCtx,
		s.lockCache,
		s.db,
		mediaBalanceHoldReconciliationLeaderLockKey,
		s.instanceID,
		mediaBalanceHoldReconciliationLeaderLockTTL,
	)
	cancel()
	if !acquired {
		return
	}
	defer release()

	ctx, cancel := context.WithTimeout(context.Background(), mediaBalanceHoldReconciliationTimeout)
	defer cancel()
	var cursor *MediaBalanceHoldReconciliationCursor
	for {
		result, err := s.repo.ReconcileExpiredMediaBalanceHolds(ctx, cursor, s.batchSize)
		if result != nil {
			s.invalidateBalanceCaches(result.ReconciledUserIDs)
		}
		if err != nil {
			slog.Error("[MediaBalanceHoldReconciliation] batch failed", "error", err)
		}
		if result == nil || result.ScannedUsers < s.batchSize || result.NextCursor == nil {
			return
		}
		cursor = result.NextCursor
		if err := ctx.Err(); err != nil {
			slog.Warn("[MediaBalanceHoldReconciliation] run deadline reached", "error", err)
			return
		}
	}
}

func (s *MediaBalanceHoldReconciliationService) invalidateBalanceCaches(userIDs []int64) {
	if s.balanceCache == nil {
		return
	}
	for _, userID := range userIDs {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := s.balanceCache.InvalidateUserBalance(ctx, userID)
		cancel()
		if err != nil {
			slog.Warn("[MediaBalanceHoldReconciliation] failed to invalidate balance cache", "user_id", userID, "error", err)
		}
	}
}

func ProvideMediaBalanceHoldReconciliationService(
	usageBillingRepo UsageBillingRepository,
	balanceCache *BillingCacheService,
	lockCache LeaderLockCache,
	db *sql.DB,
) *MediaBalanceHoldReconciliationService {
	repo, ok := usageBillingRepo.(MediaBalanceHoldReconciliationRepository)
	if !ok {
		slog.Error("[MediaBalanceHoldReconciliation] repository capability is unavailable")
	}
	svc := NewMediaBalanceHoldReconciliationService(repo, balanceCache, mediaBalanceHoldReconciliationInterval, mediaBalanceHoldReconciliationBatchSize)
	svc.SetLeaderLock(lockCache, db)
	svc.Start()
	return svc
}
