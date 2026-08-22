package chatjimmy

import (
	"encoding/json"
	"fmt"
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
	if !strings.Contains(prompt, `{"name": "read", "arguments": {"path": "value"}}`) {
		t.Fatalf("prompt missing real tool example: %q", prompt)
	}
	if !strings.Contains(prompt, "<tools>") {
		t.Fatal("expected compact tool schemas block")
	}
}

func TestFormatToolsForPromptToolChoiceNone(t *testing.T) {
	tools := []Tool{{Type: "function", Function: ToolFunction{Name: "read"}}}
	prompt := FormatToolsForPrompt(tools, json.RawMessage(`"none"`))
	if prompt != "" {
		t.Fatalf("prompt = %q, want empty (no schema inject)", prompt)
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

func TestParseToolCallsDoesNotInventRequiredArgs(t *testing.T) {
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
	if _, ok := args["path"]; ok {
		t.Fatalf("path should stay missing when model omitted required arg: %#v", args["path"])
	}
}

func TestParseToolCallsSkipsWhenNoTools(t *testing.T) {
	content := `Sure. <tool_call>{"name":"read","arguments":{"path":"x"}}</tool_call>`
	text, calls := ParseToolCalls(content, nil, func() string { return "call_1" })
	if len(calls) != 0 {
		t.Fatalf("calls = %d, want 0 when tools list is empty", len(calls))
	}
	if text != "Sure." {
		t.Fatalf("text = %q, want XML stripped when tools list is empty", text)
	}
	if strings.Contains(text, "<tool_call>") {
		t.Fatalf("tool_call tag leaked into text: %q", text)
	}
}

func TestParseToolCallsDropsUnknownNames(t *testing.T) {
	tools := []Tool{{
		Type: "function",
		Function: ToolFunction{
			Name:       "bash",
			Parameters: json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}},"required":["command"]}`),
		},
	}}
	content := `<tool_call>
{"name":"webfetch","arguments":{"url":"https://example.com"}}
</tool_call>
<tool_call>
{"name":"bash","arguments":{"command":"ls"}}
</tool_call>`
	text, calls := ParseToolCalls(content, tools, func() string { return "call_1" })
	if len(calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(calls))
	}
	if calls[0].Function.Name != "bash" {
		t.Fatalf("name = %q, want bash", calls[0].Function.Name)
	}
	if calls[0].Function.Arguments != `{"command":"ls"}` {
		t.Fatalf("arguments = %q", calls[0].Function.Arguments)
	}
	if strings.Contains(text, "<tool_call>") || strings.Contains(text, "webfetch") {
		t.Fatalf("text leaked tool xml or dropped name: %q", text)
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

func TestFilterToolsDropsOpenCodeNames(t *testing.T) {
	in := []Tool{
		{Type: "function", Function: ToolFunction{Name: "bash"}},
		{Type: "function", Function: ToolFunction{Name: "WebFetch"}},
		{Type: "function", Function: ToolFunction{Name: "todowrite"}},
		{Type: "function", Function: ToolFunction{Name: "read"}},
		{Type: "function", Function: ToolFunction{Name: "skill"}},
	}
	out := FilterTools(in)
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2: %+v", len(out), out)
	}
	if out[0].Function.Name != "bash" || out[1].Function.Name != "read" {
		t.Fatalf("names = %q %q", out[0].Function.Name, out[1].Function.Name)
	}
	if FilterTools(nil) != nil {
		t.Fatal("nil tools should stay nil")
	}
}

func TestParseToolCallsEdgeCases(t *testing.T) {
	tools := []Tool{{
		Type: "function",
		Function: ToolFunction{
			Name:       "bash",
			Parameters: json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}},"required":["command"]}`),
		},
	}}
	id := func() string { return "call_1" }

	t.Run("malformed json strips without calls", func(t *testing.T) {
		content := `Hi <tool_call>{not-json</tool_call> there`
		text, calls := ParseToolCalls(content, tools, id)
		if len(calls) != 0 {
			t.Fatalf("calls = %+v", calls)
		}
		if strings.Contains(text, "<tool_call>") {
			t.Fatalf("xml leaked: %q", text)
		}
		if !strings.Contains(text, "Hi") || !strings.Contains(text, "there") {
			t.Fatalf("text = %q", text)
		}
	})

	t.Run("empty tool_call block", func(t *testing.T) {
		text, calls := ParseToolCalls(`ok <tool_call></tool_call>`, tools, id)
		if len(calls) != 0 {
			t.Fatalf("calls = %+v", calls)
		}
		if text != "ok" {
			t.Fatalf("text = %q", text)
		}
	})

	t.Run("case mismatched name dropped", func(t *testing.T) {
		content := `<tool_call>{"name":"Bash","arguments":{"command":"ls"}}</tool_call>`
		text, calls := ParseToolCalls(content, tools, id)
		if len(calls) != 0 {
			t.Fatalf("calls = %+v, want none for case mismatch", calls)
		}
		if strings.Contains(text, "<tool_call>") || strings.Contains(text, "Bash") {
			t.Fatalf("text = %q", text)
		}
	})

	t.Run("arguments as json string", func(t *testing.T) {
		content := `<tool_call>{"name":"bash","arguments":"{\"command\":\"echo hi\"}"}</tool_call>`
		_, calls := ParseToolCalls(content, tools, id)
		if len(calls) != 1 {
			t.Fatalf("calls = %d", len(calls))
		}
		var args map[string]any
		if err := json.Unmarshal([]byte(calls[0].Function.Arguments), &args); err != nil {
			t.Fatalf("args: %v", err)
		}
		if args["command"] != "echo hi" {
			t.Fatalf("args = %#v", args)
		}
	})

	t.Run("tool alias name keys", func(t *testing.T) {
		for _, body := range []string{
			`{"tool":"bash","arguments":{"command":"x"}}`,
			`{"tool_name":"bash","parameters":{"command":"y"}}`,
			`{"function":{"name":"bash","arguments":{"command":"z"}}}`,
		} {
			content := "<tool_call>" + body + "</tool_call>"
			_, calls := ParseToolCalls(content, tools, id)
			if len(calls) != 1 || calls[0].Function.Name != "bash" {
				t.Fatalf("body=%s calls=%+v", body, calls)
			}
		}
	})

	t.Run("duplicate allowed calls kept", func(t *testing.T) {
		n := 0
		newID := func() string {
			n++
			return fmt.Sprintf("call_%d", n)
		}
		content := `<tool_call>{"name":"bash","arguments":{"command":"a"}}</tool_call>
<tool_call>{"name":"bash","arguments":{"command":"b"}}</tool_call>`
		_, calls := ParseToolCalls(content, tools, newID)
		if len(calls) != 2 {
			t.Fatalf("calls = %d", len(calls))
		}
	})

	t.Run("unclosed tool_call stripped", func(t *testing.T) {
		text, calls := ParseToolCalls(`Before <tool_call>{"name":"bash","arguments":{"command":"x"}`, tools, id)
		if len(calls) != 0 {
			t.Fatalf("calls = %+v, want 0 for unclosed", calls)
		}
		if text != "Before" {
			t.Fatalf("text = %q, want Before", text)
		}
		if strings.Contains(text, "<tool_call>") {
			t.Fatalf("xml leaked: %q", text)
		}
	})

	t.Run("unclosed tool_call empty tools", func(t *testing.T) {
		text, calls := ParseToolCalls(`Sure <tool_call>{"name":"read"`, nil, id)
		if len(calls) != 0 {
			t.Fatalf("calls = %+v", calls)
		}
		if text != "Sure" {
			t.Fatalf("text = %q", text)
		}
	})
}
