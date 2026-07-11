package service

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const compatPromptCacheKeyPrefix = "compat_cc_"
const compatAutoPromptCacheKeyPrefix = "compat_auto_"

// injectOpenAIResponsesPromptCacheKey propagates the stable compatibility key
// into Responses request bodies for credential-based upstream routes. Keeping
// this in one helper prevents Chat and Anthropic compatibility paths from
// drifting (in particular for ServiceAccount routes).
func injectOpenAIResponsesPromptCacheKey(account *Account, body []byte, promptCacheKey string) ([]byte, error) {
	if account == nil || (account.Type != AccountTypeAPIKey && account.Type != AccountTypeServiceAccount) {
		return body, nil
	}
	trimmedKey := strings.TrimSpace(promptCacheKey)
	if trimmedKey == "" {
		return body, nil
	}

	var reqBody map[string]any
	if err := json.Unmarshal(body, &reqBody); err != nil {
		return nil, fmt.Errorf("unmarshal for prompt cache key injection: %w", err)
	}
	if existing, ok := reqBody["prompt_cache_key"].(string); ok && strings.TrimSpace(existing) != "" {
		return body, nil
	}
	reqBody["prompt_cache_key"] = trimmedKey
	updated, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("remarshal after prompt cache key injection: %w", err)
	}
	return updated, nil
}

func shouldAutoInjectPromptCacheKeyForCompat(model string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(model))
	// 仅对 Codex OAuth 路径支持的 GPT-5 族开启自动注入，避免 normalizeCodexModel
	// 的默认兜底把任意模型（如 gpt-4o、claude-*）误判为 gpt-5.4。
	if !strings.Contains(trimmed, "gpt-5") && !strings.Contains(trimmed, "codex") {
		return false
	}
	normalized := strings.TrimSpace(strings.ToLower(normalizeCodexModel(trimmed)))
	return strings.HasPrefix(normalized, "gpt-5") || strings.Contains(normalized, "codex")
}

func deriveCompatPromptCacheKey(req *apicompat.ChatCompletionsRequest, mappedModel string) string {
	if req == nil {
		return ""
	}

	normalizedModel := normalizeCodexModel(strings.TrimSpace(mappedModel))
	if normalizedModel == "" {
		normalizedModel = normalizeCodexModel(strings.TrimSpace(req.Model))
	}
	if normalizedModel == "" {
		normalizedModel = strings.TrimSpace(req.Model)
	}

	seedParts := []string{"model=" + normalizedModel}
	if req.ReasoningEffort != "" {
		seedParts = append(seedParts, "reasoning_effort="+strings.TrimSpace(req.ReasoningEffort))
	}
	if len(req.ToolChoice) > 0 {
		seedParts = append(seedParts, "tool_choice="+normalizeCompatSeedJSON(req.ToolChoice))
	}
	if len(req.Tools) > 0 {
		if raw, err := json.Marshal(req.Tools); err == nil {
			seedParts = append(seedParts, "tools="+normalizeCompatToolSeedJSON(raw))
		}
	}
	if len(req.Functions) > 0 {
		if raw, err := json.Marshal(req.Functions); err == nil {
			seedParts = append(seedParts, "functions="+normalizeCompatToolSeedJSON(raw))
		}
	}

	firstUserCaptured := false
	for _, msg := range req.Messages {
		switch strings.TrimSpace(msg.Role) {
		case "system":
			seedParts = append(seedParts, "system="+normalizeCompatSeedJSON(msg.Content))
		case "user":
			if !firstUserCaptured {
				seedParts = append(seedParts, "first_user="+normalizeCompatSeedJSON(msg.Content))
				firstUserCaptured = true
			}
		}
	}

	return compatPromptCacheKeyPrefix + hashSensitiveValueForLog(strings.Join(seedParts, "|"))
}

