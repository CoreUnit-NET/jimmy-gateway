package chatjimmy

import (
	"encoding/json"
	"fmt"
	"strings"
)

type AnthropicRequest struct {
	Model         string          `json:"model"`
	MaxTokens     int             `json:"max_tokens"`
	Messages      json.RawMessage `json:"messages"`
	System        json.RawMessage `json:"system,omitempty"`
	Temperature   *float64        `json:"temperature,omitempty"`
	TopP          *float64        `json:"top_p,omitempty"`
	TopK          *int            `json:"top_k,omitempty"`
	StopSequences []string        `json:"stop_sequences,omitempty"`
	Stream        bool            `json:"stream,omitempty"`
	ToolChoice    json.RawMessage `json:"tool_choice,omitempty"`
	Tools         json.RawMessage `json:"tools,omitempty"`
}

type AnthropicMessage struct {
	ID           string           `json:"id"`
	Type         string           `json:"type"`
	Role         string           `json:"role"`
	Model        string           `json:"model"`
	Content      []AnthropicBlock `json:"content"`
	StopReason   string           `json:"stop_reason"`
	StopSequence *string          `json:"stop_sequence"`
	Usage        AnthropicUsage   `json:"usage"`
}

type AnthropicBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
}

type AnthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type AnthropicErrorBody struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func NewAnthropicError(message, typ string) AnthropicErrorBody {
	body := AnthropicErrorBody{Type: "error"}
	if typ == "" {
		typ = "invalid_request_error"
	}
	body.Error.Type = typ
	body.Error.Message = message
	return body
}

func AnthropicToChatRequest(raw []byte) (ChatRequest, error) {
	var req AnthropicRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return ChatRequest{}, fmt.Errorf("Request body must be JSON")
	}

	var incoming []json.RawMessage
	if !isEmptyJSON(req.Messages) {
		if err := json.Unmarshal(req.Messages, &incoming); err != nil {
			return ChatRequest{}, fmt.Errorf("messages must be a non-empty array")
		}
	}
	if len(incoming) == 0 {
		return ChatRequest{}, fmt.Errorf("messages must be a non-empty array")
	}

	out := ChatRequest{
		Model:       strings.TrimSpace(req.Model),
		Stream:      req.Stream,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		TopK:        req.TopK,
		Tools:       anthropicTools(req.Tools),
		ToolChoice:  anthropicToolChoice(req.ToolChoice),
	}
	if req.MaxTokens >= 1 {
		v := req.MaxTokens
		out.MaxTokens = &v
	}
	if len(req.StopSequences) > 0 {
		stop, _ := json.Marshal(req.StopSequences)
		out.Stop = stop
	}

	if sys := anthropicSystemText(req.System); sys != "" {
		out.Messages = append(out.Messages, Message{
			Role:    "system",
			Content: mustJSON(sys),
		})
	}

	for _, rawMsg := range incoming {
		msgs, err := anthropicMessages(rawMsg)
		if err != nil {
			return ChatRequest{}, err
		}
		out.Messages = append(out.Messages, msgs...)
	}
	return out, nil
}

func CompletionToAnthropic(completion Completion) AnthropicMessage {
	id := completion.ID
	if strings.HasPrefix(id, "chatcmpl-") {
		id = "msg_" + strings.TrimPrefix(id, "chatcmpl-")
	}
	if id == "" {
		id = "msg_" + strings.TrimPrefix(NewCompletionID(), "chatcmpl-")
	}

	choice := CompletionChoice{}
	if len(completion.Choices) > 0 {
		choice = completion.Choices[0]
	}
	blocks := make([]AnthropicBlock, 0, 1+len(choice.Message.ToolCalls))
	if choice.Message.Content != nil && *choice.Message.Content != "" {
		blocks = append(blocks, AnthropicBlock{Type: "text", Text: *choice.Message.Content})
	}
	for _, tc := range choice.Message.ToolCalls {
		input := json.RawMessage(tc.Function.Arguments)
		if !json.Valid(input) {
			input = mustJSON(map[string]any{})
		}
		blocks = append(blocks, AnthropicBlock{
			Type:  "tool_use",
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: input,
		})
	}
	if len(blocks) == 0 {
		blocks = []AnthropicBlock{{Type: "text", Text: ""}}
	}

	stop := "end_turn"
	switch choice.FinishReason {
	case "tool_calls":
		stop = "tool_use"
	case "length":
		stop = "max_tokens"
	}

	return AnthropicMessage{
		ID:         id,
		Type:       "message",
		Role:       "assistant",
		Model:      completion.Model,
		Content:    blocks,
		StopReason: stop,
		Usage: AnthropicUsage{
			InputTokens:  completion.Usage.PromptTokens,
			OutputTokens: completion.Usage.CompletionTokens,
		},
	}
}

