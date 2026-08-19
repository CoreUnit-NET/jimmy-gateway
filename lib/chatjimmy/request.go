package chatjimmy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

func (r *ChatRequest) UnmarshalJSON(data []byte) error {
	type alias struct {
		Model               string          `json:"model"`
		Messages            json.RawMessage `json:"messages"`
		Stream              json.RawMessage `json:"stream"`
		Tools               []Tool          `json:"tools"`
		ToolChoice          json.RawMessage `json:"tool_choice"`
		Temperature         *float64        `json:"temperature"`
		TopP                *float64        `json:"top_p"`
		TopK                *int            `json:"top_k"`
		TopKCamel           *int            `json:"topK"`
		MaxTokens           *int            `json:"max_tokens"`
		MaxCompletionTokens *int            `json:"max_completion_tokens"`
		Stop                json.RawMessage `json:"stop"`
		N                   *int            `json:"n"`
		StreamOptions       *StreamOptions  `json:"stream_options"`
		ChatOptions         *ChatOptions    `json:"chatOptions"`
	}
	var aux alias
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	r.Model = aux.Model
	r.Tools = aux.Tools
	r.ToolChoice = aux.ToolChoice
	r.Temperature = aux.Temperature
	r.TopP = aux.TopP
	r.TopK = aux.TopK
	r.TopKCamel = aux.TopKCamel
	r.MaxTokens = aux.MaxTokens
	r.MaxCompletionTokens = aux.MaxCompletionTokens
	r.Stop = aux.Stop
	r.N = aux.N
	r.StreamOptions = aux.StreamOptions
	r.ChatOptions = aux.ChatOptions

	if err := parseBoolish(aux.Stream, &r.Stream); err != nil {
		return err
	}
	if err := parseMessages(aux.Messages, &r.Messages); err != nil {
		return err
	}
	return nil
}

func parseMessages(raw json.RawMessage, dest *[]Message) error {
	if isEmptyJSON(raw) {
		return nil
	}
	if bytes.TrimSpace(raw)[0] != '[' {
		return fmt.Errorf("messages must be a non-empty array")
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		return fmt.Errorf("messages must be a non-empty array")
	}
	return nil
}

func parseBoolish(raw json.RawMessage, dest *bool) error {
	if isEmptyJSON(raw) {
		*dest = false
		return nil
	}
	var v bool
	if err := json.Unmarshal(raw, &v); err == nil {
		*dest = v
		return nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "true", "1", "yes":
			*dest = true
		default:
			*dest = false
		}
		return nil
	}
	var n float64
	if err := json.Unmarshal(raw, &n); err == nil {
		*dest = n != 0
		return nil
	}
	return fmt.Errorf("stream must be a boolean")
}
