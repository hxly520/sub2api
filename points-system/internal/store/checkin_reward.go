package store

import (
	"math"

	"github.com/hxly520/sub2api/points-system/internal/domain"
	"github.com/hxly520/sub2api/points-system/internal/security"
)

func tierForPoints(policy domain.Policy, points int64) (domain.Tier, bool) {
	return tierForValue(policy.Tiers, domain.CheckinTierBasisPoints, points)
}

func tierForMetrics(policy domain.Policy, metrics checkinMetrics) (domain.Tier, bool) {
	basis := policy.CheckinTierBasis
	if basis == "" {
		basis = domain.CheckinTierBasisPoints
	}
	value := metrics.BasisPointsHundredths
	if basis == domain.CheckinTierBasisSpend {
		value = metrics.RewardBaseMicroUSD
	}
	return tierForValue(policy.Tiers, basis, value)
}

func tierForValue(tiers []domain.Tier, basis string, value int64) (domain.Tier, bool) {
	for _, tier := range tiers {
		var lower, upper *int64
		switch basis {
		case domain.CheckinTierBasisPoints:
			lower, upper = tier.LowerPointsHundredths, tier.UpperPointsHundredths
		case domain.CheckinTierBasisSpend:
			lower, upper = tier.LowerSpendMicroUSD, tier.UpperSpendMicroUSD
		default:
			return domain.Tier{}, false
		}
		if lower == nil || value < *lower {
			continue
		}
		if upper == nil || value < *upper {
			return tier, true
		}
	}
	return domain.Tier{}, false
}

func calculateCheckinReward(tier domain.Tier, rewardBaseMicroUSD int64) (domain.RewardCalculation, error) {
	var (
		sampled int64
		err     error
	)
	switch tier.RewardMode {
	case domain.RewardModeFixedRange:
		sampled, err = security.RandomSteppedInt64(
			*tier.FixedRewardMinMicroUSD,
			*tier.FixedRewardMaxMicroUSD,
			domain.MicroUSDPerCent,
		)
	case domain.RewardModePercentageRange:
		sampled, err = security.RandomSteppedInt64(
			*tier.RewardPercentageMinPPM,
			*tier.RewardPercentageMaxPPM,
			1,
		)
	default:
		return domain.RewardCalculation{}, domain.ErrPolicyIncomplete
	}
	if err != nil {
		return domain.RewardCalculation{}, err
	}
	return domain.CalculateReward(tier, rewardBaseMicroUSD, sampled)
}

func limitCheckinReward(calculated int64, singleCap, platformCap *int64, platformUsed int64,
	userCap *int64, userUsed int64) (int64, error) {
	if calculated < 0 || platformUsed < 0 || userUsed < 0 {
		return 0, domain.ErrPolicyIncomplete
	}
	limited := calculated
	for _, limit := range []struct {
		cap  *int64
		used int64
	}{
		{cap: singleCap},
		{cap: platformCap, used: platformUsed},
		{cap: userCap, used: userUsed},
	} {
		if limit.cap == nil {
			continue
		}
		if *limit.cap <= 0 || *limit.cap%domain.MicroUSDPerCent != 0 {
			return 0, domain.ErrPolicyIncomplete
		}
		limited = min64(limited, *limit.cap-limit.used)
	}
	if limited < 0 {
		return 0, domain.ErrInvalidState
	}
	if calculated > 0 && limited == 0 {
		return 0, domain.ErrCapExhausted
	}
	if limited > math.MaxInt64-platformUsed || limited > math.MaxInt64-userUsed {
		return 0, domain.ErrInvalidState
	}
	return limited, nil
}

func validateCheckinSpend(policy domain.Policy, yesterdaySpendMicroUSD int64) error {
	minimum := policy.MinimumCheckinSpendMicroUSD
	if policy.Mode == domain.PolicyModeConsumerOnly && minimum == 0 {
		minimum = 1
	}
	if yesterdaySpendMicroUSD < minimum {
		return domain.ErrCheckinSpendMinimum
	}
	return nil
}
