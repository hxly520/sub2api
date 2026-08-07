package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLinkCardMigrationPreservesFinancialInvariants(t *testing.T) {
	raw, err := FS.ReadFile("194_link_cards.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(raw))

	require.Contains(t, sql, "key_type varchar(16) not null default 'standard'")
	require.Contains(t, sql, "check (key_type in ('standard', 'link'))")
	require.Contains(t, sql, "link_total_funded numeric(20, 8) not null default 0")
	require.Contains(t, sql, "link_total_refunded numeric(20, 8) not null default 0")
	require.Contains(t, sql, "unique (scope, actor_user_id, idempotency_key_hash)")
	require.Contains(t, sql, "uq_link_card_ledger_usage_request")
	require.Contains(t, sql, "where request_id is not null and entry_type = 'usage'")
	require.Contains(t, sql, "before update or delete on link_card_ledger")
	require.Contains(t, sql, "raise exception 'link_card_ledger is append-only'")
}

func TestLinkCardMigrationStartsInUserOneDevelopmentMode(t *testing.T) {
	raw, err := FS.ReadFile("194_link_cards.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(raw))

	require.Contains(t, sql, "('link_cards_enabled', 'false'")
	require.Contains(t, sql, "('link_cards_development_mode', 'true'")
	require.Contains(t, sql, "('link_cards_development_user_ids', '[1]'")
}
