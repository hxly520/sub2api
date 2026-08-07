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

// LinkCardHoldReconciliationRepository is optional so existing standard-media
// test doubles and deployments remain source compatible. Implementations
// settle expired prepaid-card holds independently of user frozen balance.
type LinkCardHoldReconciliationRepository interface {
	ReconcileExpiredLinkCardHolds(ctx context.Context, limit int) ([]int64, error)
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
	runCtx       context.Context
	cancel       context.CancelFunc

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
	runCtx, cancel := context.WithCancel(context.Background())
	return &MediaBalanceHoldReconciliationService{
		repo:         repo,
		balanceCache: balanceCache,
		interval:     interval,
		batchSize:    batchSize,
		stopCh:       make(chan struct{}),
		runCtx:       runCtx,
		cancel:       cancel,
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
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
}

func (s *MediaBalanceHoldReconciliationService) runOnce() {
	if s == nil {
		return
	}
	runCtx := s.runCtx
	if runCtx == nil {
		runCtx = context.Background()
	}
	lockCtx, cancel := context.WithTimeout(runCtx, 2*time.Second)
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

	ctx, cancel := context.WithTimeout(runCtx, mediaBalanceHoldReconciliationTimeout)
	defer cancel()
	var cursor *MediaBalanceHoldReconciliationCursor
	for {
		result, err := s.repo.ReconcileExpiredMediaBalanceHolds(ctx, cursor, s.batchSize)
		if result != nil {
			s.invalidateBalanceCaches(ctx, result.ReconciledUserIDs)
		}
		if err != nil {
			slog.Error("[MediaBalanceHoldReconciliation] batch failed", "error", err)
		}
		if result == nil || result.ScannedUsers < s.batchSize || result.NextCursor == nil {
			break
		}
		cursor = result.NextCursor
		if err := ctx.Err(); err != nil {
			slog.Warn("[MediaBalanceHoldReconciliation] run deadline reached", "error", err)
			break
		}
	}
	if linkRepo, ok := s.repo.(LinkCardHoldReconciliationRepository); ok {
		linkCtx, linkCancel := context.WithTimeout(runCtx, mediaBalanceHoldReconciliationTimeout)
		userIDs, err := linkRepo.ReconcileExpiredLinkCardHolds(linkCtx, s.batchSize)
		linkCancel()
		if err != nil {
			slog.Error("[MediaBalanceHoldReconciliation] link-card batch failed", "error", err)
		}
		s.invalidateBalanceCaches(runCtx, userIDs)
	}
}

func (s *MediaBalanceHoldReconciliationService) invalidateBalanceCaches(parent context.Context, userIDs []int64) {
	if s.balanceCache == nil {
		return
	}
	if parent == nil {
		parent = context.Background()
	}
	for _, userID := range userIDs {
		if parent.Err() != nil {
			return
		}
		ctx, cancel := context.WithTimeout(parent, 2*time.Second)
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
