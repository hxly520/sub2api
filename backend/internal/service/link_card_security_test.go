//go:build unit

package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

type linkCardSecuritySettingRepo struct {
	values map[string]string
}

func (r *linkCardSecuritySettingRepo) Get(context.Context, string) (*Setting, error) {
	return nil, ErrSettingNotFound
}

func (r *linkCardSecuritySettingRepo) GetValue(_ context.Context, key string) (string, error) {
	return r.values[key], nil
}

func (r *linkCardSecuritySettingRepo) Set(_ context.Context, key, value string) error {
	if r.values == nil {
		r.values = make(map[string]string)
	}
	r.values[key] = value
	return nil
}

func (r *linkCardSecuritySettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		out[key] = r.values[key]
	}
	return out, nil
}

func (r *linkCardSecuritySettingRepo) SetMultiple(_ context.Context, values map[string]string) error {
	if r.values == nil {
		r.values = make(map[string]string)
	}
	for key, value := range values {
		r.values[key] = value
	}
	return nil
}

func (r *linkCardSecuritySettingRepo) GetAll(context.Context) (map[string]string, error) {
	out := make(map[string]string, len(r.values))
	for key, value := range r.values {
		out[key] = value
	}
	return out, nil
}

func (r *linkCardSecuritySettingRepo) Delete(_ context.Context, key string) error {
	delete(r.values, key)
	return nil
}

type linkCardSecurityRepo struct {
	group           *LinkCardGroup
	groups          []LinkCardGroup
	activateCard    *LinkCard
	portalCard      *LinkCard
	usage           []LinkCardUsageLog
	activationCalls int
	createCalls     int
	lastCreate      CreateLinkCardsCommand
	lastUsageOwner  *int64
	lastUsageFilter LinkCardUsageFilters
	removedGroupID  int64
	removeGroupErr  error
	rechargeResult  *LinkCardMutationResult
	rechargeErr     error
}

type linkCardSecurityAPIKeyRepo struct {
	APIKeyRepository
	apiKey *APIKey
}

func (r *linkCardSecurityAPIKeyRepo) GetByKeyForAuth(context.Context, string) (*APIKey, error) {
	if r.apiKey == nil {
		return nil, ErrAPIKeyNotFound
	}
	key := *r.apiKey
	return &key, nil
}

func newLinkCardSecurityAPIKeyService(apiKey *APIKey) *APIKeyService {
	return &APIKeyService{apiKeyRepo: &linkCardSecurityAPIKeyRepo{apiKey: apiKey}}
}

type linkCardGroupPolicyUserRepo struct {
	UserRepository
	user *User
}

func (r *linkCardGroupPolicyUserRepo) GetByID(context.Context, int64) (*User, error) {
	user := *r.user
	user.AllowedGroups = append([]int64(nil), r.user.AllowedGroups...)
	return &user, nil
}

type linkCardGroupPolicyGroupRepo struct {
	GroupRepository
	groups []Group
}

func (r *linkCardGroupPolicyGroupRepo) ListActive(context.Context) ([]Group, error) {
	return append([]Group(nil), r.groups...), nil
}

type linkCardGroupPolicySubscriptionRepo struct{ UserSubscriptionRepository }

func (*linkCardGroupPolicySubscriptionRepo) ListActiveByUserID(context.Context, int64) ([]UserSubscription, error) {
	return nil, nil
}

type linkCardGroupPolicyRateRepo struct {
	UserGroupRateRepository
	rates map[int64]float64
}

func (r *linkCardGroupPolicyRateRepo) GetByUserID(context.Context, int64) (map[int64]float64, error) {
	out := make(map[int64]float64, len(r.rates))
	for groupID, rate := range r.rates {
		out[groupID] = rate
	}
	return out, nil
}

func newLinkCardGroupPolicyAPIKeyService(user *User, groups []Group, rates map[int64]float64) *APIKeyService {
	return NewAPIKeyService(
		nil,
		&linkCardGroupPolicyUserRepo{user: user},
		&linkCardGroupPolicyGroupRepo{groups: groups},
		&linkCardGroupPolicySubscriptionRepo{},
		&linkCardGroupPolicyRateRepo{rates: rates},
		nil,
		&config.Config{},
	)
}

