package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/shopspring/decimal"
)

type linkCardRepository struct{ db *sql.DB }

func NewLinkCardRepository(_ *dbent.Client, db *sql.DB) service.LinkCardRepository {
	return &linkCardRepository{db: db}
}

func (r *linkCardRepository) ListAuthorizedGroups(ctx context.Context, includeDisabled bool, defaultConcurrency int) ([]service.LinkCardGroup, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("link card repository db is nil")
	}
	query := `
		SELECT g.id, g.name, g.platform, COALESCE(g.description, ''), g.rate_multiplier,
		       a.enabled, a.sort_order, a.created_at, a.updated_at
		FROM link_card_group_authorizations a
		JOIN groups g ON g.id = a.group_id
		WHERE g.deleted_at IS NULL`
	if !includeDisabled {
		query += ` AND a.enabled = TRUE AND g.status = 'active'`
	}
	query += ` ORDER BY a.sort_order ASC, g.rate_multiplier ASC, g.id ASC`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]service.LinkCardGroup, 0)
	for rows.Next() {
		var item service.LinkCardGroup
		if err := rows.Scan(&item.GroupID, &item.Name, &item.Platform, &item.Description, &item.RateMultiplier,
			&item.Enabled, &item.SortOrder, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.ID = item.GroupID
		item.Authorized = true
		item.DefaultConcurrency = defaultConcurrency
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *linkCardRepository) UpsertAuthorizedGroup(ctx context.Context, groupID int64, enabled bool, sortOrder int, actorUserID int64, defaultConcurrency int) (*service.LinkCardGroup, error) {
	if groupID <= 0 {
		return nil, service.ErrLinkCardGroupNotAuthorized
	}
	var subscriptionType, status string
	var rate float64
	err := r.db.QueryRowContext(ctx, `SELECT subscription_type, status, rate_multiplier FROM groups WHERE id=$1 AND deleted_at IS NULL`, groupID).
		Scan(&subscriptionType, &status, &rate)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrLinkCardGroupNotAuthorized
	}
	if err != nil {
		return nil, err
	}
	if subscriptionType != service.SubscriptionTypeStandard || status != service.StatusActive || rate <= 0 {
		return nil, service.ErrLinkCardGroupNotAuthorized
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO link_card_group_authorizations(group_id, enabled, sort_order, created_by)
		VALUES($1,$2,$3,$4)
		ON CONFLICT(group_id) DO UPDATE SET enabled=EXCLUDED.enabled, sort_order=EXCLUDED.sort_order, updated_at=NOW()
	`, groupID, enabled, sortOrder, actorUserID)
	if err != nil {
		return nil, err
	}
	if !enabled {
		if err := freezeLinkCardsForUnauthorizedGroup(ctx, tx, groupID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetAuthorizedGroup(ctx, groupID, defaultConcurrency)
}

func (r *linkCardRepository) RemoveAuthorizedGroup(ctx context.Context, groupID int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `DELETE FROM link_card_group_authorizations WHERE group_id=$1`, groupID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return service.ErrLinkCardGroupNotAuthorized
	}
	if err := freezeLinkCardsForUnauthorizedGroup(ctx, tx, groupID); err != nil {
		return err
	}
	return tx.Commit()
}

func freezeLinkCardsForUnauthorizedGroup(ctx context.Context, tx *sql.Tx, groupID int64) error {
	if tx == nil || groupID <= 0 {
		return service.ErrLinkCardGroupNotAuthorized
	}
	_, err := tx.ExecContext(ctx, `
		UPDATE api_keys
		SET status=$1,
		    link_state=CASE
		        WHEN link_state=$2 THEN $2
		        ELSE $3
		    END,
		    link_frozen_reason='link-card group authorization removed',
		    updated_at=NOW()
		WHERE group_id=$4 AND key_type=$5 AND deleted_at IS NULL
		  AND COALESCE(link_state,'') NOT IN ($6,$7)
	`, service.StatusAPIKeyDisabled, service.LinkCardStatePendingActivation, service.LinkCardStateFrozen,
		groupID, service.APIKeyTypeLink, service.LinkCardStateRefunded, service.LinkCardStateRevoked)
	return err
}

func (r *linkCardRepository) GetAuthorizedGroup(ctx context.Context, groupID int64, defaultConcurrency int) (*service.LinkCardGroup, error) {
	item := &service.LinkCardGroup{}
	err := r.db.QueryRowContext(ctx, `
		SELECT g.id, g.name, g.platform, COALESCE(g.description, ''), g.rate_multiplier,
		       a.enabled, a.sort_order, a.created_at, a.updated_at
		FROM link_card_group_authorizations a
		JOIN groups g ON g.id=a.group_id
		WHERE g.id=$1 AND g.deleted_at IS NULL AND g.status='active'
		  AND g.subscription_type=$2
	`, groupID, service.SubscriptionTypeStandard).Scan(&item.GroupID, &item.Name, &item.Platform, &item.Description,
		&item.RateMultiplier, &item.Enabled, &item.SortOrder, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrLinkCardGroupNotAuthorized
	}
	if err != nil {
		return nil, err
	}
	item.ID = item.GroupID
	item.Authorized = true
	item.DefaultConcurrency = defaultConcurrency
	return item, nil
}

func (r *linkCardRepository) CreateCards(ctx context.Context, cmd service.CreateLinkCardsCommand) (_ *service.CreateLinkCardsResult, err error) {
	if r == nil || r.db == nil {
		return nil, errors.New("link card repository db is nil")
	}
	// Issuance reads authorization, exclusive-group grants and optional user
	// rate rows before moving money. Serializable isolation closes the phantom
	// window where a missing grant/override could be inserted or revoked while
	// this transaction is still creating cards at the old policy.
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	opID, replay, err := claimLinkCardOperation(ctx, tx, "create", cmd.CreatorUserID, cmd.CreatorUserID, nil,
		cmd.IdempotencyKeyHash, cmd.RequestFingerprint)
	if err != nil {
		return nil, err
	}
	if replay != nil {
		var prior service.CreateLinkCardsResult
		if err := json.Unmarshal(replay, &prior); err != nil {
			return nil, err
		}
		prior.Replayed = true
		return &prior, nil
	}

	// Re-read and lock the authorization, native user access, and group pricing.
	// The client and the service-layer snapshot never decide a monetary
	// multiplier.  The effective rate is resolved from the native user override
	// table inside this same transaction so a forged request cannot select an
	// exclusive group or inject a cheaper rate.
	group := cmd.Group
	err = tx.QueryRowContext(ctx, `
		SELECT g.id, g.name, g.platform, COALESCE(g.description,''),
		       COALESCE(ugr.rate_multiplier, g.rate_multiplier),
		       a.enabled, a.sort_order, a.created_at, a.updated_at
		FROM link_card_group_authorizations a
		JOIN groups g ON g.id=a.group_id
		LEFT JOIN user_allowed_groups uag
		  ON uag.user_id=$2 AND uag.group_id=g.id
		LEFT JOIN user_group_rate_multipliers ugr
		  ON ugr.user_id=$2 AND ugr.group_id=g.id
		WHERE g.id=$1 AND a.enabled=TRUE AND g.deleted_at IS NULL AND g.status='active'
		  AND g.subscription_type=$3
		  AND (g.is_exclusive=FALSE OR uag.user_id IS NOT NULL)
		  AND COALESCE(ugr.rate_multiplier, g.rate_multiplier) > 0
		FOR SHARE OF a, g
	`, cmd.Group.GroupID, cmd.CreatorUserID, service.SubscriptionTypeStandard).Scan(&group.GroupID, &group.Name, &group.Platform,
		&group.Description, &group.RateMultiplier, &group.Enabled, &group.SortOrder, &group.CreatedAt, &group.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrLinkCardGroupNotAuthorized
	}
	if err != nil {
		return nil, err
	}

	var remaining decimal.Decimal
	err = tx.QueryRowContext(ctx, `
		UPDATE users SET balance=balance-$1, updated_at=NOW()
		WHERE id=$2 AND deleted_at IS NULL AND status='active' AND balance >= $1
		RETURNING balance
	`, cmd.TotalDebit.StringFixed(8), cmd.CreatorUserID).Scan(&remaining)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrLinkCardInsufficientBalance
	}
	if err != nil {
		return nil, err
	}

	result := &service.CreateLinkCardsResult{Cards: make([]service.LinkCard, 0, cmd.Quantity), Quantity: cmd.Quantity,
		AmountPerCard: cmd.AmountPerCard, TotalDebited: cmd.TotalDebit, RemainingUserBalance: remaining}
	for i := 0; i < cmd.Quantity; i++ {
		var id int64
		var createdAt, updatedAt time.Time
		err = tx.QueryRowContext(ctx, `
			INSERT INTO api_keys(user_id, key, name, group_id, status, key_type, link_state,
				link_rate_multiplier, link_original_debit, link_total_funded, link_total_refunded,
				link_concurrency, link_rpm_limit, quota, quota_used, created_at, updated_at)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$9,0,$10,$11,$9,0,NOW(),NOW())
			RETURNING id, created_at, updated_at
		`, cmd.CreatorUserID, cmd.Keys[i], fmt.Sprintf("link-card-%d-%d", time.Now().Unix(), i+1), group.GroupID,
			service.StatusAPIKeyDisabled, service.APIKeyTypeLink, service.LinkCardStatePendingActivation,
			group.RateMultiplier, cmd.AmountPerCard.StringFixed(8), cmd.Concurrency, cmd.RPMLimit).Scan(&id, &createdAt, &updatedAt)
		if err != nil {
			return nil, err
		}
		var ledgerID int64
		err = tx.QueryRowContext(ctx, `
			INSERT INTO link_card_ledger(operation_id,api_key_id,creator_user_id,entry_type,reserve_delta,
				creator_balance_delta,quota_before,quota_after,quota_used_before,quota_used_after,actor_user_id,
				reason,metadata)
			VALUES($1,$2,$3,'issue',$4,-$4,0,$4,0,0,$3,'initial prepaid issue',
				jsonb_build_object('group_id',$5,'rate_multiplier',$6,'batch_quantity',$7)) RETURNING id
		`, opID, id, cmd.CreatorUserID, cmd.AmountPerCard.StringFixed(8), group.GroupID, group.RateMultiplier, cmd.Quantity).Scan(&ledgerID)
		if err != nil {
			return nil, err
		}
		card := service.LinkCard{APIKeyID: id, CreatorUserID: cmd.CreatorUserID, Key: cmd.Keys[i], GroupID: group.GroupID,
			GroupName: group.Name, Platform: group.Platform, IssueRateMultiplier: group.RateMultiplier,
			Status: service.LinkCardStatePendingActivation, OriginalDepositAmount: cmd.AmountPerCard,
			TotalDepositAmount: cmd.AmountPerCard, Concurrency: cmd.Concurrency, RPMLimit: cmd.RPMLimit,
			CreatedAt: createdAt, UpdatedAt: updatedAt}
		card.SetFinancialState(decimal.Zero, decimal.Zero)
		card.NormalizeDerivedFields()
		result.Cards = append(result.Cards, card)
	}
	if err := storeLinkCardOperationResponse(ctx, tx, opID, result); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	tx = nil
	return result, nil
}

func (r *linkCardRepository) ListCards(ctx context.Context, creatorUserID *int64, params pagination.PaginationParams, filters service.LinkCardListFilters) ([]service.LinkCard, *pagination.PaginationResult, error) {
	where, args := buildLinkCardWhere(creatorUserID, filters)
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM api_keys ak JOIN users u ON u.id=ak.user_id JOIN groups g ON g.id=ak.group_id `+where, args...).Scan(&total); err != nil {
		return nil, nil, err
	}
	order := linkCardOrder(params)
	args = append(args, params.Limit(), params.Offset())
	query := linkCardSelectSQL() + where + ` ORDER BY ` + order + fmt.Sprintf(` LIMIT $%d OFFSET $%d`, len(args)-1, len(args))
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	out := make([]service.LinkCard, 0)
	for rows.Next() {
		card, err := scanLinkCard(rows)
		if err != nil {
			return nil, nil, err
		}
		out = append(out, *card)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return out, linkCardPagination(total, params), nil
}

