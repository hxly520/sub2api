package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestRemoveAuthorizedGroupFreezesExistingLinkCardsAtomically(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM link_card_group_authorizations WHERE group_id=\$1`).
		WithArgs(int64(8)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)UPDATE api_keys.*link_state=CASE.*WHERE group_id=\$4 AND key_type=\$5`).
		WithArgs(
			service.StatusAPIKeyDisabled,
			service.LinkCardStatePendingActivation,
			service.LinkCardStateFrozen,
			int64(8),
			service.APIKeyTypeLink,
			service.LinkCardStateRefunded,
			service.LinkCardStateRevoked,
		).
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectCommit()

	repo := &linkCardRepository{db: db}
	require.NoError(t, repo.RemoveAuthorizedGroup(context.Background(), 8))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRemoveAuthorizedGroupRollsBackAuthorizationWhenFreezeFails(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	freezeErr := errors.New("freeze failed")
	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM link_card_group_authorizations WHERE group_id=\$1`).
		WithArgs(int64(8)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)UPDATE api_keys.*WHERE group_id=\$4 AND key_type=\$5`).
		WithArgs(
			service.StatusAPIKeyDisabled,
			service.LinkCardStatePendingActivation,
			service.LinkCardStateFrozen,
			int64(8),
			service.APIKeyTypeLink,
			service.LinkCardStateRefunded,
			service.LinkCardStateRevoked,
		).
		WillReturnError(freezeErr)
	mock.ExpectRollback()

	repo := &linkCardRepository{db: db}
	err = repo.RemoveAuthorizedGroup(context.Background(), 8)
	require.ErrorIs(t, err, freezeErr)
	require.NoError(t, mock.ExpectationsWereMet())
}
