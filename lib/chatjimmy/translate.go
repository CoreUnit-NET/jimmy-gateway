package chatjimmy

import (
	"encoding/json"
	"fmt"
	"strings"
)

type TranslateOptions struct {
	DefaultModel string
	TopK         int
}

type TranslateResult struct {
	Model           string
	Payload         UpstreamPayload
	SystemTruncated bool
}

func TranslateRequest(req ChatRequest, opts TranslateOptions) (TranslateResult, error) {
	if len(req.Messages) == 0 {
		return TranslateResult{}, fmt.Errorf("messages must be a non-empty array")
	}

	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = strings.TrimSpace(opts.DefaultModel)
	}
	if model == "" {
		model = DefaultModel
	}

	topK := opts.TopK
	if topK <= 0 {
		topK = DefaultTopK
	}

	tools := FilterTools(req.Tools)
	systemParts := make([]string, 0, 2)
	chatMessages := make([]UpstreamMessage, 0, len(req.Messages))

	for _, msg := range req.Messages {
		role := strings.TrimSpace(msg.Role)
		if role == "" {
			role = "user"
		}
		content := ContentToText(msg.Content)

		switch role {
		case "system":
			if content != "" {
				systemParts = append(systemParts, content)
			}
		case "assistant":
			if len(msg.ToolCalls) > 0 {
				parts := make([]string, 0, len(msg.ToolCalls)+1)
				if content != "" {
					parts = append(parts, content)
				}
				for _, tc := range msg.ToolCalls {
					var args any
					if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
						args = tc.Function.Arguments
					}
					block, _ := json.Marshal(map[string]any{
						"name":      tc.Function.Name,
						"arguments": args,
					})
					parts = append(parts, "<tool_call>\n"+string(block)+"\n</tool_call>")
				}
				chatMessages = append(chatMessages, UpstreamMessage{
					Role:    "assistant",
					Content: strings.Join(parts, "\n"),
				})
			} else {
				chatMessages = append(chatMessages, UpstreamMessage{Role: "assistant", Content: content})
			}
		case "tool":
			toolName := msg.Name
			if toolName == "" {
				toolName = "unknown"
			}
			result := map[string]any{
				"name":         toolName,
				"tool_call_id": msg.ToolCallID,
				"content":      content,
			}
			block, _ := json.MarshalIndent(result, "", "  ")
			chatMessages = append(chatMessages, UpstreamMessage{
				Role:    "user",
				Content: "<tool_result>\n" + string(block) + "\n</tool_result>",
			})
		default:
			chatMessages = append(chatMessages, UpstreamMessage{Role: role, Content: content})
		}
	}

	if len(chatMessages) == 0 {
		return TranslateResult{}, fmt.Errorf("no valid non-system messages found")
	}

	systemPrompt := strings.TrimSpace(strings.Join(systemParts, "\n"))
	if len(tools) > 0 {
		systemPrompt += FormatToolsForPrompt(tools, req.ToolChoice)
	}

	truncated := false
	if len(systemPrompt) > MaxSystemPrompt {
		systemPrompt = systemPrompt[:MaxSystemPrompt]
		truncated = true
	}

	return TranslateResult{
		Model: model,
		Payload: UpstreamPayload{
			Messages: chatMessages,
			ChatOptions: UpstreamOptions{
				SelectedModel: model,
				SystemPrompt:  systemPrompt,
				TopK:          topK,
			},
			Attachment: nil,
		},
		SystemTruncated: truncated,
	}, nil
}