func (r *linkCardSecurityRepo) ListAuthorizedGroups(context.Context, bool, int) ([]LinkCardGroup, error) {
	return append([]LinkCardGroup(nil), r.groups...), nil
}

func (r *linkCardSecurityRepo) UpsertAuthorizedGroup(context.Context, int64, bool, int, int64, int) (*LinkCardGroup, error) {
	return nil, nil
}

func (r *linkCardSecurityRepo) RemoveAuthorizedGroup(_ context.Context, groupID int64) error {
	r.removedGroupID = groupID
	return r.removeGroupErr
}

func (r *linkCardSecurityRepo) GetAuthorizedGroup(context.Context, int64, int) (*LinkCardGroup, error) {
	return r.group, nil
}

func (r *linkCardSecurityRepo) CreateCards(_ context.Context, cmd CreateLinkCardsCommand) (*CreateLinkCardsResult, error) {
	r.createCalls++
	r.lastCreate = cmd
	return &CreateLinkCardsResult{
		Quantity:      cmd.Quantity,
		AmountPerCard: cmd.AmountPerCard,
		TotalDebited:  cmd.TotalDebit,
	}, nil
}

func (r *linkCardSecurityRepo) ListCards(context.Context, *int64, pagination.PaginationParams, LinkCardListFilters) ([]LinkCard, *pagination.PaginationResult, error) {
	return nil, nil, nil
}

func (r *linkCardSecurityRepo) Summary(context.Context) (*LinkCardSummary, error) {
	return &LinkCardSummary{}, nil
}

func (r *linkCardSecurityRepo) GetCard(context.Context, int64, *int64) (*LinkCard, error) {
	if r.portalCard == nil {
		return nil, ErrLinkCardNotFound
	}
	card := *r.portalCard
	return &card, nil
}

func (r *linkCardSecurityRepo) FreezeForRefund(ctx context.Context, id int64) (*LinkCard, error) {
	return r.GetCard(ctx, id, nil)
}

func (r *linkCardSecurityRepo) Recharge(context.Context, LinkCardMutationCommand) (*LinkCardMutationResult, error) {
	return r.rechargeResult, r.rechargeErr
}

func (r *linkCardSecurityRepo) Refund(context.Context, LinkCardMutationCommand) (*LinkCardMutationResult, error) {
	return nil, nil
}

func (r *linkCardSecurityRepo) SetState(context.Context, LinkCardMutationCommand, string) (*LinkCardMutationResult, error) {
	return nil, nil
}

func (r *linkCardSecurityRepo) SetLimits(context.Context, LinkCardMutationCommand) (*LinkCardMutationResult, error) {
	return nil, nil
}

func (r *linkCardSecurityRepo) ActivateByKey(context.Context, string) (*LinkCard, error) {
	r.activationCalls++
	if r.activateCard == nil {
		return nil, ErrLinkCardNotFound
	}
	card := *r.activateCard
	return &card, nil
}

func (r *linkCardSecurityRepo) ListUsage(_ context.Context, owner *int64, _ pagination.PaginationParams, filters LinkCardUsageFilters) ([]LinkCardUsageLog, *pagination.PaginationResult, error) {
	r.lastUsageOwner = owner
	r.lastUsageFilter = filters
	return r.usage, &pagination.PaginationResult{Total: int64(len(r.usage)), Page: 1, PageSize: 10, Pages: 1}, nil
}

func TestRemoveAuthorizedGroupInvalidatesStandardAndLinkAuthSnapshots(t *testing.T) {
	repo := &linkCardSecurityRepo{}
	cache := &authCacheStub{}
	authRepo := &authRepoStub{listKeysByGroupID: func(_ context.Context, groupID int64) ([]string, error) {
		require.Equal(t, int64(8), groupID)
		return []string{"sk-standard", "sk-card-link"}, nil
	}}
	apiKeys := NewAPIKeyService(authRepo, nil, nil, nil, nil, cache, &config.Config{
		APIKeyAuth: config.APIKeyAuthCacheConfig{L2TTLSeconds: 60},
	})
	svc := NewLinkCardService(repo, nil, apiKeys, nil, nil, &config.Config{})

	require.NoError(t, svc.RemoveAuthorizedGroup(context.Background(), 8))
	require.Equal(t, int64(8), repo.removedGroupID)
	require.Len(t, cache.deleteAuthKeys, 2)
}