func EncodeAnthropicSSE(msg AnthropicMessage) []byte {
	start := msg
	start.Content = []AnthropicBlock{}
	start.StopReason = ""

	var out []byte
	out = appendAnthropicEvent(out, "message_start", map[string]any{
		"type":    "message_start",
		"message": start,
	})

	for i, block := range msg.Content {
		startBlock := block
		switch startBlock.Type {
		case "text":
			startBlock.Text = ""
		case "tool_use":
			startBlock.Input = json.RawMessage(`{}`)
		}
		out = appendAnthropicEvent(out, "content_block_start", map[string]any{
			"type":          "content_block_start",
			"index":         i,
			"content_block": startBlock,
		})
		if block.Type == "text" && block.Text != "" {
			out = appendAnthropicEvent(out, "content_block_delta", map[string]any{
				"type":  "content_block_delta",
				"index": i,
				"delta": map[string]any{"type": "text_delta", "text": block.Text},
			})
		}
		if block.Type == "tool_use" && len(block.Input) > 0 {
			out = appendAnthropicEvent(out, "content_block_delta", map[string]any{
				"type":  "content_block_delta",
				"index": i,
				"delta": map[string]any{"type": "input_json_delta", "partial_json": string(block.Input)},
			})
		}
		out = appendAnthropicEvent(out, "content_block_stop", map[string]any{
			"type":  "content_block_stop",
			"index": i,
		})
	}

	out = appendAnthropicEvent(out, "message_delta", map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": msg.StopReason, "stop_sequence": nil},
		"usage": map[string]any{"output_tokens": msg.Usage.OutputTokens},
	})
	out = appendAnthropicEvent(out, "message_stop", map[string]any{"type": "message_stop"})
	return out
}

func appendAnthropicEvent(out []byte, event string, payload any) []byte {
	b, _ := json.Marshal(payload)
	out = append(out, []byte("event: "+event+"\n")...)
	out = append(out, []byte("data: "+string(b)+"\n\n")...)
	return out
}

func anthropicSystemText(raw json.RawMessage) string {
	if isEmptyJSON(raw) {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return strings.TrimSpace(s)
	}
	var blocks []AnthropicBlock
	if json.Unmarshal(raw, &blocks) == nil {
		parts := make([]string, 0, len(blocks))
		for _, block := range blocks {
			if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
				parts = append(parts, block.Text)
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
	}
	return ""
}

func anthropicTools(raw json.RawMessage) []Tool {
	if isEmptyJSON(raw) {
		return nil
	}
	var tools []struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		InputSchema json.RawMessage `json:"input_schema"`
	}
	if json.Unmarshal(raw, &tools) != nil {
		return nil
	}
	out := make([]Tool, 0, len(tools))
	for _, tool := range tools {
		if strings.TrimSpace(tool.Name) == "" {
			continue
		}
		out = append(out, Tool{
			Type: "function",
			Function: ToolFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.InputSchema,
			},
		})
	}
	return out
}

