package store

import (
	"github.com/hxly520/sub2api/points-system/internal/domain"
	"github.com/hxly520/sub2api/points-system/internal/security"
)

func tierForPoints(policy domain.Policy, points int64) (domain.Tier, bool) {
	for _, tier := range policy.Tiers {
		if points < tier.LowerPointsHundredths {
			continue
		}
		if tier.UpperPointsHundredths == nil || points < *tier.UpperPointsHundredths {
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

func limitCheckinReward(calculated, singleCap, platformCap, platformUsed, userCap, userUsed int64) (int64, error) {
	if calculated < 0 || singleCap <= 0 || platformCap <= 0 || platformUsed < 0 ||
		userCap <= 0 || userUsed < 0 {
		return 0, domain.ErrPolicyIncomplete
	}
	limited := min64(calculated, singleCap, platformCap-platformUsed, userCap-userUsed)
	if calculated > 0 && limited <= 0 {
		return 0, domain.ErrCapExhausted
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
