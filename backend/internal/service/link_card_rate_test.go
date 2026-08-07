package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLinkCardRateConversionRoundsUpToOneDecimalWithoutUndercharge(t *testing.T) {
	tests := []struct {
		name        string
		issueRate   float64
		currentRate float64
		quotaRate   float64
		chargeRate  float64
	}{
		{name: "exact", issueRate: 0.10, currentRate: 0.15, quotaRate: 1.5, chargeRate: 0.15},
		{name: "six to seven", issueRate: 0.06, currentRate: 0.07, quotaRate: 1.2, chargeRate: 0.072},
		{name: "seven to eight", issueRate: 0.07, currentRate: 0.08, quotaRate: 1.2, chargeRate: 0.084},
		{name: "eight to nine", issueRate: 0.08, currentRate: 0.09, quotaRate: 1.2, chargeRate: 0.096},
		{name: "nine to eight", issueRate: 0.09, currentRate: 0.08, quotaRate: 0.9, chargeRate: 0.081},
		{name: "unchanged", issueRate: 0.07, currentRate: 0.07, quotaRate: 1.0, chargeRate: 0.07},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			quotaRate := LinkCardQuotaRateMultiplierFromIssue(test.currentRate, test.issueRate)
			chargeRate := LinkCardChargeRateMultiplierFromIssue(test.currentRate, test.issueRate)
			require.InDelta(t, test.quotaRate, quotaRate, 1e-12)
			require.InDelta(t, test.chargeRate, chargeRate, 1e-12)
			require.GreaterOrEqual(t, chargeRate+1e-12, test.currentRate)
		})
	}
}

func TestStandardKeyRateConversionIsUnchanged(t *testing.T) {
	key := &APIKey{KeyType: APIKeyTypeStandard, LinkRateMultiplier: 0.06}
	require.Equal(t, 0.07, LinkCardQuotaRateMultiplier(key, 0.07))
	require.Equal(t, 0.07, LinkCardChargeRateMultiplier(key, 0.07))
}

func TestLinkCardRateConversionGridAlwaysRoundsTowardCostCoverage(t *testing.T) {
	rates := []float64{0.06, 0.07, 0.08, 0.09}
	expected := [][]float64{
		{1.0, 1.2, 1.4, 1.5},
		{0.9, 1.0, 1.2, 1.3},
		{0.8, 0.9, 1.0, 1.2},
		{0.7, 0.8, 0.9, 1.0},
	}

	for issueIndex, issueRate := range rates {
		for currentIndex, currentRate := range rates {
			quotaRate := LinkCardQuotaRateMultiplierFromIssue(currentRate, issueRate)
			chargeRate := LinkCardChargeRateMultiplierFromIssue(currentRate, issueRate)

			require.InDelta(t, expected[issueIndex][currentIndex], quotaRate, 1e-12)
			require.GreaterOrEqual(t, chargeRate+1e-12, currentRate)
			require.Less(t, (quotaRate-0.1)*issueRate, currentRate+1e-12)
		}
	}
}
