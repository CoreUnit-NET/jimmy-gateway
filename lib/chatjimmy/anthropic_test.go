package chatjimmy

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAnthropicToChatRequestBasic(t *testing.T) {
	raw := []byte(`{
		"model":"claude-3-haiku-20240307",
		"max_tokens":128,
		"temperature":0.4,
		"messages":[{"role":"user","content":"hi"}]
	}`)
	req, err := AnthropicToChatRequest(raw)
	if err != nil {
		t.Fatalf("AnthropicToChatRequest: %v", err)
	}
	if req.Model != "claude-3-haiku-20240307" {
		t.Fatalf("model = %q", req.Model)
	}
	if req.MaxTokens == nil || *req.MaxTokens != 128 {
		t.Fatalf("max_tokens = %#v", req.MaxTokens)
	}
	if len(req.Messages) != 1 || req.Messages[0].Role != "user" {
		t.Fatalf("messages = %+v", req.Messages)
	}
}

func TestAnthropicToChatRequestToolsAndChoice(t *testing.T) {
	raw := []byte(`{
		"model":"claude-3-haiku-20240307",
		"max_tokens":32,
		"tool_choice":{"type":"none"},
		"tools":[{"name":"read","description":"Read","input_schema":{"type":"object"}}],
		"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]
	}`)
	req, err := AnthropicToChatRequest(raw)
	if err != nil {
		t.Fatalf("AnthropicToChatRequest: %v", err)
	}
	if string(req.ToolChoice) != `"none"` {
		t.Fatalf("tool_choice = %s", req.ToolChoice)
	}
	if len(req.Tools) != 1 || req.Tools[0].Function.Name != "read" {
		t.Fatalf("tools = %+v", req.Tools)
	}
}

func TestAnthropicToChatRequestToolUseAndResult(t *testing.T) {
	raw := []byte(`{
		"model":"claude-3-haiku-20240307",
		"max_tokens":32,
		"messages":[
			{"role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"read","input":{"path":"x"}}]},
			{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"file"}]}
		]
	}`)
	req, err := AnthropicToChatRequest(raw)
	if err != nil {
		t.Fatalf("AnthropicToChatRequest: %v", err)
	}
	if len(req.Messages) != 2 {
		t.Fatalf("messages = %+v", req.Messages)
	}
	if req.Messages[0].Role != "assistant" || len(req.Messages[0].ToolCalls) != 1 {
		t.Fatalf("assistant = %+v", req.Messages[0])
	}
	if req.Messages[1].Role != "tool" || req.Messages[1].ToolCallID != "toolu_1" {
		t.Fatalf("tool = %+v", req.Messages[1])
	}
}

func TestCompletionToAnthropicToolUse(t *testing.T) {
	content := "working"
	completion := Completion{
		ID:    "chatcmpl-abc",
		Model: "claude-3-haiku-20240307",
		Choices: []CompletionChoice{{
			FinishReason: "tool_calls",
			Message: AssistantMessage{
				Role:    "assistant",
				Content: &content,
				ToolCalls: []ToolCall{{
					ID:   "call_1",
					Type: "function",
					Function: ToolCallFunction{
						Name:      "read",
						Arguments: `{"path":"x"}`,
					},
				}},
			},
		}},
		Usage: Usage{PromptTokens: 3, CompletionTokens: 5},
	}
	msg := CompletionToAnthropic(completion)
	if msg.ID != "msg_abc" {
		t.Fatalf("id = %q", msg.ID)
	}
	if msg.StopReason != "tool_use" {
		t.Fatalf("stop_reason = %q", msg.StopReason)
	}
	if len(msg.Content) != 2 || msg.Content[1].Type != "tool_use" {
		t.Fatalf("content = %+v", msg.Content)
	}
	sse := string(EncodeAnthropicSSE(msg))
	if !strings.Contains(sse, "event: message_start") || !strings.Contains(sse, "event: message_stop") {
		t.Fatalf("sse = %q", sse)
	}
}

