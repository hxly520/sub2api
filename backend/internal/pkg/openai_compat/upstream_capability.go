// Package openai_compat 提供 OpenAI 协议族在不同上游间的能力差异判定工具。
//
// 背景：sub2api 的 OpenAI APIKey 账号通过 base_url 接入多种第三方 OpenAI 兼容上游
// （DeepSeek、Kimi、GLM、Qwen 等）。这些上游普遍只支持 /v1/chat/completions，
// 不存在 /v1/responses 端点。但网关历史代码无差别走 CC→Responses 转换并打到
// /v1/responses，导致兼容上游 404。
//
// 本包提供基于"账号探测标记"的能力判定，配合
// internal/service/openai_apikey_responses_probe.go 在创建/修改账号时一次性
// 探测并落标。
//
// 设计取舍：
//   - 不维护静态 host 白名单——避免新增厂商时必须改代码（讨论沉淀于
//     pensieve/short-term/knowledge/upstream-capability-detection-design-tradeoffs）
//   - 标记缺失时默认 true（即"走 Responses"），保持与重构前老代码完全一致的存量
//     账号行为（"现状即证据"原则；详见
//     pensieve/short-term/maxims/preserve-existing-runtime-behavior-when-replacing-logic-in-stateful-systems）
package openai_compat

// AccountResponsesSupport 描述账号上游对 OpenAI Responses API 的有效支持状态。
//
// 仅用于 platform=openai + type=apikey 的账号；其他账号类型不应调用本包判定。
type AccountResponsesSupport int

const (
	// ResponsesSupportUnknown 表示账号尚未完成能力探测（extra 字段缺失）。
	// 上游路由层应按"现状即证据"原则默认走 Responses，保持与重构前一致。
	ResponsesSupportUnknown AccountResponsesSupport = iota

	// ResponsesSupportYes 探测确认上游支持 /v1/responses。
	ResponsesSupportYes

	// ResponsesSupportNo 探测确认上游不支持 /v1/responses，应走
	// /v1/chat/completions 直转路径。
	ResponsesSupportNo
)

// ResponsesSupportMode 描述账号级 Responses API 路由覆盖模式。
type ResponsesSupportMode string

const (
	// ResponsesSupportModeAuto 表示跟随自动探测结果。
	ResponsesSupportModeAuto ResponsesSupportMode = "auto"

	// ResponsesSupportModeForceResponses 强制使用 /v1/responses。
	ResponsesSupportModeForceResponses ResponsesSupportMode = "force_responses"

	// ResponsesSupportModeForceChatCompletions 强制使用 /v1/chat/completions。
	ResponsesSupportModeForceChatCompletions ResponsesSupportMode = "force_chat_completions"
)

// ChatCompletionsMode describes an endpoint-specific override for inbound
// /v1/chat/completions requests. Unlike openai_responses_mode, this does not
// change native /responses routing.
type ChatCompletionsMode string

const (
	ChatCompletionsModeAuto            ChatCompletionsMode = "auto"
	ChatCompletionsModeRawChat         ChatCompletionsMode = "raw_chat"
	ChatCompletionsModeResponsesBridge ChatCompletionsMode = "responses_bridge"
)

// ExtraKeyResponsesMode 是 accounts.extra JSON 中存储手动覆盖模式的键名。
// 值类型为 string：auto=跟随探测，force_responses=强制 Responses，
// force_chat_completions=强制 Chat Completions。
const ExtraKeyResponsesMode = "openai_responses_mode"

// ExtraKeyChatCompletionsMode stores the endpoint-specific routing override for
// inbound /v1/chat/completions. Values:
// auto=follow capability detection, raw_chat=force upstream chat completions,
// responses_bridge=force Chat→Responses bridge for chat-completions ingress.
const ExtraKeyChatCompletionsMode = "openai_chat_completions_mode"

// ExtraKeyResponsesEarlyFlush enables early flushing of Responses preamble SSE
// events for passthrough streams. It is intentionally opt-in because flushing
// response.created/response.in_progress commits the downstream HTTP response
// before a later response.failed event can be converted into a failover HTTP
// error.
const ExtraKeyResponsesEarlyFlush = "openai_responses_early_flush"