func anthropicToolChoice(raw json.RawMessage) json.RawMessage {
	if isEmptyJSON(raw) {
		return nil
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		switch s {
		case "none":
			return json.RawMessage(`"none"`)
		case "any", "required":
			return json.RawMessage(`"required"`)
		default:
			return json.RawMessage(`"auto"`)
		}
	}
	var obj struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}
	if json.Unmarshal(raw, &obj) != nil {
		return nil
	}
	switch obj.Type {
	case "none":
		return json.RawMessage(`"none"`)
	case "any":
		return json.RawMessage(`"required"`)
	case "tool":
		b, _ := json.Marshal(map[string]any{
			"type":     "function",
			"function": map[string]string{"name": obj.Name},
		})
		return b
	default:
		return json.RawMessage(`"auto"`)
	}
}

func anthropicMessages(raw json.RawMessage) ([]Message, error) {
	var msg struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &msg); err != nil {
		return nil, fmt.Errorf("invalid message")
	}
	role := strings.TrimSpace(msg.Role)
	if role == "" {
		role = "user"
	}

	var asString string
	if json.Unmarshal(msg.Content, &asString) == nil {
		return []Message{{Role: role, Content: mustJSON(asString)}}, nil
	}

	var blocks []map[string]json.RawMessage
	if json.Unmarshal(msg.Content, &blocks) != nil {
		return []Message{{Role: role, Content: msg.Content}}, nil
	}

	var out []Message
	var parts []json.RawMessage
	flushParts := func() {
		if len(parts) == 0 {
			return
		}
		content, _ := json.Marshal(parts)
		out = append(out, Message{Role: role, Content: content})
		parts = nil
	}

	for _, block := range blocks {
		typ := rawString(block["type"])
		switch typ {
		case "tool_use":
			flushParts()
			name := rawString(block["name"])
			id := rawString(block["id"])
			args := block["input"]
			if isEmptyJSON(args) {
				args = json.RawMessage(`{}`)
			}
			out = append(out, Message{
				Role:    "assistant",
				Content: mustJSON(""),
				ToolCalls: []ToolCall{{
					ID:   id,
					Type: "function",
					Function: ToolCallFunction{
						Name:      name,
						Arguments: string(args),
					},
				}},
			})
		case "tool_result":
			flushParts()
			content := anthropicToolResultContent(block["content"])
			out = append(out, Message{
				Role:       "tool",
				Name:       rawString(block["name"]),
				ToolCallID: rawString(block["tool_use_id"]),
				Content:    mustJSON(content),
			})
		case "image":
			if part := anthropicImagePart(block["source"]); part != nil {
				parts = append(parts, part)
			}
		default:
			text := rawString(block["text"])
			if text != "" {
				part, _ := json.Marshal(map[string]string{"type": "text", "text": text})
				parts = append(parts, part)
			}
		}
	}
	flushParts()
	if len(out) == 0 {
		return []Message{{Role: role, Content: mustJSON("")}}, nil
	}
	return out, nil
}

func anthropicToolResultContent(raw json.RawMessage) string {
	if isEmptyJSON(raw) {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []AnthropicBlock
	if json.Unmarshal(raw, &blocks) == nil {
		parts := make([]string, 0, len(blocks))
		for _, block := range blocks {
			if block.Text != "" {
				parts = append(parts, block.Text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return strings.TrimSpace(string(raw))
}

func anthropicImagePart(source json.RawMessage) json.RawMessage {
	var src struct {
		Type      string `json:"type"`
		MediaType string `json:"media_type"`
		Data      string `json:"data"`
	}
	if json.Unmarshal(source, &src) != nil || strings.TrimSpace(src.Data) == "" {
		return nil
	}
	mime := strings.TrimSpace(src.MediaType)
	if mime == "" {
		mime = "image/png"
	}
	url := "data:" + mime + ";base64," + src.Data
	part, _ := json.Marshal(map[string]any{
		"type":      "image_url",
		"image_url": map[string]string{"url": url},
	})
	return part
}

func rawString(raw json.RawMessage) string {
	if isEmptyJSON(raw) {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return strings.Trim(string(raw), `"`)
}
