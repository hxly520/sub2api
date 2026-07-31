package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

type publicAdminUserPoints struct {
	LoginEmail                string     `json:"login_email"`
	TotalPointsHundredths     int64      `json:"total_points_hundredths"`
	YesterdayPointsHundredths int64      `json:"yesterday_points_hundredths"`
	TotalSpendMicroUSD        int64      `json:"total_spend_microusd"`
	YesterdaySpendMicroUSD    int64      `json:"yesterday_spend_microusd"`
	SnapshotBusinessDate      *time.Time `json:"snapshot_business_date"`
	SnapshotStatus            string     `json:"snapshot_status"`
}

type publicAdminUserPointsPage struct {
	Items        []publicAdminUserPoints `json:"items"`
	Total        int64                   `json:"total"`
	Limit        int                     `json:"limit"`
	Offset       int                     `json:"offset"`
	BusinessDate time.Time               `json:"business_date"`
}

func (s *Server) adminUserPoints(w http.ResponseWriter, r *http.Request) {
	yesterday := s.Store.BusinessDate(time.Now()).AddDate(0, 0, -1)
	page, err := s.Store.ListAdminUserPoints(r.Context(), yesterday, queryLimit(r), queryOffset(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	response := publicAdminUserPointsPage{
		Items:        make([]publicAdminUserPoints, 0, len(page.Items)),
		Total:        page.Total,
		Limit:        page.Limit,
		Offset:       page.Offset,
		BusinessDate: page.BusinessDate,
	}
	for _, item := range page.Items {
		response.Items = append(response.Items, publicAdminUserPoints{
			LoginEmail:                publicLoginEmail(item.LoginEmail),
			TotalPointsHundredths:     item.TotalPointsHundredths,
			YesterdayPointsHundredths: item.YesterdayPointsHundredths,
			TotalSpendMicroUSD:        item.TotalSpendMicroUSD,
			YesterdaySpendMicroUSD:    item.YesterdaySpendMicroUSD,
			SnapshotBusinessDate:      item.SnapshotBusinessDate,
			SnapshotStatus:            item.SnapshotStatus,
		})
	}
	writeJSON(w, http.StatusOK, response)
}

func queryOffset(r *http.Request) int {
	offset, err := strconv.Atoi(r.URL.Query().Get("offset"))
	if err != nil || offset < 0 || offset > 1_000_000 {
		return 0
	}
	return offset
}

func (s *Server) checkinBalanceGrants(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	cursor, err := s.decodeGrantPageCursor(r.URL.Query().Get("cursor"), p.Session.UserID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_cursor", "Invalid pagination cursor")
		return
	}
	items, err := s.Store.ListCheckinBalanceGrantsPage(r.Context(), p.Session.UserID,
		userRecordPageSize+1, cursor)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	hasNext := len(items) > userRecordPageSize
	if hasNext {
		items = items[:userRecordPageSize]
	}
	result := make([]publicBalanceGrant, 0, len(items))
	for _, item := range items {
		result = append(result, publicBalanceGrant{
			AmountMicroUSD: item.AmountMicroUSD, Kind: item.Kind, Status: item.Status,
			CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
		})
	}
	nextCursor := ""
	if hasNext {
		last := items[len(items)-1]
		nextCursor, err = s.encodeGrantPageCursor(p.Session.UserID, last.CreatedAt, last.ID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, publicCursorPage[publicBalanceGrant]{Items: result, NextCursor: nextCursor})
}

func (s *Server) adminCheckinBalanceGrants(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	cursor, err := s.decodeAdminGrantPageCursor(r.URL.Query().Get("cursor"), p.Session.UserID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_cursor", "Invalid pagination cursor")
		return
	}
	items, err := s.Store.ListAdminCheckinBalanceGrantsPage(r.Context(), adminGrantPageSize+1, cursor)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	hasNext := len(items) > adminGrantPageSize
	if hasNext {
		items = items[:adminGrantPageSize]
	}
	result := make([]publicAdminBalanceGrant, 0, len(items))
	for _, item := range items {
		grant := item.Grant
		result = append(result, publicAdminBalanceGrant{
			ID: grant.ID, LoginEmail: publicLoginEmail(item.LoginEmail), AmountMicroUSD: grant.AmountMicroUSD,
			Kind: grant.Kind, Status: grant.Status, Attempts: grant.Attempts, LastError: grant.LastError,
			CreatedAt: grant.CreatedAt, UpdatedAt: grant.UpdatedAt,
		})
	}
	nextCursor := ""
	if hasNext {
		last := items[len(items)-1].Grant
		nextCursor, err = s.encodeAdminGrantPageCursor(p.Session.UserID, last.CreatedAt, last.ID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, publicCursorPage[publicAdminBalanceGrant]{Items: result, NextCursor: nextCursor})
}

type publicAdminBalanceGrant struct {
	ID             string    `json:"id"`
	LoginEmail     string    `json:"login_email"`
	AmountMicroUSD int64     `json:"amount_microusd"`
	Kind           string    `json:"kind"`
	Status         string    `json:"status"`
	Attempts       int       `json:"attempts"`
	LastError      string    `json:"last_error,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (s *Server) adminCheckinBalanceGrantSummary(w http.ResponseWriter, r *http.Request) {
	counts, err := s.Store.SummarizeCheckinBalanceGrants(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"counts": counts})
}

func (s *Server) retryCheckinBalanceGrant(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	id := r.PathValue("id")
	if err := s.Store.RequireCheckinBalanceGrant(r.Context(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.RetryBalanceGrant(r.Context(), id, p.Session.UserID, time.Now()); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"id": id, "status": "pending"})
}

func (s *Server) reverseCheckinBalanceGrant(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r)
	id := r.PathValue("id")
	var request struct {
		Reason string `json:"reason"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "A reversal reason is required")
		return
	}
	request.Reason = strings.TrimSpace(request.Reason)
	if request.Reason == "" || len(request.Reason) > 500 {
		writeError(w, http.StatusBadRequest, "invalid_request", "A reversal reason is required")
		return
	}
	if err := s.Store.RequireCheckinBalanceGrant(r.Context(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.ReverseBalanceGrant(r.Context(), id, request.Reason, p.Session.UserID, time.Now()); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"id": id, "status": "reversal_pending"})
}