// deriveAutoPromptCacheKeyFromBody restores the compatibility behavior used by
// API-key/ServiceAccount Responses upstreams. Explicit client session signals
// always win; the content-derived fallback only uses the stable request prefix.
// API key isolation prevents two tenants with identical prompts from sharing a
// routing/cache key.
func deriveAutoPromptCacheKeyFromBody(c interface{ GetHeader(string) string }, body []byte, model string, apiKeyID int64) string {
	rawSession := ""
	if c != nil {
		rawSession = strings.TrimSpace(c.GetHeader("session_id"))
		if rawSession == "" {
			rawSession = strings.TrimSpace(c.GetHeader("conversation_id"))
		}
	}
	if rawSession == "" && len(body) > 0 {
		rawSession = strings.TrimSpace(gjson.GetBytes(body, "prompt_cache_key").String())
	}
	if rawSession == "" {
		rawSession = deriveOpenAIContentSessionSeed(body)
	}
	if rawSession == "" {
		return ""
	}

	normalizedModel := strings.TrimSpace(model)
	if normalizedModel == "" && len(body) > 0 {
		normalizedModel = strings.TrimSpace(gjson.GetBytes(body, "model").String())
	}
	seed := fmt.Sprintf("api_key=%d|model=%s|session=%s", apiKeyID, normalizedModel, rawSession)
	return compatAutoPromptCacheKeyPrefix + hashSensitiveValueForLog(seed)
}

func deriveAnthropicCompatPromptCacheKey(req *apicompat.AnthropicRequest, mappedModel string) string {
	if req == nil {
		return ""
	}
	if anchorKey := deriveAnthropicCacheControlPromptCacheKey(req); anchorKey != "" {
		return anchorKey
	}

	normalizedModel := normalizeCodexModel(strings.TrimSpace(mappedModel))
	if normalizedModel == "" {
		normalizedModel = normalizeCodexModel(strings.TrimSpace(req.Model))
	}
	if normalizedModel == "" {
		normalizedModel = strings.TrimSpace(req.Model)
	}

	seedParts := []string{"model=" + normalizedModel}
	if req.OutputConfig != nil && strings.TrimSpace(req.OutputConfig.Effort) != "" {
		seedParts = append(seedParts, "effort="+strings.TrimSpace(req.OutputConfig.Effort))
	}
	if len(req.ToolChoice) > 0 {
		seedParts = append(seedParts, "tool_choice="+normalizeCompatSeedJSON(req.ToolChoice))
	}
	if len(req.Tools) > 0 {
		if raw, err := json.Marshal(req.Tools); err == nil {
			seedParts = append(seedParts, "tools="+normalizeCompatToolSeedJSON(raw))
		}
	}
	if len(req.System) > 0 {
		seedParts = append(seedParts, "system="+normalizeCompatSeedJSON(req.System))
	}

	firstUserCaptured := false
	for _, msg := range req.Messages {
		if strings.TrimSpace(msg.Role) != "user" || firstUserCaptured {
			continue
		}
		seedParts = append(seedParts, "first_user="+normalizeCompatSeedJSON(msg.Content))
		firstUserCaptured = true
	}

	return compatPromptCacheKeyPrefix + hashSensitiveValueForLog(strings.Join(seedParts, "|"))
}

func deriveAnthropicCacheControlPromptCacheKey(req *apicompat.AnthropicRequest) string {
	if req == nil {
		return ""
	}

	var parts []string
	var systemBlocks []apicompat.AnthropicContentBlock
	if len(req.System) > 0 && json.Unmarshal(req.System, &systemBlocks) == nil {
		for _, block := range systemBlocks {
			if block.Type == "text" &&
				block.CacheControl != nil &&
				strings.TrimSpace(block.CacheControl.Type) == "ephemeral" &&
				strings.TrimSpace(block.Text) != "" {
				parts = append(parts, "system:"+strings.TrimSpace(block.Text))
			}
		}
	}

	firstUserAnchor := ""
	for _, msg := range req.Messages {
		var blocks []apicompat.AnthropicContentBlock
		if len(msg.Content) == 0 || json.Unmarshal(msg.Content, &blocks) != nil {
			continue
		}
		role := strings.TrimSpace(msg.Role)
		for _, block := range blocks {
			if block.Type != "text" ||
				block.CacheControl == nil ||
				strings.TrimSpace(block.CacheControl.Type) != "ephemeral" ||
				strings.TrimSpace(block.Text) == "" {
				continue
			}
			switch role {
			case "user":
				if firstUserAnchor == "" {
					firstUserAnchor = strings.TrimSpace(block.Text)
				}
			case "assistant":
				parts = append(parts, "assistant:"+strings.TrimSpace(block.Text))
			}
		}
	}
	if firstUserAnchor != "" {
		parts = append(parts, "user_anchor:"+firstUserAnchor)
	}
	if len(parts) == 0 {
		return ""
	}
	sum := sha256.Sum256([]byte("anthropic-cache:" + strings.Join(parts, "\n")))
	return fmt.Sprintf("anthropic-cache-%x", sum[:16])
}

