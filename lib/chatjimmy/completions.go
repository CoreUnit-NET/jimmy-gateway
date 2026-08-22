package chatjimmy

import (
	"encoding/json"
	"fmt"
	"strings"
)

// CompletionsRequest is the OpenAI legacy text-completions request subset we support.
type CompletionsRequest struct {
	Model         string          `json:"model"`
	Prompt        json.RawMessage `json:"prompt"`
	MaxTokens     *int            `json:"max_tokens,omitempty"`
	Temperature   *float64        `json:"temperature,omitempty"`
	TopP          *float64        `json:"top_p,omitempty"`
	Stop          json.RawMessage `json:"stop,omitempty"`
	Stream        bool            `json:"stream"`
	StreamOptions *StreamOptions  `json:"stream_options,omitempty"`
	N             *int            `json:"n,omitempty"`
}

// TextCompletion is an OpenAI legacy text-completions response.
type TextCompletion struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	Created int64                  `json:"created"`
	Model   string                 `json:"model"`
	Choices []TextCompletionChoice `json:"choices"`
	Usage   Usage                  `json:"usage"`
}

type TextCompletionChoice struct {
	Text         string  `json:"text"`
	Index        int     `json:"index"`
	Logprobs     *any    `json:"logprobs"`
	FinishReason *string `json:"finish_reason"`
}

// TextCompletionChunk is one buffered SSE event for text completions.
type TextCompletionChunk struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	Created int64                  `json:"created"`
	Model   string                 `json:"model"`
	Choices []TextCompletionChoice `json:"choices"`
	Usage   *Usage                 `json:"usage,omitempty"`
}

func (r *CompletionsRequest) UnmarshalJSON(data []byte) error {
	type alias CompletionsRequest
	var aux struct {
		alias
		Stream json.RawMessage `json:"stream"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	*r = CompletionsRequest(aux.alias)
	if err := parseBoolish(aux.Stream, &r.Stream); err != nil {
		return err
	}
	return nil
}

// CompletionsToChatRequest maps a legacy completions body onto the shared chat path.
// Tools are ignored. Only n==1 (or omitted) is supported.
func CompletionsToChatRequest(raw []byte) (ChatRequest, error) {
	var req CompletionsRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return ChatRequest{}, fmt.Errorf("Request body must be JSON")
	}
	if req.N != nil && *req.N != 1 {
		return ChatRequest{}, fmt.Errorf("n must be 1")
	}
	prompt, err := parseCompletionsPrompt(req.Prompt)
	if err != nil {
		return ChatRequest{}, err
	}
	content, err := json.Marshal(prompt)
	if err != nil {
		return ChatRequest{}, fmt.Errorf("prompt must be a string or array of strings")
	}
	return ChatRequest{
		Model:         req.Model,
		Messages:      []Message{{Role: "user", Content: content}},
		Stream:        req.Stream,
		StreamOptions: req.StreamOptions,
		Temperature:   req.Temperature,
		TopP:          req.TopP,
		MaxTokens:     req.MaxTokens,
		Stop:          req.Stop,
		N:             req.N,
	}, nil
}

func parseCompletionsPrompt(raw json.RawMessage) (string, error) {
	if isEmptyJSON(raw) {
		return "", fmt.Errorf("prompt is required")
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if strings.TrimSpace(s) == "" {
			return "", fmt.Errorf("prompt is required")
		}
		return s, nil
	}
	var parts []string
	if err := json.Unmarshal(raw, &parts); err == nil {
		if len(parts) == 0 {
			return "", fmt.Errorf("prompt is required")
		}
		joined := strings.Join(parts, "\n")
		if strings.TrimSpace(joined) == "" {
			return "", fmt.Errorf("prompt is required")
		}
		return joined, nil
	}
	return "", fmt.Errorf("prompt must be a string or array of strings")
}

// ChatToTextCompletion maps a chat completion onto the legacy text-completions shape.
func ChatToTextCompletion(completion Completion) TextCompletion {
	text := ""
	finish := "stop"
	if len(completion.Choices) > 0 {
		choice := completion.Choices[0]
		if choice.Message.Content != nil {
			text = *choice.Message.Content
		}
		if choice.FinishReason != "" {
			finish = choice.FinishReason
		}
		// Legacy completions have no tool_calls; treat tool_calls like stop text.
		if finish == "tool_calls" {
			finish = "stop"
		}
	}
	reason := finish
	return TextCompletion{
		ID:      chatIDToCompletionID(completion.ID),
		Object:  "text_completion",
		Created: completion.Created,
		Model:   completion.Model,
		Choices: []TextCompletionChoice{{
			Text:         text,
			Index:        0,
			Logprobs:     nil,
			FinishReason: &reason,
		}},
		Usage: completion.Usage,
	}
}

func chatIDToCompletionID(id string) string {
	switch {
	case strings.HasPrefix(id, "cmpl-"):
		return id
	case strings.HasPrefix(id, "chatcmpl-"):
		return "cmpl-" + strings.TrimPrefix(id, "chatcmpl-")
	default:
		if id == "" {
			return "cmpl-unknown"
		}
		return "cmpl-" + id
	}
}

// BuildTextStreamChunks emits buffered text-completion SSE chunks (role-less deltas).
func BuildTextStreamChunks(completion TextCompletion) []TextCompletionChunk {
	text := ""
	finish := "stop"
	if len(completion.Choices) > 0 {
		text = completion.Choices[0].Text
		if completion.Choices[0].FinishReason != nil && *completion.Choices[0].FinishReason != "" {
			finish = *completion.Choices[0].FinishReason
		}
	}
	reason := finish
	return []TextCompletionChunk{
		{
			ID: completion.ID, Object: "text_completion", Created: completion.Created, Model: completion.Model,
			Choices: []TextCompletionChoice{{
				Text:         text,
				Index:        0,
				Logprobs:     nil,
				FinishReason: nil,
			}},
		},
		{
			ID: completion.ID, Object: "text_completion", Created: completion.Created, Model: completion.Model,
			Choices: []TextCompletionChoice{{
				Text:         "",
				Index:        0,
				Logprobs:     nil,
				FinishReason: &reason,
			}},
		},
	}
}

// AppendTextUsageChunk adds a final text-completion stream chunk with usage and empty choices.
// Call only when the client requested stream_options.include_usage.
func AppendTextUsageChunk(chunks []TextCompletionChunk, completion TextCompletion) []TextCompletionChunk {
	usage := completion.Usage
	return append(chunks, TextCompletionChunk{
		ID:      completion.ID,
		Object:  "text_completion",
		Created: completion.Created,
		Model:   completion.Model,
		Choices: []TextCompletionChoice{},
		Usage:   &usage,
	})
}

// EncodeTextSSEChunks encodes text-completion chunks as OpenAI SSE + [DONE].
func EncodeTextSSEChunks(chunks []TextCompletionChunk) []byte {
	var out []byte
	for _, chunk := range chunks {
		b, _ := MarshalJSON(chunk)
		out = append(out, []byte("data: "+string(b)+"\n\n")...)
	}
	out = append(out, []byte("data: [DONE]\n\n")...)
	return out
}
