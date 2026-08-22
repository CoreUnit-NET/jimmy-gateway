package chatjimmy

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCompletionsToChatRequestPromptString(t *testing.T) {
	raw := []byte(`{"model":"llama3.1-8B","prompt":"hello","max_tokens":16,"temperature":0.2}`)
	req, err := CompletionsToChatRequest(raw)
	if err != nil {
		t.Fatalf("CompletionsToChatRequest: %v", err)
	}
	if req.Model != "llama3.1-8B" {
		t.Fatalf("model = %q", req.Model)
	}
	if len(req.Messages) != 1 || req.Messages[0].Role != "user" {
		t.Fatalf("messages = %+v", req.Messages)
	}
	var content string
	if err := json.Unmarshal(req.Messages[0].Content, &content); err != nil {
		t.Fatalf("content json: %v", err)
	}
	if content != "hello" {
		t.Fatalf("content = %q", content)
	}
	if req.MaxTokens == nil || *req.MaxTokens != 16 {
		t.Fatalf("max_tokens = %#v", req.MaxTokens)
	}
	if req.Temperature == nil || *req.Temperature != 0.2 {
		t.Fatalf("temperature = %#v", req.Temperature)
	}
	if len(req.Tools) != 0 {
		t.Fatalf("tools should be ignored/empty: %+v", req.Tools)
	}
}

func TestCompletionsToChatRequestPromptArray(t *testing.T) {
	raw := []byte(`{"prompt":["line1","line2"],"stream":"true"}`)
	req, err := CompletionsToChatRequest(raw)
	if err != nil {
		t.Fatalf("CompletionsToChatRequest: %v", err)
	}
	var content string
	if err := json.Unmarshal(req.Messages[0].Content, &content); err != nil {
		t.Fatalf("content json: %v", err)
	}
	if content != "line1\nline2" {
		t.Fatalf("content = %q", content)
	}
	if !req.Stream {
		t.Fatal("stream want true")
	}
}

func TestCompletionsToChatRequestRejectsN(t *testing.T) {
	raw := []byte(`{"prompt":"hi","n":2}`)
	_, err := CompletionsToChatRequest(raw)
	if err == nil || !strings.Contains(err.Error(), "n must be 1") {
		t.Fatalf("err = %v", err)
	}
}

func TestCompletionsToChatRequestRequiresPrompt(t *testing.T) {
	for _, raw := range []string{
		`{}`,
		`{"prompt":""}`,
		`{"prompt":[]}`,
		`{"prompt":123}`,
	} {
		_, err := CompletionsToChatRequest([]byte(raw))
		if err == nil {
			t.Fatalf("raw=%s: expected error", raw)
		}
	}
}

func TestChatToTextCompletionMapping(t *testing.T) {
	content := "answer"
	completion := Completion{
		ID:      "chatcmpl-abc",
		Object:  "chat.completion",
		Created: 42,
		Model:   "llama3.1-8B",
		Choices: []CompletionChoice{{
			Index: 0,
			Message: AssistantMessage{
				Role:    "assistant",
				Content: &content,
			},
			FinishReason: "length",
		}},
		Usage: Usage{PromptTokens: 3, CompletionTokens: 4, TotalTokens: 7},
	}
	out := ChatToTextCompletion(completion)
	if out.ID != "cmpl-abc" {
		t.Fatalf("id = %q", out.ID)
	}
	if out.Object != "text_completion" {
		t.Fatalf("object = %q", out.Object)
	}
	if len(out.Choices) != 1 || out.Choices[0].Text != "answer" {
		t.Fatalf("choices = %+v", out.Choices)
	}
	if out.Choices[0].FinishReason == nil || *out.Choices[0].FinishReason != "length" {
		t.Fatalf("finish_reason = %#v", out.Choices[0].FinishReason)
	}
	if out.Usage.TotalTokens != 7 {
		t.Fatalf("usage = %+v", out.Usage)
	}
}

func TestBuildTextStreamChunks(t *testing.T) {
	reason := "stop"
	completion := TextCompletion{
		ID: "cmpl-1", Object: "text_completion", Created: 1, Model: "m",
		Choices: []TextCompletionChoice{{Text: "hi", Index: 0, FinishReason: &reason}},
	}
	chunks := BuildTextStreamChunks(completion)
	if len(chunks) != 2 {
		t.Fatalf("chunks = %d", len(chunks))
	}
	if chunks[0].Choices[0].Text != "hi" || chunks[0].Choices[0].FinishReason != nil {
		t.Fatalf("first = %+v", chunks[0].Choices[0])
	}
	if chunks[1].Choices[0].FinishReason == nil || *chunks[1].Choices[0].FinishReason != "stop" {
		t.Fatalf("second = %+v", chunks[1].Choices[0])
	}
	sse := string(EncodeTextSSEChunks(chunks))
	if !strings.Contains(sse, `"text":"hi"`) || !strings.Contains(sse, "data: [DONE]") {
		t.Fatalf("sse = %q", sse)
	}
}