func TestRemoveAuthorizedGroupDoesNotInvalidateCacheAfterRollback(t *testing.T) {
	repo := &linkCardSecurityRepo{removeGroupErr: ErrLinkCardGroupNotAuthorized}
	cache := &authCacheStub{}
	authRepo := &authRepoStub{listKeysByGroupID: func(context.Context, int64) ([]string, error) {
		return []string{"sk-card-link"}, nil
	}}
	apiKeys := NewAPIKeyService(authRepo, nil, nil, nil, nil, cache, &config.Config{
		APIKeyAuth: config.APIKeyAuthCacheConfig{L2TTLSeconds: 60},
	})
	svc := NewLinkCardService(repo, nil, apiKeys, nil, nil, &config.Config{})

	require.ErrorIs(t, svc.RemoveAuthorizedGroup(context.Background(), 8), ErrLinkCardGroupNotAuthorized)
	require.Empty(t, cache.deleteAuthKeys)
}

func TestLinkCardRechargeInvalidatesRestoredKeyAuthCache(t *testing.T) {
	cache := &authCacheStub{}
	apiKeys := NewAPIKeyService(&authRepoStub{}, nil, nil, nil, nil, cache, &config.Config{
		APIKeyAuth: config.APIKeyAuthCacheConfig{L2TTLSeconds: 60},
	})
	repo := &linkCardSecurityRepo{rechargeResult: &LinkCardMutationResult{Card: LinkCard{
		APIKeyID:      91,
		CreatorUserID: 1,
		Key:           "sk-card-restored-after-recharge",
		Status:        LinkCardStateActive,
	}}}
	svc := NewLinkCardService(repo, linkCardSecuritySettings(false, "[1]"), apiKeys, nil, nil, &config.Config{})

	result, err := svc.Recharge(context.Background(), 1, 91, decimal.NewFromInt(10), "restore-depleted-card", false)
	require.NoError(t, err)
	require.Equal(t, LinkCardStateActive, result.Card.Status)
	require.Len(t, cache.deleteAuthKeys, 1, "recharge must evict the cached depleted credential")
}

func linkCardSecuritySettings(enabled bool, developmentIDs string) *linkCardSecuritySettingRepo {
	return &linkCardSecuritySettingRepo{values: map[string]string{
		SettingKeyLinkCardsEnabled:            map[bool]string{true: "true", false: "false"}[enabled],
		SettingKeyLinkCardsDevelopmentMode:    "true",
		SettingKeyLinkCardsDevelopmentUserIDs: developmentIDs,
		SettingKeyLinkCardsMaxBatchSize:       "100",
	}}
}

