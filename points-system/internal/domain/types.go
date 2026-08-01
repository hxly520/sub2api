package domain

import (
	"errors"
	"time"
)

const (
	PolicyModeAllUsers     = "all_users"
	PolicyModeConsumerOnly = "consumer_only"
	PolicyBasisYesterday   = "yesterday"
	PolicyBasisTotal       = "total"
	CheckinTierBasisPoints = "points"
	CheckinTierBasisSpend  = "spend"

	RewardModeFixedRange      = "fixed_range"
	RewardModePercentageRange = "percentage_range"
	PercentagePPMDenominator  = int64(1_000_000)
)

var (
	ErrDisabled            = errors.New("points system is disabled")
	ErrPolicyIncomplete    = errors.New("points policy is incomplete")
	ErrIdempotencyConflict = errors.New("idempotency key conflicts with an existing request")
	ErrCapExhausted        = errors.New("points award cap is exhausted")
	ErrCheckinLimit        = errors.New("daily check-in limit reached")
	ErrCheckinSpendMinimum = errors.New("minimum check-in spend was not met")
	ErrSnapshotNotReady    = errors.New("yesterday points snapshot is not ready")
	ErrNoMatchingTier      = errors.New("no check-in reward tier matched")
	ErrNotFound            = errors.New("record not found")
	ErrForbidden           = errors.New("forbidden")
	ErrInvalidState        = errors.New("invalid state transition")
)

type Tier struct {
	LowerPointsHundredths  *int64 `json:"lower_points_hundredths,omitempty"`
	UpperPointsHundredths  *int64 `json:"upper_points_hundredths,omitempty"`
	LowerSpendMicroUSD     *int64 `json:"lower_spend_microusd,omitempty"`
	UpperSpendMicroUSD     *int64 `json:"upper_spend_microusd,omitempty"`
	RewardMode             string `json:"reward_mode"`
	FixedRewardMinMicroUSD *int64 `json:"fixed_reward_min_microusd,omitempty"`
	FixedRewardMaxMicroUSD *int64 `json:"fixed_reward_max_microusd,omitempty"`
	RewardPercentageMinPPM *int64 `json:"reward_percentage_min_ppm,omitempty"`
	RewardPercentageMaxPPM *int64 `json:"reward_percentage_max_ppm,omitempty"`
}

type Policy struct {
	ID                              int64     `json:"id"`
	VersionNo                       int64     `json:"version_no"`
	EffectiveDate                   time.Time `json:"effective_date"`
	Enabled                         bool      `json:"enabled"`
	Mode                            string    `json:"mode"`
	Basis                           string    `json:"basis"`
	CheckinTierBasis                string    `json:"checkin_tier_basis"`
	CheckinEnabled                  bool      `json:"checkin_enabled"`
	CheckinDailyLimit               *int      `json:"checkin_daily_limit,omitempty"`
	MinimumCheckinSpendMicroUSD     int64     `json:"minimum_checkin_spend_microusd"`
	CheckinPlatformDailyCapMicroUSD *int64    `json:"checkin_platform_daily_cap_microusd"`
	CheckinUserDailyCapMicroUSD     *int64    `json:"checkin_user_daily_cap_microusd"`
	CheckinSingleAwardCapMicroUSD   *int64    `json:"checkin_single_award_cap_microusd"`
	PointsPerUSDHundredths          int64     `json:"points_per_usd_hundredths"`
	RefreshMinute                   int       `json:"refresh_minute"`
	CreatedBy                       int64     `json:"created_by"`
	CreatedAt                       time.Time `json:"created_at"`
	Tiers                           []Tier    `json:"tiers"`
}

func (p Policy) ValidateForEnable() error {
	tierBasis := p.CheckinTierBasis
	if tierBasis == "" {
		tierBasis = CheckinTierBasisPoints
	}
	if (p.Mode != PolicyModeAllUsers && p.Mode != PolicyModeConsumerOnly) ||
		(p.Basis != PolicyBasisYesterday && p.Basis != PolicyBasisTotal) ||
		(tierBasis != CheckinTierBasisPoints && tierBasis != CheckinTierBasisSpend) ||
		p.MinimumCheckinSpendMicroUSD < 0 || p.MinimumCheckinSpendMicroUSD%MicroUSDPerCent != 0 ||
		p.PointsPerUSDHundredths <= 0 ||
		p.RefreshMinute < 0 || p.RefreshMinute >= 24*60 {
		return ErrPolicyIncomplete
	}
	// Consumer-only check-in is defined by prior-day consumption, so a total
	// policy basis would be internally contradictory and is rejected server-side.
	if p.Mode == PolicyModeConsumerOnly && p.Basis != PolicyBasisYesterday {
		return ErrPolicyIncomplete
	}
	// Spend-based tiers must always use the prior natural day's raw spend.
	// Reject total-spend policies even if they bypass the administrator UI.
	if tierBasis == CheckinTierBasisSpend && p.Basis != PolicyBasisYesterday {
		return ErrPolicyIncomplete
	}
	if p.CheckinEnabled {
		if !p.Enabled {
			return ErrPolicyIncomplete
		}
		if p.CheckinDailyLimit == nil || *p.CheckinDailyLimit <= 0 || len(p.Tiers) == 0 {
			return ErrPolicyIncomplete
		}
	}
	for _, cap := range []*int64{
		p.CheckinPlatformDailyCapMicroUSD,
		p.CheckinUserDailyCapMicroUSD,
		p.CheckinSingleAwardCapMicroUSD,
	} {
		if cap != nil && (*cap <= 0 || *cap%MicroUSDPerCent != 0) {
			return ErrPolicyIncomplete
		}
	}
	if p.CheckinDailyLimit != nil && *p.CheckinDailyLimit <= 0 {
		return ErrPolicyIncomplete
	}
	if len(p.Tiers) > 0 {
		return ValidateTiersForBasis(p.Tiers, tierBasis)
	}
	return nil
}

