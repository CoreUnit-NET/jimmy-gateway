package chatjimmy

import "encoding/json"

const (
	DefaultUpstreamURL = "https://chatjimmy.ai/api/chat"
	DefaultModel       = "llama3.1-8B"
	DefaultTopK        = 8
	MaxSystemPrompt    = 28000
)

var FilteredTools = map[string]struct{}{
	"webfetch":  {},
	"todowrite": {},
	"skill":     {},
	"question":  {},
	"task":      {},
}

type ChatRequest struct {
	Model       string          `json:"model"`
	Messages    []Message       `json:"messages"`
	Stream      bool            `json:"stream"`
	Tools       []Tool          `json:"tools,omitempty"`
	ToolChoice  json.RawMessage `json:"tool_choice,omitempty"`
	Temperature *float64        `json:"temperature,omitempty"`
	TopP        *float64        `json:"top_p,omitempty"`
	TopK        *int            `json:"top_k,omitempty"`
	TopKCamel   *int            `json:"topK,omitempty"`
	MaxTokens   *int            `json:"max_tokens,omitempty"`
	Stop        json.RawMessage `json:"stop,omitempty"`
	ChatOptions *NativeOptions  `json:"chatOptions,omitempty"`
}

type NativeOptions struct {
	SelectedModel string   `json:"selectedModel"`
	SystemPrompt  string   `json:"systemPrompt"`
	TopK          int      `json:"topK"`
	Temperature   *float64 `json:"temperature,omitempty"`
	TopP          *float64 `json:"topP,omitempty"`
	MaxTokens     *int     `json:"maxTokens,omitempty"`
	StopSequences []string `json:"stopSequences,omitempty"`
	Stream        bool     `json:"stream,omitempty"`
}

type Message struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	ToolCalls  []ToolCall      `json:"tool_calls,omitempty"`
	Name       string          `json:"name,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type ToolCall struct {
	Index    *int             `json:"index,omitempty"`
	ID       string           `json:"id,omitempty"`
	Type     string           `json:"type,omitempty"`
	Function ToolCallFunction `json:"function,omitempty"`
}

type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type UpstreamPayload struct {
	Messages    []UpstreamMessage `json:"messages"`
	ChatOptions UpstreamOptions   `json:"chatOptions"`
	Attachment  *Attachment       `json:"attachment"`
}

type UpstreamMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type UpstreamOptions struct {
	SelectedModel string   `json:"selectedModel"`
	SystemPrompt  string   `json:"systemPrompt"`
	TopK          int      `json:"topK"`
	Temperature   *float64 `json:"temperature,omitempty"`
	TopP          *float64 `json:"topP,omitempty"`
	MaxTokens     *int     `json:"maxTokens,omitempty"`
	StopSequences []string `json:"stopSequences,omitempty"`
	Stream        bool     `json:"stream,omitempty"`
}

type Attachment struct {
	Type     string `json:"type"`
	Data     string `json:"data"`
	MimeType string `json:"mimeType"`
	Filename string `json:"filename,omitempty"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type Completion struct {
	ID             string             `json:"id"`
	Object         string             `json:"object"`
	Created        int64              `json:"created"`
	Model          string             `json:"model"`
	Choices        []CompletionChoice `json:"choices"`
	Usage          Usage              `json:"usage"`
	ChatJimmyStats any                `json:"chatjimmy_stats,omitempty"`
}

type CompletionChoice struct {
	Index        int              `json:"index"`
	Message      AssistantMessage `json:"message"`
	FinishReason string           `json:"finish_reason"`
}

type AssistantMessage struct {
	Role      string     `json:"role"`
	Content   *string    `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

type StreamChunk struct {
	ID      string              `json:"id"`
	Object  string              `json:"object"`
	Created int64               `json:"created"`
	Model   string              `json:"model"`
	Choices []StreamChunkChoice `json:"choices"`
}

type StreamChunkChoice struct {
	Index        int              `json:"index"`
	Delta        StreamChunkDelta `json:"delta"`
	FinishReason *string          `json:"finish_reason"`
}

type StreamChunkDelta struct {
	Role      string     `json:"role,omitempty"`
	Content   string     `json:"content,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

type OpenAIError struct {
	Error struct {
		Message string  `json:"message"`
		Type    string  `json:"type"`
		Param   *string `json:"param"`
		Code    string  `json:"code,omitempty"`
	} `json:"error"`
}
