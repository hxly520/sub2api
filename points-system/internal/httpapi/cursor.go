package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hxly520/sub2api/points-system/internal/store"
)

const (
	pageCursorPrefix           = "pc1"
	pageCursorMaxLength        = 1024
	pageCursorScopeLedger      = "ledger"
	pageCursorScopeGrants      = "checkin_grants"
	pageCursorScopeAdminGrants = "admin_checkin_grants"
)

var errInvalidPageCursor = errors.New("invalid page cursor")

const (
	userRecordPageSize = 10
	adminGrantPageSize = 50
)

type publicCursorPage[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor"`
}

type pageCursorPayload struct {
	Version        int    `json:"v"`
	Scope          string `json:"s"`
	UserID         int64  `json:"u"`
	LedgerID       int64  `json:"l,omitempty"`
	GrantCreatedAt int64  `json:"t,omitempty"`
	GrantID        string `json:"g,omitempty"`
}

func (s *Server) encodeLedgerPageCursor(userID, ledgerID int64) (string, error) {
	if userID <= 0 || ledgerID <= 0 {
		return "", errInvalidPageCursor
	}
	return s.sealPageCursor(pageCursorPayload{
		Version: 1, Scope: pageCursorScopeLedger, UserID: userID, LedgerID: ledgerID,
	})
}

func (s *Server) decodeLedgerPageCursor(raw string, userID int64) (*int64, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	payload, err := s.openPageCursor(raw, pageCursorScopeLedger, userID)
	if err != nil || payload.LedgerID <= 0 || payload.GrantCreatedAt != 0 || payload.GrantID != "" {
		return nil, errInvalidPageCursor
	}
	return &payload.LedgerID, nil
}

func (s *Server) encodeGrantPageCursor(userID int64, createdAt time.Time, id string) (string, error) {
	return s.encodeBalanceGrantPageCursor(pageCursorScopeGrants, userID, createdAt, id)
}

func (s *Server) encodeAdminGrantPageCursor(userID int64, createdAt time.Time, id string) (string, error) {
	return s.encodeBalanceGrantPageCursor(pageCursorScopeAdminGrants, userID, createdAt, id)
}

func (s *Server) encodeBalanceGrantPageCursor(scope string, userID int64, createdAt time.Time, id string) (string, error) {
	parsedID, err := uuid.Parse(id)
	if userID <= 0 || createdAt.IsZero() || err != nil {
		return "", errInvalidPageCursor
	}
	return s.sealPageCursor(pageCursorPayload{
		Version: 1, Scope: scope, UserID: userID,
		GrantCreatedAt: createdAt.UTC().UnixMicro(), GrantID: parsedID.String(),
	})
}

func (s *Server) decodeGrantPageCursor(raw string, userID int64) (*store.BalanceGrantPageCursor, error) {
	return s.decodeBalanceGrantPageCursor(raw, pageCursorScopeGrants, userID)
}

func (s *Server) decodeAdminGrantPageCursor(raw string, userID int64) (*store.BalanceGrantPageCursor, error) {
	return s.decodeBalanceGrantPageCursor(raw, pageCursorScopeAdminGrants, userID)
}

func (s *Server) decodeBalanceGrantPageCursor(raw, scope string, userID int64) (*store.BalanceGrantPageCursor, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	payload, err := s.openPageCursor(raw, scope, userID)
	parsedID, idErr := uuid.Parse(payload.GrantID)
	if err != nil || payload.LedgerID != 0 || payload.GrantCreatedAt <= 0 || idErr != nil {
		return nil, errInvalidPageCursor
	}
	return &store.BalanceGrantPageCursor{
		CreatedAt: time.UnixMicro(payload.GrantCreatedAt).UTC(), ID: parsedID.String(),
	}, nil
}

func (s *Server) sealPageCursor(payload pageCursorPayload) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(body)
	signingInput := pageCursorPrefix + "." + encoded
	mac := hmac.New(sha256.New, s.Config.SessionSecret)
	_, _ = mac.Write([]byte(signingInput))
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (s *Server) openPageCursor(raw, scope string, userID int64) (pageCursorPayload, error) {
	if len(raw) == 0 || len(raw) > pageCursorMaxLength || raw != strings.TrimSpace(raw) {
		return pageCursorPayload{}, errInvalidPageCursor
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 || parts[0] != pageCursorPrefix {
		return pageCursorPayload{}, errInvalidPageCursor
	}
	provided, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return pageCursorPayload{}, errInvalidPageCursor
	}
	mac := hmac.New(sha256.New, s.Config.SessionSecret)
	_, _ = mac.Write([]byte(parts[0] + "." + parts[1]))
	if !hmac.Equal(mac.Sum(nil), provided) {
		return pageCursorPayload{}, errInvalidPageCursor
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return pageCursorPayload{}, errInvalidPageCursor
	}
	var payload pageCursorPayload
	if err := json.Unmarshal(body, &payload); err != nil || payload.Version != 1 ||
		payload.Scope != scope || payload.UserID != userID || userID <= 0 {
		return pageCursorPayload{}, errInvalidPageCursor
	}
	return payload, nil
}