func (r *linkCardRepository) Summary(ctx context.Context) (*service.LinkCardSummary, error) {
	var total, active int64
	var reserved, consumed decimal.Decimal
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE link_state = $2),
		       COALESCE(SUM(link_total_funded), 0),
		       COALESCE(SUM(quota_used), 0)
		FROM api_keys
		WHERE key_type=$1 AND deleted_at IS NULL
	`, service.APIKeyTypeLink, service.LinkCardStateActive).Scan(&total, &active, &reserved, &consumed)
	if err != nil {
		return nil, err
	}
	return &service.LinkCardSummary{TotalCards: total, ActiveCards: active, TotalReserved: reserved, TotalConsumed: consumed}, nil
}

func (r *linkCardRepository) GetCard(ctx context.Context, apiKeyID int64, creatorUserID *int64) (*service.LinkCard, error) {
	filters := service.LinkCardListFilters{}
	where, args := buildLinkCardWhere(creatorUserID, filters)
	args = append(args, apiKeyID)
	where += fmt.Sprintf(" AND ak.id=$%d", len(args))
	card, err := scanLinkCard(r.db.QueryRowContext(ctx, linkCardSelectSQL()+where, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrLinkCardNotFound
	}
	return card, err
}

func (r *linkCardRepository) FreezeForRefund(ctx context.Context, apiKeyID int64) (_ *service.LinkCard, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()
	card, err := scanLinkCard(tx.QueryRowContext(ctx, linkCardSelectSQL()+` WHERE ak.key_type=$1 AND ak.deleted_at IS NULL AND ak.id=$2 FOR UPDATE OF ak`, service.APIKeyTypeLink, apiKeyID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrLinkCardNotFound
	}
	if err != nil {
		return nil, err
	}
	if card.Status == service.LinkCardStateRefunded || card.Status == service.LinkCardStateRevoked {
		return nil, service.ErrLinkCardOperationNotAllowed
	}
	if card.Status != service.LinkCardStateFrozen {
		const reason = "refund pending"
		if _, err := tx.ExecContext(ctx, `
			UPDATE api_keys SET status=$1,link_state=$2,link_frozen_reason=$3,updated_at=NOW()
			WHERE id=$4 AND key_type=$5
		`, service.StatusAPIKeyDisabled, service.LinkCardStateFrozen, reason, card.APIKeyID, service.APIKeyTypeLink); err != nil {
			return nil, err
		}
		card.Status = service.LinkCardStateFrozen
		card.FrozenReason = reason
		card.UpdatedAt = time.Now()
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	tx = nil
	return card, nil
}

func (r *linkCardRepository) Recharge(ctx context.Context, cmd service.LinkCardMutationCommand) (*service.LinkCardMutationResult, error) {
	return r.mutate(ctx, cmd, func(ctx context.Context, tx *sql.Tx, card *service.LinkCard, opID int64) (*service.LinkCardMutationResult, error) {
		if card.Status == service.LinkCardStateRefunded || card.Status == service.LinkCardStateRevoked {
			return nil, service.ErrLinkCardOperationNotAllowed
		}
		var balance decimal.Decimal
		err := tx.QueryRowContext(ctx, `UPDATE users SET balance=balance-$1,updated_at=NOW() WHERE id=$2 AND deleted_at IS NULL AND balance >= $1 RETURNING balance`, cmd.Amount.StringFixed(8), card.CreatorUserID).Scan(&balance)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, service.ErrLinkCardInsufficientBalance
		}
		if err != nil {
			return nil, err
		}
		oldQuota := card.TotalDepositAmount
		newQuota := oldQuota.Add(cmd.Amount)
		state := card.Status
		apiStatus := service.StatusAPIKeyDisabled
		if state == service.LinkCardStateDepleted {
			state = service.LinkCardStateActive
		}
		if state == service.LinkCardStateActive {
			apiStatus = service.StatusActive
		}
		_, err = tx.ExecContext(ctx, `UPDATE api_keys SET quota=quota+$1,link_total_funded=link_total_funded+$1,link_state=$2,status=$3,updated_at=NOW() WHERE id=$4 AND key_type=$5`, cmd.Amount.StringFixed(8), state, apiStatus, card.APIKeyID, service.APIKeyTypeLink)
		if err != nil {
			return nil, err
		}
		var ledgerID int64
		err = tx.QueryRowContext(ctx, `INSERT INTO link_card_ledger(operation_id,api_key_id,creator_user_id,entry_type,reserve_delta,creator_balance_delta,quota_before,quota_after,quota_used_before,quota_used_after,actor_user_id,reason) VALUES($1,$2,$3,'recharge',$4,-$4,$5,$6,$7,$7,$8,$9) RETURNING id`, opID, card.APIKeyID, card.CreatorUserID, cmd.Amount.StringFixed(8), oldQuota.StringFixed(8), newQuota.StringFixed(8), card.UsedActualAmount().StringFixed(8), cmd.ActorUserID, cmd.Reason).Scan(&ledgerID)
		if err != nil {
			return nil, err
		}
		card.TotalDepositAmount = newQuota
		card.Status = state
		card.UpdatedAt = time.Now()
		card.SetFinancialState(card.UsedActualAmount(), card.TotalRefundedAmount())
		card.NormalizeDerivedFields()
		return &service.LinkCardMutationResult{Card: *card, Action: "recharge", DebitedAmount: cmd.Amount, RemainingUserBalance: balance, LedgerID: ledgerID}, nil
	})
}

func (r *linkCardRepository) Refund(ctx context.Context, cmd service.LinkCardMutationCommand) (*service.LinkCardMutationResult, error) {
	return r.mutate(ctx, cmd, func(ctx context.Context, tx *sql.Tx, card *service.LinkCard, opID int64) (*service.LinkCardMutationResult, error) {
		if card.Status == service.LinkCardStateRefunded || card.Status == service.LinkCardStateRevoked {
			return nil, service.ErrLinkCardOperationNotAllowed
		}
		if !cmd.Admin && (card.Status != service.LinkCardStatePendingActivation || !card.UsedActualAmount().IsZero()) {
			return nil, service.ErrLinkCardOperationNotAllowed
		}
		if card.ReservedActualAmount().IsPositive() {
			return nil, service.ErrLinkCardInFlight
		}
		refundable := card.TotalDepositAmount.Sub(card.UsedActualAmount()).Sub(card.TotalRefundedAmount()).Round(8)
		if !refundable.IsPositive() {
			return nil, service.ErrLinkCardNoRefundableBalance
		}
		var balance decimal.Decimal
		if err := tx.QueryRowContext(ctx, `UPDATE users SET balance=balance+$1,updated_at=NOW() WHERE id=$2 AND deleted_at IS NULL RETURNING balance`, refundable.StringFixed(8), card.CreatorUserID).Scan(&balance); err != nil {
			return nil, err
		}
		tombstone := fmt.Sprintf("__link_refunded__%d__%d", card.APIKeyID, time.Now().UnixNano())
		_, err := tx.ExecContext(ctx, `UPDATE api_keys SET key=$1,status=$2,link_state=$3,link_total_refunded=link_total_refunded+$4,link_revoked_at=NOW(),updated_at=NOW() WHERE id=$5 AND key_type=$6`, tombstone, service.StatusAPIKeyDisabled, service.LinkCardStateRefunded, refundable.StringFixed(8), card.APIKeyID, service.APIKeyTypeLink)
		if err != nil {
			return nil, err
		}
		var ledgerID int64
		err = tx.QueryRowContext(ctx, `INSERT INTO link_card_ledger(operation_id,api_key_id,creator_user_id,entry_type,reserve_delta,creator_balance_delta,quota_before,quota_after,quota_used_before,quota_used_after,actor_user_id,reason) VALUES($1,$2,$3,'refund',-$4,$4,$5,$5,$6,$6,$7,$8) RETURNING id`, opID, card.APIKeyID, card.CreatorUserID, refundable.StringFixed(8), card.TotalDepositAmount.StringFixed(8), card.UsedActualAmount().StringFixed(8), cmd.ActorUserID, cmd.Reason).Scan(&ledgerID)
		if err != nil {
			return nil, err
		}
		card.Status = service.LinkCardStateRefunded
		card.RevokedAt = timePtr(time.Now())
		card.SetFinancialState(card.UsedActualAmount(), card.TotalRefundedAmount().Add(refundable))
		card.NormalizeDerivedFields()
		return &service.LinkCardMutationResult{Card: *card, Action: "refund", RefundedAmount: refundable, UserBalance: balance, LedgerID: ledgerID}, nil
	})
}

func (r *linkCardRepository) SetState(ctx context.Context, cmd service.LinkCardMutationCommand, state string) (*service.LinkCardMutationResult, error) {
	return r.mutate(ctx, cmd, func(ctx context.Context, tx *sql.Tx, card *service.LinkCard, opID int64) (*service.LinkCardMutationResult, error) {
		if card.Status == service.LinkCardStateRefunded || card.Status == service.LinkCardStateRevoked {
			return nil, service.ErrLinkCardOperationNotAllowed
		}
		apiStatus := service.StatusAPIKeyDisabled
		keyValue := card.Key
		var revoked any = nil
		switch state {
		case service.LinkCardStateActive:
			if card.ActivatedAt == nil {
				return nil, service.ErrLinkCardOperationNotAllowed
			}
			apiStatus = service.StatusActive
		case service.LinkCardStateFrozen:
		case service.LinkCardStateRevoked:
			keyValue = fmt.Sprintf("__link_revoked__%d__%d", card.APIKeyID, time.Now().UnixNano())
			revoked = time.Now()
		default:
			return nil, service.ErrLinkCardOperationNotAllowed
		}
		_, err := tx.ExecContext(ctx, `UPDATE api_keys SET key=$1,status=$2,link_state=$3,link_frozen_reason=$4,link_revoked_at=COALESCE($5,link_revoked_at),updated_at=NOW() WHERE id=$6 AND key_type=$7`, keyValue, apiStatus, state, cmd.Reason, revoked, card.APIKeyID, service.APIKeyTypeLink)
		if err != nil {
			return nil, err
		}
		card.Status = state
		card.FrozenReason = cmd.Reason
		card.UpdatedAt = time.Now()
		if state == service.LinkCardStateRevoked {
			card.RevokedAt = timePtr(time.Now())
		}
		card.NormalizeDerivedFields()
		return &service.LinkCardMutationResult{Card: *card, Action: cmd.Scope}, nil
	})
}

func (r *linkCardRepository) SetLimits(ctx context.Context, cmd service.LinkCardMutationCommand) (*service.LinkCardMutationResult, error) {
	return r.mutate(ctx, cmd, func(ctx context.Context, tx *sql.Tx, card *service.LinkCard, opID int64) (*service.LinkCardMutationResult, error) {
		concurrency := card.Concurrency
		rpm := card.RPMLimit
		if cmd.Concurrency != nil {
			concurrency = *cmd.Concurrency
		}
		if cmd.RPMLimit != nil {
			rpm = *cmd.RPMLimit
		}
		if _, err := tx.ExecContext(ctx, `UPDATE api_keys SET link_concurrency=$1,link_rpm_limit=$2,updated_at=NOW() WHERE id=$3 AND key_type=$4`, concurrency, rpm, card.APIKeyID, service.APIKeyTypeLink); err != nil {
			return nil, err
		}
		card.Concurrency = concurrency
		card.RPMLimit = rpm
		card.UpdatedAt = time.Now()
		card.NormalizeDerivedFields()
		return &service.LinkCardMutationResult{Card: *card, Action: "set_limits"}, nil
	})
}

type linkCardMutator func(context.Context, *sql.Tx, *service.LinkCard, int64) (*service.LinkCardMutationResult, error)

func (r *linkCardRepository) mutate(ctx context.Context, cmd service.LinkCardMutationCommand, apply linkCardMutator) (_ *service.LinkCardMutationResult, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()
	card, err := scanLinkCard(tx.QueryRowContext(ctx, linkCardSelectSQL()+` WHERE ak.key_type=$1 AND ak.deleted_at IS NULL AND ak.id=$2 FOR UPDATE OF ak`, service.APIKeyTypeLink, cmd.APIKeyID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrLinkCardNotFound
	}
	if err != nil {
		return nil, err
	}
	if !cmd.Admin && card.CreatorUserID != cmd.ActorUserID {
		return nil, service.ErrLinkCardNotFound
	}
	opID, replay, err := claimLinkCardOperation(ctx, tx, cmd.Scope, cmd.ActorUserID, card.CreatorUserID, &card.APIKeyID, cmd.IdempotencyKeyHash, cmd.RequestFingerprint)
	if err != nil {
		return nil, err
	}
	if replay != nil {
		var prior service.LinkCardMutationResult
		if err := json.Unmarshal(replay, &prior); err != nil {
			return nil, err
		}
		prior.Replayed = true
		return &prior, nil
	}
	result, err := apply(ctx, tx, card, opID)
	if err != nil {
		return nil, err
	}
	if err := storeLinkCardOperationResponse(ctx, tx, opID, result); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	tx = nil
	return result, nil
}

func (r *linkCardRepository) ActivateByKey(ctx context.Context, key string) (_ *service.LinkCard, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()
	card, err := scanLinkCard(tx.QueryRowContext(ctx, linkCardSelectSQL()+` WHERE ak.key_type=$1 AND ak.deleted_at IS NULL AND ak.key=$2 FOR UPDATE OF ak`, service.APIKeyTypeLink, key))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrLinkCardNotFound
	}
	if err != nil {
		return nil, err
	}
	if card.Status == service.LinkCardStatePendingActivation {
		var authorized bool
		if err := tx.QueryRowContext(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM link_card_group_authorizations a
				JOIN groups g ON g.id=a.group_id
				WHERE a.group_id=$1 AND a.enabled=TRUE AND g.deleted_at IS NULL
				  AND g.status='active' AND g.subscription_type=$2
			)
		`, card.GroupID, service.SubscriptionTypeStandard).Scan(&authorized); err != nil {
			return nil, err
		}
		if !authorized {
			return nil, service.ErrLinkCardGroupNotAuthorized
		}
		now := time.Now()
		if _, err := tx.ExecContext(ctx, `UPDATE api_keys SET link_state=$1,status=$2,link_activated_at=$3,updated_at=$3 WHERE id=$4 AND key_type=$5`, service.LinkCardStateActive, service.StatusActive, now, card.APIKeyID, service.APIKeyTypeLink); err != nil {
			return nil, err
		}
		card.Status = service.LinkCardStateActive
		card.ActivatedAt = &now
		card.UpdatedAt = now
	} else if card.Status != service.LinkCardStateActive && card.Status != service.LinkCardStateDepleted {
		return nil, service.ErrLinkCardOperationNotAllowed
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	tx = nil
	card.NormalizeDerivedFields()
	return card, nil
}

