package httpapi

import (
	"context"
	"encoding/json"
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
	account, err := s.Store.Account(r.Context(), p.Session.UserID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	count, awarded, err := s.Store.CheckinStatus(r.Context(), p.Session.UserID, time.Now())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	yesterday := s.Store.BusinessDate(time.Now()).AddDate(0, 0, -1)
	snapshot, snapshotErr := s.Store.Snapshot(r.Context(), p.Session.UserID, yesterday)
	if snapshotErr != nil && !errors.Is(snapshotErr, domain.ErrNotFound) {
		s.fail(w, r, snapshotErr)
		return
	}
	var snapshotValue any
	if snapshotErr == nil {
		snapshotValue = publicSnapshot(snapshot)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user_id": p.Session.UserID, "role": p.Session.Role, "theme": p.Session.Theme,
		"language": p.Session.Language, "expires_at": p.Session.ExpiresAt,
		"csrf_token": security.CSRFToken(p.Token, s.Config.SessionSecret), "account": account,
		"checkin":            map[string]any{"count": count, "awarded_microusd": awarded},
		"yesterday_snapshot": snapshotValue,
	})
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

func (s *Server) ledger(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	entries, err := s.Store.Ledger(r.Context(), p.Session.UserID, queryLimit(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, entries)
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
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) balanceGrants(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	items, err := s.Store.ListBalanceGrants(r.Context(), p.Session.UserID, false, queryLimit(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
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

func (s *Server) manualGrant(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	key, err := idempotencyKey(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "idempotency_required", err.Error())
		return
	}
	var request struct {
		UserID int64  `json:"user_id"`
		Amount string `json:"amount"`
		Reason string `json:"reason"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid grant")
		return
	}
	amountMicroUSD, amountErr := parseDollarAmount(request.Amount)
	if request.UserID <= 0 || amountErr != nil || strings.TrimSpace(request.Reason) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid grant")
		return
	}
	if len(request.Reason) > 500 {
		writeError(w, http.StatusBadRequest, "invalid_request", "Reason is too long")
		return
	}
	result, err := s.Store.CreateManualBalanceGrant(r.Context(), store.ManualBalanceGrantRequest{
		UserID: request.UserID, AmountMicroUSD: amountMicroUSD, ActorID: p.Session.UserID,
		IdempotencyKey: key, Reason: request.Reason, Now: time.Now(),
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) adminBalanceGrants(w http.ResponseWriter, r *http.Request) {
	items, err := s.Store.ListBalanceGrants(r.Context(), 0, true, queryLimit(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) retryBalanceGrant(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	id := r.PathValue("id")
	if err := s.Store.RetryBalanceGrant(r.Context(), id, p.Session.UserID, time.Now()); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"id": id, "status": "pending"})
}

func (s *Server) reverseBalanceGrant(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	id := r.PathValue("id")
	var request struct {
		Reason string `json:"reason"`
	}
	if err := decodeJSON(w, r, &request); err != nil || strings.TrimSpace(request.Reason) == "" || len(request.Reason) > 500 {
		writeError(w, http.StatusBadRequest, "invalid_request", "A reversal reason is required")
		return
	}
	if err := s.Store.ReverseBalanceGrant(r.Context(), id, request.Reason, p.Session.UserID, time.Now()); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"id": id, "status": "reversal_pending"})
}

func (s *Server) refreshSnapshots(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	var request struct {
		BusinessDate string `json:"business_date"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid refresh request")
		return
	}
	date, err := time.ParseInLocation("2006-01-02", request.BusinessDate, s.Config.Timezone)
	if err != nil || !date.Before(s.Store.BusinessDate(time.Now())) {
		writeError(w, http.StatusBadRequest, "invalid_business_date", "Business date must be before today")
		return
	}
	result, err := s.Store.ProcessUsageDay(r.Context(), date)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	detail, err := json.Marshal(result)
	if err != nil {
		s.fail(w, r, fmt.Errorf("marshal snapshot refresh audit: %w", err))
		return
	}
	auditCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Store.Audit(auditCtx, p.Session.UserID, "snapshot.refresh", "business_date", request.BusinessDate,
		r.Header.Get("X-Request-ID"), detail); err != nil {
		s.fail(w, r, fmt.Errorf("write snapshot refresh audit: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func queryLimit(r *http.Request) int {
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || limit <= 0 || limit > 200 {
		return 50
	}
	return limit
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

func parseDollarAmount(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	parts := strings.Split(raw, ".")
	if len(parts) > 2 || parts[0] == "" || len(parts[0]) > 12 {
		return 0, fmt.Errorf("invalid monetary amount")
	}
	for _, value := range parts[0] {
		if value < '0' || value > '9' {
			return 0, fmt.Errorf("invalid monetary amount")
		}
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
		if len(fraction) > 2 {
			return 0, fmt.Errorf("invalid monetary amount")
		}
		for _, value := range fraction {
			if value < '0' || value > '9' {
				return 0, fmt.Errorf("invalid monetary amount")
			}
		}
	}
	for len(fraction) < 2 {
		fraction += "0"
	}
	dollars, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || dollars > (1<<63-1)/1_000_000 {
		return 0, fmt.Errorf("invalid monetary amount")
	}
	cents := int64(0)
	if fraction != "" {
		cents, err = strconv.ParseInt(fraction, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid monetary amount")
		}
	}
	amount := dollars*1_000_000 + cents*domain.MicroUSDPerCent
	if amount <= 0 {
		return 0, fmt.Errorf("invalid monetary amount")
	}
	return amount, nil
}
