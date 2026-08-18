package chatjimmy

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGeminiToChatRequestBasic(t *testing.T) {
	raw := []byte(`{
		"contents":[{"role":"user","parts":[{"text":"hi"}]}],
		"generationConfig":{"temperature":0.2,"maxOutputTokens":64}
	}`)
	req, err := GeminiToChatRequest("gemini-1.5-flash", raw)
	if err != nil {
		t.Fatalf("GeminiToChatRequest: %v", err)
	}
	if req.Model != "gemini-1.5-flash" {
		t.Fatalf("model = %q", req.Model)
	}
	if req.MaxTokens == nil || *req.MaxTokens != 64 {
		t.Fatalf("max_tokens = %#v", req.MaxTokens)
	}
	if len(req.Messages) != 1 || req.Messages[0].Role != "user" {
		t.Fatalf("messages = %+v", req.Messages)
	}
}

func TestGeminiToChatRequestFunctionCallAndResponse(t *testing.T) {
	raw := []byte(`{
		"contents":[
			{"role":"model","parts":[{"functionCall":{"name":"read","args":{"path":"x"}}}]},
			{"role":"user","parts":[{"functionResponse":{"name":"read","response":{"ok":true}}}]}
		],
		"toolConfig":{"functionCallingConfig":{"mode":"NONE"}},
		"tools":[{"functionDeclarations":[{"name":"read","parameters":{"type":"object"}}]}]
	}`)
	req, err := GeminiToChatRequest("gemini-1.5-flash", raw)
	if err != nil {
		t.Fatalf("GeminiToChatRequest: %v", err)
	}
	if string(req.ToolChoice) != `"none"` {
		t.Fatalf("tool_choice = %s", req.ToolChoice)
	}
	if len(req.Tools) != 1 || req.Tools[0].Function.Name != "read" {
		t.Fatalf("tools = %+v", req.Tools)
	}
	if len(req.Messages) != 2 {
		t.Fatalf("messages = %+v", req.Messages)
	}
	if req.Messages[0].Role != "assistant" || len(req.Messages[0].ToolCalls) != 1 {
		t.Fatalf("assistant = %+v", req.Messages[0])
	}
	if req.Messages[1].Role != "tool" || req.Messages[1].Name != "read" {
		t.Fatalf("tool = %+v", req.Messages[1])
	}
}

func TestCompletionToGemini(t *testing.T) {
	content := "hello"
	completion := Completion{
		Model: "gemini-1.5-flash",
		Choices: []CompletionChoice{{
			FinishReason: "stop",
			Message: AssistantMessage{
				Role:    "assistant",
				Content: &content,
			},
		}},
		Usage: Usage{PromptTokens: 1, CompletionTokens: 2, TotalTokens: 3},
	}
	resp := CompletionToGemini(completion)
	if len(resp.Candidates) != 1 || resp.Candidates[0].FinishReason != "STOP" {
		t.Fatalf("candidates = %+v", resp.Candidates)
	}
	if resp.UsageMetadata.TotalTokenCount != 3 {
		t.Fatalf("usage = %+v", resp.UsageMetadata)
	}
	sse := string(EncodeGeminiSSE(resp))
	if !strings.HasPrefix(sse, "data: ") || !strings.Contains(sse, `"hello"`) {
		t.Fatalf("sse = %q", sse)
	}
}

func TestGeminiToChatRequestRejectsEmptyContents(t *testing.T) {
	_, err := GeminiToChatRequest("gemini-1.5-flash", []byte(`{"contents":[]}`))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNewGeminiError(t *testing.T) {
	body := NewGeminiError("nope", 401)
	raw, _ := json.Marshal(body)
	if !strings.Contains(string(raw), `"UNAUTHENTICATED"`) {
		t.Fatalf("body = %s", raw)
	}
}
