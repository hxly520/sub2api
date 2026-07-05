package apicompat

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// custom_tool_call（custom/freeform 工具，如新版 apply_patch）应像 function_call 一样
// 注册为工具调用，其 *_input.delta 增量映射到正确的工具索引。
func TestResponsesEventToChatChunks_CustomToolCallInputDelta(t *testing.T) {
	state := NewResponsesEventToChatState()
	state.Model = "gpt-5-codex"
	state.SentRole = true

	chunks := ResponsesEventToChatChunks(&ResponsesStreamEvent{
		Type:        "response.output_item.added",
		OutputIndex: 1,
		Item: &ResponsesOutput{
			Type:   "custom_tool_call",
			CallID: "call_patch",
			Name:   "apply_patch",
		},
	}, state)
	require.Len(t, chunks, 1)
	require.Len(t, chunks[0].Choices[0].Delta.ToolCalls, 1)
	tc := chunks[0].Choices[0].Delta.ToolCalls[0]
	assert.Equal(t, "call_patch", tc.ID)
	assert.Equal(t, "apply_patch", tc.Function.Name)

	chunks = ResponsesEventToChatChunks(&ResponsesStreamEvent{
		Type:        "response.custom_tool_call_input.delta",
		OutputIndex: 1,
		Delta:       "*** Begin Patch",
	}, state)
	require.Len(t, chunks, 1)
	tc = chunks[0].Choices[0].Delta.ToolCalls[0]
	require.NotNil(t, tc.Index)
	assert.Equal(t, 0, *tc.Index)
	assert.Equal(t, "*** Begin Patch", tc.Function.Arguments)
}

func TestResponsesEventToChatChunks_CustomToolCallInputDoneWithoutDelta(t *testing.T) {
	state := NewResponsesEventToChatState()
	state.Model = "gpt-5-codex"
	state.SentRole = true

	ResponsesEventToChatChunks(&ResponsesStreamEvent{
		Type:        "response.output_item.added",
		OutputIndex: 2,
		Item: &ResponsesOutput{
			Type:   "custom_tool_call",
			CallID: "call_patch",
			Name:   "apply_patch",
		},
	}, state)

	chunks := ResponsesEventToChatChunks(&ResponsesStreamEvent{
		Type:        "response.custom_tool_call_input.done",
		OutputIndex: 2,
		Input:       "patch-body",
	}, state)
	require.Len(t, chunks, 1)
	tc := chunks[0].Choices[0].Delta.ToolCalls[0]
	require.NotNil(t, tc.Index)
	assert.Equal(t, 0, *tc.Index)
	assert.Equal(t, "patch-body", tc.Function.Arguments)
}

func TestResponsesEventToChatChunks_CustomToolOutputItemDoneCarriesInput(t *testing.T) {
	state := NewResponsesEventToChatState()
	state.Model = "gpt-5-codex"
	state.SentRole = true

	chunks := ResponsesEventToChatChunks(&ResponsesStreamEvent{
		Type:        "response.output_item.done",
		OutputIndex: 4,
		Item: &ResponsesOutput{
			Type:   "custom_tool_call",
			CallID: "call_patch",
			Name:   "apply_patch",
			Input:  "patch-body",
			Status: "completed",
		},
	}, state)
	require.Len(t, chunks, 2)
	first := chunks[0].Choices[0].Delta.ToolCalls[0]
	require.NotNil(t, first.Index)
	assert.Equal(t, "call_patch", first.ID)
	assert.Equal(t, "apply_patch", first.Function.Name)
	assert.Equal(t, 0, *first.Index)

	second := chunks[1].Choices[0].Delta.ToolCalls[0]
	require.NotNil(t, second.Index)
	assert.Equal(t, 0, *second.Index)
	assert.Equal(t, "patch-body", second.Function.Arguments)
}

// 原始推理文本增量 reasoning_text.delta 应像 reasoning_summary_text.delta 一样
// 映射为 reasoning_content。
func TestResponsesEventToChatChunks_ReasoningTextDelta(t *testing.T) {
	state := NewResponsesEventToChatState()
	state.Model = "gpt-5-codex"
	state.SentRole = true

	chunks := ResponsesEventToChatChunks(&ResponsesStreamEvent{
		Type:  "response.reasoning_text.delta",
		Delta: "thinking step",
	}, state)
	require.Len(t, chunks, 1)
	require.NotNil(t, chunks[0].Choices[0].Delta.ReasoningContent)
	assert.Equal(t, "thinking step", *chunks[0].Choices[0].Delta.ReasoningContent)
}

// 缓冲（非流式）累加器同样需识别两类新事件。
func TestBufferedResponseAccumulator_CodexEvents(t *testing.T) {
	acc := NewBufferedResponseAccumulator()
	acc.ProcessEvent(&ResponsesStreamEvent{
		Type:        "response.output_item.added",
		OutputIndex: 0,
		Item:        &ResponsesOutput{Type: "custom_tool_call", CallID: "c1", Name: "apply_patch"},
	})
	acc.ProcessEvent(&ResponsesStreamEvent{
		Type:        "response.custom_tool_call_input.delta",
		OutputIndex: 0,
		Delta:       "patch-body",
	})
	acc.ProcessEvent(&ResponsesStreamEvent{
		Type:  "response.reasoning_text.delta",
		Delta: "raw-reasoning",
	})
	require.True(t, acc.HasContent())
}

func TestBufferedResponseAccumulator_CustomToolInputDoneWithoutDelta(t *testing.T) {
	acc := NewBufferedResponseAccumulator()
	acc.ProcessEvent(&ResponsesStreamEvent{
		Type:        "response.output_item.added",
		OutputIndex: 3,
		Item:        &ResponsesOutput{Type: "custom_tool_call", CallID: "c1", Name: "apply_patch"},
	})
	acc.ProcessEvent(&ResponsesStreamEvent{
		Type:        "response.custom_tool_call_input.done",
		OutputIndex: 3,
		Input:       "patch-body",
	})

	output := acc.BuildOutput()
	require.Len(t, output, 1)
	assert.Equal(t, "function_call", output[0].Type)
	assert.Equal(t, "patch-body", output[0].Arguments)
}

func TestBufferedResponseAccumulator_CustomToolOutputItemDoneCarriesInput(t *testing.T) {
	acc := NewBufferedResponseAccumulator()
	acc.ProcessEvent(&ResponsesStreamEvent{
		Type:        "response.output_item.added",
		OutputIndex: 5,
		Item:        &ResponsesOutput{Type: "custom_tool_call", CallID: "c1", Name: "apply_patch"},
	})
	acc.ProcessEvent(&ResponsesStreamEvent{
		Type:        "response.output_item.done",
		OutputIndex: 5,
		Item:        &ResponsesOutput{Type: "custom_tool_call", CallID: "c1", Name: "apply_patch", Input: "patch-body"},
	})

	output := acc.BuildOutput()
	require.Len(t, output, 1)
	assert.Equal(t, "function_call", output[0].Type)
	assert.Equal(t, "patch-body", output[0].Arguments)
}