func (r *linkCardRepository) ListUsage(ctx context.Context, creatorUserID *int64, params pagination.PaginationParams, filters service.LinkCardUsageFilters) ([]service.LinkCardUsageLog, *pagination.PaginationResult, error) {
	where, args := buildLinkCardUsageWhere(creatorUserID, filters)
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM usage_logs ul JOIN api_keys ak ON ak.id=ul.api_key_id JOIN users u ON u.id=ak.user_id LEFT JOIN groups g ON g.id=ul.group_id `+where, args...).Scan(&total); err != nil {
		return nil, nil, err
	}
	args = append(args, params.Limit(), params.Offset())
	query := linkCardUsageSelectSQL() + where + ` ORDER BY ul.created_at DESC,ul.id DESC` + fmt.Sprintf(` LIMIT $%d OFFSET $%d`, len(args)-1, len(args))
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	out := make([]service.LinkCardUsageLog, 0)
	for rows.Next() {
		item, err := scanLinkCardUsage(rows)
		if err != nil {
			return nil, nil, err
		}
		out = append(out, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return out, linkCardPagination(total, params), nil
}

func claimLinkCardOperation(ctx context.Context, tx *sql.Tx, scope string, actor, creator int64, apiKeyID *int64, keyHash, fingerprint string) (int64, []byte, error) {
	var id int64
	err := tx.QueryRowContext(ctx, `INSERT INTO link_card_operations(scope,actor_user_id,creator_user_id,api_key_id,idempotency_key_hash,request_fingerprint) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(scope,actor_user_id,idempotency_key_hash) DO NOTHING RETURNING id`, scope, actor, creator, apiKeyID, keyHash, fingerprint).Scan(&id)
	if err == nil {
		return id, nil, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, nil, err
	}
	var existingFingerprint, response string
	err = tx.QueryRowContext(ctx, `SELECT id,request_fingerprint,response_body::text FROM link_card_operations WHERE scope=$1 AND actor_user_id=$2 AND idempotency_key_hash=$3`, scope, actor, keyHash).Scan(&id, &existingFingerprint, &response)
	if err != nil {
		return 0, nil, err
	}
	if existingFingerprint != fingerprint {
		return 0, nil, service.ErrLinkCardIdempotencyConflict
	}
	return id, []byte(response), nil
}
func storeLinkCardOperationResponse(ctx context.Context, tx *sql.Tx, id int64, value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE link_card_operations SET response_body=$1::jsonb WHERE id=$2`, string(body), id)
	return err
}

