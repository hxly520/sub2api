package store

import (
	"context"
	"fmt"
	"time"

	"github.com/hxly520/sub2api/points-system/internal/security"
	"github.com/jackc/pgx/v5"
)

type Session struct {
	UserID    int64
	Role      string
	Theme     string
	Language  string
	ExpiresAt time.Time
}

func (s *Store) ConsumeLaunchTicket(ctx context.Context, claims security.LaunchClaims,
	sessionToken string, sessionTTL time.Duration, now time.Time) (Session, error) {
	userID := claims.Subject
	if userID <= 0 {
		return Session{}, security.ErrInvalidTicket
	}
	ticketExpiry := time.Unix(claims.ExpiresAt, 0)
	sessionTTL = sessionTTLForRole(claims.Role, sessionTTL)
	sessionExpiry := now.Add(sessionTTL)
	if sessionExpiry.Before(now) {
		return Session{}, fmt.Errorf("invalid session expiry")
	}
	session := Session{UserID: userID, Role: claims.Role, Theme: claims.Theme,
		Language: claims.Language, ExpiresAt: sessionExpiry}
	err := s.withSerializableTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO points_launch_ticket_nonces(jti_hash,subject_user_id,role,expires_at)
			VALUES($1,$2,$3,$4)`, security.HashToken(claims.Nonce), userID, claims.Role, ticketExpiry); err != nil {
			return fmt.Errorf("consume launch ticket: %w", err)
		}
		_, err := tx.Exec(ctx, `INSERT INTO points_sessions(token_hash,user_id,role,theme,language,expires_at)
			VALUES($1,$2,$3,$4,$5,$6)`, security.HashToken(sessionToken), userID, claims.Role,
			claims.Theme, claims.Language, sessionExpiry)
		return err
	})
	return session, err
}

func sessionTTLForRole(role string, configured time.Duration) time.Duration {
	const adminMaximum = 30 * time.Minute
	if role == "admin" && configured > adminMaximum {
		return adminMaximum
	}
	return configured
}

func (s *Store) Session(ctx context.Context, token string, now time.Time) (Session, error) {
	var session Session
	err := s.DB.QueryRow(ctx, `UPDATE points_sessions SET last_seen_at=NOW()
		WHERE token_hash=$1 AND expires_at>$2 RETURNING user_id,role,theme,language,expires_at`,
		security.HashToken(token), now).Scan(&session.UserID, &session.Role, &session.Theme,
		&session.Language, &session.ExpiresAt)
	return session, translateNotFound(err)
}

func (s *Store) DeleteSession(ctx context.Context, token string) error {
	_, err := s.DB.Exec(ctx, `DELETE FROM points_sessions WHERE token_hash=$1`, security.HashToken(token))
	return err
}

func (s *Store) CleanupSecurityState(ctx context.Context, now time.Time) error {
	return s.withSerializableTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM points_sessions WHERE expires_at <= $1`, now); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `DELETE FROM points_launch_ticket_nonces
			WHERE expires_at <= $1::timestamptz - INTERVAL '1 day'`, now)
		return err
	})
}
