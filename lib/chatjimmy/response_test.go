package chatjimmy

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildCompletionWithToolCalls(t *testing.T) {
	tools := []Tool{{
		Type: "function",
		Function: ToolFunction{
			Name:       "read",
			Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
		},
	}}
	upstream := `I'll read it.
<tool_call>
{"name":"read","arguments":{"path":"main.go"}}
</tool_call>`
	completion := BuildCompletion("llama3.1-8B", upstream, Usage{TotalTokens: 1}, tools, nil)
	if len(completion.Choices) != 1 {
		t.Fatalf("choices = %d", len(completion.Choices))
	}
	choice := completion.Choices[0]
	if choice.FinishReason != "tool_calls" {
		t.Fatalf("finish_reason = %q", choice.FinishReason)
	}
	if len(choice.Message.ToolCalls) != 1 {
		t.Fatalf("tool_calls = %d", len(choice.Message.ToolCalls))
	}
	if choice.Message.ToolCalls[0].Function.Name != "read" {
		t.Fatalf("tool name = %q", choice.Message.ToolCalls[0].Function.Name)
	}
}

func TestBuildCompletionPlainText(t *testing.T) {
	completion := BuildCompletion("llama3.1-8B", "hello", Usage{PromptTokens: 1}, nil, nil)
	if completion.Choices[0].FinishReason != "stop" {
		t.Fatalf("finish_reason = %q", completion.Choices[0].FinishReason)
	}
	if completion.Choices[0].Message.Content == nil || *completion.Choices[0].Message.Content != "hello" {
		t.Fatalf("content = %#v", completion.Choices[0].Message.Content)
	}
}

func TestBuildCompletionUnknownToolsStayStop(t *testing.T) {
	tools := []Tool{{
		Type: "function",
		Function: ToolFunction{
			Name:       "bash",
			Parameters: json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}}}`),
		},
	}}
	upstream := `Nope.
<tool_call>
{"name":"webfetch","arguments":{"url":"https://example.com"}}
</tool_call>`
	completion := BuildCompletion("llama3.1-8B", upstream, Usage{}, tools, nil)
	if completion.Choices[0].FinishReason != "stop" {
		t.Fatalf("finish_reason = %q, want stop when all tool names are unknown", completion.Choices[0].FinishReason)
	}
	if len(completion.Choices[0].Message.ToolCalls) != 0 {
		t.Fatalf("tool_calls = %+v, want none", completion.Choices[0].Message.ToolCalls)
	}
	if completion.Choices[0].Message.Content == nil || *completion.Choices[0].Message.Content != "Nope." {
		t.Fatalf("content = %#v", completion.Choices[0].Message.Content)
	}
}

func TestBuildStreamChunksWithToolCalls(t *testing.T) {
	content := "working"
	completion := Completion{
		ID: "id", Object: "chat.completion.chunk", Created: 1, Model: "llama3.1-8B",
		Choices: []CompletionChoice{{
			Index: 0,
			Message: AssistantMessage{
				Role:      "assistant",
				Content:   &content,
				ToolCalls: []ToolCall{{ID: "call_1", Type: "function", Function: ToolCallFunction{Name: "read", Arguments: `{"path":"x"}`}}},
			},
			FinishReason: "tool_calls",
		}},
	}
	chunks := BuildStreamChunks(completion)
	if len(chunks) < 3 {
		t.Fatalf("chunks = %d, want at least 3", len(chunks))
	}
	last := chunks[len(chunks)-1]
	if last.Choices[0].FinishReason == nil || *last.Choices[0].FinishReason != "tool_calls" {
		t.Fatalf("last finish_reason = %#v", last.Choices[0].FinishReason)
	}

	sse := string(EncodeSSEChunks(chunks))
	if !strings.Contains(sse, `"index":0`) {
		t.Fatalf("expected stream tool_calls index in SSE: %q", sse)
	}
}

func TestNewOpenAIError(t *testing.T) {
	body := NewOpenAIError("bad", "invalid_request_error", "bad_code")
	if body.Error.Message != "bad" || body.Error.Type != "invalid_request_error" || body.Error.Code != "bad_code" {
		t.Fatalf("body = %+v", body)
	}
}

func TestBuildStreamChunksEmptyChoices(t *testing.T) {
	chunks := BuildStreamChunks(Completion{ID: "id", Created: 1, Model: "llama3.1-8B"})
	if len(chunks) != 1 {
		t.Fatalf("chunks = %d, want 1", len(chunks))
	}
	if chunks[0].Choices[0].FinishReason == nil || *chunks[0].Choices[0].FinishReason != "stop" {
		t.Fatalf("finish_reason = %#v", chunks[0].Choices[0].FinishReason)
	}
	if chunks[0].Choices[0].Delta.Role != "assistant" {
		t.Fatalf("delta.role = %q", chunks[0].Choices[0].Delta.Role)
	}
}

func TestNewCompletionIDFormat(t *testing.T) {
	id := NewCompletionID()
	if !strings.HasPrefix(id, "chatcmpl-") {
		t.Fatalf("id = %q", id)
	}
	if len(id) <= len("chatcmpl-") {
		t.Fatalf("id too short: %q", id)
	}
}

func TestAppendUsageChunkOnOff(t *testing.T) {
	content := "hello"
	completion := Completion{
		ID: "chatcmpl-test", Object: "chat.completion", Created: 1, Model: "llama3.1-8B",
		Choices: []CompletionChoice{{
			Index: 0,
			Message: AssistantMessage{
				Role:    "assistant",
				Content: &content,
			},
			FinishReason: "stop",
		}},
		Usage: Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
	}
	base := BuildStreamChunks(completion)

	without := base
	if len(without) != len(base) {
		t.Fatalf("without usage mutated base length")
	}
	for _, chunk := range without {
		if chunk.Usage != nil {
			t.Fatalf("unexpected usage on base chunk: %+v", chunk.Usage)
		}
	}

	with := AppendUsageChunk(base, completion)
	if len(with) != len(base)+1 {
		t.Fatalf("chunks = %d, want %d", len(with), len(base)+1)
	}
	last := with[len(with)-1]
	if len(last.Choices) != 0 {
		t.Fatalf("usage chunk choices = %d, want 0", len(last.Choices))
	}
	if last.Usage == nil {
		t.Fatal("usage chunk missing usage")
	}
	if last.Usage.PromptTokens != 10 || last.Usage.CompletionTokens != 5 || last.Usage.TotalTokens != 15 {
		t.Fatalf("usage = %+v", last.Usage)
	}
	if last.ID != completion.ID || last.Object != "chat.completion.chunk" {
		t.Fatalf("usage chunk id/object = %q %q", last.ID, last.Object)
	}

	sse := string(EncodeSSEChunks(with))
	if !strings.Contains(sse, `"prompt_tokens":10`) {
		t.Fatalf("SSE missing usage: %q", sse)
	}
	if !strings.HasSuffix(strings.TrimSpace(sse), "data: [DONE]") {
		t.Fatalf("SSE missing [DONE]: %q", sse)
	}

	sseOff := string(EncodeSSEChunks(base))
	if strings.Contains(sseOff, `"prompt_tokens"`) {
		t.Fatalf("SSE without include_usage leaked usage: %q", sseOff)
	}
}
