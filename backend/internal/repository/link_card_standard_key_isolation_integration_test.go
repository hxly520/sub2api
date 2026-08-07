//go:build integration

package repository

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (s *APIKeyRepoSuite) TestStandardKeyManagementExcludesLinkCards() {
	user := s.mustCreateUser("link-card-isolation@test.com")
	standard := s.mustCreateApiKey(user.ID, "sk-standard-isolation", "standard", nil)

	_, err := s.repo.sql.ExecContext(s.ctx, `
		INSERT INTO api_keys (
			user_id, key, name, status, key_type, link_state,
			link_rate_multiplier, link_original_debit, link_total_funded,
			link_total_refunded, link_concurrency, link_rpm_limit, quota, quota_used
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8, 0, 5, 0, $8, 0)
	`, user.ID, "sk-card-isolation-000000000000000000000001", "link-card",
		service.StatusAPIKeyDisabled, service.APIKeyTypeLink, service.LinkCardStatePendingActivation,
		0.08, 100.0)
	s.Require().NoError(err)

	linkKey, err := s.repo.GetByKeyForAuth(s.ctx, "sk-card-isolation-000000000000000000000001")
	s.Require().NoError(err)
	s.Require().True(linkKey.IsLinkKey(), "gateway authentication must retain link-key metadata")

	keys, page, err := s.repo.ListByUserID(
		s.ctx,
		user.ID,
		pagination.PaginationParams{Page: 1, PageSize: 20},
		service.APIKeyListFilters{},
	)
	s.Require().NoError(err)
	s.Require().Equal(int64(1), page.Total)
	s.Require().Len(keys, 1)
	s.Require().Equal(standard.ID, keys[0].ID)

	count, err := s.repo.CountByUserID(s.ctx, user.ID)
	s.Require().NoError(err)
	s.Require().Equal(int64(1), count)

	owned, err := s.repo.VerifyOwnership(s.ctx, user.ID, []int64{standard.ID, linkKey.ID})
	s.Require().NoError(err)
	s.Require().Equal([]int64{standard.ID}, owned)

	_, err = s.repo.GetByID(s.ctx, linkKey.ID)
	s.Require().ErrorIs(err, service.ErrAPIKeyNotFound)
}
