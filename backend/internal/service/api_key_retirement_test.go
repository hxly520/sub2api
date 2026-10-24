//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyService_RetirementRejectsOldSnapshotAndReloadsRepository(t *testing.T) {
	var cached APIKeyAuthCacheEntry
	// The old payload includes a discriminator no longer present in runtime
	// types. Its version must force a database lookup rather than silently
	// accepting it as an ordinary owner-funded key after JSON unmarshalling.
	require.NoError(t, json.Unmarshal([]byte(`{"snapshot":{"version":24,"api_key_id":7,"user_id":42,"key_type":"link","status":"active","user":{"id":42,"status":"active","balance":100}}}`), &cached))
	calls := 0
	repo := &authRepoStub{getByKeyForAuth: func(context.Context, string) (*APIKey, error) {
		calls++
		return nil, ErrAPIKeyNotFound
	}}
	cache := &authCacheStub{getAuthCache: func(context.Context, string) (*APIKeyAuthCacheEntry, error) {
		return &cached, nil
	}}
	svc := NewAPIKeyService(repo, nil, nil, nil, nil, cache, &config.Config{
		APIKeyAuth: config.APIKeyAuthCacheConfig{L2TTLSeconds: 60},
	})
	key, err := svc.GetByKey(context.Background(), "sk-retired-key")
	require.Nil(t, key)
	require.ErrorIs(t, err, ErrAPIKeyNotFound)
	require.Equal(t, 1, calls)
	require.Empty(t, cache.setAuthKeys, "a retired key must never populate a fresh positive snapshot")
}
