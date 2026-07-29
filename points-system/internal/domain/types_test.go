package domain

import "testing"

func TestValidateTiersUsesHundredthPointThresholdsAndPercentageRanges(t *testing.T) {
	upper := int64(10_000)
	percentageMin, percentageMax := int64(10_000), int64(50_000)
	fixedMin, fixedMax := int64(0), int64(1_000_000)
	tiers := []Tier{
		{
			LowerPointsHundredths:  0,
			UpperPointsHundredths:  &upper,
			RewardMode:             RewardModePercentageRange,
			RewardPercentageMinPPM: &percentageMin,
			RewardPercentageMaxPPM: &percentageMax,
		},
		{
			LowerPointsHundredths:  10_000,
			RewardMode:             RewardModeFixedRange,
			FixedRewardMinMicroUSD: &fixedMin,
			FixedRewardMaxMicroUSD: &fixedMax,
		},
	}
	if err := ValidateTiers(tiers); err != nil {
		t.Fatalf("valid tiers rejected: %v", err)
	}
}

func TestValidateTiersRejectsMixedRewardConfiguration(t *testing.T) {
	percentageMin, percentageMax := int64(10_000), int64(50_000)
	fixedMin, fixedMax := int64(10_000), int64(20_000)
	tier := Tier{
		LowerPointsHundredths:  0,
		RewardMode:             RewardModePercentageRange,
		FixedRewardMinMicroUSD: &fixedMin,
		FixedRewardMaxMicroUSD: &fixedMax,
		RewardPercentageMinPPM: &percentageMin,
		RewardPercentageMaxPPM: &percentageMax,
	}
	if err := ValidateTiers([]Tier{tier}); err == nil {
		t.Fatal("mixed fixed and percentage configuration was accepted")
	}
}

func TestPolicyConsumerOnlyLocksBasisToYesterday(t *testing.T) {
	limit := 1
	cap := int64(1_000_000)
	percentageMin, percentageMax := int64(10_000), int64(50_000)
	policy := Policy{
		Enabled:                         true,
		Mode:                            PolicyModeConsumerOnly,
		Basis:                           PolicyBasisTotal,
		CheckinEnabled:                  true,
		CheckinDailyLimit:               &limit,
		CheckinPlatformDailyCapMicroUSD: &cap,
		CheckinUserDailyCapMicroUSD:     &cap,
		CheckinSingleAwardCapMicroUSD:   &cap,
		PointsPerUSDHundredths:          1_000,
		RefreshMinute:                   5,
		Tiers: []Tier{{
			LowerPointsHundredths:  0,
			RewardMode:             RewardModePercentageRange,
			RewardPercentageMinPPM: &percentageMin,
			RewardPercentageMaxPPM: &percentageMax,
		}},
	}
	if err := policy.ValidateForEnable(); err == nil {
		t.Fatal("consumer-only policy with total basis was accepted")
	}
}

func TestPolicyAcceptsTwoDecimalPointsRatioAndZeroMinimumSpend(t *testing.T) {
	policy := Policy{
		Enabled:                     true,
		Mode:                        PolicyModeAllUsers,
		Basis:                       PolicyBasisTotal,
		PointsPerUSDHundredths:      1_025,
		RefreshMinute:               5,
		MinimumCheckinSpendMicroUSD: 0,
	}
	if err := policy.ValidateForEnable(); err != nil {
		t.Fatalf("valid 10.25 points/U policy rejected: %v", err)
	}
}

func TestPolicyRejectsNonCentMinimumSpend(t *testing.T) {
	policy := Policy{
		Enabled:                     true,
		Mode:                        PolicyModeAllUsers,
		Basis:                       PolicyBasisYesterday,
		PointsPerUSDHundredths:      1_000,
		RefreshMinute:               5,
		MinimumCheckinSpendMicroUSD: 1,
	}
	if err := policy.ValidateForEnable(); err == nil {
		t.Fatal("non-cent minimum spend was accepted")
	}
}

func TestValidateTiersRejectsInvalidPercentageBounds(t *testing.T) {
	tests := []struct {
		name       string
		minimumPPM int64
		maximumPPM int64
	}{
		{name: "negative minimum", minimumPPM: -1, maximumPPM: 10_000},
		{name: "zero maximum", minimumPPM: 0, maximumPPM: 0},
		{name: "reversed", minimumPPM: 50_000, maximumPPM: 10_000},
		{name: "over one hundred percent", minimumPPM: 0, maximumPPM: 1_000_001},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tier := Tier{
				LowerPointsHundredths:  0,
				RewardMode:             RewardModePercentageRange,
				RewardPercentageMinPPM: &test.minimumPPM,
				RewardPercentageMaxPPM: &test.maximumPPM,
			}
			if err := ValidateTiers([]Tier{tier}); err == nil {
				t.Fatal("invalid percentage bounds were accepted")
			}
		})
	}
}

func TestValidateTiersRejectsOverlappingPointIntervals(t *testing.T) {
	firstUpper, secondUpper := int64(10_000), int64(15_000)
	minimum, maximum := int64(0), int64(10_000)
	tiers := []Tier{
		{
			LowerPointsHundredths:  0,
			UpperPointsHundredths:  &firstUpper,
			RewardMode:             RewardModeFixedRange,
			FixedRewardMinMicroUSD: &minimum,
			FixedRewardMaxMicroUSD: &maximum,
		},
		{
			LowerPointsHundredths:  9_900,
			UpperPointsHundredths:  &secondUpper,
			RewardMode:             RewardModeFixedRange,
			FixedRewardMinMicroUSD: &minimum,
			FixedRewardMaxMicroUSD: &maximum,
		},
	}
	if err := ValidateTiers(tiers); err == nil {
		t.Fatal("overlapping point intervals were accepted")
	}
}
