package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hxly520/sub2api/points-system/internal/domain"
	"github.com/hxly520/sub2api/points-system/internal/security"
	"github.com/jackc/pgx/v5"
)

type BalanceGrant struct {
	ID              string     `json:"id"`
	UserID          int64      `json:"user_id"`
	AmountMicroUSD  int64      `json:"amount_microusd"`
	Kind            string     `json:"kind"`
	Status          string     `json:"status"`
	ExternalEventID string     `json:"external_event_id"`
	PolicyVersion   int64      `json:"policy_version"`
	Attempts        int        `json:"attempts"`
	NextAttemptAt   *time.Time `json:"next_attempt_at,omitempty"`
	LastError       string     `json:"last_error,omitempty"`
	Reason          string     `json:"reason,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type EnqueueBalanceGrantRequest struct {
	UserID          int64
	AmountMicroUSD  int64
	Kind            string
	ExternalEventID string
	PolicyVersion   int64
	Reason          string
	Now             time.Time
}

func enqueueBalanceGrantTx(ctx context.Context, tx pgx.Tx, req EnqueueBalanceGrantRequest) (BalanceGrant, error) {
	if req.UserID <= 0 || req.AmountMicroUSD <= 0 || req.AmountMicroUSD%domain.MicroUSDPerCent != 0 ||
		req.Kind != "checkin" || req.ExternalEventID == "" || req.PolicyVersion <= 0 {
		return BalanceGrant{}, fmt.Errorf("invalid balance grant")
	}
	id, err := uuid.NewRandom()
	if err != nil {
		return BalanceGrant{}, fmt.Errorf("generate balance grant id: %w", err)
	}
	fingerprint := security.Fingerprint(req.UserID, req.AmountMicroUSD, req.Kind, req.ExternalEventID, req.PolicyVersion)
	grant := BalanceGrant{
		ID: id.String(), UserID: req.UserID, AmountMicroUSD: req.AmountMicroUSD, Kind: req.Kind,
		Status: "pending", ExternalEventID: req.ExternalEventID, PolicyVersion: req.PolicyVersion,
		NextAttemptAt: &req.Now, Reason: req.Reason,
	}
	err = tx.QueryRow(ctx, `INSERT INTO points_balance_grants(
		id,user_id,amount_microusd,kind,status,external_event_id,request_fingerprint,
		policy_version,next_attempt_at,reason
	) VALUES($1,$2,$3,$4,'pending',$5,$6,$7,$8,$9)
	RETURNING created_at,updated_at`, id, req.UserID, req.AmountMicroUSD, req.Kind,
		req.ExternalEventID, fingerprint, req.PolicyVersion, req.Now, req.Reason).Scan(&grant.CreatedAt, &grant.UpdatedAt)
	return grant, err
}

type ClaimedBalanceGrant struct {
	Grant      BalanceGrant
	Operation  string
	LeaseToken string
}

func (s *Store) ClaimBalanceGrant(ctx context.Context, now time.Time, lease time.Duration) (ClaimedBalanceGrant, error) {
	var claimed ClaimedBalanceGrant
	leaseToken := uuid.New()
	err := s.withSerializableTx(ctx, func(tx pgx.Tx) error {
		claimed = ClaimedBalanceGrant{}
		var operation string
		err := scanBalanceGrantWithOperation(tx.QueryRow(ctx, balanceGrantSelect+`,operation FROM points_balance_grants WHERE (
			(status IN ('pending','failed') AND COALESCE(next_attempt_at,NOW()) <= $1) OR
			(status='reversal_pending' AND COALESCE(next_attempt_at,NOW()) <= $1) OR
			(status IN ('processing','reversal_processing') AND lease_until <= $1)
		) ORDER BY next_attempt_at NULLS FIRST,created_at FOR UPDATE SKIP LOCKED LIMIT 1`, now), &claimed.Grant, &operation)
		if err != nil {
			return translateNotFound(err)
		}
		if claimed.Grant.Status == "reversal_pending" || claimed.Grant.Status == "reversal_processing" {
			operation = "debit"
			claimed.Grant.Status = "reversal_processing"
		} else {
			operation = "credit"
			claimed.Grant.Status = "processing"
		}
		claimed.Operation = operation
		claimed.LeaseToken = leaseToken.String()
		_, err = tx.Exec(ctx, `UPDATE points_balance_grants SET status=$1,operation=$2,lease_token=$3,
			lease_until=$4,updated_at=NOW() WHERE id=$5`, claimed.Grant.Status, operation, leaseToken,
			now.Add(lease), claimed.Grant.ID)
		return err
	})
	return claimed, err
}

func (s *Store) CompleteBalanceGrantAttempt(ctx context.Context, claim ClaimedBalanceGrant,
	attempt AttemptResult, now time.Time) error {
	return s.withSerializableTx(ctx, func(tx pgx.Tx) error {
		var status string
		var attempts int
		if err := tx.QueryRow(ctx, `SELECT status,attempts FROM points_balance_grants
			WHERE id=$1 AND lease_token=$2 FOR UPDATE`, claim.Grant.ID, claim.LeaseToken).Scan(&status, &attempts); err != nil {
			return translateNotFound(err)
		}
		attempts++
		requestID := claim.Grant.ID
		if claim.Operation == "debit" {
			requestID = BalanceGrantReversalTransactionID(claim.Grant.ID)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO points_balance_grant_attempts(
			balance_grant_id,operation,attempt_no,request_id,http_status,outcome,error_code,error_message
		) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, claim.Grant.ID, claim.Operation, attempts, requestID,
			nullableInt(attempt.HTTPStatus), attempt.Outcome, nullableString(attempt.ErrorCode), truncate(attempt.Error, 1000)); err != nil {
			return err
		}
		if attempt.Outcome != "success" {
			if attempt.Outcome == "permanent_failure" {
				terminalStatus := "permanently_failed"
				if claim.Operation == "debit" {
					terminalStatus = "reversal_permanently_failed"
				}
				_, err := tx.Exec(ctx, `UPDATE points_balance_grants SET status=$1,attempts=$2,
					next_attempt_at=NULL,last_error=$3,lease_token=NULL,lease_until=NULL,updated_at=NOW()
					WHERE id=$4`, terminalStatus, attempts, truncate(attempt.Error, 1000), claim.Grant.ID)
				return err
			}
			next := now.Add(retryDelay(attempts))
			failedStatus := "failed"
			if claim.Operation == "debit" {
				failedStatus = "reversal_pending"
			}
			_, err := tx.Exec(ctx, `UPDATE points_balance_grants SET status=$1,attempts=$2,next_attempt_at=$3,
				last_error=$4,lease_token=NULL,lease_until=NULL,updated_at=NOW() WHERE id=$5`, failedStatus,
				attempts, next, truncate(attempt.Error, 1000), claim.Grant.ID)
			return err
		}
		settledStatus := "settled"
		settledColumn := "settled_at"
		if claim.Operation == "debit" {
			settledStatus = "reversed"
			settledColumn = "reversed_at"
		}
		_, err := tx.Exec(ctx, `UPDATE points_balance_grants SET status=$1,attempts=$2,
			`+settledColumn+`=$3,last_error=NULL,next_attempt_at=NULL,lease_token=NULL,lease_until=NULL,
			updated_at=NOW() WHERE id=$4`, settledStatus, attempts, now, claim.Grant.ID)
		return err
	})
}

func (s *Store) RetryBalanceGrant(ctx context.Context, id string, actorID int64, now time.Time) error {
	return s.withSerializableTx(ctx, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `UPDATE points_balance_grants SET next_attempt_at=$1,
			status=CASE WHEN status IN ('reversal_pending','reversal_permanently_failed')
				THEN 'reversal_pending' ELSE 'pending' END,
			lease_token=NULL,lease_until=NULL,updated_at=NOW()
			WHERE id=$2 AND status IN ('failed','permanently_failed','reversal_pending','reversal_permanently_failed')`, now, id)
		if err != nil {
			return err
		}
		if result.RowsAffected() != 1 {
			return domain.ErrInvalidState
		}
		_, err = tx.Exec(ctx, `INSERT INTO points_admin_audit(actor_user_id,action,target_type,target_id)
			VALUES($1,'balance_grant.retry','balance_grant',$2)`, actorID, id)
		return err
	})
}

func (s *Store) ReverseBalanceGrant(ctx context.Context, id, reason string, actorID int64, now time.Time) error {
	return s.withSerializableTx(ctx, func(tx pgx.Tx) error {
		var status string
		var attempts int
		if err := tx.QueryRow(ctx, `SELECT status,attempts FROM points_balance_grants WHERE id=$1 FOR UPDATE`, id).Scan(
			&status, &attempts); err != nil {
			return translateNotFound(err)
		}
		// A failed credit, or a pending credit that has already been attempted,
		// has an unknown remote outcome. It must be retried with the original
		// transaction ID until Sub2API confirms settlement before a debit can be
		// queued. Only a never-attempted pending grant is safe to cancel locally.
		if status != "settled" && !(status == "pending" && attempts == 0) {
			return domain.ErrInvalidState
		}
		reversalID := uuid.New()
		if _, err := tx.Exec(ctx, `INSERT INTO points_balance_grant_reversals(
			id,balance_grant_id,reason,created_by) VALUES($1,$2,$3,$4)`, reversalID, id, reason, actorID); err != nil {
			return err
		}
		switch status {
		case "settled":
			_, err := tx.Exec(ctx, `UPDATE points_balance_grants SET status='reversal_pending',operation='debit',
				next_attempt_at=$1,updated_at=NOW() WHERE id=$2`, now, id)
			return err
		case "pending":
			_, err := tx.Exec(ctx, `UPDATE points_balance_grants SET status='reversed',reversed_at=$1,
				next_attempt_at=NULL,lease_token=NULL,lease_until=NULL,updated_at=NOW() WHERE id=$2`, now, id)
			return err
		default:
			return domain.ErrInvalidState
		}
	})
}

func (s *Store) ListBalanceGrants(ctx context.Context, userID int64, admin bool, limit int) ([]BalanceGrant, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	query := balanceGrantSelect + ` FROM points_balance_grants`
	args := []any{}
	if !admin {
		query += ` WHERE user_id=$1`
		args = append(args, userID)
	}
	query += ` ORDER BY created_at DESC LIMIT $` + fmt.Sprint(len(args)+1)
	args = append(args, limit)
	rows, err := s.DB.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []BalanceGrant
	for rows.Next() {
		var item BalanceGrant
		if err := scanBalanceGrant(rows, &item); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

const balanceGrantSelect = `SELECT id,user_id,amount_microusd,kind,status,external_event_id,
	policy_version,attempts,next_attempt_at,COALESCE(last_error,''),reason,created_at,updated_at`

func scanBalanceGrant(row rowScanner, grant *BalanceGrant) error {
	return row.Scan(balanceGrantScanTargets(grant)...)
}

func balanceGrantScanTargets(grant *BalanceGrant) []any {
	return []any{&grant.ID, &grant.UserID, &grant.AmountMicroUSD, &grant.Kind, &grant.Status,
		&grant.ExternalEventID, &grant.PolicyVersion, &grant.Attempts, &grant.NextAttemptAt,
		&grant.LastError, &grant.Reason, &grant.CreatedAt, &grant.UpdatedAt}
}

func scanBalanceGrantWithOperation(row rowScanner, grant *BalanceGrant, operation *string) error {
	return row.Scan(&grant.ID, &grant.UserID, &grant.AmountMicroUSD, &grant.Kind, &grant.Status,
		&grant.ExternalEventID, &grant.PolicyVersion, &grant.Attempts, &grant.NextAttemptAt,
		&grant.LastError, &grant.Reason, &grant.CreatedAt, &grant.UpdatedAt, operation)
}

func BalanceGrantReversalTransactionID(grantID string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("points-balance-grant-reversal\x00"+grantID)).String()
}
