package store

import (
	"context"
	"errors"
	rand "math/rand/v2"
	"time"

	"github.com/hxly520/sub2api/points-system/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const serializableTransactionMaxAttempts = 8

type Store struct {
	DB          *pgxpool.Pool
	Location    *time.Location
	UsageSource UsageSource
}

func New(db *pgxpool.Pool, location *time.Location) *Store {
	return &Store{DB: db, Location: location}
}

func (s *Store) SetUsageSource(source UsageSource) {
	s.UsageSource = source
}

func (s *Store) BusinessDate(now time.Time) time.Time {
	local := now.In(s.Location)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, s.Location)
}

// fn may run more than once and must reset any mutable output it captures.
func (s *Store) withSerializableTx(ctx context.Context, fn func(pgx.Tx) error) error {
	var err error
	for attempt := 0; attempt < serializableTransactionMaxAttempts; attempt++ {
		err = s.runSerializableTx(ctx, fn)
		if err == nil || !IsRetryableTransactionFailure(err) {
			return err
		}
		if attempt == serializableTransactionMaxAttempts-1 {
			break
		}
		baseDelay := time.Millisecond << min(attempt, 6)
		delay := baseDelay + time.Duration(rand.Int64N(int64(baseDelay)+1))
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return err
}

func (s *Store) runSerializableTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func translateNotFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	return err
}

func dateString(value time.Time) string {
	return value.Format("2006-01-02")
}
