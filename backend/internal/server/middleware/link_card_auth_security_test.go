//go:build unit

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestLinkCardAuthDoesNotDependOnCreatorPostPaymentBalance(t *testing.T) {
	gin.SetMode(gin.TestMode)
	group := &service.Group{
		ID:               8,
		Name:             "link-group",
		Status:           service.StatusActive,
		Hydrated:         true,
		SubscriptionType: service.SubscriptionTypeStandard,
		RateMultiplier:   0.08,
	}
	user := &service.User{
		ID:          1,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Balance:     0,
		Concurrency: 20,
	}
	key := &service.APIKey{
		ID:                 91,
		UserID:             user.ID,
		Key:                "sk-card-prepaid-zero-owner-balance",
		Status:             service.StatusAPIKeyActive,
		KeyType:            service.APIKeyTypeLink,
		LinkState:          service.LinkCardStateActive,
		LinkRateMultiplier: 0.08,
		LinkConcurrency:    5,
		Quota:              100,
		QuotaUsed:          10,
		User:               user,
		Group:              group,
		GroupID:            &group.ID,
	}
	repo := &stubApiKeyRepo{getByKey: func(_ context.Context, value string) (*service.APIKey, error) {
		if value != key.Key {
			return nil, service.ErrAPIKeyNotFound
		}
		clone := *key
		return &clone, nil
	}}
	cfg := &config.Config{RunMode: config.RunModeStandard}
	apiKeys := service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, cfg)

	reached := false
	router := gin.New()
	router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(apiKeys, nil, cfg)))
	router.POST("/v1/responses", func(c *gin.Context) {
		reached = true
		subject, ok := GetAuthSubjectFromContext(c)
		require.True(t, ok)
		require.Equal(t, 5, subject.Concurrency)
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	request.Header.Set("Authorization", "Bearer "+key.Key)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.True(t, reached)
	require.Equal(t, http.StatusNoContent, response.Code)
}

func TestLinkCardAuthSkipsCreatorSubscriptionLookup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	group := &service.Group{
		ID:               18,
		Name:             "link-subscription-group",
		Status:           service.StatusActive,
		Hydrated:         true,
		SubscriptionType: service.SubscriptionTypeSubscription,
		RateMultiplier:   0.08,
	}
	user := &service.User{ID: 1, Role: service.RoleUser, Status: service.StatusActive, Balance: 0, Concurrency: 10}
	key := &service.APIKey{
		ID: 94, UserID: user.ID, Key: "sk-card-subscription-group", Status: service.StatusAPIKeyActive,
		KeyType: service.APIKeyTypeLink, LinkState: service.LinkCardStateActive, LinkRateMultiplier: 0.08,
		LinkConcurrency: 5, Quota: 100, QuotaUsed: 0, User: user, Group: group, GroupID: &group.ID,
	}
	repo := &stubApiKeyRepo{getByKey: func(_ context.Context, value string) (*service.APIKey, error) {
		if value != key.Key {
			return nil, service.ErrAPIKeyNotFound
		}
		clone := *key
		return &clone, nil
	}}
	cfg := &config.Config{RunMode: config.RunModeStandard}
	apiKeys := service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, cfg)

	reached := false
	router := gin.New()
	// An empty subscription service would panic if consulted. A link key must
	// bypass that creator-account lookup and enter the native handler.
	router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(apiKeys, &service.SubscriptionService{}, cfg)))
	router.POST("/v1/responses", func(c *gin.Context) {
		reached = true
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	request.Header.Set("Authorization", "Bearer "+key.Key)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.True(t, reached)
	require.Equal(t, http.StatusNoContent, response.Code)
}

func TestLinkCardAuthAllowsDepletedReadOnlyUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	group := &service.Group{ID: 8, Name: "link-group", Status: service.StatusActive, Hydrated: true, SubscriptionType: service.SubscriptionTypeStandard, RateMultiplier: 0.08}
	user := &service.User{ID: 1, Role: service.RoleUser, Status: service.StatusActive, Balance: 0, Concurrency: 10}
	key := &service.APIKey{ID: 95, UserID: user.ID, Key: "sk-card-depleted-read", Status: service.StatusAPIKeyQuotaExhausted, KeyType: service.APIKeyTypeLink, LinkState: service.LinkCardStateDepleted, Quota: 1, QuotaUsed: 1, User: user, Group: group, GroupID: &group.ID}
	repo := &stubApiKeyRepo{getByKey: func(_ context.Context, value string) (*service.APIKey, error) {
		if value != key.Key {
			return nil, service.ErrAPIKeyNotFound
		}
		clone := *key
		return &clone, nil
	}}
	cfg := &config.Config{RunMode: config.RunModeStandard}
	apiKeys := service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, cfg)
	reached := false
	router := gin.New()
	router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(apiKeys, nil, cfg)))
	router.GET("/v1/usage", func(c *gin.Context) { reached = true; c.Status(http.StatusNoContent) })

	request := httptest.NewRequest(http.MethodGet, "/v1/usage", nil)
	request.Header.Set("Authorization", "Bearer "+key.Key)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.True(t, reached)
	require.Equal(t, http.StatusNoContent, response.Code)
}

