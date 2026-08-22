package chatjimmy

import (
	"encoding/json"
	"fmt"
	"strings"
)

type GeminiRequest struct {
	Contents          []GeminiContent   `json:"contents"`
	GenerationConfig  *GeminiGenConfig  `json:"generationConfig,omitempty"`
	Tools             json.RawMessage   `json:"tools,omitempty"`
	SystemInstruction *GeminiContent    `json:"systemInstruction,omitempty"`
	ToolConfig        *GeminiToolConfig `json:"toolConfig,omitempty"`
}

type GeminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []GeminiPart `json:"parts"`
}

type GeminiPart struct {
	Text             string            `json:"text,omitempty"`
	InlineData       *GeminiInlineData `json:"inlineData,omitempty"`
	FunctionCall     *GeminiFnCall     `json:"functionCall,omitempty"`
	FunctionResponse *GeminiFnResponse `json:"functionResponse,omitempty"`
}

type GeminiInlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type GeminiFnCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

type GeminiFnResponse struct {
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

type GeminiGenConfig struct {
	StopSequences   []string `json:"stopSequences,omitempty"`
	Temperature     *float64 `json:"temperature,omitempty"`
	MaxOutputTokens *int     `json:"maxOutputTokens,omitempty"`
	TopP            *float64 `json:"topP,omitempty"`
	TopK            *int     `json:"topK,omitempty"`
}

type GeminiToolConfig struct {
	FunctionCallingConfig *GeminiFunctionCallingConfig `json:"functionCallingConfig,omitempty"`
}

type GeminiFunctionCallingConfig struct {
	Mode                 string   `json:"mode,omitempty"`
	AllowedFunctionNames []string `json:"allowedFunctionNames,omitempty"`
}

type GeminiResponse struct {
	Candidates    []GeminiCandidate `json:"candidates"`
	UsageMetadata GeminiUsage       `json:"usageMetadata"`
}

type GeminiCandidate struct {
	Content       GeminiContent `json:"content"`
	FinishReason  string        `json:"finishReason"`
	SafetyRatings []any         `json:"safetyRatings"`
	TokenCount    int           `json:"tokenCount,omitempty"`
}

type GeminiUsage struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

type GeminiErrorBody struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

func NewGeminiError(message string, status int) GeminiErrorBody {
	body := GeminiErrorBody{}
	body.Error.Code = status
	body.Error.Message = message
	body.Error.Status = geminiStatusName(status)
	return body
}

func GeminiToChatRequest(model string, raw []byte) (ChatRequest, error) {
	var req GeminiRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return ChatRequest{}, fmt.Errorf("Request body must be JSON")
	}
	if len(req.Contents) == 0 {
		return ChatRequest{}, fmt.Errorf("contents must be a non-empty array")
	}

	out := ChatRequest{
		Model: strings.TrimSpace(model),
		Tools: geminiTools(req.Tools),
	}
	if req.GenerationConfig != nil {
		out.Temperature = req.GenerationConfig.Temperature
		out.TopP = req.GenerationConfig.TopP
		out.TopK = req.GenerationConfig.TopK
		out.MaxTokens = req.GenerationConfig.MaxOutputTokens
		if len(req.GenerationConfig.StopSequences) > 0 {
			stop, _ := json.Marshal(req.GenerationConfig.StopSequences)
			out.Stop = stop
		}
	}
	out.ToolChoice = geminiToolChoice(req.ToolConfig)

	if req.SystemInstruction != nil {
		if text := geminiPartsText(req.SystemInstruction.Parts); text != "" {
			out.Messages = append(out.Messages, Message{
				Role:    "system",
				Content: mustJSON(text),
			})
		}
	}

	for _, content := range req.Contents {
		out.Messages = append(out.Messages, geminiContentMessages(content)...)
	}
	return out, nil
}

func CompletionToGemini(completion Completion) GeminiResponse {
	choice := CompletionChoice{}
	if len(completion.Choices) > 0 {
		choice = completion.Choices[0]
	}

	parts := make([]GeminiPart, 0, 1+len(choice.Message.ToolCalls))
	if choice.Message.Content != nil && *choice.Message.Content != "" {
		parts = append(parts, GeminiPart{Text: *choice.Message.Content})
	}
	for _, tc := range choice.Message.ToolCalls {
		args := map[string]any{}
		_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
		parts = append(parts, GeminiPart{
			FunctionCall: &GeminiFnCall{Name: tc.Function.Name, Args: args},
		})
	}
	if len(parts) == 0 {
		parts = []GeminiPart{{Text: ""}}
	}

	finish := "STOP"
	switch choice.FinishReason {
	case "length":
		finish = "MAX_TOKENS"
	case "tool_calls":
		finish = "STOP"
	}

	return GeminiResponse{
		Candidates: []GeminiCandidate{{
			Content:       GeminiContent{Role: "model", Parts: parts},
			FinishReason:  finish,
			SafetyRatings: []any{},
			TokenCount:    completion.Usage.CompletionTokens,
		}},
		UsageMetadata: GeminiUsage{
			PromptTokenCount:     completion.Usage.PromptTokens,
			CandidatesTokenCount: completion.Usage.CompletionTokens,
			TotalTokenCount:      completion.Usage.TotalTokens,
		},
	}
}

