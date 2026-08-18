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
	body, status := NewOpenAIError("bad", "invalid_request_error", "bad_code", 400)
	if status != 400 {
		t.Fatalf("status = %d", status)
	}
	if body.Error.Message != "bad" || body.Error.Code != "bad_code" {
		t.Fatalf("body = %+v", body)
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
