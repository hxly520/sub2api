package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// marshalEvent marshals through the custom MarshalJSON and returns the decoded
// object plus the set of top-level keys.
func marshalEvent(t *testing.T, e ResponsesStreamEvent) map[string]any {
	t.Helper()
	b, err := json.Marshal(e)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(b, &m))
	return m
}

// TestWire_IndexFieldsPresentAtZero guards the omitempty trap: output_index/
// content_index/summary_index must serialize even when 0.
func TestWire_IndexFieldsPresentAtZero(t *testing.T) {
	m := marshalEvent(t, ResponsesStreamEvent{
		Type: "response.output_text.delta", OutputIndex: 0, ContentIndex: 0, ItemID: "msg_1", Delta: "hi",
	})
	require.Contains(t, m, "output_index")
	require.Contains(t, m, "content_index")
	require.EqualValues(t, 0, m["output_index"])

	r := marshalEvent(t, ResponsesStreamEvent{
		Type: "response.reasoning_summary_text.delta", OutputIndex: 0, SummaryIndex: 0, ItemID: "rs_1", Delta: "think",
	})
	require.Contains(t, r, "output_index")
	require.Contains(t, r, "summary_index")
}

// TestWire_FunctionCallItemAlwaysComplete guards that a function_call item
// always carries call_id/name/arguments, including arguments:"" on .added.
func TestWire_FunctionCallItemAlwaysComplete(t *testing.T) {
	added := marshalEvent(t, ResponsesStreamEvent{
		Type:        "response.output_item.added",
		OutputIndex: 1,
		Item:        &ResponsesOutput{Type: "function_call", ID: "fc_1", CallID: "call_a", Name: "exec", Status: "in_progress"},
	})
	item, ok := added["item"].(map[string]any)
	require.True(t, ok, "item must be an object")
	for _, k := range []string{"call_id", "name", "arguments"} {
		require.Containsf(t, item, k, "function_call item missing %q", k)
	}
	require.Equal(t, "", item["arguments"])
}

// TestWire_MessageItemContentAlwaysArray guards content:[] presence.
func TestWire_MessageItemContentAlwaysArray(t *testing.T) {
	m := marshalEvent(t, ResponsesStreamEvent{
		Type:        "response.output_item.added",
		OutputIndex: 0,
		Item:        &ResponsesOutput{Type: "message", ID: "msg_1", Role: "assistant", Status: "in_progress"},
	})
	item, ok := m["item"].(map[string]any)
	require.True(t, ok, "item must be an object")
	require.Contains(t, item, "content")
	_, ok = item["content"].([]any)
	require.True(t, ok, "content must be an array")
}

// TestWire_ReasoningItemSummaryAlwaysArray guards summary:[] presence.
func TestWire_ReasoningItemSummaryAlwaysArray(t *testing.T) {
	m := marshalEvent(t, ResponsesStreamEvent{
		Type:        "response.output_item.added",
		OutputIndex: 0,
		Item:        &ResponsesOutput{Type: "reasoning", ID: "rs_1", Status: "in_progress"},
	})
	item, ok := m["item"].(map[string]any)
	require.True(t, ok, "item must be an object")
	require.Contains(t, item, "summary")
	_, ok = item["summary"].([]any)
	require.True(t, ok, "summary must be an array")
}

// TestWire_ContentPartCarriesAnnotationsLogprobs guards the output_text part shape.
func TestWire_ContentPartCarriesAnnotationsLogprobs(t *testing.T) {
	m := marshalEvent(t, ResponsesStreamEvent{
		Type: "response.content_part.added", OutputIndex: 0, ContentIndex: 0, ItemID: "msg_1",
		Part: &ResponsesContentPart{Type: "output_text", Text: ""},
	})
	part, ok := m["part"].(map[string]any)
	require.True(t, ok, "part must be an object")
	require.Equal(t, "output_text", part["type"])
	require.Contains(t, part, "text")
	require.Contains(t, part, "annotations")
	require.Contains(t, part, "logprobs")
}

// TestWire_ArgumentsDonePresentEvenEmpty guards arguments presence on done.
func TestWire_ArgumentsDonePresentEvenEmpty(t *testing.T) {
	m := marshalEvent(t, ResponsesStreamEvent{
		Type: "response.function_call_arguments.done", OutputIndex: 1, ItemID: "fc_1", CallID: "call_a", Name: "exec", Arguments: "",
	})
	require.Contains(t, m, "arguments")
	require.Equal(t, "", m["arguments"])
}

// TestWire_CustomToolInputDoneUsesInput guards the freeform custom-tool event
// shape. It carries input, not arguments.
func TestWire_CustomToolInputDoneUsesInput(t *testing.T) {
	m := marshalEvent(t, ResponsesStreamEvent{
		Type:        "response.custom_tool_call_input.done",
		OutputIndex: 1,
		ItemID:      "ct_1",
		CallID:      "call_patch",
		Name:        "apply_patch",
		Input:       "*** Begin Patch",
	})
	require.Contains(t, m, "input")
	require.Equal(t, "*** Begin Patch", m["input"])
	require.NotContains(t, m, "arguments")
}

// TestWire_CompletedOutputItemsStayProtocolComplete guards that the terminal
// response.output shape stays as complete as the streamed output_item.done item.
func TestWire_CompletedOutputItemsStayProtocolComplete(t *testing.T) {
	m := marshalEvent(t, ResponsesStreamEvent{
		Type: "response.completed",
		Response: &ResponsesResponse{
			ID:     "resp_1",
			Object: "response",
			Model:  "gpt-5.5",
			Status: "completed",
			Output: []ResponsesOutput{
				{Type: "message", ID: "msg_1", Role: "assistant", Status: "completed"},
				{Type: "function_call", ID: "fc_1", CallID: "call_a", Name: "exec", Status: "completed"},
				{Type: "custom_tool_call", ID: "ct_1", CallID: "call_b", Name: "apply_patch", Input: "patch", Status: "completed"},
			},
		},
	})

	response, ok := m["response"].(map[string]any)
	require.True(t, ok, "response must be an object")
	output, ok := response["output"].([]any)
	require.True(t, ok, "output must be an array")
	require.Len(t, output, 3)

	message, ok := output[0].(map[string]any)
	require.True(t, ok, "message output must be an object")
	require.Contains(t, message, "content")
	require.Equal(t, []any{}, message["content"])

	fn, ok := output[1].(map[string]any)
	require.True(t, ok, "function output must be an object")
	require.Equal(t, "call_a", fn["call_id"])
	require.Equal(t, "exec", fn["name"])
	require.Contains(t, fn, "arguments")
	require.Equal(t, "", fn["arguments"])

	custom, ok := output[2].(map[string]any)
	require.True(t, ok, "custom tool output must be an object")
	require.Equal(t, "custom_tool_call", custom["type"])
	require.Equal(t, "call_b", custom["call_id"])
	require.Equal(t, "apply_patch", custom["name"])
	require.Equal(t, "patch", custom["input"])
	require.NotContains(t, custom, "arguments")
}

// TestWire_UnknownEventFallsBackToDefault ensures non-streamed event types keep
// default marshalling (the response object is preserved).
func TestWire_UnknownEventFallsBackToDefault(t *testing.T) {
	m := marshalEvent(t, ResponsesStreamEvent{
		Type:     "response.completed",
		Response: &ResponsesResponse{ID: "resp_1", Object: "response", Status: "completed"},
	})
	require.Contains(t, m, "response")
}
