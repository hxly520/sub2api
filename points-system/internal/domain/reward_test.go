package domain

import (
	"errors"
	"testing"
)

func TestCalculateRewardPercentageUsesRawSpendAndFloorsToCent(t *testing.T) {
	minimum, maximum := int64(50_000), int64(50_000) // 5%
	tier := Tier{
		LowerPointsHundredths:  100,
		UpperPointsHundredths:  int64Pointer(10_100),
		RewardMode:             RewardModePercentageRange,
		RewardPercentageMinPPM: &minimum,
		RewardPercentageMaxPPM: &maximum,
	}

	result, err := CalculateReward(tier, 1_000_000, maximum) // 1 U raw spend
	if err != nil {
		t.Fatal(err)
	}
	if result.CalculatedRewardMicroUSD != 50_000 { // 0.05 U
		t.Fatalf("reward = %d, want 50000", result.CalculatedRewardMicroUSD)
	}
	if result.BaseMicroUSD != 1_000_000 || result.SampledPercentagePPM == nil || *result.SampledPercentagePPM != 50_000 {
		t.Fatalf("calculation snapshot was not preserved: %+v", result)
	}
}

func TestCalculateRewardPercentageDoesNotDeriveSpendFromPoints(t *testing.T) {
	minimum, maximum := int64(0), int64(50_000)
	tier := Tier{
		LowerPointsHundredths:  1_000,
		RewardMode:             RewardModePercentageRange,
		RewardPercentageMinPPM: &minimum,
		RewardPercentageMaxPPM: &maximum,
	}

	result, err := CalculateReward(tier, 1_000_000, maximum)
	if err != nil {
		t.Fatal(err)
	}
	if result.CalculatedRewardMicroUSD != 50_000 {
		t.Fatalf("reward = %d, want percentage of raw 1 U spend", result.CalculatedRewardMicroUSD)
	}
}

func TestCalculateRewardPercentageFloorsWithoutOverpaying(t *testing.T) {
	minimum, maximum := int64(33_333), int64(33_333)
	tier := Tier{
		LowerPointsHundredths:  0,
		RewardMode:             RewardModePercentageRange,
		RewardPercentageMinPPM: &minimum,
		RewardPercentageMaxPPM: &maximum,
	}

	result, err := CalculateReward(tier, 1_000_000, maximum)
	if err != nil {
		t.Fatal(err)
	}
	if result.CalculatedRewardMicroUSD != 30_000 {
		t.Fatalf("reward = %d, want 30000 after cent flooring", result.CalculatedRewardMicroUSD)
	}
}

func TestCalculateRewardFixedRangeAllowsZeroLowerBound(t *testing.T) {
	minimum, maximum := int64(0), int64(1_000_000)
	tier := Tier{
		LowerPointsHundredths:  0,
		RewardMode:             RewardModeFixedRange,
		FixedRewardMinMicroUSD: &minimum,
		FixedRewardMaxMicroUSD: &maximum,
	}

	result, err := CalculateReward(tier, 9_999_999, 0)
	if err != nil {
		t.Fatal(err)
	}
	if result.CalculatedRewardMicroUSD != 0 {
		t.Fatalf("reward = %d, want 0", result.CalculatedRewardMicroUSD)
	}
}

func TestCalculateRewardRejectsOutOfRangeServerSample(t *testing.T) {
	minimum, maximum := int64(10_000), int64(20_000)
	tier := Tier{
		LowerPointsHundredths:  0,
		RewardMode:             RewardModeFixedRange,
		FixedRewardMinMicroUSD: &minimum,
		FixedRewardMaxMicroUSD: &maximum,
	}

	_, err := CalculateReward(tier, 0, 30_000)
	if err == nil || errors.Is(err, ErrPolicyIncomplete) {
		t.Fatalf("expected an out-of-range sample error, got %v", err)
	}
}