func TestCompletionsToChatRequestEdgeCases(t *testing.T) {
	t.Run("n equals 1 allowed", func(t *testing.T) {
		req, err := CompletionsToChatRequest([]byte(`{"prompt":"hi","n":1}`))
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if req.N == nil || *req.N != 1 {
			t.Fatalf("n = %#v", req.N)
		}
	})

	t.Run("whitespace only prompt string", func(t *testing.T) {
		_, err := CompletionsToChatRequest([]byte(`{"prompt":"  \t\n"}`))
		if err == nil || !strings.Contains(err.Error(), "prompt is required") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("whitespace only prompt array", func(t *testing.T) {
		_, err := CompletionsToChatRequest([]byte(`{"prompt":[" ","\t"]}`))
		if err == nil || !strings.Contains(err.Error(), "prompt is required") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("invalid json body", func(t *testing.T) {
		_, err := CompletionsToChatRequest([]byte(`not-json`))
		if err == nil || !strings.Contains(err.Error(), "JSON") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("sampling and stop passthrough", func(t *testing.T) {
		raw := []byte(`{"prompt":"hi","top_p":0.9,"stop":["END","STOP"]}`)
		req, err := CompletionsToChatRequest(raw)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if req.TopP == nil || *req.TopP != 0.9 {
			t.Fatalf("top_p = %#v", req.TopP)
		}
		if string(req.Stop) != `["END","STOP"]` {
			t.Fatalf("stop = %s", req.Stop)
		}
	})

	t.Run("tools in body ignored", func(t *testing.T) {
		raw := []byte(`{"prompt":"hi","tools":[{"type":"function","function":{"name":"bash"}}]}`)
		req, err := CompletionsToChatRequest(raw)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if len(req.Tools) != 0 {
			t.Fatalf("tools = %+v, want empty", req.Tools)
		}
	})
}

func TestChatToTextCompletionEdgeCases(t *testing.T) {
	t.Run("tool_calls finish becomes stop", func(t *testing.T) {
		content := "calling"
		out := ChatToTextCompletion(Completion{
			ID: "chatcmpl-x", Model: "m",
			Choices: []CompletionChoice{{
				Message:      AssistantMessage{Role: "assistant", Content: &content},
				FinishReason: "tool_calls",
			}},
		})
		if out.Choices[0].FinishReason == nil || *out.Choices[0].FinishReason != "stop" {
			t.Fatalf("finish = %#v", out.Choices[0].FinishReason)
		}
		if out.ID != "cmpl-x" {
			t.Fatalf("id = %q", out.ID)
		}
	})

	t.Run("already cmpl id preserved", func(t *testing.T) {
		out := ChatToTextCompletion(Completion{ID: "cmpl-keep", Model: "m"})
		if out.ID != "cmpl-keep" {
			t.Fatalf("id = %q", out.ID)
		}
	})

	t.Run("empty id becomes cmpl-unknown", func(t *testing.T) {
		out := ChatToTextCompletion(Completion{ID: "", Model: "m"})
		if out.ID != "cmpl-unknown" {
			t.Fatalf("id = %q", out.ID)
		}
	})

	t.Run("bare id prefixed", func(t *testing.T) {
		out := ChatToTextCompletion(Completion{ID: "raw123", Model: "m"})
		if out.ID != "cmpl-raw123" {
			t.Fatalf("id = %q", out.ID)
		}
	})

	t.Run("nil content and empty choices", func(t *testing.T) {
		out := ChatToTextCompletion(Completion{
			ID: "chatcmpl-z", Model: "m",
			Choices: []CompletionChoice{{
				Message:      AssistantMessage{Role: "assistant", Content: nil},
				FinishReason: "stop",
			}},
		})
		if out.Choices[0].Text != "" {
			t.Fatalf("text = %q", out.Choices[0].Text)
		}

		empty := ChatToTextCompletion(Completion{ID: "chatcmpl-e", Model: "m"})
		if len(empty.Choices) != 1 || empty.Choices[0].Text != "" {
			t.Fatalf("empty choices = %+v", empty.Choices)
		}
		if empty.Choices[0].FinishReason == nil || *empty.Choices[0].FinishReason != "stop" {
			t.Fatalf("finish = %#v", empty.Choices[0].FinishReason)
		}
	})
}

func TestBuildTextStreamChunksEmpty(t *testing.T) {
	chunks := BuildTextStreamChunks(TextCompletion{ID: "cmpl-1", Model: "m"})
	if len(chunks) != 2 {
		t.Fatalf("chunks = %d", len(chunks))
	}
	if chunks[0].Choices[0].Text != "" {
		t.Fatalf("first text = %q", chunks[0].Choices[0].Text)
	}
	if chunks[1].Choices[0].FinishReason == nil || *chunks[1].Choices[0].FinishReason != "stop" {
		t.Fatalf("second = %+v", chunks[1].Choices[0])
	}
}
