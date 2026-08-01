package store

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/hxly520/sub2api/points-system/internal/domain"
)

func TestTierForPointsUsesConfiguredPointBasis(t *testing.T) {
	upper := int64(10_000)
	minimum, maximum := int64(0), int64(10_000)
	policy := domain.Policy{Tiers: []domain.Tier{
		{
			LowerPointsHundredths:  storeInt64Pointer(0),
			UpperPointsHundredths:  &upper,
			RewardMode:             domain.RewardModeFixedRange,
			FixedRewardMinMicroUSD: &minimum,
			FixedRewardMaxMicroUSD: &maximum,
		},
		{
			LowerPointsHundredths:  storeInt64Pointer(10_000),
			RewardMode:             domain.RewardModeFixedRange,
			FixedRewardMinMicroUSD: &minimum,
			FixedRewardMaxMicroUSD: &maximum,
		},
	}}

	tier, ok := tierForPoints(policy, 9_999)
	if !ok || tier.LowerPointsHundredths == nil || *tier.LowerPointsHundredths != 0 {
		t.Fatalf("99.99 points matched %+v, %v", tier, ok)
	}
	tier, ok = tierForPoints(policy, 10_000)
	if !ok || tier.LowerPointsHundredths == nil || *tier.LowerPointsHundredths != 10_000 {
		t.Fatalf("100 points matched %+v, %v", tier, ok)
	}
}

func TestLimitCheckinRewardAppliesEveryMonetarySafetyCap(t *testing.T) {
	tests := []struct {
		name                                               string
		calculated, single, platform, used, user, userUsed int64
		want                                               int64
	}{
		{name: "single", calculated: 1_000_000, single: 400_000, platform: 10_000_000, user: 10_000_000, want: 400_000},
		{name: "platform remaining", calculated: 1_000_000, single: 2_000_000, platform: 1_200_000, used: 700_000, user: 10_000_000, want: 500_000},
		{name: "user remaining", calculated: 1_000_000, single: 2_000_000, platform: 10_000_000, user: 900_000, userUsed: 300_000, want: 600_000},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := limitCheckinReward(test.calculated, storeInt64Pointer(test.single),
				storeInt64Pointer(test.platform), test.used, storeInt64Pointer(test.user), test.userUsed)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("limited reward = %d, want %d", got, test.want)
			}
		})
	}
}

func TestLimitCheckinRewardRejectsExhaustedCap(t *testing.T) {
	_, err := limitCheckinReward(100_000, storeInt64Pointer(100_000),
		storeInt64Pointer(100_000), 100_000, storeInt64Pointer(100_000), 0)
	if err != domain.ErrCapExhausted {
		t.Fatalf("error = %v, want ErrCapExhausted", err)
	}
}

func TestTierForMetricsUsesRawSpendAtFinalPolicyBoundaries(t *testing.T) {
	maximumPPM := int64(50_000)
	spend10, spend50, spend100 := int64(10_000_000), int64(50_000_000), int64(100_000_000)
	policy := domain.Policy{
		CheckinTierBasis: domain.CheckinTierBasisSpend,
		Tiers: []domain.Tier{
			percentageSpendTier(1_000_000, &spend10, 10_000, maximumPPM),
			percentageSpendTier(spend10, &spend50, 20_000, maximumPPM),
			percentageSpendTier(spend50, &spend100, 30_000, maximumPPM),
			percentageSpendTier(spend100, nil, 40_000, maximumPPM),
		},
	}
	tests := []struct {
		name       string
		spend      int64
		wantFound  bool
		wantMinPPM int64
	}{
		{name: "below one U", spend: 999_999},
		{name: "one U inclusive", spend: 1_000_000, wantFound: true, wantMinPPM: 10_000},
		{name: "below ten U", spend: 9_999_999, wantFound: true, wantMinPPM: 10_000},
		{name: "ten U enters second", spend: spend10, wantFound: true, wantMinPPM: 20_000},
		{name: "below fifty U", spend: spend50 - 1, wantFound: true, wantMinPPM: 20_000},
		{name: "fifty U enters third", spend: spend50, wantFound: true, wantMinPPM: 30_000},
		{name: "below hundred U", spend: spend100 - 1, wantFound: true, wantMinPPM: 30_000},
		{name: "hundred U enters fourth", spend: spend100, wantFound: true, wantMinPPM: 40_000},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tier, found := tierForMetrics(policy, checkinMetrics{
				BasisPointsHundredths:  9_999_999_999,
				RewardBaseMicroUSD:     test.spend,
				YesterdaySpendMicroUSD: test.spend,
			})
			if found != test.wantFound {
				t.Fatalf("found = %v, want %v", found, test.wantFound)
			}
			if found && (tier.RewardPercentageMinPPM == nil || *tier.RewardPercentageMinPPM != test.wantMinPPM) {
				t.Fatalf("minimum PPM = %v, want %d", tier.RewardPercentageMinPPM, test.wantMinPPM)
			}
		})
	}
}

