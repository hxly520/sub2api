package store

import "context"

func (s *Store) LoginEmail(ctx context.Context, userID int64) (string, error) {
	var loginEmail string
	err := s.DB.QueryRow(ctx, `SELECT COALESCE(email,'') FROM users WHERE id=$1 AND deleted_at IS NULL`, userID).Scan(&loginEmail)
	return loginEmail, translateNotFound(err)
}
