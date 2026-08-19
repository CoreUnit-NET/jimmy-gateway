package chatjimmy

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

var toolsJSONBlock = regexp.MustCompile(`(?s)\n?<tools>.*?</tools>`)

type TranslateOptions struct {
	DefaultModel string
}

type TranslateResult struct {
	Model           string
	Payload         UpstreamPayload
	Tools           []Tool
	SystemTruncated bool
	ToolsCompacted  bool
	SkipToolParse   bool
}

func TranslateRequest(req ChatRequest, opts TranslateOptions) (TranslateResult, error) {
	if req.N != nil && *req.N != 1 {
		return TranslateResult{}, fmt.Errorf("n must be 1")
	}
	if len(req.Messages) == 0 {
		return TranslateResult{}, fmt.Errorf("messages must be a non-empty array")
	}

	responseModel := strings.TrimSpace(req.Model)
	if responseModel == "" && req.ChatOptions != nil {
		responseModel = strings.TrimSpace(req.ChatOptions.SelectedModel)
	}
	if responseModel == "" {
		responseModel = strings.TrimSpace(opts.DefaultModel)
	}
	if responseModel == "" {
		responseModel = DefaultModel
	}

	tools := FilterTools(req.Tools)
	systemParts := make([]string, 0, 2)
	chatMessages := make([]UpstreamMessage, 0, len(req.Messages))
	var attachment *Attachment
	toolCallIDToName := map[string]string{}

	for _, msg := range req.Messages {
		role := normalizeRole(msg.Role)
		content, partAtt := ParseContent(msg.Content)
		if attachment == nil && partAtt != nil {
			attachment = partAtt
		}

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
					if tc.ID != "" && tc.Function.Name != "" {
						toolCallIDToName[tc.ID] = tc.Function.Name
					}
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
			if toolName == "" && msg.ToolCallID != "" {
				toolName = toolCallIDToName[msg.ToolCallID]
			}
			if toolName == "" {
				toolName = "unknown"
			}
			result := map[string]any{
				"name":         toolName,
				"tool_call_id": msg.ToolCallID,
				"content":      capToolResult(content),
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
	if systemPrompt == "" && req.ChatOptions != nil {
		systemPrompt = strings.TrimSpace(req.ChatOptions.SystemPrompt)
	}
	skipToolParse := parseToolChoice(req.ToolChoice) == "none"
	if len(tools) > 0 && !skipToolParse {
		systemPrompt += FormatToolsForPrompt(tools, req.ToolChoice)
	}

	truncated := false
	compacted := false
	if len(systemPrompt) > MaxSystemPrompt {
		stripped := stripToolsJSON(systemPrompt)
		if stripped != systemPrompt {
			systemPrompt = stripped
			compacted = true
		}
	}
	if len(systemPrompt) > MaxSystemPrompt {
		systemPrompt = limitBytes(systemPrompt, MaxSystemPrompt)
		truncated = true
	}

	options := ChatOptions{
		SelectedModel: MapModel(responseModel),
		SystemPrompt:  systemPrompt,
		TopK:          resolveTopK(req),
		Temperature:   resolveTemperature(req),
		TopP:          resolveTopP(req),
		MaxTokens:     resolveMaxTokens(req),
		StopSequences: resolveStop(req),
		Stream:        req.Stream,
	}
	if !options.Stream && req.ChatOptions != nil {
		options.Stream = req.ChatOptions.Stream
	}

	return TranslateResult{
		Model: responseModel,
		Payload: UpstreamPayload{
			Messages:    chatMessages,
			ChatOptions: options,
			Attachment:  attachment,
		},
		Tools:           tools,
		SystemTruncated: truncated,
		ToolsCompacted:  compacted,
		SkipToolParse:   skipToolParse,
	}, nil
}

func stripToolsJSON(prompt string) string {
	return strings.TrimSpace(toolsJSONBlock.ReplaceAllString(prompt, ""))
}

func capToolResult(content string) string {
	if len(content) <= MaxToolResultChars {
		return content
	}
	return limitBytes(content, MaxToolResultChars) + "\n...[truncated]"
}

func limitBytes(s string, max int) string {
	if max < 0 || len(s) <= max {
		return s
	}
	s = s[:max]
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}

func resolveTopK(req ChatRequest) int {
	for _, v := range []*int{req.TopK, req.TopKCamel} {
		if v != nil && *v > 0 {
			return *v
		}
	}
	if req.ChatOptions != nil && req.ChatOptions.TopK > 0 {
		return req.ChatOptions.TopK
	}
	return DefaultTopK
}

func resolveTemperature(req ChatRequest) *float64 {
	if req.Temperature != nil {
		return normalizeOpenAITemperature(*req.Temperature)
	}
	if req.ChatOptions != nil && req.ChatOptions.Temperature != nil {
		return clampUnit(*req.ChatOptions.Temperature)
	}
	return nil
}

func resolveTopP(req ChatRequest) *float64 {
	if req.TopP != nil {
		return clampUnit(*req.TopP)
	}
	if req.ChatOptions != nil && req.ChatOptions.TopP != nil {
		return clampUnit(*req.ChatOptions.TopP)
	}
	return nil
}

func resolveMaxTokens(req ChatRequest) *int {
	for _, v := range []*int{req.MaxTokens, req.MaxCompletionTokens} {
		if v != nil && *v >= 1 {
			n := *v
			return &n
		}
	}
	if req.ChatOptions != nil && req.ChatOptions.MaxTokens != nil && *req.ChatOptions.MaxTokens >= 1 {
		v := *req.ChatOptions.MaxTokens
		return &v
	}
	return nil
}

func normalizeRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "system", "developer":
		return "system"
	case "assistant":
		return "assistant"
	case "tool", "function":
		return "tool"
	default:
		return "user"
	}
}

func resolveStop(req ChatRequest) []string {
	if seq := parseStop(req.Stop); len(seq) > 0 {
		return seq
	}
	if req.ChatOptions != nil && len(req.ChatOptions.StopSequences) > 0 {
		return req.ChatOptions.StopSequences
	}
	return nil
}

func parseStop(raw json.RawMessage) []string {
	if isEmptyJSON(raw) {
		return nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		s = strings.TrimSpace(s)
		if s == "" {
			return nil
		}
		return []string{s}
	}
	var arr []string
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeOpenAITemperature(v float64) *float64 {
	if v < 0 {
		v = 0
	}
	if v > 2 {
		v = 2
	}
	if v > 1 {
		v = v / 2
	}
	return &v
}

func clampUnit(v float64) *float64 {
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	return &v
}