func TestLinkCardGroupsIntersectNativeAccessAndApplyUserRate(t *testing.T) {
	nativeGroups := []Group{
		{ID: 8, Name: "public", Status: StatusActive, SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 0.08},
		{ID: 50, Name: "allowed-exclusive", Status: StatusActive, SubscriptionType: SubscriptionTypeStandard, IsExclusive: true, RateMultiplier: 0.05},
		{ID: 51, Name: "denied-exclusive", Status: StatusActive, SubscriptionType: SubscriptionTypeStandard, IsExclusive: true, RateMultiplier: 0.04},
	}
	repo := &linkCardSecurityRepo{groups: []LinkCardGroup{
		{GroupID: 8, Name: "public", Enabled: true, RateMultiplier: 0.08},
		{GroupID: 50, Name: "allowed-exclusive", Enabled: true, RateMultiplier: 0.05},
		{GroupID: 51, Name: "denied-exclusive", Enabled: true, RateMultiplier: 0.04},
		{GroupID: 99, Name: "not-native", Enabled: true, RateMultiplier: 0.01},
	}}
	apiKeys := newLinkCardGroupPolicyAPIKeyService(
		&User{ID: 1, AllowedGroups: []int64{50}},
		nativeGroups,
		map[int64]float64{8: 0.07},
	)
	svc := NewLinkCardService(repo, linkCardSecuritySettings(false, "[1]"), apiKeys, nil, nil, &config.Config{})

	groups, err := svc.ListGroups(context.Background(), 1, false)
	require.NoError(t, err)
	require.Len(t, groups, 2)
	require.Equal(t, int64(8), groups[0].GroupID)
	require.InDelta(t, 0.07, groups[0].RateMultiplier, 0.000001, "user rate must override the native default")
	require.Equal(t, int64(50), groups[1].GroupID)
	require.InDelta(t, 0.05, groups[1].RateMultiplier, 0.000001)
}

func TestLinkCardCreateUsesNativeExclusiveAccessAndEffectiveRate(t *testing.T) {
	ctx := context.Background()

	t.Run("exclusive group without native user grant is rejected", func(t *testing.T) {
		repo := &linkCardSecurityRepo{group: &LinkCardGroup{GroupID: 51, Enabled: true, RateMultiplier: 0.04}}
		apiKeys := newLinkCardGroupPolicyAPIKeyService(
			&User{ID: 1},
			[]Group{{ID: 51, Status: StatusActive, SubscriptionType: SubscriptionTypeStandard, IsExclusive: true, RateMultiplier: 0.04}},
			nil,
		)
		svc := NewLinkCardService(repo, linkCardSecuritySettings(false, "[1]"), apiKeys, nil, nil, &config.Config{})

		_, err := svc.Create(ctx, 1, CreateLinkCardsRequest{
			GroupID: 51, Quantity: 1, Amount: decimal.NewFromInt(10), IdempotencyKey: "exclusive-denied",
		})
		require.ErrorIs(t, err, ErrLinkCardGroupNotAuthorized)
		require.Zero(t, repo.createCalls)
	})

	t.Run("native user rate is passed to the atomic issuer", func(t *testing.T) {
		repo := &linkCardSecurityRepo{group: &LinkCardGroup{GroupID: 50, Enabled: true, RateMultiplier: 0.08}}
		apiKeys := newLinkCardGroupPolicyAPIKeyService(
			&User{ID: 1, AllowedGroups: []int64{50}},
			[]Group{{ID: 50, Status: StatusActive, SubscriptionType: SubscriptionTypeStandard, IsExclusive: true, RateMultiplier: 0.08}},
			map[int64]float64{50: 0.07},
		)
		svc := NewLinkCardService(repo, linkCardSecuritySettings(false, "[1]"), apiKeys, nil, nil, &config.Config{})

		_, err := svc.Create(ctx, 1, CreateLinkCardsRequest{
			GroupID: 50, Quantity: 1, Amount: decimal.NewFromInt(10), IdempotencyKey: "exclusive-allowed",
		})
		require.NoError(t, err)
		require.Equal(t, 1, repo.createCalls)
		require.InDelta(t, 0.07, repo.lastCreate.Group.RateMultiplier, 0.000001)
	})
}

func TestLinkCardAccessFailsClosedOutsideDevelopmentRollout(t *testing.T) {
	ctx := context.Background()
	repo := &linkCardSecurityRepo{group: &LinkCardGroup{
		GroupID:        8,
		Enabled:        true,
		RateMultiplier: 0.08,
	}}
	apiKeys := newLinkCardGroupPolicyAPIKeyService(
		&User{ID: 1},
		[]Group{{ID: 8, Status: StatusActive, SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 0.08}},
		nil,
	)
	svc := NewLinkCardService(repo, linkCardSecuritySettings(false, "[1]"), apiKeys, nil, nil, nil)

	preview, err := svc.Access(ctx, 1)
	require.NoError(t, err)
	require.True(t, preview.Allowed)
	require.False(t, preview.Enabled)

	denied, err := svc.Access(ctx, 2)
	require.NoError(t, err)
	require.False(t, denied.Allowed)

	_, err = svc.Create(ctx, 2, CreateLinkCardsRequest{
		GroupID:        8,
		Quantity:       1,
		Amount:         decimal.NewFromInt(10),
		IdempotencyKey: "denied-user",
	})
	require.ErrorIs(t, err, ErrLinkCardsDisabled)
	require.Zero(t, repo.createCalls, "a denied rollout user must not reach a financial repository")
}

