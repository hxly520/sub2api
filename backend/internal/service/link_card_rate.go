package service

import (
	"math"

	"github.com/shopspring/decimal"
)

const linkCardQuotaRateDecimalPlaces int32 = 1

// LinkCardQuotaRateMultiplier converts a native Sub2API rate into the rate
// charged against a card's fixed issuance-time 1x quota. Non-exact ratios are
// rounded upward to one decimal place so conversion never undercharges.
func LinkCardQuotaRateMultiplier(apiKey *APIKey, nativeRate float64) float64 {
	if apiKey == nil || !apiKey.IsLinkKey() {
		return nativeRate
	}
	return LinkCardQuotaRateMultiplierFromIssue(nativeRate, apiKey.LinkRateMultiplier)
}

func LinkCardQuotaRateMultiplierFromIssue(nativeRate, issueRate float64) float64 {
	if nativeRate <= 0 || issueRate <= 0 || math.IsNaN(nativeRate) || math.IsNaN(issueRate) || math.IsInf(nativeRate, 0) || math.IsInf(issueRate, 0) {
		return nativeRate
	}
	native := decimal.NewFromFloat(nativeRate)
	issue := decimal.NewFromFloat(issueRate)
	ratio := native.Div(issue)
	factor := decimal.New(1, linkCardQuotaRateDecimalPlaces)
	return ratio.Mul(factor).Ceil().Div(factor).InexactFloat64()
}

// LinkCardChargeRateMultiplier returns the actual monetary multiplier stored
// in usage and reserve ledgers. Dividing it by the issuance rate yields the
// one-decimal quota multiplier returned to card users.
func LinkCardChargeRateMultiplier(apiKey *APIKey, nativeRate float64) float64 {
	if apiKey == nil || !apiKey.IsLinkKey() {
		return nativeRate
	}
	return LinkCardChargeRateMultiplierFromIssue(nativeRate, apiKey.LinkRateMultiplier)
}

func LinkCardChargeRateMultiplierFromIssue(nativeRate, issueRate float64) float64 {
	quotaRate := LinkCardQuotaRateMultiplierFromIssue(nativeRate, issueRate)
	if nativeRate <= 0 || issueRate <= 0 || math.IsNaN(quotaRate) || math.IsInf(quotaRate, 0) {
		return nativeRate
	}
	return decimal.NewFromFloat(quotaRate).Mul(decimal.NewFromFloat(issueRate)).InexactFloat64()
}