func linkCardSelectSQL() string {
	return `SELECT ak.id,ak.user_id,u.email,ak.key,g.id,g.name,g.platform,COALESCE(ak.link_rate_multiplier,1),COALESCE(ak.link_state,''),COALESCE(ak.link_original_debit,0),COALESCE(ak.link_total_funded,ak.quota),COALESCE(ak.link_total_refunded,0),ak.quota_used,COALESCE(ak.link_reserved_amount,0),COALESCE(ak.link_concurrency,5),COALESCE(ak.link_rpm_limit,0),ak.link_activated_at,ak.link_revoked_at,COALESCE(ak.link_frozen_reason,''),ak.created_at,ak.updated_at,(SELECT COUNT(*) FROM usage_logs ul WHERE ul.api_key_id=ak.id) FROM api_keys ak JOIN users u ON u.id=ak.user_id JOIN groups g ON g.id=ak.group_id `
}

type linkCardRowScanner interface{ Scan(...any) error }

func scanLinkCard(row linkCardRowScanner) (*service.LinkCard, error) {
	var card service.LinkCard
	var original, funded, refunded, used decimal.Decimal
	var reserved decimal.Decimal
	err := row.Scan(&card.APIKeyID, &card.CreatorUserID, &card.CreatorEmail, &card.Key, &card.GroupID, &card.GroupName, &card.Platform, &card.IssueRateMultiplier, &card.Status, &original, &funded, &refunded, &used, &reserved, &card.Concurrency, &card.RPMLimit, &card.ActivatedAt, &card.RevokedAt, &card.FrozenReason, &card.CreatedAt, &card.UpdatedAt, &card.RequestCount)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(card.Key, "__link_") {
		card.Key = ""
	}
	card.OriginalDepositAmount = original
	card.TotalDepositAmount = funded
	card.SetFinancialState(used, refunded)
	card.SetReservedAmount(reserved)
	card.NormalizeDerivedFields()
	return &card, nil
}