func TestLinkCardCreateCanonicalizesMoneyBeforeAtomicBatch(t *testing.T) {
	ctx := context.Background()
	repo := &linkCardSecurityRepo{group: &LinkCardGroup{
		GroupID:        8,
		Enabled:        true,
		RateMultiplier: 0.08,
	}}
	apiKeys := newLinkCardGroupPolicyAPIKeyService(
		&User{ID: 1},
		[]Group{{ID: 8, Status: StatusActive, SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 0.08}},
		nil,
	)
	svc := NewLinkCardService(repo, linkCardSecuritySettings(false, "[1]"), apiKeys, nil, nil, nil)

	result, err := svc.Create(ctx, 1, CreateLinkCardsRequest{
		GroupID:        8,
		Quantity:       3,
		Amount:         decimal.RequireFromString("0.123456789"),
		IdempotencyKey: "fixed-point-batch",
	})
	require.NoError(t, err)
	require.Equal(t, 1, repo.createCalls)
	require.True(t, decimal.RequireFromString("0.12345679").Equal(repo.lastCreate.AmountPerCard))
	require.True(t, decimal.RequireFromString("0.37037037").Equal(repo.lastCreate.TotalDebit))
	require.True(t, result.TotalDebited.Equal(repo.lastCreate.TotalDebit))
	require.Len(t, repo.lastCreate.Keys, 3)
	require.NotEqual(t, repo.lastCreate.Keys[0], repo.lastCreate.Keys[1])
	require.NotEqual(t, repo.lastCreate.Keys[1], repo.lastCreate.Keys[2])
}

func TestLinkCardCreateFailsClosedWithoutNativeGroupPolicy(t *testing.T) {
	repo := &linkCardSecurityRepo{group: &LinkCardGroup{GroupID: 8, Enabled: true, RateMultiplier: 0.08}}
	svc := NewLinkCardService(repo, linkCardSecuritySettings(false, "[1]"), nil, nil, nil, nil)

	_, err := svc.Create(context.Background(), 1, CreateLinkCardsRequest{
		GroupID: 8, Quantity: 1, Amount: decimal.NewFromInt(10), IdempotencyKey: "missing-native-policy",
	})
	require.ErrorIs(t, err, ErrLinkCardGroupPolicyUnavailable)
	require.Zero(t, repo.createCalls)
}

func TestLinkCardFingerprintSeparatesStructuredFields(t *testing.T) {
	first := hashLinkCardFingerprint("create", int64(1), int64(1), 23, "4.00000000")
	second := hashLinkCardFingerprint("create", int64(1), int64(12), 3, "4.00000000")
	require.NotEqual(t, first, second, "adjacent request fields must not admit concatenation collisions")
}

func TestLinkCardFingerprintIsStableAcrossPointerInstances(t *testing.T) {
	firstConcurrency := 5
	secondConcurrency := 5

	first := hashLinkCardFingerprint("set_limits", int64(1), int64(9), &firstConcurrency, (*int)(nil), "")
	second := hashLinkCardFingerprint("set_limits", int64(1), int64(9), &secondConcurrency, (*int)(nil), "")
	require.Equal(t, first, second, "an idempotent retry must not depend on pointer addresses")
}

