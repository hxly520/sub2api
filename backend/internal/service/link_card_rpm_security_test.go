package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type linkCardRPMCacheStub struct {
	userIDs []int64
}

type linkCardRPMStateLoader struct{}

func (linkCardRPMStateLoader) GetRateLimitData(context.Context, int64) (*APIKeyRateLimitData, error) {
	return &APIKeyRateLimitData{}, nil
}

func (linkCardRPMStateLoader) GetLinkCardBillingState(context.Context, int64) (*LinkCardBillingState, error) {
	return &LinkCardBillingState{
		Status:    StatusAPIKeyActive,
		LinkState: LinkCardStateActive,
		Quota:     100,
		QuotaUsed: 0,
	}, nil
}

func (s *linkCardRPMCacheStub) IncrementUserGroupRPM(context.Context, int64, int64) (int, error) {
	return 0, nil
}

func (s *linkCardRPMCacheStub) IncrementUserRPM(_ context.Context, userID int64) (int, error) {
	s.userIDs = append(s.userIDs, userID)
	return 1, nil
}

func (s *linkCardRPMCacheStub) GetUserGroupRPM(context.Context, int64, int64) (int, error) {
	return 0, nil
}

func (s *linkCardRPMCacheStub) GetUserRPM(context.Context, int64) (int, error) {
	return 0, nil
}

func TestLinkCardsUseIndependentRPMSubjects(t *testing.T) {
	cache := &linkCardRPMCacheStub{}
	svc := &BillingCacheService{cfg: &config.Config{}, userRPMCache: cache, apiKeyRateLimitLoader: linkCardRPMStateLoader{}}
	user := &User{ID: 1}
	group := &Group{ID: 8, RPMLimit: 100}

	for _, cardID := range []int64{91, 92} {
		err := svc.CheckBillingEligibility(context.Background(), user, &APIKey{
			ID: cardID, KeyType: APIKeyTypeLink, LinkState: LinkCardStateActive,
			Quota: 100, LinkRPMLimit: 5,
		}, group, nil, "")
		require.NoError(t, err)
	}

	require.Equal(t, []int64{-91, -92}, cache.userIDs)
}
