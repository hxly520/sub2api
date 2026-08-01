package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateTiersUsesHundredthPointThresholdsAndPercentageRanges(t *testing.T) {
	upper := int64(10_000)
	percentageMin, percentageMax := int64(10_000), int64(50_000)
	fixedMin, fixedMax := int64(0), int64(1_000_000)
	tiers := []Tier{
		{
			LowerPointsHundredths:  int64Pointer(0),
			UpperPointsHundredths:  &upper,
			RewardMode:             RewardModePercentageRange,
			RewardPercentageMinPPM: &percentageMin,
			RewardPercentageMaxPPM: &percentageMax,
		},
		{
			LowerPointsHundredths:  int64Pointer(10_000),
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
		LowerPointsHundredths:  int64Pointer(0),
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
			LowerPointsHundredths:  int64Pointer(0),
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
				LowerPointsHundredths:  int64Pointer(0),
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
			LowerPointsHundredths:  int64Pointer(0),
			UpperPointsHundredths:  &firstUpper,
			RewardMode:             RewardModeFixedRange,
			FixedRewardMinMicroUSD: &minimum,
			FixedRewardMaxMicroUSD: &maximum,
		},
		{
			LowerPointsHundredths:  int64Pointer(9_900),
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

func TestPolicyAllowsUnlimitedCheckinCapsButRequiresDailyLimit(t *testing.T) {
	dailyLimit := 1
	minimumPPM, maximumPPM := int64(10_000), int64(50_000)
	policy := Policy{
		Enabled:                     true,
		Mode:                        PolicyModeConsumerOnly,
		Basis:                       PolicyBasisYesterday,
		CheckinTierBasis:            CheckinTierBasisSpend,
		CheckinEnabled:              true,
		CheckinDailyLimit:           &dailyLimit,
		MinimumCheckinSpendMicroUSD: 1_000_000,
		PointsPerUSDHundredths:      1_000,
		RefreshMinute:               5,
		Tiers: []Tier{{
			LowerSpendMicroUSD:     int64Pointer(1_000_000),
			RewardMode:             RewardModePercentageRange,
			RewardPercentageMinPPM: &minimumPPM,
			RewardPercentageMaxPPM: &maximumPPM,
		}},
	}
	if err := policy.ValidateForEnable(); err != nil {
		t.Fatalf("unlimited check-in caps were rejected: %v", err)
	}
	policy.CheckinDailyLimit = nil
	if err := policy.ValidateForEnable(); err == nil {
		t.Fatal("enabled check-in without a daily count limit was accepted")
	}
}

func TestPolicyRejectsSpendTiersWithCumulativeSpendBasis(t *testing.T) {
	dailyLimit := 1
	minimumPPM, maximumPPM := int64(10_000), int64(50_000)
	policy := Policy{
		Enabled:                     true,
		Mode:                        PolicyModeAllUsers,
		Basis:                       PolicyBasisTotal,
		CheckinTierBasis:            CheckinTierBasisSpend,
		CheckinEnabled:              true,
		CheckinDailyLimit:           &dailyLimit,
		MinimumCheckinSpendMicroUSD: 1_000_000,
		PointsPerUSDHundredths:      1_000,
		RefreshMinute:               5,
		Tiers: []Tier{{
			LowerSpendMicroUSD:     int64Pointer(1_000_000),
			RewardMode:             RewardModePercentageRange,
			RewardPercentageMinPPM: &minimumPPM,
			RewardPercentageMaxPPM: &maximumPPM,
		}},
	}
	if err := policy.ValidateForEnable(); err == nil {
		t.Fatal("spend tiers with cumulative spend basis were accepted")
	}
}

func TestPolicyJSONRepresentsUnlimitedCapsAsNull(t *testing.T) {
	body, err := json.Marshal(Policy{})
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(body)
	for _, field := range []string{
		`"checkin_platform_daily_cap_microusd":null`,
		`"checkin_user_daily_cap_microusd":null`,
		`"checkin_single_award_cap_microusd":null`,
	} {
		if !strings.Contains(encoded, field) {
			t.Fatalf("policy JSON does not explicitly represent unlimited cap %s: %s", field, encoded)
		}
	}
}

func TestValidateTiersRejectsThresholdFamilyMismatch(t *testing.T) {
	minimumPPM, maximumPPM := int64(10_000), int64(50_000)
	tier := Tier{
		LowerPointsHundredths:  int64Pointer(100),
		LowerSpendMicroUSD:     int64Pointer(1_000_000),
		RewardMode:             RewardModePercentageRange,
		RewardPercentageMinPPM: &minimumPPM,
		RewardPercentageMaxPPM: &maximumPPM,
	}
	if err := ValidateTiersForBasis([]Tier{tier}, CheckinTierBasisSpend); err == nil {
		t.Fatal("tier with mixed point and spend thresholds was accepted")
	}
}