func ValidateTiers(tiers []Tier) error {
	if len(tiers) == 0 {
		return ErrPolicyIncomplete
	}
	basis := CheckinTierBasisPoints
	if tiers[0].LowerSpendMicroUSD != nil || tiers[0].UpperSpendMicroUSD != nil {
		basis = CheckinTierBasisSpend
	}
	return ValidateTiersForBasis(tiers, basis)
}

func ValidateTiersForBasis(tiers []Tier, basis string) error {
	if len(tiers) == 0 || (basis != CheckinTierBasisPoints && basis != CheckinTierBasisSpend) {
		return ErrPolicyIncomplete
	}
	for i, tier := range tiers {
		lower, upper, ok := tierBounds(tier, basis)
		if !ok || lower < 0 || (basis == CheckinTierBasisSpend && lower%MicroUSDPerCent != 0) ||
			(upper != nil && (*upper <= lower || (basis == CheckinTierBasisSpend && *upper%MicroUSDPerCent != 0))) {
			return ErrPolicyIncomplete
		}
		switch tier.RewardMode {
		case RewardModeFixedRange:
			if tier.FixedRewardMinMicroUSD == nil || tier.FixedRewardMaxMicroUSD == nil ||
				*tier.FixedRewardMinMicroUSD < 0 || *tier.FixedRewardMaxMicroUSD <= 0 ||
				*tier.FixedRewardMaxMicroUSD < *tier.FixedRewardMinMicroUSD ||
				*tier.FixedRewardMinMicroUSD%10000 != 0 || *tier.FixedRewardMaxMicroUSD%10000 != 0 ||
				tier.RewardPercentageMinPPM != nil || tier.RewardPercentageMaxPPM != nil {
				return ErrPolicyIncomplete
			}
		case RewardModePercentageRange:
			if tier.RewardPercentageMinPPM == nil || tier.RewardPercentageMaxPPM == nil ||
				*tier.RewardPercentageMinPPM < 0 || *tier.RewardPercentageMaxPPM <= 0 ||
				*tier.RewardPercentageMaxPPM < *tier.RewardPercentageMinPPM ||
				*tier.RewardPercentageMaxPPM > PercentagePPMDenominator ||
				tier.FixedRewardMinMicroUSD != nil || tier.FixedRewardMaxMicroUSD != nil {
				return ErrPolicyIncomplete
			}
		default:
			return ErrPolicyIncomplete
		}
		for j := i + 1; j < len(tiers); j++ {
			if overlaps(tier, tiers[j], basis) {
				return ErrPolicyIncomplete
			}
		}
	}
	return nil
}

func tierBounds(tier Tier, basis string) (int64, *int64, bool) {
	switch basis {
	case CheckinTierBasisPoints:
		if tier.LowerPointsHundredths == nil || tier.LowerSpendMicroUSD != nil || tier.UpperSpendMicroUSD != nil {
			return 0, nil, false
		}
		return *tier.LowerPointsHundredths, tier.UpperPointsHundredths, true
	case CheckinTierBasisSpend:
		if tier.LowerSpendMicroUSD == nil || tier.LowerPointsHundredths != nil || tier.UpperPointsHundredths != nil {
			return 0, nil, false
		}
		return *tier.LowerSpendMicroUSD, tier.UpperSpendMicroUSD, true
	default:
		return 0, nil, false
	}
}

func overlaps(a, b Tier, basis string) bool {
	aLower, aUpper, aOK := tierBounds(a, basis)
	bLower, bUpper, bOK := tierBounds(b, basis)
	if !aOK || !bOK {
		return true
	}
	if aUpper != nil && *aUpper <= bLower {
		return false
	}
	if bUpper != nil && *bUpper <= aLower {
		return false
	}
	return true
}

type Account struct {
	UserID                       int64     `json:"user_id"`
	TotalPointsHundredths        int64     `json:"total_points_hundredths"`
	TotalSpendMicroUSD           int64     `json:"total_spend_microusd"`
	SettledCheckinRewardMicroUSD int64     `json:"settled_checkin_reward_microusd"`
	CreatedAt                    time.Time `json:"created_at"`
	UpdatedAt                    time.Time `json:"updated_at"`
}

type LedgerEntry struct {
	ID                    int64          `json:"id"`
	UserID                int64          `json:"user_id"`
	Kind                  string         `json:"kind"`
	DeltaPointsHundredths int64          `json:"delta_points_hundredths"`
	TotalAfterHundredths  int64          `json:"total_after_hundredths"`
	Source                string         `json:"source"`
	ExternalEventID       string         `json:"external_event_id"`
	PolicyVersion         *int64         `json:"policy_version,omitempty"`
	BusinessDate          *time.Time     `json:"business_date,omitempty"`
	ReferenceID           string         `json:"reference_id,omitempty"`
	Metadata              map[string]any `json:"metadata,omitempty"`
	CreatedAt             time.Time      `json:"created_at"`
	AwardedAt             time.Time      `json:"awarded_at"`
}