func TestLinkCardGoogleAuthUsesPrepaidBalanceAndCardConcurrency(t *testing.T) {
	gin.SetMode(gin.TestMode)
	group := &service.Group{
		ID:               8,
		Name:             "link-group",
		Status:           service.StatusActive,
		Hydrated:         true,
		SubscriptionType: service.SubscriptionTypeStandard,
		RateMultiplier:   0.08,
	}
	user := &service.User{
		ID:          1,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Balance:     0,
		Concurrency: 20,
	}
	key := &service.APIKey{
		ID:                 92,
		UserID:             user.ID,
		Key:                "sk-card-google-prepaid-zero-owner-balance",
		Status:             service.StatusAPIKeyActive,
		KeyType:            service.APIKeyTypeLink,
		LinkState:          service.LinkCardStateActive,
		LinkRateMultiplier: 0.08,
		LinkConcurrency:    5,
		Quota:              100,
		QuotaUsed:          10,
		User:               user,
		Group:              group,
		GroupID:            &group.ID,
	}
	repo := &stubApiKeyRepo{getByKey: func(_ context.Context, value string) (*service.APIKey, error) {
		if value != key.Key {
			return nil, service.ErrAPIKeyNotFound
		}
		clone := *key
		return &clone, nil
	}}
	cfg := &config.Config{RunMode: config.RunModeStandard}
	apiKeys := service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, cfg)

	reached := false
	router := gin.New()
	router.Use(APIKeyAuthGoogle(apiKeys, cfg))
	router.POST("/v1beta/models/MODEL:generateContent", func(c *gin.Context) {
		reached = true
		subject, ok := GetAuthSubjectFromContext(c)
		require.True(t, ok)
		require.Equal(t, 5, subject.Concurrency)
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodPost, "/v1beta/models/MODEL:generateContent", nil)
	request.Header.Set("x-goog-api-key", key.Key)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.True(t, reached)
	require.Equal(t, http.StatusNoContent, response.Code)
}

func TestLinkCardGoogleAuthRejectsFrozenCard(t *testing.T) {
	gin.SetMode(gin.TestMode)
	group := &service.Group{
		ID:               8,
		Name:             "link-group",
		Status:           service.StatusActive,
		Hydrated:         true,
		SubscriptionType: service.SubscriptionTypeStandard,
		RateMultiplier:   0.08,
	}
	user := &service.User{
		ID:          1,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Balance:     100,
		Concurrency: 20,
	}
	key := &service.APIKey{
		ID:              93,
		UserID:          user.ID,
		Key:             "sk-card-google-frozen-card",
		Status:          service.StatusAPIKeyActive,
		KeyType:         service.APIKeyTypeLink,
		LinkState:       service.LinkCardStateFrozen,
		LinkConcurrency: 5,
		Quota:           100,
		User:            user,
		Group:           group,
		GroupID:         &group.ID,
	}
	repo := &stubApiKeyRepo{getByKey: func(_ context.Context, _ string) (*service.APIKey, error) {
		clone := *key
		return &clone, nil
	}}
	cfg := &config.Config{RunMode: config.RunModeStandard}
	apiKeys := service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, cfg)

	reached := false
	router := gin.New()
	router.Use(APIKeyAuthGoogle(apiKeys, cfg))
	router.POST("/v1beta/models/MODEL:generateContent", func(c *gin.Context) {
		reached = true
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodPost, "/v1beta/models/MODEL:generateContent", nil)
	request.Header.Set("x-goog-api-key", key.Key)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.False(t, reached)
	require.Equal(t, http.StatusUnauthorized, response.Code)
}

func TestLinkCardGoogleAuthAllowsDepletedReadOnlyUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	group := &service.Group{ID: 8, Name: "link-group", Status: service.StatusActive, Hydrated: true, SubscriptionType: service.SubscriptionTypeStandard, RateMultiplier: 0.08}
	user := &service.User{ID: 1, Role: service.RoleUser, Status: service.StatusActive, Balance: 0, Concurrency: 10}
	key := &service.APIKey{ID: 96, UserID: user.ID, Key: "sk-card-google-depleted-read", Status: service.StatusAPIKeyQuotaExhausted, KeyType: service.APIKeyTypeLink, LinkState: service.LinkCardStateDepleted, Quota: 1, QuotaUsed: 1, User: user, Group: group, GroupID: &group.ID}
	repo := &stubApiKeyRepo{getByKey: func(_ context.Context, value string) (*service.APIKey, error) {
		if value != key.Key {
			return nil, service.ErrAPIKeyNotFound
		}
		clone := *key
		return &clone, nil
	}}
	cfg := &config.Config{RunMode: config.RunModeStandard}
	apiKeys := service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, cfg)
	reached := false
	router := gin.New()
	router.Use(APIKeyAuthGoogle(apiKeys, cfg))
	router.GET("/v1/usage", func(c *gin.Context) { reached = true; c.Status(http.StatusNoContent) })

	request := httptest.NewRequest(http.MethodGet, "/v1/usage", nil)
	request.Header.Set("x-goog-api-key", key.Key)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	t.Logf("google depleted read response: %d %s", response.Code, response.Body.String())
	require.True(t, reached)
	require.Equal(t, http.StatusNoContent, response.Code)
}

func TestLinkCardAuthUsesNativeExclusiveGroupPermission(t *testing.T) {
	group := &service.Group{ID: 8, IsExclusive: true, SubscriptionType: service.SubscriptionTypeStandard}
	user := &service.User{ID: 1}
	key := &service.APIKey{
		KeyType: service.APIKeyTypeLink,
		GroupID: &group.ID,
		Group:   group,
		User:    user,
	}

	require.False(t, validateAPIKeyGroupAllowed(key))
	user.AllowedGroups = []int64{group.ID}
	require.True(t, validateAPIKeyGroupAllowed(key))
	group.IsExclusive = false
	user.AllowedGroups = nil
	require.True(t, validateAPIKeyGroupAllowed(key))
}
