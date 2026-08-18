package service

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/CoreUnit-NET/jimmy-gateway/lib/chatjimmy"
)

type chatCompletionRequest struct {
	chatjimmy.ChatRequest
	TopK        json.RawMessage `json:"top_k,omitempty"`
	TopKAlt     json.RawMessage `json:"topK,omitempty"`
	ChatOptions *chatOptions    `json:"chatOptions,omitempty"`
}

type chatOptions struct {
	SelectedModel string `json:"selectedModel"`
	SystemPrompt  string `json:"systemPrompt"`
	TopK          int    `json:"topK"`
}

type completionPayload struct {
	ID             string `json:"id"`
	Object         string `json:"object"`
	Created        int64  `json:"created"`
	Model          string `json:"model"`
	Choices        []any  `json:"choices"`
	Usage          usage  `json:"usage"`
	ChatJimmyStats any    `json:"chatjimmy_stats,omitempty"`
}

type usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type chatResult struct {
	completion completionPayload
	stream     bool
	tools      []chatjimmy.Tool
}

func (h *Handler) completeChat(r *http.Request) (*chatResult, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, badRequest("failed to read request body")
	}

	var req chatCompletionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, badRequest("Request body must be JSON")
	}

	if len(req.Messages) == 0 {
		return nil, badRequest("messages must be a non-empty array")
	}

	model := resolveModel(req)
	topK := resolveTopK(req)

	translated, err := chatjimmy.TranslateRequest(req.ChatRequest, chatjimmy.TranslateOptions{
		DefaultModel: model,
		TopK:         topK,
	})
	if err != nil {
		return nil, badRequest(err.Error())
	}

	if translated.Payload.ChatOptions.SystemPrompt == "" && req.ChatOptions != nil {
		translated.Payload.ChatOptions.SystemPrompt = strings.TrimSpace(req.ChatOptions.SystemPrompt)
	}

	if h.client == nil {
		return nil, upstreamError("upstream client is nil", "upstream_error")
	}

	raw, err := h.client.Chat(r.Context(), translated.Payload)
	if err != nil {
		return nil, fmtUpstreamError(err)
	}

	parsed := chatjimmy.ParseUpstream(raw)
	completion := buildCompletionResponse(
		translated.Model,
		parsed.Text,
		parsed.Usage,
		parsed.Stats,
		req.Tools,
	)

	return &chatResult{completion: completion, stream: req.Stream, tools: req.Tools}, nil
}

func resolveModel(req chatCompletionRequest) string {
	if model := strings.TrimSpace(req.Model); model != "" {
		return model
	}
	if req.ChatOptions != nil {
		if model := strings.TrimSpace(req.ChatOptions.SelectedModel); model != "" {
			return model
		}
	}
	return chatjimmy.DefaultModel
}

func resolveTopK(req chatCompletionRequest) int {
	for _, raw := range []json.RawMessage{req.TopK, req.TopKAlt} {
		if k := parseTopKRaw(raw); k > 0 {
			return k
		}
	}
	if req.ChatOptions != nil && req.ChatOptions.TopK > 0 {
		return req.ChatOptions.TopK
	}
	return chatjimmy.DefaultTopK
}

func parseTopKRaw(raw json.RawMessage) int {
	if len(raw) == 0 || string(raw) == "null" {
		return 0
	}
	var n int
	if err := json.Unmarshal(raw, &n); err == nil && n > 0 {
		return n
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if parsed, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && parsed > 0 {
			return parsed
		}
	}
	return 0
}

func buildCompletionResponse(model, text string, tokenUsage chatjimmy.Usage, stats map[string]any, tools []chatjimmy.Tool) completionPayload {
	built := chatjimmy.BuildCompletion(model, text, tokenUsage, chatjimmy.FilterTools(tools))

	choices := make([]any, len(built.Choices))
	for i, choice := range built.Choices {
		msg := map[string]any{"role": choice.Message.Role}
		if choice.Message.Content != nil {
			msg["content"] = *choice.Message.Content
		}
		if len(choice.Message.ToolCalls) > 0 {
			msg["tool_calls"] = choice.Message.ToolCalls
		}
		choices[i] = map[string]any{
			"index":         choice.Index,
			"message":       msg,
			"finish_reason": choice.FinishReason,
		}
	}

	var statsOut any
	if len(stats) > 0 {
		statsOut = stats
	}

	return completionPayload{
		ID:             built.ID,
		Object:         built.Object,
		Created:        built.Created,
		Model:          built.Model,
		Choices:        choices,
		Usage:          usage{PromptTokens: built.Usage.PromptTokens, CompletionTokens: built.Usage.CompletionTokens, TotalTokens: built.Usage.TotalTokens},
		ChatJimmyStats: statsOut,
	}
}

func encodeStream(completion completionPayload, tools []chatjimmy.Tool) []byte {
	built := chatjimmy.Completion{
		ID:      completion.ID,
		Object:  completion.Object,
		Created: completion.Created,
		Model:   completion.Model,
		Usage: chatjimmy.Usage{
			PromptTokens:     completion.Usage.PromptTokens,
			CompletionTokens: completion.Usage.CompletionTokens,
			TotalTokens:      completion.Usage.TotalTokens,
		},
	}

	for _, rawChoice := range completion.Choices {
		choiceMap, ok := rawChoice.(map[string]any)
		if !ok {
			continue
		}
		msgMap, ok := choiceMap["message"].(map[string]any)
		if !ok {
			continue
		}
		msg := chatjimmy.AssistantMessage{Role: "assistant"}
		if content, ok := msgMap["content"].(string); ok {
			msg.Content = &content
		}
		if rawCalls, ok := msgMap["tool_calls"].([]chatjimmy.ToolCall); ok {
			msg.ToolCalls = rawCalls
		} else if rawCalls, ok := msgMap["tool_calls"].([]any); ok {
			for _, item := range rawCalls {
				b, _ := json.Marshal(item)
				var tc chatjimmy.ToolCall
				if json.Unmarshal(b, &tc) == nil {
					msg.ToolCalls = append(msg.ToolCalls, tc)
				}
			}
		}
		finishReason, _ := choiceMap["finish_reason"].(string)
		index, _ := choiceMap["index"].(float64)
		built.Choices = append(built.Choices, chatjimmy.CompletionChoice{
			Index:        int(index),
			Message:      msg,
			FinishReason: finishReason,
		})
	}

	_ = tools
	return chatjimmy.EncodeSSEChunks(chatjimmy.BuildStreamChunks(built))
}