func normalizeCompatSeedJSON(v json.RawMessage) string {
	if len(v) == 0 {
		return ""
	}
	var tmp any
	if err := json.Unmarshal(v, &tmp); err != nil {
		return string(v)
	}
	out, err := json.Marshal(tmp)
	if err != nil {
		return string(v)
	}
	return string(out)
}

// normalizeCompatToolSeedJSON treats the top-level tool/function list as a set.
// Third-party clients often rebuild the same tool registry in a different order
// on every turn; sorting the canonical JSON prevents that non-semantic churn from
// changing the derived session/cache key.
func normalizeCompatToolSeedJSON(v json.RawMessage) string {
	if len(v) == 0 {
		return ""
	}
	var items []json.RawMessage
	if err := json.Unmarshal(v, &items); err != nil {
		return normalizeCompatSeedJSON(v)
	}
	canonical := make([]string, 0, len(items))
	for _, item := range items {
		canonical = append(canonical, normalizeCompatSeedJSON(item))
	}
	sort.Strings(canonical)
	return "[" + strings.Join(canonical, ",") + "]"
}

// canonicalizeOpenAICompatToolOrder also stabilizes the actual upstream prefix,
// not just its hash. It is only used when the gateway auto-generates a cache key;
// requests carrying an explicit client key retain their original tool order.
func canonicalizeOpenAICompatToolOrder(body []byte) ([]byte, bool, error) {
	if len(body) == 0 {
		return body, false, nil
	}
	updated := body
	changed := false
	for _, field := range []string{"tools", "functions"} {
		raw := gjson.GetBytes(updated, field)
		if !raw.Exists() || !raw.IsArray() || raw.Raw == "[]" {
			continue
		}
		canonical := normalizeCompatToolSeedJSON(json.RawMessage(raw.Raw))
		if canonical == "" || canonical == normalizeCompatSeedJSON(json.RawMessage(raw.Raw)) {
			continue
		}
		var err error
		updated, err = sjson.SetRawBytes(updated, field, []byte(canonical))
		if err != nil {
			return body, false, fmt.Errorf("canonicalize %s: %w", field, err)
		}
		changed = true
	}
	return updated, changed, nil
}

// canonicalizeAnthropicCompatToolOrder applies the same set-style ordering to
// the parsed Messages request. Callers use it only for gateway-derived digest
// sessions; explicit client cache/session anchors keep the original order.
func canonicalizeAnthropicCompatToolOrder(req *apicompat.AnthropicRequest) (bool, error) {
	if req == nil || len(req.Tools) <= 1 {
		return false, nil
	}
	raw, err := json.Marshal(req.Tools)
	if err != nil {
		return false, fmt.Errorf("marshal anthropic tools: %w", err)
	}
	canonical := normalizeCompatToolSeedJSON(raw)
	if canonical == "" || canonical == normalizeCompatSeedJSON(raw) {
		return false, nil
	}
	var tools []apicompat.AnthropicTool
	if err := json.Unmarshal([]byte(canonical), &tools); err != nil {
		return false, fmt.Errorf("unmarshal canonical anthropic tools: %w", err)
	}
	req.Tools = tools
	return true, nil
}
