package store

import "context"

func (s *Store) Username(ctx context.Context, userID int64) (string, error) {
	var username string
	err := s.DB.QueryRow(ctx, `SELECT username FROM users WHERE id=$1 AND deleted_at IS NULL`, userID).Scan(&username)
	return username, translateNotFound(err)
}