func TestLinkCardActivationResponseDoesNotLeakCreatorOrFundingInternals(t *testing.T) {
	privateCard := &LinkCard{
		APIKeyID:              91,
		CreatorUserID:         1,
		CreatorEmail:          "creator@example.test",
		Key:                   "sk-card-secret-value-that-must-not-be-returned",
		GroupID:               8,
		GroupName:             "authorized-group",
		IssueRateMultiplier:   0.08,
		Status:                LinkCardStateActive,
		OriginalDepositAmount: decimal.NewFromInt(100),
		TotalDepositAmount:    decimal.NewFromInt(100),
	}
	repo := &linkCardSecurityRepo{
		activateCard: privateCard,
		portalCard:   privateCard,
		usage: []LinkCardUsageLog{{
			ID:            101,
			APIKeyID:      91,
			CreatorUserID: 1,
			CreatorEmail:  "creator@example.test",
			RequestID:     "request-101",
		}},
	}
	apiKeys := newLinkCardSecurityAPIKeyService(&APIKey{
		ID:      privateCard.APIKeyID,
		UserID:  privateCard.CreatorUserID,
		KeyType: APIKeyTypeLink,
		User:    &User{ID: privateCard.CreatorUserID, Status: StatusActive},
	})
	svc := NewLinkCardService(
		repo,
		linkCardSecuritySettings(true, "[1]"),
		apiKeys,
		nil,
		nil,
		&config.Config{JWT: config.JWTConfig{Secret: "link-card-test-secret"}},
	)

	result, err := svc.Activate(context.Background(), repo.activateCard.Key)
	require.NoError(t, err)
	require.Equal(t, 1, repo.activationCalls)

	payload, err := json.Marshal(result)
	require.NoError(t, err)
	serialized := string(payload)
	for _, forbidden := range []string{
		"creator@example.test",
		"sk-card-secret-value-that-must-not-be-returned",
		`"api_key_id"`,
		`"id"`,
		`"creator_user_id"`,
		`"creator_email"`,
		`"issue_rate_multiplier"`,
		`"original_deposit_amount"`,
		`"total_deposit_amount"`,
		`"refundable_amount"`,
	} {
		require.Falsef(t, strings.Contains(serialized, forbidden), "public activation leaked %s: %s", forbidden, serialized)
	}

	profile, err := svc.PortalCard(context.Background(), result.SessionToken)
	require.NoError(t, err)
	profilePayload, err := json.Marshal(profile)
	require.NoError(t, err)
	require.NotContains(t, string(profilePayload), "creator@example.test")
	require.NotContains(t, string(profilePayload), `"creator_user_id"`)
	require.NotContains(t, string(profilePayload), `"issue_rate_multiplier"`)
	require.NotContains(t, string(profilePayload), `"id"`)

	usage, _, err := svc.PortalUsage(
		context.Background(),
		result.SessionToken,
		pagination.PaginationParams{Page: 1, PageSize: 10},
		LinkCardUsageFilters{
			CreatorUserID: func() *int64 { value := int64(999); return &value }(),
			CreatorEmail:  "victim@example.test",
			Key:           "sk-card-probe",
			RequestID:     "request-probe",
			Model:         "model-probe",
			GroupID:       func() *int64 { value := int64(999); return &value }(),
			RequestType:   "stream",
		},
	)
	require.NoError(t, err)
	require.Nil(t, repo.lastUsageOwner)
	require.NotNil(t, repo.lastUsageFilter.CardID)
	require.Equal(t, privateCard.APIKeyID, *repo.lastUsageFilter.CardID)
	require.Nil(t, repo.lastUsageFilter.CreatorUserID)
	require.Empty(t, repo.lastUsageFilter.CreatorEmail)
	require.Empty(t, repo.lastUsageFilter.Key)
	require.Empty(t, repo.lastUsageFilter.RequestID)
	require.Empty(t, repo.lastUsageFilter.Model)
	require.Nil(t, repo.lastUsageFilter.GroupID)
	require.Empty(t, repo.lastUsageFilter.RequestType)
	usagePayload, err := json.Marshal(usage)
	require.NoError(t, err)
	publicUsageJSON := string(usagePayload)
	require.NotContains(t, publicUsageJSON, "creator@example.test")
	for _, internalField := range []string{`"id"`, `"link_card_id"`, `"api_key_id"`, `"creator_user_id"`, `"creator_email"`, `"key_prefix"`, `"masked_key"`, `"group_id"`} {
		require.NotContains(t, publicUsageJSON, internalField)
	}
}