// ExtraKeyResponsesSupported 是 accounts.extra JSON 中存储自动探测结果的键名。
// 值类型为 bool：true=支持、false=不支持、键缺失=未探测。
const ExtraKeyResponsesSupported = "openai_responses_supported"

// NormalizeResponsesSupportMode 归一化账号级 Responses API 路由覆盖模式。
// 缺失或非法值按 auto 处理，以保持存量行为。
func NormalizeResponsesSupportMode(mode string) ResponsesSupportMode {
	switch ResponsesSupportMode(mode) {
	case ResponsesSupportModeForceResponses:
		return ResponsesSupportModeForceResponses
	case ResponsesSupportModeForceChatCompletions:
		return ResponsesSupportModeForceChatCompletions
	default:
		return ResponsesSupportModeAuto
	}
}

func NormalizeChatCompletionsMode(mode string) ChatCompletionsMode {
	switch ChatCompletionsMode(mode) {
	case ChatCompletionsModeRawChat:
		return ChatCompletionsModeRawChat
	case ChatCompletionsModeResponsesBridge:
		return ChatCompletionsModeResponsesBridge
	default:
		return ChatCompletionsModeAuto
	}
}

// ResolveResponsesSupport 从账号的 extra map 中读取手动覆盖模式与探测标记。
//
// 标记缺失或类型不匹配时返回 ResponsesSupportUnknown——调用方应按
// "未探测=保留旧行为=走 Responses" 处理（参见 ShouldUseResponsesAPI）。
func ResolveResponsesSupport(extra map[string]any) AccountResponsesSupport {
	if extra == nil {
		return ResponsesSupportUnknown
	}
	if mode, ok := extra[ExtraKeyResponsesMode].(string); ok {
		switch NormalizeResponsesSupportMode(mode) {
		case ResponsesSupportModeForceResponses:
			return ResponsesSupportYes
		case ResponsesSupportModeForceChatCompletions:
			return ResponsesSupportNo
		}
	}
	v, ok := extra[ExtraKeyResponsesSupported]
	if !ok {
		return ResponsesSupportUnknown
	}
	supported, ok := v.(bool)
	if !ok {
		return ResponsesSupportUnknown
	}
	if supported {
		return ResponsesSupportYes
	}
	return ResponsesSupportNo
}

// ShouldUseResponsesAPI 判断 OpenAI APIKey 账号是否应使用上游 Responses API。
//
// 返回 true 的两种情况：
//  1. 账号已探测确认支持 Responses
//  2. 账号未探测（标记缺失）——按"现状即证据"原则保留旧行为
//
// 仅当账号已探测且确认不支持时返回 false。该函数用于原生 /responses 路由，
// 不读取 openai_chat_completions_mode，避免 Chat 入站优化误伤 Codex/Responses。
func ShouldUseResponsesAPI(extra map[string]any) bool {
	return ResolveResponsesSupport(extra) != ResponsesSupportNo
}

// ShouldUseResponsesAPIForChatIngress 判断 OpenAI APIKey 账号的入站
// /v1/chat/completions 请求是否应走"CC→Responses 转换 + 上游
// /v1/responses"路径。openai_chat_completions_mode 只在这里生效。
func ShouldUseResponsesAPIForChatIngress(extra map[string]any) bool {
	if extra != nil {
		if mode, ok := extra[ExtraKeyChatCompletionsMode].(string); ok {
			switch NormalizeChatCompletionsMode(mode) {
			case ChatCompletionsModeRawChat:
				return false
			case ChatCompletionsModeResponsesBridge:
				return true
			}
		}
	}
	return ShouldUseResponsesAPI(extra)
}

// ShouldEarlyFlushResponsesPreamble reports whether passthrough Responses
// streams may flush response.created/response.in_progress before the first
// content event. Only boolean true enables it; other values keep the safer
// default behavior.
func ShouldEarlyFlushResponsesPreamble(extra map[string]any) bool {
	if extra == nil {
		return false
	}
	v, ok := extra[ExtraKeyResponsesEarlyFlush]
	if !ok {
		return false
	}
	enabled, ok := v.(bool)
	return ok && enabled
}
