//go:build integration

package repository

import (
	"context"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func (s *APIKeyRepoSuite) TestRetiredKeysCannotLoadAsOrdinaryKeys() {
	// The current fresh schema deliberately omits this audit-only column.
	// Add it transactionally to reproduce an upgraded historical database.
	_, err := s.repo.sql.ExecContext(s.ctx, `ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS key_type VARCHAR(16) NOT NULL DEFAULT 'standard'`)
	s.Require().NoError(err)
	user := s.mustCreateUser("retired-key-auth@test.com")
	ordinary := &service.APIKey{UserID: user.ID, Key: "sk-ordinary-retirement", Name: "ordinary", Status: service.StatusActive}
	retired := &service.APIKey{UserID: user.ID, Key: "sk-retired-product", Name: "retired", Status: service.StatusActive}
	s.Require().NoError(s.repo.Create(s.ctx, ordinary))
	s.Require().NoError(s.repo.Create(s.ctx, retired))
	_, err = s.repo.sql.ExecContext(s.ctx, `UPDATE api_keys SET key_type = 'link' WHERE id = $1`, retired.ID)
	s.Require().NoError(err)

	_, err = s.repo.GetByID(s.ctx, retired.ID)
	s.Require().ErrorIs(err, service.ErrAPIKeyNotFound)
	_, err = s.repo.GetByKey(s.ctx, retired.Key)
	s.Require().ErrorIs(err, service.ErrAPIKeyNotFound)
	_, err = s.repo.GetByKeyForAuth(s.ctx, retired.Key)
	s.Require().ErrorIs(err, service.ErrAPIKeyNotFound)
	got, err := s.repo.GetByKeyForAuth(s.ctx, ordinary.Key)
	s.Require().NoError(err)
	s.Require().Equal(ordinary.ID, got.ID)

	keys, page, err := s.repo.ListByUserID(s.ctx, user.ID, pagination.PaginationParams{Page: 1, PageSize: 20}, service.APIKeyListFilters{})
	s.Require().NoError(err)
	s.Require().Equal(int64(1), page.Total)
	s.Require().Len(keys, 1)
	s.Require().Equal(ordinary.ID, keys[0].ID)
}

func TestAPIKeyRetirement_PostgresBarrierWorksWithAndWithoutLegacyColumn(t *testing.T) {
	ctx := context.Background()
	tx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `CREATE TEMPORARY TABLE api_keys (id BIGINT PRIMARY KEY, deleted_at TIMESTAMPTZ) ON COMMIT DROP`)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `INSERT INTO api_keys (id) VALUES (1)`)
	require.NoError(t, err)
	require.NoError(t, requireStandardUsageBillingAPIKey(ctx, tx, 1), "fresh schema has no retired discriminator")

	selector := entsql.Dialect(dialect.Postgres).Select("id").From(entsql.Table("api_keys").As("auth_key"))
	standardAPIKeyPredicate(selector)
	query, args := selector.Query()
	var id int64
	require.NoError(t, tx.QueryRowContext(ctx, query, args...).Scan(&id))
	require.Equal(t, int64(1), id)

	_, err = tx.ExecContext(ctx, `ALTER TABLE api_keys ADD COLUMN key_type VARCHAR(16) NOT NULL DEFAULT 'standard'`)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `INSERT INTO api_keys (id, key_type, deleted_at) VALUES (2, 'link', NULL), (3, 'link', NOW()), (4, 'standard', NOW())`)
	require.NoError(t, err)
	for _, retiredID := range []int64{2, 3} {
		require.ErrorIs(t, requireStandardUsageBillingAPIKey(ctx, tx, retiredID), service.ErrAPIKeyNotFound,
			"active and soft-deleted retired keys must both remain isolated from owner funds")
	}
	require.NoError(t, requireStandardUsageBillingAPIKey(ctx, tx, 4), "ordinary in-flight usage still settles after soft deletion")

	rows, err := tx.QueryContext(ctx, query, args...)
	require.NoError(t, err)
	var ids []int64
	for rows.Next() {
		require.NoError(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	require.ElementsMatch(t, []int64{1, 4}, ids)
}
