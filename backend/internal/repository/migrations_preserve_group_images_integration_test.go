//go:build integration

package repository

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/url"
	"testing"
	"testing/fstest"
	"time"

	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestMigrationsRunner_PreservesExistingGroupImageConfiguration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db := isolatedGroupImageMigrationDB(t, ctx)
	// An update audit proves that preserving booleans does not hide trigger side effects.
	_, err := db.ExecContext(ctx, `
CREATE TABLE groups (id BIGINT PRIMARY KEY, name TEXT NOT NULL, platform TEXT NOT NULL, allow_image_generation BOOLEAN NOT NULL, updated_at TIMESTAMPTZ NOT NULL DEFAULT TIMESTAMPTZ '2026-10-01 00:00:00+00', metadata JSONB NOT NULL DEFAULT '{"preserve":"unchanged"}'::jsonb);
CREATE TABLE group_image_update_audit (group_id BIGINT NOT NULL);
CREATE FUNCTION audit_group_image_update() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO group_image_update_audit (group_id) VALUES (NEW.id);
    RETURN NEW;
END;
$$;
CREATE TRIGGER audit_group_image_update AFTER UPDATE ON groups FOR EACH ROW EXECUTE FUNCTION audit_group_image_update();
INSERT INTO groups (id, name, platform, allow_image_generation) VALUES
    (1, 'gemini-disabled', 'gemini', false), (2, 'gemini-enabled', 'gemini', true),
    (3, 'antigravity-disabled', 'antigravity', false), (4, 'antigravity-enabled', 'antigravity', true),
    (5, 'composite-disabled', 'composite', false), (6, 'composite-enabled', 'composite', true),
    (7, 'openai-disabled', 'openai', false), (8, 'openai-enabled', 'openai', true);
`)
	require.NoError(t, err)
	const migrationName = "246_enable_gemini_image_generation_groups.sql"
	const originalChecksum = "87267be7ab657173768989e2b0b105c5569c5930a7f1ed124f4cf81e5198297c"
	originalSQL, err := dbmigrations.FS.ReadFile(migrationName)
	require.NoError(t, err)
	require.Equal(t, originalChecksum, migrationChecksum(string(originalSQL)))
	// A different complete filename with the same numeric prefix must still run.
	const otherName = "246_zz_preserve_configuration_fixture.sql"
	const otherSQL = "CREATE TABLE other_246_executed (id BIGINT PRIMARY KEY); INSERT INTO other_246_executed (id) VALUES (1);"
	fsys := fstest.MapFS{migrationName: {Data: originalSQL}, otherName: {Data: []byte(otherSQL)}}
	groupSnapshot := func() string {
		t.Helper()
		var value string
		require.NoError(t, db.QueryRowContext(ctx, "SELECT jsonb_agg(to_jsonb(g) ORDER BY id)::text FROM groups g").Scan(&value))
		return value
	}
	ledgerSnapshot := func() string {
		t.Helper()
		var value string
		require.NoError(t, db.QueryRowContext(ctx, "SELECT jsonb_agg(to_jsonb(m) ORDER BY filename)::text FROM schema_migrations m").Scan(&value))
		return value
	}
	assertAuditCount := func(want int) {
		t.Helper()
		var count int
		require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM group_image_update_audit").Scan(&count))
		require.Equal(t, want, count, "migration must not emit group update side effects")
	}
	before := groupSnapshot()
	require.NoError(t, applyMigrationsFS(ctx, db, fsys))
	require.JSONEq(t, before, groupSnapshot(), "all existing row fields and true/false flags must remain unchanged")
	assertAuditCount(0)
	var checksum string
	require.NoError(t, db.QueryRowContext(ctx, "SELECT checksum FROM schema_migrations WHERE filename = $1", migrationName).Scan(&checksum))
	require.Equal(t, originalChecksum, checksum)
	require.NoError(t, db.QueryRowContext(ctx, "SELECT checksum FROM schema_migrations WHERE filename = $1", otherName).Scan(&checksum))
	require.Equal(t, migrationChecksum(otherSQL), checksum)
	var count int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&count))
	require.Equal(t, 2, count)
	require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM other_246_executed").Scan(&count))
	require.Equal(t, 1, count)
	ledgerBefore := ledgerSnapshot()
	require.NoError(t, applyMigrationsFS(ctx, db, fsys))
	require.JSONEq(t, before, groupSnapshot())
	require.JSONEq(t, ledgerBefore, ledgerSnapshot(), "restarts must preserve migration identity and applied_at")
	assertAuditCount(0)
	// Later administrator decisions in both directions must survive restarts.
	_, err = db.ExecContext(ctx, "UPDATE groups SET allow_image_generation = NOT allow_image_generation WHERE id IN (1, 2)")
	require.NoError(t, err)
	afterAdminChange := groupSnapshot()
	require.NotEqual(t, before, afterAdminChange)
	assertAuditCount(2)
	require.NoError(t, applyMigrationsFS(ctx, db, fsys))
	require.JSONEq(t, afterAdminChange, groupSnapshot())
	require.JSONEq(t, ledgerBefore, ledgerSnapshot())
	assertAuditCount(2)
}

func isolatedGroupImageMigrationDB(t *testing.T, ctx context.Context) *sql.DB {
	t.Helper()
	require.NotEmpty(t, integrationDSN)
	var suffix [8]byte
	_, err := rand.Read(suffix[:])
	require.NoError(t, err)
	schema := "preserve_group_images_" + hex.EncodeToString(suffix[:])
	quotedSchema := pq.QuoteIdentifier(schema)
	_, err = integrationDB.ExecContext(ctx, "CREATE SCHEMA "+quotedSchema)
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, cleanupErr := integrationDB.ExecContext(cleanupCtx, "DROP SCHEMA IF EXISTS "+quotedSchema+" CASCADE")
		if cleanupErr != nil {
			t.Errorf("drop isolated migration schema: %v", cleanupErr)
		}
	})
	dsn, err := url.Parse(integrationDSN)
	require.NoError(t, err)
	require.Contains(t, []string{"postgres", "postgresql"}, dsn.Scheme)
	params := dsn.Query()
	params.Set("search_path", schema)
	dsn.RawQuery = params.Encode()
	db, err := sql.Open("postgres", dsn.String())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	db.SetMaxOpenConns(1)
	require.NoError(t, db.PingContext(ctx))
	var currentSchema string
	require.NoError(t, db.QueryRowContext(ctx, "SELECT current_schema()").Scan(&currentSchema))
	require.Equal(t, schema, currentSchema)
	// tableExists checks public metadata, while all unqualified work remains
	// isolated. Seed a private baseline so the startup check never reads public rows.
	_, err = db.ExecContext(ctx, atlasSchemaRevisionsTableDDL)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, "INSERT INTO atlas_schema_revisions (version, description, type) VALUES ('fixture', 'isolated fixture', 1)")
	require.NoError(t, err)
	return db
}
