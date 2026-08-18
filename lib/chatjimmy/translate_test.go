package chatjimmy

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFilterTools(t *testing.T) {
	tools := []Tool{
		{Type: "function", Function: ToolFunction{Name: "read"}},
		{Type: "function", Function: ToolFunction{Name: "webfetch"}},
		{Type: "function", Function: ToolFunction{Name: "bash"}},
	}
	got := FilterTools(tools)
	if len(got) != 2 {
		t.Fatalf("FilterTools len = %d, want 2", len(got))
	}
}

func TestTranslateRequestSystemAndUser(t *testing.T) {
	req := ChatRequest{
		Model: "llama3.1-8B",
		Messages: []Message{
			{Role: "system", Content: json.RawMessage(`"You are helpful"`)},
			{Role: "user", Content: json.RawMessage(`"hello"`)},
		},
	}
	out, err := TranslateRequest(req, TranslateOptions{DefaultModel: DefaultModel})
	if err != nil {
		t.Fatalf("TranslateRequest: %v", err)
	}
	if out.Payload.ChatOptions.SystemPrompt != "You are helpful" {
		t.Fatalf("system prompt = %q", out.Payload.ChatOptions.SystemPrompt)
	}
	if len(out.Payload.Messages) != 1 || out.Payload.Messages[0].Role != "user" {
		t.Fatalf("messages = %+v", out.Payload.Messages)
	}
}

func TestTranslateRequestTruncatesSystemPrompt(t *testing.T) {
	long := strings.Repeat("a", MaxSystemPrompt+100)
	req := ChatRequest{
		Messages: []Message{
			{Role: "system", Content: json.RawMessage(`"` + long + `"`)},
			{Role: "user", Content: json.RawMessage(`"hi"`)},
		},
	}
	out, err := TranslateRequest(req, TranslateOptions{DefaultModel: DefaultModel})
	if err != nil {
		t.Fatalf("TranslateRequest: %v", err)
	}
	if !out.SystemTruncated {
		t.Fatal("expected SystemTruncated true")
	}
	if len(out.Payload.ChatOptions.SystemPrompt) != MaxSystemPrompt {
		t.Fatalf("system prompt len = %d, want %d", len(out.Payload.ChatOptions.SystemPrompt), MaxSystemPrompt)
	}
}

func TestParseToolCalls(t *testing.T) {
	tools := []Tool{{
		Type: "function",
		Function: ToolFunction{
			Name:       "read",
			Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
		},
	}}
	content := `I'll read the file.
<tool_call>
{"name":"read","arguments":{"path":"main.go"}}
</tool_call>`
	text, calls := ParseToolCalls(content, tools, func() string { return "call_test" })
	if text == "" {
		t.Fatal("expected leftover text")
	}
	if len(calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(calls))
	}
	if calls[0].Function.Name != "read" {
		t.Fatalf("tool name = %q", calls[0].Function.Name)
	}
}

func TestParseUpstreamStats(t *testing.T) {
	raw := `Hello world<|stats|>{"prefill_tokens":10,"decode_tokens":5,"total_tokens":15}<|/stats|>`
	parsed := ParseUpstream(raw)
	if parsed.Text != "Hello world" {
		t.Fatalf("text = %q", parsed.Text)
	}
	if parsed.Usage.TotalTokens != 15 {
		t.Fatalf("usage = %+v", parsed.Usage)
	}
}

func TestTranslateRequestAssistantToolCalls(t *testing.T) {
	req := ChatRequest{
		Messages: []Message{
			{
				Role:    "assistant",
				Content: json.RawMessage(`"Checking file"`),
				ToolCalls: []ToolCall{{
					ID:   "call_1",
					Type: "function",
					Function: ToolCallFunction{
						Name:      "read",
						Arguments: `{"path":"main.go"}`,
					},
				}},
			},
			{Role: "user", Content: json.RawMessage(`"continue"`)},
		},
	}
	out, err := TranslateRequest(req, TranslateOptions{DefaultModel: DefaultModel})
	if err != nil {
		t.Fatalf("TranslateRequest: %v", err)
	}
	if len(out.Payload.Messages) != 2 {
		t.Fatalf("messages = %d", len(out.Payload.Messages))
	}
	assistant := out.Payload.Messages[0]
	if assistant.Role != "assistant" {
		t.Fatalf("role = %q", assistant.Role)
	}
	if !strings.Contains(assistant.Content, "<tool_call>") {
		t.Fatalf("content = %q", assistant.Content)
	}
	if !strings.Contains(assistant.Content, `"name":"read"`) && !strings.Contains(assistant.Content, `"name": "read"`) {
		t.Fatalf("content missing tool name: %q", assistant.Content)
	}
}

func TestTranslateRequestToolResultMessage(t *testing.T) {
	req := ChatRequest{
		Messages: []Message{
			{
				Role:       "tool",
				Name:       "read",
				ToolCallID: "call_1",
				Content:    json.RawMessage(`"file contents"`),
			},
			{Role: "user", Content: json.RawMessage(`"thanks"`)},
		},
	}
	out, err := TranslateRequest(req, TranslateOptions{DefaultModel: DefaultModel})
	if err != nil {
		t.Fatalf("TranslateRequest: %v", err)
	}
	toolMsg := out.Payload.Messages[0]
	if toolMsg.Role != "user" {
		t.Fatalf("role = %q", toolMsg.Role)
	}
	if !strings.Contains(toolMsg.Content, "<tool_result>") {
		t.Fatalf("content = %q", toolMsg.Content)
	}
	if !strings.Contains(toolMsg.Content, `"tool_call_id": "call_1"`) && !strings.Contains(toolMsg.Content, `"tool_call_id":"call_1"`) {
		t.Fatalf("content missing call id: %q", toolMsg.Content)
	}
}

func TestTranslateRequestInjectToolsIntoSystemPrompt(t *testing.T) {
	req := ChatRequest{
		Messages: []Message{
			{Role: "user", Content: json.RawMessage(`"run tool"`)},
		},
		Tools: []Tool{{
			Type:     "function",
			Function: ToolFunction{Name: "bash", Description: "Run shell."},
		}},
	}
	out, err := TranslateRequest(req, TranslateOptions{DefaultModel: DefaultModel})
	if err != nil {
		t.Fatalf("TranslateRequest: %v", err)
	}
	if !strings.Contains(out.Payload.ChatOptions.SystemPrompt, "# Tools") {
		t.Fatalf("system prompt = %q", out.Payload.ChatOptions.SystemPrompt)
	}
	if !strings.Contains(out.Payload.ChatOptions.SystemPrompt, "bash") {
		t.Fatalf("system prompt missing bash: %q", out.Payload.ChatOptions.SystemPrompt)
	}
}

func TestTranslateRequestOnlySystemMessagesError(t *testing.T) {
	req := ChatRequest{
		Messages: []Message{
			{Role: "system", Content: json.RawMessage(`"rules"`)},
		},
	}
	_, err := TranslateRequest(req, TranslateOptions{DefaultModel: DefaultModel})
	if err == nil {
		t.Fatal("expected error for system-only messages")
	}
}

func TestBuildStreamChunks(t *testing.T) {
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
	}
	chunks := BuildStreamChunks(completion)
	if len(chunks) != 3 {
		t.Fatalf("chunks = %d, want 3", len(chunks))
	}
	sse := string(EncodeSSEChunks(chunks))
	if !strings.Contains(sse, "[DONE]") {
		t.Fatal("expected [DONE] in SSE output")
	}
}
