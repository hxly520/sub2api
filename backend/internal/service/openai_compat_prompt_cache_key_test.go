package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
)

func mustRawJSON(t *testing.T, s string) json.RawMessage {
	t.Helper()
	return json.RawMessage(s)
}

func TestInjectOpenAIResponsesPromptCacheKey_CredentialAccountTypes(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name      string
		account   *Account
		wantCache bool
	}{
		{name: "api key", account: &Account{Type: AccountTypeAPIKey}, wantCache: true},
		{name: "service account", account: &Account{Type: AccountTypeServiceAccount}, wantCache: true},
		{name: "oauth", account: &Account{Type: AccountTypeOAuth}, wantCache: false},
		{name: "nil account", account: nil, wantCache: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			body := []byte(`{"model":"gpt-5.6-luna","input":"hello"}`)
			got, err := injectOpenAIResponsesPromptCacheKey(tt.account, body, " stable-cache-key ")
			require.NoError(t, err)
			if tt.wantCache {
				require.JSONEq(t, `{"model":"gpt-5.6-luna","input":"hello","prompt_cache_key":"stable-cache-key"}`, string(got))
			} else {
				require.Equal(t, body, got)
			}
		})
	}
}

func TestInjectOpenAIResponsesPromptCacheKey_PreservesExplicitKey(t *testing.T) {
	t.Parallel()
	body := []byte(`{"model":"gpt-5.6-luna","prompt_cache_key":"client-key"}`)

	got, err := injectOpenAIResponsesPromptCacheKey(
		&Account{Type: AccountTypeServiceAccount},
		body,
		"derived-key",
	)

	require.NoError(t, err)
	require.Equal(t, body, got)
}