func buildLinkCardWhere(owner *int64, f service.LinkCardListFilters) (string, []any) {
	conditions := []string{"ak.key_type='link'", "ak.deleted_at IS NULL"}
	args := []any{}
	add := func(expr string, v any) {
		args = append(args, v)
		conditions = append(conditions, fmt.Sprintf(expr, len(args)))
	}
	if owner != nil {
		add("ak.user_id=$%d", *owner)
	}
	if f.CreatorUserID != nil {
		add("ak.user_id=$%d", *f.CreatorUserID)
	}
	if f.CreatorEmail != "" {
		add("u.email ILIKE '%%'||$%d||'%%'", f.CreatorEmail)
	}
	if f.Status != "" {
		add("ak.link_state=$%d", f.Status)
	}
	if f.GroupID != nil {
		add("ak.group_id=$%d", *f.GroupID)
	}
	if f.Search != "" {
		args = append(args, f.Search)
		n := len(args)
		conditions = append(conditions, fmt.Sprintf("(ak.key ILIKE '%%'||$%d||'%%' OR u.email ILIKE '%%'||$%d||'%%' OR g.name ILIKE '%%'||$%d||'%%')", n, n, n))
	}
	return " WHERE " + strings.Join(conditions, " AND "), args
}
func linkCardOrder(p pagination.PaginationParams) string {
	field := "ak.created_at"
	switch strings.ToLower(strings.TrimSpace(p.SortBy)) {
	case "id":
		field = "ak.id"
	case "status":
		field = "ak.link_state"
	case "remaining_quota":
		field = "(ak.quota-ak.quota_used)"
	}
	direction := "DESC"
	if p.NormalizedSortOrder(pagination.SortOrderDesc) == pagination.SortOrderAsc {
		direction = "ASC"
	}
	return field + " " + direction + ", ak.id " + direction
}
func linkCardPagination(total int64, p pagination.PaginationParams) *pagination.PaginationResult {
	size := p.Limit()
	page := p.Page
	if page < 1 {
		page = 1
	}
	pages := int(total) / size
	if int(total)%size != 0 {
		pages++
	}
	if pages < 1 {
		pages = 1
	}
	return &pagination.PaginationResult{Total: total, Page: page, PageSize: size, Pages: pages}
}

