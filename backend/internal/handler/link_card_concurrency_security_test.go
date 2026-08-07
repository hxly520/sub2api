//go:build unit

package handler

import (
	"context"
	"errors"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type linkCardConcurrencyCacheStub struct {
	*helperConcurrencyCacheStub
	mu              sync.Mutex
	acquiredUserIDs []int64
	trackedKeyIDs   []int64
	trackErr        error
}

func newLinkCardConcurrencyCacheStub() *linkCardConcurrencyCacheStub {
	return &linkCardConcurrencyCacheStub{helperConcurrencyCacheStub: &helperConcurrencyCacheStub{}}
}

func (s *linkCardConcurrencyCacheStub) AcquireUserSlot(_ context.Context, userID int64, _ int, _ string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.acquiredUserIDs = append(s.acquiredUserIDs, userID)
	return true, nil
}

func (s *linkCardConcurrencyCacheStub) TrackAPIKeySlot(_ context.Context, apiKeyID int64, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.trackedKeyIDs = append(s.trackedKeyIDs, apiKeyID)
	return s.trackErr
}

func TestLinkCardConcurrencyUsesCardScopedSlot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cache := newLinkCardConcurrencyCacheStub()
	helper := NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, 0)
	card := &service.APIKey{ID: 91, UserID: 1, KeyType: service.APIKeyTypeLink, LinkConcurrency: 5}

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), card)
	streamStarted := false
	release, err := helper.AcquireUserSlotWithWait(c, card.UserID, card.LinkConcurrency, false, &streamStarted)
	require.NoError(t, err)
	require.NotNil(t, release)
	require.Equal(t, []int64{-card.ID}, cache.acquiredUserIDs)
	require.Equal(t, []int64{card.ID}, cache.trackedKeyIDs)

	release()
	require.Equal(t, 1, cache.userReleaseCalls)
	require.Equal(t, 1, cache.apiKeyReleaseCalls)
}

func TestLinkCardConcurrencySeparatesCardsFromSameCreator(t *testing.T) {
	require.Equal(t, int64(-91), effectiveUserSlotID(&service.APIKey{ID: 91, UserID: 1, KeyType: service.APIKeyTypeLink}, 1))
	require.Equal(t, int64(-92), effectiveUserSlotID(&service.APIKey{ID: 92, UserID: 1, KeyType: service.APIKeyTypeLink}, 1))
	require.Equal(t, int64(1), effectiveUserSlotID(&service.APIKey{ID: 93, UserID: 1, KeyType: service.APIKeyTypeStandard}, 1))
}

func TestLinkCardConcurrencyFailsClosedWhenKeyTrackingFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cache := newLinkCardConcurrencyCacheStub()
	cache.trackErr = errors.New("redis unavailable")
	helper := NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, 0)
	card := &service.APIKey{ID: 94, UserID: 1, KeyType: service.APIKeyTypeLink, LinkConcurrency: 5}

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), card)
	streamStarted := false
	release, err := helper.AcquireUserSlotWithWait(c, card.UserID, card.LinkConcurrency, false, &streamStarted)

	require.Error(t, err)
	require.Nil(t, release)
	require.Equal(t, 1, cache.userReleaseCalls, "the card-scoped slot must be released after strict tracking fails")
}

func TestLinkCardBillingInfoUsesCurrentToIssueRateRatio(t *testing.T) {
	group := &service.Group{
		ID:             8,
		Name:           "link-group",
		Platform:       service.PlatformOpenAI,
		RateMultiplier: 0.15, // current group value changed after issuance
	}
	key := &service.APIKey{
		ID:                 95,
		UserID:             1,
		KeyType:            service.APIKeyTypeLink,
		LinkState:          service.LinkCardStateActive,
		LinkRateMultiplier: 0.10,
		GroupID:            &group.ID,
		Group:              group,
	}

	response := buildKeyBillingInfo(key, group.RateMultiplier, time.Now())
	require.InDelta(t, 1.5, response.GroupRateMultiplier, 1e-12)
	require.InDelta(t, 1.5, response.ResolvedRateMultiplier, 1e-12)
	require.InDelta(t, 1.5, response.EffectiveRateMultiplier, 1e-12)
	require.Nil(t, response.UserRateMultiplier)

	response = buildKeyBillingInfo(key, 0.12, time.Now())
	require.NotNil(t, response.UserRateMultiplier)
	require.InDelta(t, 1.2, *response.UserRateMultiplier, 1e-12)
	require.InDelta(t, 1.2, response.ResolvedRateMultiplier, 1e-12)
}

func TestLinkCardBillingInfoRoundsQuotaRateUpToOneDecimal(t *testing.T) {
	group := &service.Group{ID: 8, RateMultiplier: 0.08}
	key := &service.APIKey{
		KeyType: service.APIKeyTypeLink, LinkRateMultiplier: 0.07, GroupID: &group.ID, Group: group,
	}

	response := buildKeyBillingInfo(key, 0.08, time.Now())
	require.InDelta(t, 1.2, response.GroupRateMultiplier, 1e-12)
	require.InDelta(t, 1.2, response.ResolvedRateMultiplier, 1e-12)
	require.InDelta(t, 1.2, response.EffectiveRateMultiplier, 1e-12)
}

func TestLinkCardBillingInfoAppliesNativePeakToIssuedRate(t *testing.T) {
	group := &service.Group{
		ID:                 8,
		SubscriptionType:   service.SubscriptionTypeSubscription,
		RateMultiplier:     0.15,
		PeakRateEnabled:    true,
		PeakStart:          "09:00",
		PeakEnd:            "18:00",
		PeakRateMultiplier: 1.5,
	}
	key := &service.APIKey{
		KeyType:            service.APIKeyTypeLink,
		LinkRateMultiplier: 0.10,
		GroupID:            &group.ID,
		Group:              group,
	}
	now := time.Date(2026, time.August, 7, 10, 0, 0, 0, timezone.Location())

	response := buildKeyBillingInfo(key, group.RateMultiplier, now)
	require.InDelta(t, 1.5, response.ResolvedRateMultiplier, 1e-12)
	require.True(t, response.PeakRateEnabled)
	require.NotNil(t, response.AppliedPeakMultiplier)
	require.InDelta(t, 1.5, *response.AppliedPeakMultiplier, 1e-12)
	require.InDelta(t, 2.3, response.EffectiveRateMultiplier, 1e-12)
}
