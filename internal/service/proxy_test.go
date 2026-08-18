package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/CoreUnit-NET/jimmy-gateway/lib/chatjimmy"
)

func TestResolveModel(t *testing.T) {
	tests := []struct {
		name string
		req  chatCompletionRequest
		want string
	}{
		{
			name: "request model",
			req:  chatCompletionRequest{ChatRequest: chatjimmy.ChatRequest{Model: "custom-model"}},
			want: "custom-model",
		},
		{
			name: "chatOptions selectedModel",
			req: chatCompletionRequest{
				ChatOptions: &chatOptions{SelectedModel: "from-options"},
			},
			want: "from-options",
		},
		{
			name: "default",
			req:  chatCompletionRequest{},
			want: chatjimmy.DefaultModel,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveModel(tc.req); got != tc.want {
				t.Fatalf("resolveModel() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseTopKRaw(t *testing.T) {
	tests := []struct {
		raw  json.RawMessage
		want int
	}{
		{json.RawMessage(`8`), 8},
		{json.RawMessage(`"16"`), 16},
		{json.RawMessage(`0`), 0},
		{json.RawMessage(`null`), 0},
		{json.RawMessage(`"nope"`), 0},
	}
	for _, tc := range tests {
		if got := parseTopKRaw(tc.raw); got != tc.want {
			t.Fatalf("parseTopKRaw(%s) = %d, want %d", tc.raw, got, tc.want)
		}
	}
}

func TestResolveTopKPrecedence(t *testing.T) {
	req := chatCompletionRequest{
		TopK:        json.RawMessage(`4`),
		TopKAlt:     json.RawMessage(`12`),
		ChatOptions: &chatOptions{TopK: 99},
	}
	if got := resolveTopK(req); got != 4 {
		t.Fatalf("top_k precedence = %d, want 4", got)
	}
}

func TestBuildCompletionResponseIncludesStatsAndToolCalls(t *testing.T) {
	tools := []chatjimmy.Tool{{
		Type: "function",
		Function: chatjimmy.ToolFunction{
			Name:       "read",
			Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
		},
	}}
	upstreamText := `<tool_call>
{"name":"read","arguments":{"path":"x"}}
</tool_call>`
	stats := map[string]any{"prefill_tokens": 1, "decode_tokens": 2, "total_tokens": 3}
	got := buildCompletionResponse("llama3.1-8B", upstreamText, chatjimmy.Usage{TotalTokens: 3}, stats, tools)

	if got.ChatJimmyStats == nil {
		t.Fatal("expected chatjimmy_stats")
	}
	if got.Usage.TotalTokens != 3 {
		t.Fatalf("usage = %+v", got.Usage)
	}
	if len(got.Choices) != 1 {
		t.Fatalf("choices = %d", len(got.Choices))
	}
	choice := got.Choices[0].(map[string]any)
	if choice["finish_reason"] != "tool_calls" {
		t.Fatalf("finish_reason = %#v", choice["finish_reason"])
	}
	msg := choice["message"].(map[string]any)
	if _, ok := msg["tool_calls"]; !ok {
		t.Fatalf("message = %#v", msg)
	}
}

func TestEncodeStreamPlainContent(t *testing.T) {
	completion := buildCompletionResponse(
		"llama3.1-8B",
		"hello stream",
		chatjimmy.Usage{TotalTokens: 1},
		nil,
		nil,
	)
	out := string(encodeStream(completion, nil))
	if !strings.Contains(out, "data: [DONE]") {
		t.Fatalf("missing DONE: %q", out)
	}
	if !strings.Contains(out, "hello stream") {
		t.Fatalf("missing content: %q", out)
	}
	if !strings.Contains(out, "chat.completion.chunk") {
		t.Fatal("expected chunk objects")
	}
}

func TestEncodeStreamToolCalls(t *testing.T) {
	tools := []chatjimmy.Tool{{Type: "function", Function: chatjimmy.ToolFunction{Name: "read"}}}
	completion := buildCompletionResponse(
		"llama3.1-8B",
		`<tool_call>{"name":"read","arguments":{"path":"a"}}</tool_call>`,
		chatjimmy.Usage{},
		nil,
		tools,
	)
	out := string(encodeStream(completion, tools))
	if !strings.Contains(out, "tool_calls") {
		t.Fatalf("expected tool_calls in stream: %q", out)
	}
	if !strings.Contains(out, "data: [DONE]") {
		t.Fatal("missing DONE")
	}
}
