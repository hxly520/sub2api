package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// availableChannelAccountRepository 是「可用渠道」模型聚合所需的最小账号仓库接口。
// 生产环境由 AccountRepository 注入，测试可使用只实现该方法的窄 stub。
type availableChannelAccountRepository interface {
	ListSchedulableByGroupID(ctx context.Context, groupID int64) ([]Account, error)
}

// availableAccountModel 只在服务内部保留账号映射目标，用于判断限制与匹配定价。
// 对外 DTO 只使用 Name，绝不能序列化 UpstreamModel。
type availableAccountModel struct {
	Name          string
	UpstreamModel string
	Platform      string
}

// AvailableGroupRef 渠道视图中关联分组的简要信息。
//
// 用户侧「可用渠道」页面据此展示：专属分组 vs 公开分组（IsExclusive）、
// 订阅 vs 标准（SubscriptionType）、默认倍率（RateMultiplier）与高峰倍率规则。
// 用户专属倍率不在这里暴露，前端自己通过 /groups/rates 拉取，和 API 密钥页面保持一致。
type AvailableGroupRef struct {
	ID                 int64
	Name               string
	Platform           string
	SubscriptionType   string
	RateMultiplier     float64
	PeakRateEnabled    bool
	PeakStart          string
	PeakEnd            string
	PeakRateMultiplier float64
	IsExclusive        bool
}

// AvailableChannel 可用渠道视图：用于「可用渠道」页面展示渠道基础信息 +
// 关联的分组 + 推导出的支持模型列表（无通配符）。
type AvailableChannel struct {
	ID                     int64
	Name                   string
	Description            string
	Status                 string
	BillingModelSource     string
	RestrictModels         bool
	Groups                 []AvailableGroupRef
	SupportedModels        []SupportedModel
	SupportedModelsByGroup map[int64][]SupportedModel
}