func TestShouldAutoInjectPromptCacheKeyForCompat(t *testing.T) {
	require.True(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-5.5"))
	require.True(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-5.5-pro"))
	require.True(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-5.4"))
	require.True(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-5.4-mini"))
	require.True(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-5.2"))
	require.True(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-5.3"))
	require.True(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-5.3-codex"))
	require.True(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-5.3-codex-spark"))
	require.False(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-4o"))
}

func TestDeriveCompatPromptCacheKey_StableAcrossLaterTurns(t *testing.T) {
	base := &apicompat.ChatCompletionsRequest{
		Model: "gpt-5.4",
		Messages: []apicompat.ChatMessage{
			{Role: "system", Content: mustRawJSON(t, `"You are helpful."`)},
			{Role: "user", Content: mustRawJSON(t, `"Hello"`)},
		},
	}
	extended := &apicompat.ChatCompletionsRequest{
		Model: "gpt-5.4",
		Messages: []apicompat.ChatMessage{
			{Role: "system", Content: mustRawJSON(t, `"You are helpful."`)},
			{Role: "user", Content: mustRawJSON(t, `"Hello"`)},
			{Role: "assistant", Content: mustRawJSON(t, `"Hi there!"`)},
			{Role: "user", Content: mustRawJSON(t, `"How are you?"`)},
		},
	}

	k1 := deriveCompatPromptCacheKey(base, "gpt-5.4")
	k2 := deriveCompatPromptCacheKey(extended, "gpt-5.4")
	require.Equal(t, k1, k2, "cache key should be stable across later turns")
	require.NotEmpty(t, k1)
}

func TestDeriveCompatPromptCacheKey_DiffersAcrossSessions(t *testing.T) {
	req1 := &apicompat.ChatCompletionsRequest{
		Model: "gpt-5.4",
		Messages: []apicompat.ChatMessage{
			{Role: "user", Content: mustRawJSON(t, `"Question A"`)},
		},
	}
	req2 := &apicompat.ChatCompletionsRequest{
		Model: "gpt-5.4",
		Messages: []apicompat.ChatMessage{
			{Role: "user", Content: mustRawJSON(t, `"Question B"`)},
		},
	}

	k1 := deriveCompatPromptCacheKey(req1, "gpt-5.4")
	k2 := deriveCompatPromptCacheKey(req2, "gpt-5.4")
	require.NotEqual(t, k1, k2, "different first user messages should yield different keys")
}

func TestDeriveCompatPromptCacheKey_UsesResolvedSparkFamily(t *testing.T) {
	req := &apicompat.ChatCompletionsRequest{
		Model: "gpt-5.3-codex-spark",
		Messages: []apicompat.ChatMessage{
			{Role: "user", Content: mustRawJSON(t, `"Question A"`)},
		},
	}

	k1 := deriveCompatPromptCacheKey(req, "gpt-5.3-codex-spark")
	k2 := deriveCompatPromptCacheKey(req, " openai/gpt-5.3-codex-spark ")
	require.NotEmpty(t, k1)
	require.Equal(t, k1, k2, "resolved spark family should derive a stable compat cache key")
}

func TestDeriveCompatPromptCacheKey_StableAcrossToolOrder(t *testing.T) {
	toolA := apicompat.ChatTool{Type: "function", Function: &apicompat.ChatFunction{
		Name: "alpha", Parameters: mustRawJSON(t, `{"type":"object","properties":{"z":{"type":"string"},"a":{"type":"number"}}}`),
	}}
	toolB := apicompat.ChatTool{Type: "function", Function: &apicompat.ChatFunction{
		Name: "beta", Parameters: mustRawJSON(t, `{"type":"object"}`),
	}}
	base := &apicompat.ChatCompletionsRequest{
		Model:    "gpt-5.6-luna",
		Tools:    []apicompat.ChatTool{toolA, toolB},
		Messages: []apicompat.ChatMessage{{Role: "user", Content: mustRawJSON(t, `"inspect"`)}},
	}
	reordered := *base
	reordered.Tools = []apicompat.ChatTool{toolB, toolA}

	require.Equal(t,
		deriveCompatPromptCacheKey(base, "gpt-5.6-luna"),
		deriveCompatPromptCacheKey(&reordered, "gpt-5.6-luna"),
	)
}

type promptCacheHeaderStub map[string]string

func (h promptCacheHeaderStub) GetHeader(key string) string { return h[key] }

func TestDeriveAutoPromptCacheKeyFromBody_StableAndTenantIsolated(t *testing.T) {
	base := []byte(`{"model":"gpt-5.6-luna","tools":[{"type":"function","name":"b"},{"type":"function","name":"a"}],"input":[{"role":"user","content":"inspect"}]}`)
	reorderedAndExtended := []byte(`{"model":"gpt-5.6-luna","tools":[{"name":"a","type":"function"},{"name":"b","type":"function"}],"input":[{"role":"user","content":"inspect"},{"role":"assistant","content":"ok"},{"role":"user","content":"continue"}]}`)

	key1 := deriveAutoPromptCacheKeyFromBody(promptCacheHeaderStub{}, base, "gpt-5.6-luna", 7)
	key2 := deriveAutoPromptCacheKeyFromBody(promptCacheHeaderStub{}, reorderedAndExtended, "gpt-5.6-luna", 7)
	otherTenant := deriveAutoPromptCacheKeyFromBody(promptCacheHeaderStub{}, base, "gpt-5.6-luna", 8)

	require.NotEmpty(t, key1)
	require.Equal(t, key1, key2)
	require.NotEqual(t, key1, otherTenant)
	require.True(t, strings.HasPrefix(key1, compatAutoPromptCacheKeyPrefix))
}

func TestCanonicalizeOpenAICompatToolOrder(t *testing.T) {
	body := []byte(`{"model":"gpt-5.6-luna","tools":[{"type":"function","name":"z"},{"name":"a","type":"function"}],"input":"hello"}`)
	got, changed, err := canonicalizeOpenAICompatToolOrder(body)
	require.NoError(t, err)
	require.True(t, changed)
	require.JSONEq(t, `{"model":"gpt-5.6-luna","tools":[{"name":"a","type":"function"},{"name":"z","type":"function"}],"input":"hello"}`, string(got))
}

func TestDeriveAnthropicCompatPromptCacheKey_StableAcrossLaterTurns(t *testing.T) {
	base := &apicompat.AnthropicRequest{
		Model:  "claude-sonnet-4-5",
		System: mustRawJSON(t, `"You are helpful."`),
		Messages: []apicompat.AnthropicMessage{
			{Role: "user", Content: mustRawJSON(t, `"Open repo"`)},
		},
	}
	extended := &apicompat.AnthropicRequest{
		Model:  "claude-sonnet-4-5",
		System: mustRawJSON(t, `"You are helpful."`),
		Messages: []apicompat.AnthropicMessage{
			{Role: "user", Content: mustRawJSON(t, `"Open repo"`)},
			{Role: "assistant", Content: mustRawJSON(t, `"Opened."`)},
			{Role: "user", Content: mustRawJSON(t, `"Run tests"`)},
		},
	}

	k1 := deriveAnthropicCompatPromptCacheKey(base, "gpt-5.3-codex")
	k2 := deriveAnthropicCompatPromptCacheKey(extended, "gpt-5.3-codex")
	require.NotEmpty(t, k1)
	require.Equal(t, k1, k2, "cache key should stay stable as later Claude Code turns append history")
}

func TestDeriveAnthropicCompatPromptCacheKey_UsesCacheControlAnchors(t *testing.T) {
	base := &apicompat.AnthropicRequest{
		Model: "claude-sonnet-4-5",
		System: mustRawJSON(t, `[
			{"type":"text","text":"project instructions","cache_control":{"type":"ephemeral"}}
		]`),
		Messages: []apicompat.AnthropicMessage{
			{Role: "user", Content: mustRawJSON(t, `[
				{"type":"text","text":"repo anchor","cache_control":{"type":"ephemeral"}}
			]`)},
		},
	}
	extended := &apicompat.AnthropicRequest{
		Model:  base.Model,
		System: base.System,
		Messages: []apicompat.AnthropicMessage{
			base.Messages[0],
			{Role: "assistant", Content: mustRawJSON(t, `[{"type":"text","text":"Opened."}]`)},
			{Role: "user", Content: mustRawJSON(t, `[{"type":"text","text":"Run tests"}]`)},
		},
	}

	k1 := deriveAnthropicCompatPromptCacheKey(base, "gpt-5.4")
	k2 := deriveAnthropicCompatPromptCacheKey(extended, "gpt-5.4")
	require.NotEmpty(t, k1)
	require.Equal(t, k1, k2)
	require.True(t, strings.HasPrefix(k1, "anthropic-cache-"))
	require.False(t, strings.HasPrefix(k1, compatPromptCacheKeyPrefix))
}
