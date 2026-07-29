package domain

import (
	"fmt"
	"math"
)

const MicroUSDPerCent = int64(10_000)

// RewardCalculation is the immutable rule snapshot stored with a check-in.
// SampledValue is a micro-USD amount for fixed_range and a PPM rate for
// percentage_range.
type RewardCalculation struct {
	Mode                     string `json:"mode"`
	BaseMicroUSD             int64  `json:"base_microusd"`
	SampledValue             int64  `json:"sampled_value"`
	SampledPercentagePPM     *int64 `json:"sampled_percentage_ppm,omitempty"`
	CalculatedRewardMicroUSD int64  `json:"calculated_reward_microusd"`
}

// CalculateReward validates a server-generated sample against the selected
// tier and calculates a cent-precision monetary reward. Percentage rewards are
// always rounded down so rounding can never exceed the configured percentage.
func CalculateReward(tier Tier, baseMicroUSD, sampledValue int64) (RewardCalculation, error) {
	if baseMicroUSD < 0 {
		return RewardCalculation{}, fmt.Errorf("reward base must be non-negative")
	}
	if err := ValidateTiers([]Tier{tier}); err != nil {
		return RewardCalculation{}, err
	}

	result := RewardCalculation{
		Mode:         tier.RewardMode,
		BaseMicroUSD: baseMicroUSD,
		SampledValue: sampledValue,
	}
	switch tier.RewardMode {
	case RewardModeFixedRange:
		if sampledValue < *tier.FixedRewardMinMicroUSD || sampledValue > *tier.FixedRewardMaxMicroUSD ||
			sampledValue%MicroUSDPerCent != 0 {
			return RewardCalculation{}, fmt.Errorf("fixed reward sample is outside the configured range")
		}
		result.CalculatedRewardMicroUSD = sampledValue
	case RewardModePercentageRange:
		if sampledValue < *tier.RewardPercentageMinPPM || sampledValue > *tier.RewardPercentageMaxPPM {
			return RewardCalculation{}, fmt.Errorf("percentage sample is outside the configured range")
		}
		reward, err := percentageRewardMicroUSD(baseMicroUSD, sampledValue)
		if err != nil {
			return RewardCalculation{}, err
		}
		result.SampledPercentagePPM = int64Pointer(sampledValue)
		result.CalculatedRewardMicroUSD = reward - reward%MicroUSDPerCent
	default:
		return RewardCalculation{}, ErrPolicyIncomplete
	}
	return result, nil
}

func percentageRewardMicroUSD(baseMicroUSD, ratePPM int64) (int64, error) {
	if baseMicroUSD < 0 || ratePPM < 0 || ratePPM > PercentagePPMDenominator {
		return 0, fmt.Errorf("invalid percentage reward operands")
	}
	whole := (baseMicroUSD / PercentagePPMDenominator) * ratePPM
	fraction := (baseMicroUSD % PercentagePPMDenominator) * ratePPM / PercentagePPMDenominator
	if whole > math.MaxInt64-fraction {
		return 0, fmt.Errorf("percentage reward overflows int64")
	}
	return whole + fraction, nil
}

func int64Pointer(value int64) *int64 {
	return &value
}