func TestLinkCardActivationDoesNotMutateCardBeforeRolloutAuthorization(t *testing.T) {
	repo := &linkCardSecurityRepo{activateCard: &LinkCard{
		APIKeyID:      92,
		CreatorUserID: 2,
		Key:           "sk-card-denied-rollout-00000000000000000002",
		Status:        LinkCardStateActive,
	}}
	apiKeys := newLinkCardSecurityAPIKeyService(&APIKey{
		ID:      repo.activateCard.APIKeyID,
		UserID:  repo.activateCard.CreatorUserID,
		KeyType: APIKeyTypeLink,
		User:    &User{ID: repo.activateCard.CreatorUserID, Status: StatusActive},
	})
	svc := NewLinkCardService(
		repo,
		linkCardSecuritySettings(false, "[1]"),
		apiKeys,
		nil,
		nil,
		&config.Config{JWT: config.JWTConfig{Secret: "link-card-test-secret"}},
	)

	_, err := svc.Activate(context.Background(), repo.activateCard.Key)
	require.ErrorIs(t, err, ErrLinkCardsDisabled)
	require.Zero(t, repo.activationCalls, "rollout authorization must precede the activation state transition")
}

func TestLinkCardActivationDoesNotMutateCardWithoutSessionSigningKey(t *testing.T) {
	repo := &linkCardSecurityRepo{activateCard: &LinkCard{
		APIKeyID:      93,
		CreatorUserID: 1,
		Key:           "sk-card-missing-signing-key-000000000000000001",
		Status:        LinkCardStateActive,
	}}
	apiKeys := newLinkCardSecurityAPIKeyService(&APIKey{
		ID: repo.activateCard.APIKeyID, UserID: 1, KeyType: APIKeyTypeLink,
		User: &User{ID: 1, Status: StatusActive},
	})
	svc := NewLinkCardService(repo, linkCardSecuritySettings(false, "[1]"), apiKeys, nil, nil, &config.Config{})

	_, err := svc.Activate(context.Background(), repo.activateCard.Key)
	require.Error(t, err)
	require.Zero(t, repo.activationCalls, "session prerequisites must be validated before activation")
}

func TestLinkCardSettingsRejectUnsafeURLsAndCanonicalizeValidURLs(t *testing.T) {
	settings := linkCardSecuritySettings(false, "[1]")
	svc := NewLinkCardService(&linkCardSecurityRepo{}, settings, nil, nil, nil, nil)

	unsafePortal := "https://user:secret@key.example.test/card"
	_, err := svc.UpdateSettings(context.Background(), UpdateLinkCardSettingsRequest{PublicPortalURL: &unsafePortal})
	require.Error(t, err)
	require.Empty(t, settings.values[SettingKeyLinkCardsPublicPortalURL])

	unsafeAPI := "https://api.example.test/v1?token=secret"
	_, err = svc.UpdateSettings(context.Background(), UpdateLinkCardSettingsRequest{APIBaseURL: &unsafeAPI})
	require.Error(t, err)
	require.Empty(t, settings.values[SettingKeyLinkCardsAPIBaseURL])
	missingV1 := "https://api.example.test"
	_, err = svc.UpdateSettings(context.Background(), UpdateLinkCardSettingsRequest{APIBaseURL: &missingV1})
	require.Error(t, err)
	require.Empty(t, settings.values[SettingKeyLinkCardsAPIBaseURL])

	portal := "https://key.52token.org/"
	apiBase := "https://api.52token.org/v1/"
	updated, err := svc.UpdateSettings(context.Background(), UpdateLinkCardSettingsRequest{
		PublicPortalURL: &portal,
		APIBaseURL:      &apiBase,
	})
	require.NoError(t, err)
	require.Equal(t, "https://key.52token.org", updated.PublicPortalURL)
	require.Equal(t, "https://api.52token.org/v1", updated.APIBaseURL)
}