func linkCardUsageSelectSQL() string {
	return `SELECT ul.id,ul.api_key_id,ul.user_id,u.email,ak.key,ul.request_id,ul.model,ul.inbound_endpoint,ul.group_id,COALESCE(g.name,''),COALESCE(ul.request_type,0),ul.billing_mode,ul.stream,ul.input_tokens,ul.output_tokens,ul.cache_creation_tokens,ul.cache_creation_5m_tokens,ul.cache_creation_1h_tokens,ul.cache_read_tokens,ul.image_input_tokens,ul.image_output_tokens,ul.input_cost,ul.output_cost,ul.cache_creation_cost,ul.cache_read_cost,ul.image_input_cost,ul.image_output_cost,ul.total_cost,ul.actual_cost,ul.rate_multiplier,ul.service_tier,ul.duration_ms,ul.first_token_ms,ul.created_at,COALESCE(ak.link_rate_multiplier,1) FROM usage_logs ul JOIN api_keys ak ON ak.id=ul.api_key_id JOIN users u ON u.id=ak.user_id LEFT JOIN groups g ON g.id=ul.group_id `
}
func buildLinkCardUsageWhere(owner *int64, f service.LinkCardUsageFilters) (string, []any) {
	conditions := []string{"ak.key_type='link'"}
	args := []any{}
	add := func(expr string, v any) {
		args = append(args, v)
		conditions = append(conditions, fmt.Sprintf(expr, len(args)))
	}
	if owner != nil {
		add("ak.user_id=$%d", *owner)
	}
	if f.CreatorUserID != nil {
		add("ak.user_id=$%d", *f.CreatorUserID)
	}
	if f.CreatorEmail != "" {
		add("u.email ILIKE '%%'||$%d||'%%'", f.CreatorEmail)
	}
	if f.CardID != nil {
		add("ak.id=$%d", *f.CardID)
	}
	if f.RequestID != "" {
		add("ul.request_id ILIKE '%%'||$%d||'%%'", f.RequestID)
	}
	if f.Model != "" {
		add("ul.model ILIKE '%%'||$%d||'%%'", f.Model)
	}
	if f.GroupID != nil {
		add("ul.group_id=$%d", *f.GroupID)
	}
	if f.RequestType != "" {
		requestType, err := service.ParseUsageRequestType(f.RequestType)
		if err != nil {
			conditions = append(conditions, "1=0")
		} else {
			args = append(args, int16(requestType))
			modern := fmt.Sprintf("ul.request_type = $%d", len(args))
			legacy := ""
			switch requestType {
			case service.RequestTypeSync:
				legacy = "(ul.request_type = 0 AND ul.stream = FALSE AND ul.openai_ws_mode = FALSE)"
			case service.RequestTypeStream:
				legacy = "(ul.request_type = 0 AND ul.stream = TRUE AND ul.openai_ws_mode = FALSE)"
			case service.RequestTypeWSV2:
				legacy = "(ul.request_type = 0 AND ul.openai_ws_mode = TRUE)"
			}
			if legacy != "" {
				conditions = append(conditions, "("+modern+" OR "+legacy+")")
			} else {
				conditions = append(conditions, modern)
			}
		}
	}
	if f.Stream != nil {
		add("ul.stream=$%d", *f.Stream)
	}
	if f.StartAt != nil {
		add("ul.created_at >= $%d", *f.StartAt)
	}
	if f.EndAt != nil {
		add("ul.created_at < $%d", *f.EndAt)
	}
	if f.Key != "" {
		add("ak.key=$%d", f.Key)
	}
	return " WHERE " + strings.Join(conditions, " AND "), args
}
func scanLinkCardUsage(row linkCardRowScanner) (*service.LinkCardUsageLog, error) {
	var item service.LinkCardUsageLog
	var key string
	var requestType int16
	var issueRate float64
	err := row.Scan(&item.ID, &item.APIKeyID, &item.CreatorUserID, &item.CreatorEmail, &key, &item.RequestID, &item.Model, &item.InboundEndpoint, &item.GroupID, &item.GroupName, &requestType, &item.BillingMode, &item.Stream, &item.InputTokens, &item.OutputTokens, &item.CacheCreationTokens, &item.CacheCreation5mTokens, &item.CacheCreation1hTokens, &item.CacheReadTokens, &item.ImageInputTokens, &item.ImageOutputTokens, &item.InputCost, &item.OutputCost, &item.CacheCreationCost, &item.CacheReadCost, &item.ImageInputCost, &item.ImageOutputCost, &item.TotalCost, &item.ActualCost, &item.RateMultiplier, &item.ServiceTier, &item.DurationMS, &item.FirstTokenMS, &item.CreatedAt, &issueRate)
	if err != nil {
		return nil, err
	}
	item.LinkCardID = item.APIKeyID
	item.RequestType = service.RequestTypeFromInt16(requestType).String()
	item.TotalTokens = item.InputTokens + item.OutputTokens + item.CacheCreationTokens + item.CacheReadTokens
	item.KeyPrefix = keyPrefix(key)
	item.MaskedKey = maskKey(key)
	item.SetIssueRateMultiplier(issueRate)
	return &item, nil
}
func keyPrefix(key string) string {
	if strings.HasPrefix(key, "__link_") {
		return ""
	}
	if len(key) <= 14 {
		return key
	}
	return key[:14]
}
func maskKey(key string) string {
	if strings.HasPrefix(key, "__link_") {
		return ""
	}
	if len(key) <= 12 {
		return key
	}
	return key[:8] + "..." + key[len(key)-4:]
}
func timePtr(v time.Time) *time.Time { return &v }
