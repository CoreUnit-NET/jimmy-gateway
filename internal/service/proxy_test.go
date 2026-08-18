package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/CoreUnit-NET/jimmy-gateway/lib/chatjimmy"
)

func intPtr(v int) *int { return &v }

func userMessages() []chatjimmy.Message {
	return []chatjimmy.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}}
}

func TestTranslateRequestModel(t *testing.T) {
	tests := []struct {
		name string
		req  chatjimmy.ChatRequest
		want string
	}{
		{
			name: "request model",
			req:  chatjimmy.ChatRequest{Model: "custom-model", Messages: userMessages()},
			want: "custom-model",
		},
		{
			name: "chatOptions selectedModel",
			req: chatjimmy.ChatRequest{
				Messages:    userMessages(),
				ChatOptions: &chatjimmy.NativeOptions{SelectedModel: "from-options"},
			},
			want: "from-options",
		},
		{
			name: "default",
			req:  chatjimmy.ChatRequest{Messages: userMessages()},
			want: chatjimmy.DefaultModel,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, err := chatjimmy.TranslateRequest(tc.req, chatjimmy.TranslateOptions{DefaultModel: chatjimmy.DefaultModel})
			if err != nil {
				t.Fatalf("TranslateRequest: %v", err)
			}
			if out.Model != tc.want {
				t.Fatalf("model = %q, want %q", out.Model, tc.want)
			}
		})
	}
}

func TestTranslateRequestTopKPrecedence(t *testing.T) {
	tests := []struct {
		name string
		req  chatjimmy.ChatRequest
		want int
	}{
		{
			name: "top_k wins",
			req: chatjimmy.ChatRequest{
				Messages:    userMessages(),
				TopK:        intPtr(4),
				TopKCamel:   intPtr(12),
				ChatOptions: &chatjimmy.NativeOptions{TopK: 99},
			},
			want: 4,
		},
		{
			name: "topK camel",
			req: chatjimmy.ChatRequest{
				Messages:  userMessages(),
				TopKCamel: intPtr(12),
			},
			want: 12,
		},
		{
			name: "chatOptions topK",
			req: chatjimmy.ChatRequest{
				Messages:    userMessages(),
				ChatOptions: &chatjimmy.NativeOptions{TopK: 99},
			},
			want: 99,
		},
		{
			name: "default",
			req:  chatjimmy.ChatRequest{Messages: userMessages()},
			want: chatjimmy.DefaultTopK,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, err := chatjimmy.TranslateRequest(tc.req, chatjimmy.TranslateOptions{})
			if err != nil {
				t.Fatalf("TranslateRequest: %v", err)
			}
			if got := out.Payload.ChatOptions.TopK; got != tc.want {
				t.Fatalf("topK = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestBuildCompletionIncludesStatsAndToolCalls(t *testing.T) {
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
	got := chatjimmy.BuildCompletion("llama3.1-8B", upstreamText, chatjimmy.Usage{TotalTokens: 3}, tools, stats)

	if got.ChatJimmyStats == nil {
		t.Fatal("expected chatjimmy_stats")
	}
	if got.Usage.TotalTokens != 3 {
		t.Fatalf("usage = %+v", got.Usage)
	}
	if len(got.Choices) != 1 {
		t.Fatalf("choices = %d", len(got.Choices))
	}
	choice := got.Choices[0]
	if choice.FinishReason != "tool_calls" {
		t.Fatalf("finish_reason = %q", choice.FinishReason)
	}
	if len(choice.Message.ToolCalls) == 0 {
		t.Fatalf("message = %+v", choice.Message)
	}
}

func TestEncodeSSEPlainContent(t *testing.T) {
	completion := chatjimmy.BuildCompletion(
		"llama3.1-8B",
		"hello stream",
		chatjimmy.Usage{TotalTokens: 1},
		nil,
		nil,
	)
	out := string(chatjimmy.EncodeSSEChunks(chatjimmy.BuildStreamChunks(completion)))
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

func TestEncodeSSEToolCalls(t *testing.T) {
	tools := []chatjimmy.Tool{{Type: "function", Function: chatjimmy.ToolFunction{Name: "read"}}}
	completion := chatjimmy.BuildCompletion(
		"llama3.1-8B",
		`<tool_call>{"name":"read","arguments":{"path":"a"}}</tool_call>`,
		chatjimmy.Usage{},
		tools,
		nil,
	)
	out := string(chatjimmy.EncodeSSEChunks(chatjimmy.BuildStreamChunks(completion)))
	if !strings.Contains(out, "tool_calls") {
		t.Fatalf("expected tool_calls in stream: %q", out)
	}
	if !strings.Contains(out, "data: [DONE]") {
		t.Fatal("missing DONE")
	}
}
