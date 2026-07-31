package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hxly520/sub2api/points-system/internal/domain"
	"github.com/hxly520/sub2api/points-system/internal/security"
	"github.com/hxly520/sub2api/points-system/internal/store"
)

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	now := time.Now()
	loginEmail, err := s.loginEmail(r.Context(), p.Session.UserID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	account, err := s.Store.Account(r.Context(), p.Session.UserID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	count, awarded, err := s.Store.CheckinStatus(r.Context(), p.Session.UserID, now)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	yesterday := s.Store.BusinessDate(now).AddDate(0, 0, -1)
	snapshot, snapshotErr := s.Store.Snapshot(r.Context(), p.Session.UserID, yesterday)
	if snapshotErr != nil && !errors.Is(snapshotErr, domain.ErrNotFound) {
		s.fail(w, r, snapshotErr)
		return
	}
	var snapshotValue any
	if snapshotErr == nil {
		snapshotValue = publicSnapshot(snapshot)
	}
	policy, policyLoaded := policyFrom(r)
	if !policyLoaded {
		policy, err = s.currentPolicy(r.Context(), now)
		if errors.Is(err, domain.ErrNotFound) {
			policy = domain.Policy{}
			err = nil
		}
		if err != nil {
			s.fail(w, r, err)
			return
		}
	}
	dailyLimit := 0
	if policy.CheckinDailyLimit != nil {
		dailyLimit = *policy.CheckinDailyLimit
	}
	pointsEnabled := policyAllowsUserAccess(policy)
	checkinEnabled := pointsEnabled && policy.CheckinEnabled
	checkinAvailable := false
	if checkinEnabled {
		checkinAvailable, err = s.Store.CheckinAvailable(r.Context(), p.Session.UserID, now)
		if err != nil {
			s.fail(w, r, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"login_email": publicLoginEmail(loginEmail), "role": p.Session.Role, "theme": p.Session.Theme,
		"language": p.Session.Language, "expires_at": p.Session.ExpiresAt,
		"business_date": s.Store.BusinessDate(now).Format("2006-01-02"),
		"csrf_token":    security.CSRFToken(p.Token, s.Config.SessionSecret), "account": publicAccountFrom(account),
		"checkin":            map[string]any{"count": count, "awarded_microusd": awarded},
		"yesterday_snapshot": snapshotValue,
		"features": map[string]any{
			"points_enabled":      pointsEnabled,
			"checkin_enabled":     checkinEnabled,
			"checkin_daily_limit": dailyLimit,
			"checkin_available":   checkinAvailable,
		},
	})
}

type publicAccount struct {
	TotalPointsHundredths        int64     `json:"total_points_hundredths"`
	TotalSpendMicroUSD           int64     `json:"total_spend_microusd"`
	SettledCheckinRewardMicroUSD int64     `json:"settled_checkin_reward_microusd"`
	CreatedAt                    time.Time `json:"created_at"`
	UpdatedAt                    time.Time `json:"updated_at"`
}

func publicAccountFrom(account domain.Account) publicAccount {
	return publicAccount{
		TotalPointsHundredths:        account.TotalPointsHundredths,
		TotalSpendMicroUSD:           account.TotalSpendMicroUSD,
		SettledCheckinRewardMicroUSD: account.SettledCheckinRewardMicroUSD,
		CreatedAt:                    account.CreatedAt,
		UpdatedAt:                    account.UpdatedAt,
	}
}

func publicLoginEmail(loginEmail string) string {
	loginEmail = strings.TrimSpace(loginEmail)
	if loginEmail == "" {
		return "未设置登录邮箱"
	}
	return loginEmail
}

func publicSnapshot(snapshot store.Snapshot) map[string]any {
	return map[string]any{
		"business_date":             snapshot.BusinessDate,
		"actual_cost_microusd":      snapshot.ActualCostMicroUSD,
		"awarded_points_hundredths": snapshot.AwardedPointsHundredths,
		"revision":                  snapshot.Revision,
		"status":                    snapshot.Status,
		"updated_at":                snapshot.UpdatedAt,
	}
}

func (s *Server) adminMe(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	loginEmail, err := s.loginEmail(r.Context(), p.Session.UserID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	policy, err := s.currentPolicy(r.Context(), time.Now())
	if errors.Is(err, domain.ErrNotFound) {
		policy = domain.Policy{}
		err = nil
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"login_email": publicLoginEmail(loginEmail), "role": p.Session.Role, "theme": p.Session.Theme,
		"language": p.Session.Language, "expires_at": p.Session.ExpiresAt,
		"csrf_token": security.CSRFToken(p.Token, s.Config.SessionSecret),
		"features": map[string]any{
			"points_enabled":  policyAllowsUserAccess(policy),
			"checkin_enabled": policy.Enabled && policy.CheckinEnabled,
		},
	})
}

func (s *Server) ledger(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	beforeID, err := s.decodeLedgerPageCursor(r.URL.Query().Get("cursor"), p.Session.UserID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_cursor", "Invalid pagination cursor")
		return
	}
	entries, err := s.Store.LedgerPage(r.Context(), p.Session.UserID, userRecordPageSize+1, beforeID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	hasNext := len(entries) > userRecordPageSize
	if hasNext {
		entries = entries[:userRecordPageSize]
	}
	items := make([]publicLedgerEntry, 0, len(entries))
	for _, entry := range entries {
		items = append(items, publicLedgerEntry{
			Kind: entry.Kind, DeltaPointsHundredths: entry.DeltaPointsHundredths,
			TotalAfterHundredths: entry.TotalAfterHundredths,
			BusinessDate:         entry.BusinessDate, CreatedAt: entry.CreatedAt,
			AwardedAt: entry.AwardedAt,
		})
	}
	nextCursor := ""
	if hasNext {
		nextCursor, err = s.encodeLedgerPageCursor(p.Session.UserID, entries[len(entries)-1].ID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, publicCursorPage[publicLedgerEntry]{Items: items, NextCursor: nextCursor})
}

type publicLedgerEntry struct {
	Kind                  string     `json:"kind"`
	DeltaPointsHundredths int64      `json:"delta_points_hundredths"`
	TotalAfterHundredths  int64      `json:"total_after_hundredths"`
	BusinessDate          *time.Time `json:"business_date,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	AwardedAt             time.Time  `json:"awarded_at"`
}

func (s *Server) dailyPoints(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	items, err := s.Store.DailyPoints(r.Context(), p.Session.UserID, queryDays(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) checkin(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	eventID, err := idempotencyKey(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "idempotency_required", err.Error())
		return
	}
	result, err := s.Store.CheckIn(r.Context(), p.Session.UserID, eventID, time.Now())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, publicCheckinResult{
		Ordinal: result.Ordinal, RewardMicroUSD: result.RewardMicroUSD,
		DeliveryStatus: result.DeliveryStatus,
	})
}

type publicCheckinResult struct {
	Ordinal        int    `json:"ordinal"`
	RewardMicroUSD int64  `json:"reward_microusd"`
	DeliveryStatus string `json:"delivery_status"`
}

func (s *Server) balanceGrants(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	items, err := s.Store.ListBalanceGrants(r.Context(), p.Session.UserID, false, queryLimit(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	result := make([]publicBalanceGrant, 0, len(items))
	for _, item := range items {
		result = append(result, publicBalanceGrant{AmountMicroUSD: item.AmountMicroUSD,
			Kind: item.Kind, Status: item.Status, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt})
	}
	writeJSON(w, http.StatusOK, result)
}

type publicBalanceGrant struct {
	AmountMicroUSD int64     `json:"amount_microusd"`
	Kind           string    `json:"kind"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	if err := s.Store.DeleteSession(r.Context(), p.Token); err != nil {
		s.fail(w, r, err)
		return
	}
	s.clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]bool{"logged_out": true})
}

func (s *Server) policies(w http.ResponseWriter, r *http.Request) {
	items, err := s.Store.ListPolicies(r.Context(), queryLimit(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

type policyRequest struct {
	EffectiveDate                   string        `json:"effective_date"`
	Enabled                         bool          `json:"enabled"`
	Mode                            string        `json:"mode"`
	Basis                           string        `json:"basis"`
	CheckinEnabled                  bool          `json:"checkin_enabled"`
	CheckinDailyLimit               *int          `json:"checkin_daily_limit"`
	MinimumCheckinSpendMicroUSD     int64         `json:"minimum_checkin_spend_microusd"`
	CheckinPlatformDailyCapMicroUSD *int64        `json:"checkin_platform_daily_cap_microusd"`
	CheckinUserDailyCapMicroUSD     *int64        `json:"checkin_user_daily_cap_microusd"`
	CheckinSingleAwardCapMicroUSD   *int64        `json:"checkin_single_award_cap_microusd"`
	PointsPerUSDHundredths          *int64        `json:"points_per_usd_hundredths"`
	RefreshMinute                   *int          `json:"refresh_minute"`
	Tiers                           []domain.Tier `json:"tiers"`
}

func (s *Server) createPolicy(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	var request policyRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid policy")
		return
	}
	effectiveDate, err := time.ParseInLocation("2006-01-02", request.EffectiveDate, s.Config.Timezone)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_effective_date", "Invalid effective date")
		return
	}
	policy, err := s.Store.CreatePolicy(r.Context(), request.toPolicy(effectiveDate, p.Session.UserID), time.Now())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, policy)
}

func (request policyRequest) toPolicy(effectiveDate time.Time, actorID int64) domain.Policy {
	mode := request.Mode
	if mode == "" {
		mode = domain.PolicyModeAllUsers
	}
	basis := request.Basis
	if basis == "" || mode == domain.PolicyModeConsumerOnly {
		basis = domain.PolicyBasisYesterday
	}
	dailyLimit := request.CheckinDailyLimit
	if request.CheckinEnabled && dailyLimit == nil {
		defaultLimit := 1
		dailyLimit = &defaultLimit
	}
	refreshMinute := 5
	if request.RefreshMinute != nil {
		refreshMinute = *request.RefreshMinute
	}
	pointsPerUSDHundredths := int64(1_000)
	if request.PointsPerUSDHundredths != nil {
		pointsPerUSDHundredths = *request.PointsPerUSDHundredths
	}
	return domain.Policy{
		EffectiveDate: effectiveDate, Enabled: request.Enabled, Mode: mode, Basis: basis,
		CheckinEnabled: request.CheckinEnabled, CheckinDailyLimit: dailyLimit,
		MinimumCheckinSpendMicroUSD:     request.MinimumCheckinSpendMicroUSD,
		CheckinPlatformDailyCapMicroUSD: request.CheckinPlatformDailyCapMicroUSD,
		CheckinUserDailyCapMicroUSD:     request.CheckinUserDailyCapMicroUSD,
		CheckinSingleAwardCapMicroUSD:   request.CheckinSingleAwardCapMicroUSD,
		PointsPerUSDHundredths:          pointsPerUSDHundredths, RefreshMinute: refreshMinute,
		CreatedBy: actorID, Tiers: request.Tiers,
	}
}

func queryLimit(r *http.Request) int {
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || limit <= 0 || limit > 200 {
		return 50
	}
	return limit
}

func queryDays(r *http.Request) int {
	days, err := strconv.Atoi(r.URL.Query().Get("days"))
	if err != nil || days <= 0 || days > 90 {
		return 30
	}
	return days
}

func idempotencyKey(r *http.Request) (string, error) {
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(key) < 16 || len(key) > 128 {
		return "", fmt.Errorf("Idempotency-Key must contain 16 to 128 characters")
	}
	for _, value := range key {
		if value < 0x21 || value > 0x7e {
			return "", fmt.Errorf("Idempotency-Key contains invalid characters")
		}
	}
	return key, nil
}
