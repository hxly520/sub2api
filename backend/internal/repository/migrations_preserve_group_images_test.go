package repository

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func groupImagePreservationFS(t *testing.T) fstest.MapFS {
	t.Helper()
	content, err := migrations.FS.ReadFile(geminiImageGenerationGroupsMigration)
	require.NoError(t, err)
	require.Equal(t, geminiImageGenerationGroupsMigrationChecksum, migrationChecksum(strings.TrimSpace(string(content))))
	return fstest.MapFS{geminiImageGenerationGroupsMigration: &fstest.MapFile{Data: content}}
}

func expectGroupImageMigrationUnlock(mock sqlmock.Sqlmock) {
	mock.ExpectExec("SELECT pg_advisory_unlock\\(\\$1\\)").
		WithArgs(migrationsAdvisoryLockID).WillReturnResult(sqlmock.NewResult(0, 1))
}

func TestApplyMigrationsFS_PreservesGroupImageConfiguration(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	fsys := groupImagePreservationFS(t)
	prepareMigrationsBootstrapExpectations(mock)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT checksum FROM schema_migrations WHERE filename = $1")).
		WithArgs(geminiImageGenerationGroupsMigration).WillReturnError(sql.ErrNoRows)
	mock.ExpectBegin()
	// There is deliberately no UPDATE expectation: group rows are never touched.
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO schema_migrations (filename, checksum) VALUES ($1, $2)")).
		WithArgs(geminiImageGenerationGroupsMigration, geminiImageGenerationGroupsMigrationChecksum).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	// The exception must not swallow a different migration with the same number.
	const adjacentName = "246_z_other_group_configuration.sql"
	const adjacentSQL = "SELECT 246;"
	fsys[adjacentName] = &fstest.MapFile{Data: []byte(adjacentSQL)}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT checksum FROM schema_migrations WHERE filename = $1")).
		WithArgs(adjacentName).WillReturnError(sql.ErrNoRows)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(adjacentSQL)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO schema_migrations (filename, checksum) VALUES ($1, $2)")).
		WithArgs(adjacentName, migrationChecksum(adjacentSQL)).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	expectGroupImageMigrationUnlock(mock)
	require.NoError(t, applyMigrationsFS(context.Background(), db, fsys))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyMigrationsFS_PreservedGroupImagesValidateHistory(t *testing.T) {
	for _, tc := range []struct {
		name, stored string
		wantError    bool
	}{
		{name: "already handled is idempotent", stored: geminiImageGenerationGroupsMigrationChecksum},
		{name: "mismatched history fails closed", stored: "unrecognized-checksum", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			fsys := groupImagePreservationFS(t)
			prepareMigrationsBootstrapExpectations(mock)
			mock.ExpectQuery(regexp.QuoteMeta("SELECT checksum FROM schema_migrations WHERE filename = $1")).
				WithArgs(geminiImageGenerationGroupsMigration).
				WillReturnRows(sqlmock.NewRows([]string{"checksum"}).AddRow(tc.stored))
			expectGroupImageMigrationUnlock(mock)
			err = applyMigrationsFS(context.Background(), db, fsys)
			if tc.wantError {
				require.ErrorContains(t, err, "checksum mismatch")
			} else {
				require.NoError(t, err)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestApplyMigrationsFS_UnknownGroupImageRewriteFailsClosed(t *testing.T) {
	for _, content := range []string{"UPDATE groups SET allow_image_generation = true;", "   \n\t"} {
		t.Run(content, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			prepareMigrationsBootstrapExpectations(mock)
			expectGroupImageMigrationUnlock(mock)
			fsys := fstest.MapFS{geminiImageGenerationGroupsMigration: &fstest.MapFile{Data: []byte(content)}}
			err = applyMigrationsFS(context.Background(), db, fsys)
			require.ErrorContains(t, err, "unreviewed configuration rewrite checksum")
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestApplyMigrationsFS_PreservedGroupImageRecordIsAtomic(t *testing.T) {
	for _, stage := range []string{"begin", "record", "commit"} {
		t.Run(stage, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			fsys := groupImagePreservationFS(t)
			prepareMigrationsBootstrapExpectations(mock)
			mock.ExpectQuery(regexp.QuoteMeta("SELECT checksum FROM schema_migrations WHERE filename = $1")).
				WithArgs(geminiImageGenerationGroupsMigration).WillReturnError(sql.ErrNoRows)
			if stage == "begin" {
				mock.ExpectBegin().WillReturnError(errors.New("transaction unavailable"))
			} else {
				mock.ExpectBegin()
				record := mock.ExpectExec(regexp.QuoteMeta("INSERT INTO schema_migrations (filename, checksum) VALUES ($1, $2)")).
					WithArgs(geminiImageGenerationGroupsMigration, geminiImageGenerationGroupsMigrationChecksum)
				if stage == "record" {
					record.WillReturnError(errors.New("record unavailable"))
					mock.ExpectRollback()
				} else {
					record.WillReturnResult(sqlmock.NewResult(1, 1))
					mock.ExpectCommit().WillReturnError(errors.New("commit unavailable"))
				}
			}
			expectGroupImageMigrationUnlock(mock)
			require.ErrorContains(t, applyMigrationsFS(context.Background(), db, fsys), stage+" migration")
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