func TestEncodeAnthropicSSEToolUseStartHasEmptyInput(t *testing.T) {
	content := "working"
	msg := CompletionToAnthropic(Completion{
		ID:    "chatcmpl-abc",
		Model: "claude-3-haiku-20240307",
		Choices: []CompletionChoice{{
			FinishReason: "tool_calls",
			Message: AssistantMessage{
				Role:    "assistant",
				Content: &content,
				ToolCalls: []ToolCall{{
					ID:   "call_1",
					Type: "function",
					Function: ToolCallFunction{
						Name:      "read",
						Arguments: `{"path":"x"}`,
					},
				}},
			},
		}},
	})
	sse := string(EncodeAnthropicSSE(msg))

	var startInput, partialJSON string
	for _, ev := range strings.Split(sse, "\n\n") {
		lines := strings.Split(ev, "\n")
		var event, data string
		for _, line := range lines {
			if strings.HasPrefix(line, "event: ") {
				event = strings.TrimPrefix(line, "event: ")
			}
			if strings.HasPrefix(line, "data: ") {
				data = strings.TrimPrefix(line, "data: ")
			}
		}
		switch event {
		case "content_block_start":
			var payload struct {
				ContentBlock AnthropicBlock `json:"content_block"`
			}
			if err := json.Unmarshal([]byte(data), &payload); err != nil {
				t.Fatalf("start payload: %v", err)
			}
			if payload.ContentBlock.Type == "tool_use" {
				startInput = strings.TrimSpace(string(payload.ContentBlock.Input))
			}
		case "content_block_delta":
			var payload struct {
				Delta struct {
					Type        string `json:"type"`
					PartialJSON string `json:"partial_json"`
				} `json:"delta"`
			}
			if err := json.Unmarshal([]byte(data), &payload); err != nil {
				t.Fatalf("delta payload: %v", err)
			}
			if payload.Delta.Type == "input_json_delta" {
				partialJSON = payload.Delta.PartialJSON
			}
		}
	}
	if startInput != "{}" {
		t.Fatalf("tool_use start input = %q, want {}", startInput)
	}
	if partialJSON != `{"path":"x"}` {
		t.Fatalf("input_json_delta = %q", partialJSON)
	}
}

func TestAnthropicToChatRequestRejectsEmptyMessages(t *testing.T) {
	_, err := AnthropicToChatRequest([]byte(`{"model":"x","max_tokens":1,"messages":[]}`))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAnthropicToChatRequestNullMessages(t *testing.T) {
	_, err := AnthropicToChatRequest([]byte(`{"model":"x","max_tokens":1,"messages":null}`))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAnthropicToChatRequestNullFields(t *testing.T) {
	raw := []byte(`{
		"model":"claude-3-haiku-20240307",
		"max_tokens":32,
		"system":  null,
		"tools":null,
		"tool_choice": null ,
		"messages":[{"role":"user","content":"hi"}]
	}`)
	req, err := AnthropicToChatRequest(raw)
	if err != nil {
		t.Fatalf("AnthropicToChatRequest: %v", err)
	}
	if req.Tools != nil {
		t.Fatalf("tools = %+v, want nil", req.Tools)
	}
	if req.ToolChoice != nil {
		t.Fatalf("tool_choice = %s, want nil", req.ToolChoice)
	}
	if len(req.Messages) != 1 || req.Messages[0].Role != "user" {
		t.Fatalf("messages = %+v", req.Messages)
	}
}

func TestNewAnthropicError(t *testing.T) {
	body := NewAnthropicError("nope", "")
	raw, _ := json.Marshal(body)
	if !strings.Contains(string(raw), `"invalid_request_error"`) {
		t.Fatalf("body = %s", raw)
	}
}

func TestEncodeAnthropicSSENoHTMLEscape(t *testing.T) {
	content := "done <|eot_id|>"
	msg := CompletionToAnthropic(Completion{
		ID:    "chatcmpl-abc",
		Model: "claude-3-haiku-20240307",
		Choices: []CompletionChoice{{
			FinishReason: "stop",
			Message: AssistantMessage{
				Role:    "assistant",
				Content: &content,
			},
		}},
	})
	sse := string(EncodeAnthropicSSE(msg))
	if !strings.Contains(sse, `"text":"done <|eot_id|>"`) {
		t.Fatalf("Anthropic SSE HTML-escaped: %q", sse)
	}
	if strings.Contains(sse, `\u003c`) {
		t.Fatalf("Anthropic SSE HTML-escaped <: %q", sse)
	}
}