// ListAvailable 返回所有渠道的可用视图：每个渠道附带关联分组信息与支持模型列表。
//
// 模型来源为：
//  1. 渠道级 mapping ∪ pricing；
//  2. 渠道所绑定活跃分组下，实际可调度账号 model_mapping 的公开 key。
//
// 账号 mapping 的 value 仅在服务内部用于判断真实计费模型和匹配定价，绝不进入用户 DTO。
// 模型同时按分组保存，避免用户只能访问同平台的部分分组时看到其他分组独有的模型。
// 对于渠道未配置定价的模型，进一步用 PricingService 的全局 LiteLLM 数据合成
// 一份展示用定价，让用户看到默认价格而非「未配置」。
//
// 关联分组信息通过 groupRepo.ListActive 查询后按 ID 映射；渠道 GroupIDs 中未在活跃列表中
// 的分组（已停用或删除）会被忽略。
//
// 前置条件：s.groupRepo 必须非 nil（由 wire DI 保证）。直接 nil-deref 用于 fail-fast，
// 避免静默掩盖注入缺失。availableAccountRepo 可为 nil，直接构造服务的测试场景将保持旧行为。
func (s *ChannelService) ListAvailable(ctx context.Context) ([]AvailableChannel, error) {
	channels, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}

	groups, err := s.groupRepo.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active groups: %w", err)
	}
	groupByID := make(map[int64]AvailableGroupRef, len(groups))
	for i := range groups {
		g := groups[i]
		groupByID[g.ID] = AvailableGroupRef{
			ID:                 g.ID,
			Name:               g.Name,
			Platform:           g.Platform,
			SubscriptionType:   g.SubscriptionType,
			RateMultiplier:     g.RateMultiplier,
			PeakRateEnabled:    g.PeakRateEnabled,
			PeakStart:          g.PeakStart,
			PeakEnd:            g.PeakEnd,
			PeakRateMultiplier: g.PeakRateMultiplier,
			IsExclusive:        g.IsExclusive,
		}
	}

	accountModelsByGroup, err := s.listAvailableAccountModels(ctx, channels, groupByID)
	if err != nil {
		return nil, err
	}

	out := make([]AvailableChannel, 0, len(channels))
	for i := range channels {
		ch := &channels[i]
		attachedGroups := make([]AvailableGroupRef, 0, len(ch.GroupIDs))
		for _, gid := range ch.GroupIDs {
			if ref, ok := groupByID[gid]; ok {
				attachedGroups = append(attachedGroups, ref)
			}
		}
		sort.SliceStable(attachedGroups, func(i, j int) bool {
			return strings.ToLower(attachedGroups[i].Name) < strings.ToLower(attachedGroups[j].Name)
		})

		ch.normalizeBillingModelSource()
		channelModels := ch.SupportedModels()
		supportedByGroup := make(map[int64][]SupportedModel, len(attachedGroups))
		allSupported := append([]SupportedModel(nil), channelModels...)

		for _, group := range attachedGroups {
			groupModels := supportedModelsForPlatform(channelModels, group.Platform)
			groupModels = mergeAvailableAccountModels(ch, groupModels, accountModelsByGroup[group.ID])
			s.fillGlobalPricingFallback(groupModels)
			supportedByGroup[group.ID] = groupModels
			allSupported = mergeSupportedModels(allSupported, groupModels)
		}
		s.fillGlobalPricingFallback(allSupported)

		out = append(out, AvailableChannel{
			ID:                     ch.ID,
			Name:                   ch.Name,
			Description:            ch.Description,
			Status:                 ch.Status,
			BillingModelSource:     ch.BillingModelSource,
			RestrictModels:         ch.RestrictModels,
			Groups:                 attachedGroups,
			SupportedModels:        allSupported,
			SupportedModelsByGroup: supportedByGroup,
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

func (s *ChannelService) listAvailableAccountModels(
	ctx context.Context,
	channels []Channel,
	activeGroups map[int64]AvailableGroupRef,
) (map[int64][]availableAccountModel, error) {
	if s.availableAccountRepo == nil {
		return nil, nil
	}

	groupIDs := make([]int64, 0)
	seenGroups := make(map[int64]struct{})
	for i := range channels {
		for _, groupID := range channels[i].GroupIDs {
			if _, active := activeGroups[groupID]; !active {
				continue
			}
			if _, exists := seenGroups[groupID]; exists {
				continue
			}
			seenGroups[groupID] = struct{}{}
			groupIDs = append(groupIDs, groupID)
		}
	}
	sort.Slice(groupIDs, func(i, j int) bool { return groupIDs[i] < groupIDs[j] })

	result := make(map[int64][]availableAccountModel, len(groupIDs))
	for _, groupID := range groupIDs {
		group := activeGroups[groupID]
		accounts, err := s.availableAccountRepo.ListSchedulableByGroupID(ctx, groupID)
		if err != nil {
			return nil, fmt.Errorf("list schedulable accounts for group %d: %w", groupID, err)
		}

		models := make([]availableAccountModel, 0)
		seenModels := make(map[string]struct{})
		for i := range accounts {
			account := &accounts[i]
			if !account.IsSchedulable() || !strings.EqualFold(strings.TrimSpace(account.Platform), strings.TrimSpace(group.Platform)) {
				continue
			}

			mapping := account.GetModelMapping()
			publicNames := make([]string, 0, len(mapping))
			for publicName := range mapping {
				publicNames = append(publicNames, publicName)
			}
			sort.SliceStable(publicNames, func(i, j int) bool {
				return strings.ToLower(publicNames[i]) < strings.ToLower(publicNames[j])
			})

			for _, rawPublicName := range publicNames {
				publicName := strings.TrimSpace(rawPublicName)
				upstreamName := strings.TrimSpace(mapping[rawPublicName])
				if !isConcreteAvailableModel(publicName) || !isConcreteAvailableModel(upstreamName) {
					continue
				}
				key := strings.ToLower(publicName)
				if _, exists := seenModels[key]; exists {
					continue
				}
				seenModels[key] = struct{}{}
				models = append(models, availableAccountModel{
					Name:          publicName,
					UpstreamModel: upstreamName,
					Platform:      group.Platform,
				})
			}
		}
		sort.SliceStable(models, func(i, j int) bool {
			return strings.ToLower(models[i].Name) < strings.ToLower(models[j].Name)
		})
		result[groupID] = models
	}
	return result, nil
}

func isConcreteAvailableModel(model string) bool {
	model = strings.TrimSpace(model)
	return model != "" && !strings.Contains(model, wildcardSuffix)
}

func supportedModelsForPlatform(models []SupportedModel, platform string) []SupportedModel {
	out := make([]SupportedModel, 0, len(models))
	for i := range models {
		if models[i].Platform == platform {
			out = append(out, models[i])
		}
	}
	return out
}

func mergeAvailableAccountModels(
	ch *Channel,
	base []SupportedModel,
	accountModels []availableAccountModel,
) []SupportedModel {
	out := append([]SupportedModel(nil), base...)
	seen := make(map[string]struct{}, len(out)+len(accountModels))
	for i := range out {
		seen[supportedModelDedupKey(out[i].Platform, out[i].Name)] = struct{}{}
	}

	for _, model := range accountModels {
		pricing := availableAccountModelPricing(ch, model)
		if ch.RestrictModels && pricing == nil {
			continue
		}
		key := supportedModelDedupKey(model.Platform, model.Name)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, SupportedModel{
			Name:     model.Name,
			Platform: model.Platform,
			Pricing:  pricing,
		})
	}

	sortSupportedModels(out)
	return out
}

func mergeSupportedModels(base, additions []SupportedModel) []SupportedModel {
	out := append([]SupportedModel(nil), base...)
	seen := make(map[string]struct{}, len(out)+len(additions))
	for i := range out {
		seen[supportedModelDedupKey(out[i].Platform, out[i].Name)] = struct{}{}
	}
	for i := range additions {
		key := supportedModelDedupKey(additions[i].Platform, additions[i].Name)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, additions[i])
	}
	sortSupportedModels(out)
	return out
}

func supportedModelDedupKey(platform, model string) string {
	return strings.ToLower(strings.TrimSpace(platform)) + "\x00" + strings.ToLower(strings.TrimSpace(model))
}

func sortSupportedModels(models []SupportedModel) {
	sort.SliceStable(models, func(i, j int) bool {
		if models[i].Platform != models[j].Platform {
			return models[i].Platform < models[j].Platform
		}
		return strings.ToLower(models[i].Name) < strings.ToLower(models[j].Name)
	})
}

// availableAccountModelPricing 按真实计费基准查找账号公开别名的渠道定价。
// mapping value 只参与内部查找；返回的 SupportedModel.Name 始终是公开别名。
func availableAccountModelPricing(ch *Channel, model availableAccountModel) *ChannelModelPricing {
	if ch == nil {
		return nil
	}

	billingModel := model.Name
	switch ch.BillingModelSource {
	case BillingModelSourceRequested:
		billingModel = model.Name
	case BillingModelSourceUpstream:
		billingModel = model.UpstreamModel
	case BillingModelSourceChannelMapped:
		if mapped := matchAvailableChannelMapping(ch, model.Platform, model.Name); mapped != "" {
			billingModel = mapped
		}
	default:
		if mapped := matchAvailableChannelMapping(ch, model.Platform, model.Name); mapped != "" {
			billingModel = mapped
		}
	}
	return matchAvailableChannelPricing(ch, model.Platform, billingModel)
}

func matchAvailableChannelMapping(ch *Channel, platform, model string) string {
	if ch == nil {
		return ""
	}
	mapping := ch.ModelMapping[platform]
	modelLower := strings.ToLower(model)
	for source, target := range mapping {
		if _, wildcard := splitWildcardSuffix(source); wildcard {
			continue
		}
		if strings.ToLower(source) == modelLower {
			return strings.TrimSpace(target)
		}
	}
	for source, target := range mapping {
		prefix, wildcard := splitWildcardSuffix(source)
		if wildcard && strings.HasPrefix(modelLower, strings.ToLower(prefix)) {
			return strings.TrimSpace(target)
		}
	}
	return ""
}

func matchAvailableChannelPricing(ch *Channel, platform, model string) *ChannelModelPricing {
	if ch == nil || strings.TrimSpace(model) == "" {
		return nil
	}
	modelLower := strings.ToLower(model)
	for i := range ch.ModelPricing {
		pricing := &ch.ModelPricing[i]
		if pricing.Platform != platform {
			continue
		}
		for _, configured := range pricing.Models {
			if _, wildcard := splitWildcardSuffix(configured); wildcard {
				continue
			}
			if strings.ToLower(configured) == modelLower {
				clone := pricing.Clone()
				return &clone
			}
		}
	}
	for i := range ch.ModelPricing {
		pricing := &ch.ModelPricing[i]
		if pricing.Platform != platform {
			continue
		}
		for _, configured := range pricing.Models {
			prefix, wildcard := splitWildcardSuffix(configured)
			if wildcard && strings.HasPrefix(modelLower, strings.ToLower(prefix)) {
				clone := pricing.Clone()
				return &clone
			}
		}
	}
	return nil
}

// fillGlobalPricingFallback 对未命中渠道定价的支持模型，从全局 LiteLLM 数据合成一份
// 展示用定价。仅用于「可用渠道」展示，不影响真实计费链路。
//
// 触发条件：
//  1. Pricing == nil（渠道完全没声明该模型的定价条目）
//  2. Pricing 非 nil但所有价格字段为空（admin UI 建了条目但没填价格）
//
// 当 s.pricingService 为 nil（测试场景），跳过回落。
func (s *ChannelService) fillGlobalPricingFallback(models []SupportedModel) {
	if s.pricingService == nil {
		return
	}
	for i := range models {
		if !pricingNeedsFallback(models[i].Pricing) {
			continue
		}
		lp := s.pricingService.GetModelPricing(models[i].Name)
		if lp == nil {
			continue
		}
		models[i].Pricing = synthesizePricingFromLiteLLM(lp, models[i].Pricing)
	}
}

// pricingNeedsFallback 判定一个 ChannelModelPricing 是否需要走全局回落。
// 价格全部缺失（无 flat 字段且无任何带价 interval）即视为未配置。
func pricingNeedsFallback(p *ChannelModelPricing) bool {
	if p == nil {
		return true
	}
	if p.InputPrice != nil || p.OutputPrice != nil ||
		p.CacheWritePrice != nil || p.CacheReadPrice != nil ||
		p.ImageOutputPrice != nil || p.PerRequestPrice != nil {
		return false
	}
	for _, iv := range p.Intervals {
		if iv.InputPrice != nil || iv.OutputPrice != nil ||
			iv.CacheWritePrice != nil || iv.CacheReadPrice != nil ||
			iv.PerRequestPrice != nil {
			return false
		}
	}
	return true
}

// synthesizePricingFromLiteLLM 把 LiteLLM 的定价数据转成 ChannelModelPricing 形态，
// 仅用于展示。
//
// 计费模式优先级：
//  1. 渠道已选 BillingMode（admin 在 UI 里选了 image / per_request / video 但没填价的场景，
//     按选定模式合成对应字段）
//  2. LiteLLM mode="image_generation" → image
//  3. 默认 token
//
// LiteLLM 中字段 0 视为未配置，不带入展示。
func synthesizePricingFromLiteLLM(lp *LiteLLMModelPricing, existing *ChannelModelPricing) *ChannelModelPricing {
	if lp == nil {
		return existing
	}

	mode := BillingModeToken
	switch {
	case existing != nil && existing.BillingMode != "":
		mode = existing.BillingMode
	case lp.Mode == "image_generation":
		mode = BillingModeImage
	}

	if mode == BillingModeImage || mode == BillingModePerRequest || mode == BillingModeVideo {
		return &ChannelModelPricing{
			BillingMode:      mode,
			PerRequestPrice:  nonZeroPtr(lp.OutputCostPerImage),
			ImageOutputPrice: nonZeroPtr(lp.OutputCostPerImageToken),
			InputPrice:       nonZeroPtr(lp.InputCostPerToken),
			OutputPrice:      nonZeroPtr(lp.OutputCostPerToken),
		}
	}
	return &ChannelModelPricing{
		BillingMode:      mode,
		InputPrice:       nonZeroPtr(lp.InputCostPerToken),
		OutputPrice:      nonZeroPtr(lp.OutputCostPerToken),
		CacheWritePrice:  nonZeroPtr(lp.CacheCreationInputTokenCost),
		CacheReadPrice:   nonZeroPtr(lp.CacheReadInputTokenCost),
		ImageOutputPrice: nonZeroPtr(lp.OutputCostPerImageToken),
	}
}

func nonZeroPtr(v float64) *float64 {
	if v == 0 {
		return nil
	}
	return &v
}
