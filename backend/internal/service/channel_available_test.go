//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

// stubGroupRepoForAvailable 是 ListAvailable 测试用的 GroupRepository stub，
// 仅实现 ListActive；其他方法对本测试无关，返回零值即可。
// listActiveErr 非 nil 时，ListActive 返回该错误用于错误传播测试。
// listActiveCalls 记录调用次数，用于断言「失败短路时不再访问 groupRepo」等行为。
type stubGroupRepoForAvailable struct {
	activeGroups    []Group
	listActiveErr   error
	listActiveCalls int
}

func (s *stubGroupRepoForAvailable) ListActive(ctx context.Context) ([]Group, error) {
	s.listActiveCalls++
	if s.listActiveErr != nil {
		return nil, s.listActiveErr
	}
	return s.activeGroups, nil
}

func (s *stubGroupRepoForAvailable) Create(ctx context.Context, group *Group) error { return nil }
func (s *stubGroupRepoForAvailable) GetByID(ctx context.Context, id int64) (*Group, error) {
	return nil, nil
}
func (s *stubGroupRepoForAvailable) GetByIDLite(ctx context.Context, id int64) (*Group, error) {
	return nil, nil
}
func (s *stubGroupRepoForAvailable) Update(ctx context.Context, group *Group) error { return nil }
func (s *stubGroupRepoForAvailable) Delete(ctx context.Context, id int64) error     { return nil }
func (s *stubGroupRepoForAvailable) DeleteCascade(ctx context.Context, id int64) ([]int64, error) {
	return nil, nil
}
func (s *stubGroupRepoForAvailable) List(ctx context.Context, params pagination.PaginationParams) ([]Group, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (s *stubGroupRepoForAvailable) ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, status, search string, isExclusive *bool) ([]Group, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (s *stubGroupRepoForAvailable) ListActiveByPlatform(ctx context.Context, platform string) ([]Group, error) {
	return nil, nil
}
func (s *stubGroupRepoForAvailable) ExistsByName(ctx context.Context, name string) (bool, error) {
	return false, nil
}
func (s *stubGroupRepoForAvailable) GetAccountCount(ctx context.Context, groupID int64) (int64, int64, error) {
	return 0, 0, nil
}
func (s *stubGroupRepoForAvailable) DeleteAccountGroupsByGroupID(ctx context.Context, groupID int64) (int64, error) {
	return 0, nil
}
func (s *stubGroupRepoForAvailable) GetAccountIDsByGroupIDs(ctx context.Context, groupIDs []int64) ([]int64, error) {
	return nil, nil
}
func (s *stubGroupRepoForAvailable) BindAccountsToGroup(ctx context.Context, groupID int64, accountIDs []int64) error {
	return nil
}
func (s *stubGroupRepoForAvailable) UpdateSortOrders(ctx context.Context, updates []GroupSortOrderUpdate) error {
	return nil
}

// newAvailableChannelService 构造一个 ChannelService，channelRepo.ListAll 返回给定 channels，
// groupRepo 由参数决定。传入空 stub 表示「活跃分组列表为空」。
func newAvailableChannelService(channels []Channel, groupRepo GroupRepository) *ChannelService {
	repo := &mockChannelRepository{
		listAllFn: func(ctx context.Context) ([]Channel, error) { return channels, nil },
	}
	return NewChannelService(repo, groupRepo, nil, nil)
}

// stubAccountRepoForAvailable 只提供用户可用渠道聚合所需的可调度账号查询。
type stubAccountRepoForAvailable struct {
	accountsByGroup map[int64][]Account
	errByGroup      map[int64]error
	calls           []int64
}

func (s *stubAccountRepoForAvailable) ListSchedulableByGroupID(_ context.Context, groupID int64) ([]Account, error) {
	s.calls = append(s.calls, groupID)
	if err := s.errByGroup[groupID]; err != nil {
		return nil, err
	}
	return s.accountsByGroup[groupID], nil
}

func schedulableAvailableAccount(platform string, mapping map[string]any) Account {
	return Account{
		Platform:    platform,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{"model_mapping": mapping},
	}
}

func availableModelNames(models []SupportedModel) []string {
	out := make([]string, 0, len(models))
	for _, model := range models {
		out = append(out, model.Name)
	}
	return out
}

func TestListAvailable_EmptyActiveGroups_NoGroupsAttached(t *testing.T) {
	// 活跃分组列表为空时，渠道的 Groups 应为空切片，不报错。
	channels := []Channel{{
		ID:       1,
		Name:     "chA",
		Status:   StatusActive,
		GroupIDs: []int64{10, 20},
	}}
	svc := newAvailableChannelService(channels, &stubGroupRepoForAvailable{})
	out, err := svc.ListAvailable(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Empty(t, out[0].Groups)
}

func TestListAvailable_InactiveGroupIDSilentlyDropped(t *testing.T) {
	// 渠道 GroupIDs 中引用的 group 未出现在 ListActive 结果中（已停用或删除），应被静默丢弃。
	channels := []Channel{{
		ID:       1,
		Name:     "chA",
		Status:   StatusActive,
		GroupIDs: []int64{1, 99},
	}}
	groupRepo := &stubGroupRepoForAvailable{
		activeGroups: []Group{{ID: 1, Name: "g1", Platform: "anthropic"}},
	}
	svc := newAvailableChannelService(channels, groupRepo)
	out, err := svc.ListAvailable(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Len(t, out[0].Groups, 1)
	require.Equal(t, int64(1), out[0].Groups[0].ID)
}

func TestListAvailable_SortedByName(t *testing.T) {
	channels := []Channel{
		{ID: 1, Name: "beta"},
		{ID: 2, Name: "Alpha"},
		{ID: 3, Name: "charlie"},
	}
	svc := newAvailableChannelService(channels, &stubGroupRepoForAvailable{})
	out, err := svc.ListAvailable(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 3)
	require.Equal(t, "Alpha", out[0].Name)
	require.Equal(t, "beta", out[1].Name)
	require.Equal(t, "charlie", out[2].Name)
}

func TestListAvailable_ListAllErrorPropagates(t *testing.T) {
	// ListAll 返回错误时 ListAvailable 应直接返回包装后的错误，且不再访问 groupRepo（短路）。
	sentinel := errors.New("list-all-boom")
	repo := &mockChannelRepository{
		listAllFn: func(ctx context.Context) ([]Channel, error) { return nil, sentinel },
	}
	groupRepo := &stubGroupRepoForAvailable{}
	svc := NewChannelService(repo, groupRepo, nil, nil)
	out, err := svc.ListAvailable(context.Background())
	require.Nil(t, out)
	require.ErrorIs(t, err, sentinel)
	require.Contains(t, err.Error(), "list channels", "wrap 前缀缺失，可能 %w 被改为 %v")
	require.Equal(t, 0, groupRepo.listActiveCalls, "ListAll 失败后不应再调用 groupRepo.ListActive")
}

func TestListAvailable_ListActiveErrorPropagates(t *testing.T) {
	// groupRepo.ListActive 返回错误时 ListAvailable 应直接返回包装后的错误。
	sentinel := errors.New("list-active-boom")
	svc := newAvailableChannelService(
		[]Channel{{ID: 1, Name: "chA"}},
		&stubGroupRepoForAvailable{listActiveErr: sentinel},
	)
	out, err := svc.ListAvailable(context.Background())
	require.Nil(t, out)
	require.ErrorIs(t, err, sentinel)
	require.Contains(t, err.Error(), "list active groups", "wrap 前缀缺失，可能 %w 被改为 %v")
}

func TestListAvailable_DefaultsEmptyBillingModelSource(t *testing.T) {
	// 渠道 BillingModelSource 为空时应回填为 BillingModelSourceChannelMapped，
	// 显式值应原样保留（由 service 层统一处理，避免各 handler 重复默认逻辑）。
	channels := []Channel{
		{ID: 1, Name: "empty", BillingModelSource: ""},
		{ID: 2, Name: "explicit", BillingModelSource: BillingModelSourceUpstream},
	}
	svc := newAvailableChannelService(channels, &stubGroupRepoForAvailable{})
	out, err := svc.ListAvailable(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 2)

	// 按 Name 查找，避免依赖排序副作用。
	byName := make(map[string]string, len(out))
	for _, ch := range out {
		byName[ch.Name] = ch.BillingModelSource
	}
	require.Equal(t, BillingModelSourceChannelMapped, byName["empty"])
	require.Equal(t, BillingModelSourceUpstream, byName["explicit"])
}

func TestPricingNeedsFallback(t *testing.T) {
	tests := []struct {
		name string
		in   *ChannelModelPricing
		want bool
	}{
		{"nil", nil, true},
		{"empty struct", &ChannelModelPricing{BillingMode: BillingModeToken}, true},
		{"all-empty intervals", &ChannelModelPricing{
			BillingMode: BillingModeImage,
			Intervals:   []PricingInterval{{TierLabel: "1K"}, {TierLabel: "2K"}},
		}, true},
		{"flat input set", &ChannelModelPricing{InputPrice: testPtrFloat64(3e-6)}, false},
		{"flat per_request set", &ChannelModelPricing{PerRequestPrice: testPtrFloat64(0.04)}, false},
		{"interval with price", &ChannelModelPricing{
			Intervals: []PricingInterval{{TierLabel: "1K", PerRequestPrice: testPtrFloat64(0.04)}},
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, pricingNeedsFallback(tt.in))
		})
	}
}

func TestSynthesizePricingFromLiteLLM_TokenMode(t *testing.T) {
	lp := &LiteLLMModelPricing{
		Mode:                        "chat",
		InputCostPerToken:           3e-6,
		OutputCostPerToken:          1.5e-5,
		CacheCreationInputTokenCost: 3.75e-6,
		CacheReadInputTokenCost:     3e-7,
	}
	got := synthesizePricingFromLiteLLM(lp, nil)
	require.NotNil(t, got)
	require.Equal(t, BillingModeToken, got.BillingMode)
	require.NotNil(t, got.InputPrice)
	require.InDelta(t, 3e-6, *got.InputPrice, 1e-12)
	require.NotNil(t, got.CacheReadPrice)
}

func TestSynthesizePricingFromLiteLLM_ImageGenerationMode(t *testing.T) {
	// LiteLLM mode=image_generation 且渠道未声明模式时，按 image 合成。
	lp := &LiteLLMModelPricing{
		Mode:                    "image_generation",
		OutputCostPerImageToken: 4e-5,
	}
	got := synthesizePricingFromLiteLLM(lp, nil)
	require.NotNil(t, got)
	require.Equal(t, BillingModeImage, got.BillingMode)
	require.Nil(t, got.PerRequestPrice)
	require.NotNil(t, got.ImageOutputPrice)
}

func TestSynthesizePricingFromLiteLLM_RespectsExistingChannelMode(t *testing.T) {
	// admin UI 选了 per_request 但没填价：LiteLLM 数据按 per_request 合成,
	// 即便 LiteLLM 标的是 chat 模式也尊重渠道选择。
	lp := &LiteLLMModelPricing{
		Mode:               "chat",
		InputCostPerToken:  5e-6,
		OutputCostPerImage: 0.04,
	}
	existing := &ChannelModelPricing{BillingMode: BillingModePerRequest}
	got := synthesizePricingFromLiteLLM(lp, existing)
	require.NotNil(t, got)
	require.Equal(t, BillingModePerRequest, got.BillingMode)
	require.NotNil(t, got.PerRequestPrice)
	require.InDelta(t, 0.04, *got.PerRequestPrice, 1e-12)
}

func TestFillGlobalPricingFallback_NilPricing(t *testing.T) {
	pricingSvc := newStubPricingServiceFromMap(map[string]*LiteLLMModelPricing{
		"claude-opus-4-5": {Mode: "chat", InputCostPerToken: 5e-6},
	})
	svc := &ChannelService{pricingService: pricingSvc}

	models := []SupportedModel{
		{Name: "claude-opus-4-5", Platform: "anthropic"},
	}
	svc.fillGlobalPricingFallback(models)
	require.NotNil(t, models[0].Pricing)
	require.NotNil(t, models[0].Pricing.InputPrice)
	require.InDelta(t, 5e-6, *models[0].Pricing.InputPrice, 1e-12)
}

func TestFillGlobalPricingFallback_EmptyPricingFillsFromLiteLLM(t *testing.T) {
	// 核心场景：admin UI 建了 pricing 条目（image 模式）但没填价，应走 LiteLLM 兜底。
	pricingSvc := newStubPricingServiceFromMap(map[string]*LiteLLMModelPricing{
		"gpt-image-1": {
			Mode:                    "image_generation",
			OutputCostPerImageToken: 4e-5,
		},
	})
	svc := &ChannelService{pricingService: pricingSvc}

	models := []SupportedModel{
		{
			Name:     "gpt-image-1",
			Platform: "openai",
			Pricing: &ChannelModelPricing{
				BillingMode: BillingModeImage,
				Intervals:   []PricingInterval{{TierLabel: "1K"}, {TierLabel: "2K"}},
			},
		},
	}
	svc.fillGlobalPricingFallback(models)
	require.NotNil(t, models[0].Pricing)
	require.Equal(t, BillingModeImage, models[0].Pricing.BillingMode)
	require.NotNil(t, models[0].Pricing.ImageOutputPrice)
	require.InDelta(t, 4e-5, *models[0].Pricing.ImageOutputPrice, 1e-12)
}

func TestFillGlobalPricingFallback_KeepsExistingPrice(t *testing.T) {
	// 渠道已经填了价格的条目不应被回落覆盖。
	pricingSvc := newStubPricingServiceFromMap(map[string]*LiteLLMModelPricing{
		"served-model": {Mode: "chat", InputCostPerToken: 1e-6},
	})
	svc := &ChannelService{pricingService: pricingSvc}

	existing := &ChannelModelPricing{
		BillingMode: BillingModeToken,
		InputPrice:  testPtrFloat64(9e-9),
	}
	models := []SupportedModel{
		{Name: "served-model", Platform: "anthropic", Pricing: existing},
	}
	svc.fillGlobalPricingFallback(models)
	require.Same(t, existing, models[0].Pricing)
}

func newStubPricingServiceFromMap(data map[string]*LiteLLMModelPricing) *PricingService {
	return &PricingService{pricingData: data}
}

func TestListAvailable_MergesAccountAliasesWithoutExposingTargets(t *testing.T) {
	publicVideoModels := map[string]any{
		"seedance-2.0":           "private/seedance-v2",
		"seedance-2.0-fast-480p": "private/seedance-fast-480p",
		"seedance-2.0-fast-720p": "private/seedance-fast-720p",
		"sora-2":                 "private/sora-v2",
		"sora-2-pro":             "private/sora-v2-pro",
		"omni-v1":                "private/omni-v1",
		"omni-v2v":               "private/omni-v2v",
		"grok-video":             "private/grok-video",
		"veo-3-fast":             "private/veo-3-fast",
		"video-*":                "private/wildcard-key",
		"wildcard-target":        "private/video-*",
		"":                       "private/empty",
	}
	groupRepo := &stubGroupRepoForAvailable{activeGroups: []Group{
		{ID: 1, Name: "video-public", Platform: "openai"},
		{ID: 2, Name: "video-exclusive", Platform: "openai"},
	}}
	accountRepo := &stubAccountRepoForAvailable{accountsByGroup: map[int64][]Account{
		1: {
			schedulableAvailableAccount("openai", publicVideoModels),
			{Platform: "openai", Status: StatusDisabled, Schedulable: true, Credentials: map[string]any{"model_mapping": map[string]any{"disabled-model": "private/disabled"}}},
			schedulableAvailableAccount("anthropic", map[string]any{"wrong-platform": "private/wrong"}),
		},
		2: {schedulableAvailableAccount("openai", map[string]any{"exclusive-only-video": "private/exclusive"})},
	}}
	channels := []Channel{{
		ID:       10,
		Name:     "video-channel",
		Status:   StatusActive,
		GroupIDs: []int64{1, 2},
		ModelPricing: []ChannelModelPricing{{
			Platform: "openai",
			Models:   []string{"channel-base-video"},
		}},
	}}
	svc := newAvailableChannelService(channels, groupRepo)
	svc.availableAccountRepo = accountRepo

	out, err := svc.ListAvailable(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Equal(t, []int64{1, 2}, accountRepo.calls)

	publicNames := availableModelNames(out[0].SupportedModelsByGroup[1])
	require.Len(t, publicNames, 10, "1 个渠道模型 + 9 个账号公开视频别名应全部可见")
	for _, expected := range []string{
		"channel-base-video",
		"seedance-2.0",
		"seedance-2.0-fast-480p",
		"seedance-2.0-fast-720p",
		"sora-2",
		"sora-2-pro",
		"omni-v1",
		"omni-v2v",
		"grok-video",
		"veo-3-fast",
	} {
		require.Contains(t, publicNames, expected)
	}
	for _, forbidden := range []string{
		"private/seedance-v2",
		"private/grok-video",
		"video-*",
		"wildcard-target",
		"disabled-model",
		"wrong-platform",
	} {
		require.NotContains(t, publicNames, forbidden)
	}

	exclusiveNames := availableModelNames(out[0].SupportedModelsByGroup[2])
	require.ElementsMatch(t, []string{"channel-base-video", "exclusive-only-video"}, exclusiveNames)
}

func TestListAvailable_MergesStringMapAccountAliases(t *testing.T) {
	groupRepo := &stubGroupRepoForAvailable{activeGroups: []Group{{ID: 1, Name: "video", Platform: "openai"}}}
	accountRepo := &stubAccountRepoForAvailable{accountsByGroup: map[int64][]Account{
		1: {{
			Platform:    "openai",
			Status:      StatusActive,
			Schedulable: true,
			Credentials: map[string]any{
				"model_mapping": map[string]string{
					"grok-video":              "private/grok-video",
					"seedance-2.0-fast-1080p": "private/seedance-1080p",
				},
			},
		}},
	}}
	svc := newAvailableChannelService([]Channel{{
		ID:       10,
		Name:     "video-channel",
		Status:   StatusActive,
		GroupIDs: []int64{1},
	}}, groupRepo)
	svc.availableAccountRepo = accountRepo

	out, err := svc.ListAvailable(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.ElementsMatch(t, []string{"grok-video", "seedance-2.0-fast-1080p"}, availableModelNames(out[0].SupportedModelsByGroup[1]))
}

func TestListAvailable_RestrictModelsUsesConfiguredBillingSource(t *testing.T) {
	tests := []struct {
		name               string
		billingModelSource string
		channelMapping     map[string]map[string]string
		pricedModel        string
	}{
		{name: "requested", billingModelSource: BillingModelSourceRequested, pricedModel: "public-video"},
		{name: "upstream", billingModelSource: BillingModelSourceUpstream, pricedModel: "private/upstream-video"},
		{
			name:               "channel mapped",
			billingModelSource: BillingModelSourceChannelMapped,
			channelMapping:     map[string]map[string]string{"openai": {"public-video": "channel-billing-video"}},
			pricedModel:        "channel-billing-video",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			groupRepo := &stubGroupRepoForAvailable{activeGroups: []Group{{ID: 1, Name: "video", Platform: "openai"}}}
			accountRepo := &stubAccountRepoForAvailable{accountsByGroup: map[int64][]Account{
				1: {schedulableAvailableAccount("openai", map[string]any{
					"public-video": "private/upstream-video",
					"hidden-video": "private/hidden-video",
				})},
			}}
			channels := []Channel{{
				ID:                 1,
				Name:               "restricted",
				Status:             StatusActive,
				GroupIDs:           []int64{1},
				RestrictModels:     true,
				BillingModelSource: tt.billingModelSource,
				ModelMapping:       tt.channelMapping,
				ModelPricing: []ChannelModelPricing{{
					Platform:    "openai",
					Models:      []string{tt.pricedModel},
					BillingMode: BillingModeVideo,
				}},
			}}
			svc := newAvailableChannelService(channels, groupRepo)
			svc.availableAccountRepo = accountRepo

			out, err := svc.ListAvailable(context.Background())
			require.NoError(t, err)
			require.Len(t, out, 1)
			names := availableModelNames(out[0].SupportedModelsByGroup[1])
			require.Contains(t, names, "public-video")
			require.NotContains(t, names, "hidden-video")
			for _, model := range out[0].SupportedModelsByGroup[1] {
				if model.Name == "public-video" {
					require.NotNil(t, model.Pricing)
					require.Equal(t, BillingModeVideo, model.Pricing.BillingMode)
				}
			}
		})
	}
}

func TestListAvailable_AccountRepositoryErrorPropagates(t *testing.T) {
	sentinel := errors.New("account-list-boom")
	groupRepo := &stubGroupRepoForAvailable{activeGroups: []Group{{ID: 7, Name: "video", Platform: "openai"}}}
	svc := newAvailableChannelService([]Channel{{ID: 1, Name: "ch", GroupIDs: []int64{7}}}, groupRepo)
	svc.availableAccountRepo = &stubAccountRepoForAvailable{errByGroup: map[int64]error{7: sentinel}}

	out, err := svc.ListAvailable(context.Background())
	require.Nil(t, out)
	require.ErrorIs(t, err, sentinel)
	require.Contains(t, err.Error(), "list schedulable accounts for group 7")
}
