package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type linkCardRefundRepoStub struct {
	LinkCardRepository
	card        LinkCard
	freezeCalls int
	refundCalls int
}

func (r *linkCardRefundRepoStub) GetCard(context.Context, int64, *int64) (*LinkCard, error) {
	card := r.card
	return &card, nil
}

func (r *linkCardRefundRepoStub) FreezeForRefund(context.Context, int64) (*LinkCard, error) {
	r.freezeCalls++
	card := r.card
	card.Status = LinkCardStateFrozen
	r.card = card
	return &card, nil
}

func (r *linkCardRefundRepoStub) Refund(_ context.Context, cmd LinkCardMutationCommand) (*LinkCardMutationResult, error) {
	r.refundCalls++
	card := r.card
	card.Status = LinkCardStateRefunded
	return &LinkCardMutationResult{Card: card, Action: cmd.Scope}, nil
}

type linkCardConcurrencyStub struct {
	counts []int
	err    error
	calls  int
}

func (s *linkCardConcurrencyStub) GetAPIKeyConcurrencyStrict(context.Context, int64) (int, error) {
	s.calls++
	if s.err != nil {
		return 0, s.err
	}
	if len(s.counts) == 0 {
		return 0, nil
	}
	count := s.counts[0]
	s.counts = s.counts[1:]
	return count, nil
}

func TestAdminLinkCardRefundFreezesAndDrainsBeforeMoneyMutation(t *testing.T) {
	repo := &linkCardRefundRepoStub{card: LinkCard{APIKeyID: 91, CreatorUserID: 1, Key: "sk-card-refund-test", Status: LinkCardStateActive}}
	concurrency := &linkCardConcurrencyStub{counts: []int{1, 0}}
	svc := &LinkCardService{repo: repo, concurrency: concurrency}

	result, err := svc.Refund(context.Background(), 9, 91, "admin refund", "refund-idempotency", true)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, repo.freezeCalls)
	require.Equal(t, 1, repo.refundCalls)
	require.GreaterOrEqual(t, concurrency.calls, 2)
}

func TestAdminLinkCardRefundFailsClosedWhenInFlightStateIsUnavailable(t *testing.T) {
	repo := &linkCardRefundRepoStub{card: LinkCard{APIKeyID: 91, CreatorUserID: 1, Key: "sk-card-refund-test", Status: LinkCardStateActive}}
	concurrency := &linkCardConcurrencyStub{err: errors.New("redis unavailable")}
	svc := &LinkCardService{repo: repo, concurrency: concurrency}

	_, err := svc.Refund(context.Background(), 9, 91, "admin refund", "refund-idempotency", true)
	require.Error(t, err)
	require.Equal(t, 1, repo.freezeCalls)
	require.Zero(t, repo.refundCalls)
}
