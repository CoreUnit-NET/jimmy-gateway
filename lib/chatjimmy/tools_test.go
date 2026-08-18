package chatjimmy

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFormatToolsForPromptIncludesSchemas(t *testing.T) {
	tools := []Tool{{
		Type: "function",
		Function: ToolFunction{
			Name:        "read",
			Description: "Read a file. Extra detail ignored.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
		},
	}}
	prompt := FormatToolsForPrompt(tools, json.RawMessage(`"auto"`))
	if !strings.Contains(prompt, "- read(path: string)") {
		t.Fatalf("prompt missing signature: %q", prompt)
	}
	if !strings.Contains(prompt, "<tools>") {
		t.Fatal("expected compact tool schemas block")
	}
}

func TestFormatToolsForPromptToolChoiceNone(t *testing.T) {
	tools := []Tool{{Type: "function", Function: ToolFunction{Name: "read"}}}
	prompt := FormatToolsForPrompt(tools, json.RawMessage(`"none"`))
	if !strings.Contains(prompt, "Do NOT use tools") {
		t.Fatalf("prompt = %q", prompt)
	}
}

func TestFormatToolsForPromptRequiredToolName(t *testing.T) {
	tools := []Tool{{Type: "function", Function: ToolFunction{Name: "bash"}}}
	choice := json.RawMessage(`{"type":"function","function":{"name":"bash"}}`)
	prompt := FormatToolsForPrompt(tools, choice)
	if !strings.Contains(prompt, "You MUST call 'bash'") {
		t.Fatalf("prompt = %q", prompt)
	}
}

func TestParseToolCallsNormalizesRequiredArgs(t *testing.T) {
	tools := []Tool{{
		Type: "function",
		Function: ToolFunction{
			Name:       "read",
			Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
		},
	}}
	content := `<tool_call>
{"name":"read","arguments":{}}
</tool_call>`
	_, calls := ParseToolCalls(content, tools, func() string { return "call_1" })
	if len(calls) != 1 {
		t.Fatalf("calls = %d", len(calls))
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(calls[0].Function.Arguments), &args); err != nil {
		t.Fatalf("args json: %v", err)
	}
	if args["path"] != "" {
		t.Fatalf("path = %#v, want empty string default", args["path"])
	}
}

func TestParseToolCallsSkipsWhenNoTools(t *testing.T) {
	content := `<tool_call>{"name":"read","arguments":{"path":"x"}}</tool_call>`
	text, calls := ParseToolCalls(content, nil, func() string { return "call_1" })
	if len(calls) != 0 {
		t.Fatalf("calls = %d, want 0 when tools list is empty", len(calls))
	}
	if text != content {
		t.Fatalf("text = %q, want unchanged upstream content", text)
	}
}

func TestParseToolCallsStripsBlocksFromText(t *testing.T) {
	tools := []Tool{{Type: "function", Function: ToolFunction{Name: "read"}}}
	content := `Planning.
<tool_call>
{"name":"read","arguments":{"path":"x"}}
</tool_call>`
	text, calls := ParseToolCalls(content, tools, func() string { return "call_1" })
	if !strings.Contains(text, "Planning") {
		t.Fatalf("text = %q", text)
	}
	if strings.Contains(text, "<tool_call>") {
		t.Fatalf("tool_call tag leaked into text: %q", text)
	}
	if len(calls) != 1 {
		t.Fatalf("calls = %d", len(calls))
	}
}
