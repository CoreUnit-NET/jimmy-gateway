package chatjimmy

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
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

func TestLimitBytesUTF8Safe(t *testing.T) {
	// "é" is 2 bytes (c3 a9). A 4-byte cut of "abcé" would otherwise split it.
	got := limitBytes("abcé", 4)
	if !utf8.ValidString(got) {
		t.Fatalf("invalid utf8: %q", got)
	}
	if got != "abc" {
		t.Fatalf("got %q, want abc", got)
	}
	if got := limitBytes("abcd", 4); got != "abcd" {
		t.Fatalf("exact max = %q", got)
	}
	if got := limitBytes("ab", 4); got != "ab" {
		t.Fatalf("short = %q", got)
	}
	if got := limitBytes("é", 1); got != "" {
		t.Fatalf("split rune = %q, want empty", got)
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

func TestTranslateRequestMapsAliasToUpstream(t *testing.T) {
	req := ChatRequest{
		Model:    "gpt-4o",
		Messages: []Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	}
	out, err := TranslateRequest(req, TranslateOptions{})
	if err != nil {
		t.Fatalf("TranslateRequest: %v", err)
	}
	if out.Model != "gpt-4o" {
		t.Fatalf("response model = %q, want gpt-4o", out.Model)
	}
	if out.Payload.ChatOptions.SelectedModel != DefaultModel {
		t.Fatalf("selectedModel = %q, want %q", out.Payload.ChatOptions.SelectedModel, DefaultModel)
	}
}

func TestTranslateRequestToolChoiceNoneSkipsSchemas(t *testing.T) {
	req := ChatRequest{
		Messages:   []Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
		Tools:      []Tool{{Type: "function", Function: ToolFunction{Name: "read"}}},
		ToolChoice: json.RawMessage(`"none"`),
	}
	out, err := TranslateRequest(req, TranslateOptions{})
	if err != nil {
		t.Fatalf("TranslateRequest: %v", err)
	}
	if !out.SkipToolParse {
		t.Fatal("expected SkipToolParse true")
	}
	prompt := out.Payload.ChatOptions.SystemPrompt
	if strings.Contains(prompt, "# Tools") || strings.Contains(prompt, "<tools>") {
		t.Fatalf("system prompt should omit tools: %q", prompt)
	}
}

func TestTranslateRequestCompactsToolsJSONBeforeTruncate(t *testing.T) {
	long := strings.Repeat("a", MaxSystemPrompt-20)
	req := ChatRequest{
		Messages: []Message{
			{Role: "system", Content: mustJSON(long)},
			{Role: "user", Content: json.RawMessage(`"hi"`)},
		},
		Tools: []Tool{{
			Type: "function",
			Function: ToolFunction{
				Name:        "read",
				Description: "Read a file.",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
			},
		}},
	}
	out, err := TranslateRequest(req, TranslateOptions{})
	if err != nil {
		t.Fatalf("TranslateRequest: %v", err)
	}
	if !out.ToolsCompacted {
		t.Fatal("expected ToolsCompacted true")
	}
	prompt := out.Payload.ChatOptions.SystemPrompt
	if strings.Contains(prompt, "<tools>") {
		t.Fatalf("compacted prompt still has tools JSON: %q", prompt[:120])
	}
}

func TestTranslateRequestCapsToolResult(t *testing.T) {
	huge := strings.Repeat("x", MaxToolResultChars+50)
	req := ChatRequest{
		Messages: []Message{
			{Role: "tool", Name: "read", ToolCallID: "call_1", Content: mustJSON(huge)},
			{Role: "user", Content: json.RawMessage(`"thanks"`)},
		},
	}
	out, err := TranslateRequest(req, TranslateOptions{})
	if err != nil {
		t.Fatalf("TranslateRequest: %v", err)
	}
	got := out.Payload.Messages[0].Content
	if !strings.Contains(got, "...[truncated]") {
		t.Fatalf("content missing truncate marker: %q", got[len(got)-40:])
	}
	if strings.Count(got, "x") > MaxToolResultChars {
		t.Fatalf("tool result still too long: %d", strings.Count(got, "x"))
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

func TestNormalizeRole(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"SYSTEM", "system"},
		{" developer ", "system"},
		{"Assistant", "assistant"},
		{"function", "tool"},
		{"TOOL", "tool"},
		{"", "user"},
		{"human", "user"},
	}
	for _, tc := range tests {
		if got := normalizeRole(tc.in); got != tc.want {
			t.Fatalf("normalizeRole(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestTranslateRequestMaxCompletionTokens(t *testing.T) {
	n := 32
	req := ChatRequest{
		Messages:            []Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
		MaxCompletionTokens: &n,
	}
	out, err := TranslateRequest(req, TranslateOptions{})
	if err != nil {
		t.Fatalf("TranslateRequest: %v", err)
	}
	if out.Payload.ChatOptions.MaxTokens == nil || *out.Payload.ChatOptions.MaxTokens != 32 {
		t.Fatalf("maxTokens = %#v, want 32", out.Payload.ChatOptions.MaxTokens)
	}
}

func TestTranslateRequestMaxTokensPrefersMaxTokens(t *testing.T) {
	maxTokens, maxCompletion, fromOptions := 8, 32, 64
	req := ChatRequest{
		Messages:            []Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
		MaxTokens:           &maxTokens,
		MaxCompletionTokens: &maxCompletion,
		ChatOptions:         &ChatOptions{MaxTokens: &fromOptions},
	}
	out, err := TranslateRequest(req, TranslateOptions{})
	if err != nil {
		t.Fatalf("TranslateRequest: %v", err)
	}
	if out.Payload.ChatOptions.MaxTokens == nil || *out.Payload.ChatOptions.MaxTokens != 8 {
		t.Fatalf("maxTokens = %#v, want 8", out.Payload.ChatOptions.MaxTokens)
	}
}

func TestTranslateRequestStreamFromChatOptions(t *testing.T) {
	req := ChatRequest{
		Messages:    []Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
		ChatOptions: &ChatOptions{Stream: true},
	}
	out, err := TranslateRequest(req, TranslateOptions{})
	if err != nil {
		t.Fatalf("TranslateRequest: %v", err)
	}
	if !out.Payload.ChatOptions.Stream {
		t.Fatal("expected chatOptions.stream fallback")
	}
}

func TestTranslateRequestRejectsNNotOne(t *testing.T) {
	n := 2
	req := ChatRequest{
		Messages: []Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
		N:        &n,
	}
	_, err := TranslateRequest(req, TranslateOptions{})
	if err == nil {
		t.Fatal("expected error for n != 1")
	}
}

func TestParseStop(t *testing.T) {
	tests := []struct {
		name string
		raw  json.RawMessage
		want []string
	}{
		{name: "nil"},
		{name: "null", raw: json.RawMessage("null")},
		{name: "padded null", raw: json.RawMessage("  null  ")},
		{name: "empty string", raw: json.RawMessage(`""`)},
		{name: "whitespace string", raw: json.RawMessage(`"  "`)},
		{name: "string", raw: json.RawMessage(`"STOP"`), want: []string{"STOP"}},
		{name: "array", raw: json.RawMessage(`["a",""," b "]`), want: []string{"a", "b"}},
		{name: "empty array", raw: json.RawMessage(`[]`)},
		{name: "object", raw: json.RawMessage(`{}`)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseStop(tc.raw)
			if len(got) != len(tc.want) {
				t.Fatalf("parseStop(%q) = %#v, want %#v", tc.raw, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("parseStop(%q) = %#v, want %#v", tc.raw, got, tc.want)
				}
			}
		})
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