func EncodeGeminiSSE(resp GeminiResponse) []byte {
	b, _ := MarshalJSON(resp)
	return []byte("data: " + string(b) + "\n\n")
}

func geminiTools(raw json.RawMessage) []Tool {
	if isEmptyJSON(raw) {
		return nil
	}
	var groups []struct {
		FunctionDeclarations []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			Parameters  json.RawMessage `json:"parameters"`
		} `json:"functionDeclarations"`
	}
	if json.Unmarshal(raw, &groups) != nil {
		return nil
	}
	var out []Tool
	for _, group := range groups {
		for _, fn := range group.FunctionDeclarations {
			if strings.TrimSpace(fn.Name) == "" {
				continue
			}
			out = append(out, Tool{
				Type: "function",
				Function: ToolFunction{
					Name:        fn.Name,
					Description: fn.Description,
					Parameters:  fn.Parameters,
				},
			})
		}
	}
	return out
}

func geminiToolChoice(cfg *GeminiToolConfig) json.RawMessage {
	if cfg == nil || cfg.FunctionCallingConfig == nil {
		return nil
	}
	mode := strings.ToUpper(strings.TrimSpace(cfg.FunctionCallingConfig.Mode))
	switch mode {
	case "NONE":
		return json.RawMessage(`"none"`)
	case "ANY":
		if names := cfg.FunctionCallingConfig.AllowedFunctionNames; len(names) == 1 {
			b, _ := json.Marshal(map[string]any{
				"type":     "function",
				"function": map[string]string{"name": names[0]},
			})
			return b
		}
		return json.RawMessage(`"required"`)
	default:
		return json.RawMessage(`"auto"`)
	}
}

func geminiContentMessages(content GeminiContent) []Message {
	role := strings.TrimSpace(content.Role)
	switch role {
	case "model":
		role = "assistant"
	case "":
		role = "user"
	}

	var out []Message
	var parts []json.RawMessage
	flushParts := func() {
		if len(parts) == 0 {
			return
		}
		raw, _ := json.Marshal(parts)
		out = append(out, Message{Role: role, Content: raw})
		parts = nil
	}

	for _, part := range content.Parts {
		switch {
		case part.FunctionCall != nil:
			flushParts()
			args, _ := json.Marshal(part.FunctionCall.Args)
			out = append(out, Message{
				Role:    "assistant",
				Content: mustJSON(""),
				ToolCalls: []ToolCall{{
					Type: "function",
					Function: ToolCallFunction{
						Name:      part.FunctionCall.Name,
						Arguments: string(args),
					},
				}},
			})
		case part.FunctionResponse != nil:
			flushParts()
			body, _ := json.Marshal(part.FunctionResponse.Response)
			out = append(out, Message{
				Role:    "tool",
				Name:    part.FunctionResponse.Name,
				Content: mustJSON(string(body)),
			})
		case part.InlineData != nil && strings.TrimSpace(part.InlineData.Data) != "":
			mime := strings.TrimSpace(part.InlineData.MimeType)
			if mime == "" {
				mime = "image/png"
			}
			url := "data:" + mime + ";base64," + part.InlineData.Data
			raw, _ := json.Marshal(map[string]any{
				"type":      "image_url",
				"image_url": map[string]string{"url": url},
			})
			parts = append(parts, raw)
		default:
			if part.Text != "" {
				raw, _ := json.Marshal(map[string]string{"type": "text", "text": part.Text})
				parts = append(parts, raw)
			}
		}
	}
	flushParts()
	if len(out) == 0 {
		return []Message{{Role: role, Content: mustJSON("")}}
	}
	return out
}

func geminiPartsText(parts []GeminiPart) string {
	texts := make([]string, 0, len(parts))
	for _, part := range parts {
		if part.Text != "" {
			texts = append(texts, part.Text)
		}
	}
	return strings.TrimSpace(strings.Join(texts, "\n"))
}

func geminiStatusName(status int) string {
	switch status {
	case 400:
		return "INVALID_ARGUMENT"
	case 401:
		return "UNAUTHENTICATED"
	case 403:
		return "PERMISSION_DENIED"
	case 404:
		return "NOT_FOUND"
	case 429:
		return "RESOURCE_EXHAUSTED"
	default:
		if status >= 500 {
			return "INTERNAL"
		}
		return "UNKNOWN"
	}
}