func TestLimitCheckinRewardAllowsUnlimitedCaps(t *testing.T) {
	const reward = int64(5_000_000)
	got, err := limitCheckinReward(reward, nil, nil, 900_000_000, nil, 80_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if got != reward {
		t.Fatalf("unlimited reward = %d, want %d", got, reward)
	}
}

func TestLimitCheckinRewardRejectsUnlimitedAggregateOverflow(t *testing.T) {
	if _, err := limitCheckinReward(2, nil, nil, math.MaxInt64-1, nil, 0); err != domain.ErrInvalidState {
		t.Fatalf("platform overflow error = %v, want ErrInvalidState", err)
	}
	if _, err := limitCheckinReward(2, nil, nil, 0, nil, math.MaxInt64-1); err != domain.ErrInvalidState {
		t.Fatalf("user overflow error = %v, want ErrInvalidState", err)
	}
}

func percentageSpendTier(lower int64, upper *int64, minimumPPM, maximumPPM int64) domain.Tier {
	return domain.Tier{
		LowerSpendMicroUSD:     storeInt64Pointer(lower),
		UpperSpendMicroUSD:     upper,
		RewardMode:             domain.RewardModePercentageRange,
		RewardPercentageMinPPM: storeInt64Pointer(minimumPPM),
		RewardPercentageMaxPPM: storeInt64Pointer(maximumPPM),
	}
}

func storeInt64Pointer(value int64) *int64 {
	return &value
}

func TestValidateCheckinSpendAlwaysUsesYesterdaySpend(t *testing.T) {
	policy := domain.Policy{
		Mode:                        domain.PolicyModeAllUsers,
		Basis:                       domain.PolicyBasisTotal,
		MinimumCheckinSpendMicroUSD: 2_000_000,
	}
	if err := validateCheckinSpend(policy, 1_990_000); err != domain.ErrCheckinSpendMinimum {
		t.Fatalf("error = %v, want minimum-spend rejection", err)
	}
	if err := validateCheckinSpend(policy, 2_000_000); err != nil {
		t.Fatalf("threshold spend rejected: %v", err)
	}
}

func TestValidateCheckinSpendConsumerOnlyRequiresPositiveYesterdaySpend(t *testing.T) {
	policy := domain.Policy{Mode: domain.PolicyModeConsumerOnly}
	if err := validateCheckinSpend(policy, 0); err != domain.ErrCheckinSpendMinimum {
		t.Fatalf("error = %v, want consumer-only rejection", err)
	}
	if err := validateCheckinSpend(policy, 1); err != nil {
		t.Fatalf("positive prior-day spend rejected: %v", err)
	}
}

func TestCheckinResultDoesNotExposePrivateRewardRule(t *testing.T) {
	rate := int64(50_000)
	body, err := json.Marshal(CheckinResult{
		RewardMicroUSD: 50_000,
		Calculation: domain.RewardCalculation{
			Mode:                     domain.RewardModePercentageRange,
			BaseMicroUSD:             1_000_000,
			SampledPercentagePPM:     &rate,
			CalculatedRewardMicroUSD: 50_000,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(body)
	for _, privateField := range []string{"calculation", "sampled_percentage", "base_microusd"} {
		if strings.Contains(encoded, privateField) {
			t.Fatalf("public result leaked %q: %s", privateField, encoded)
		}
	}
}

func TestCheckinRejectionEventIDConvergesByUserBusinessDateAndReason(t *testing.T) {
	location := time.FixedZone("UTC+8", 8*60*60)
	date := time.Date(2026, 7, 30, 0, 0, 0, 0, location)
	first := convergedCheckinRejectionEventID(42, date, checkinRejectionLimit)
	if first != convergedCheckinRejectionEventID(42, date.Add(12*time.Hour), checkinRejectionLimit) {
		t.Fatal("same user and business date produced different rejection event IDs")
	}
	if first == convergedCheckinRejectionEventID(43, date, checkinRejectionLimit) {
		t.Fatal("different users produced the same rejection event ID")
	}
	if first == convergedCheckinRejectionEventID(42, date.AddDate(0, 0, 1), checkinRejectionLimit) {
		t.Fatal("different business dates produced the same rejection event ID")
	}
	if first == convergedCheckinRejectionEventID(42, date, checkinRejectionMinimumSpend) {
		t.Fatal("different rejection reasons produced the same rejection event ID")
	}
	if len(first) == 0 || first[0] >= 0x21 {
		t.Fatalf("internal rejection event ID must be outside the public idempotency-key alphabet: %q", first)
	}
}
