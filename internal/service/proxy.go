package service

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/CoreUnit-NET/jimmy-gateway/lib/chatjimmy"
)

const maxRequestBytes = 2 << 20

type chatResult struct {
	completion   chatjimmy.Completion
	stream       bool
	includeUsage bool
}

func readBody(r *http.Request) ([]byte, error) {
	if r == nil || r.Body == nil {
		return nil, badRequest("failed to read request body")
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBytes+1))
	if err != nil {
		return nil, badRequest("failed to read request body")
	}
	if len(body) > maxRequestBytes {
		return nil, &proxyError{
			message: "request body too large",
			typ:     "invalid_request_error",
			code:    "invalid_request",
			status:  http.StatusRequestEntityTooLarge,
		}
	}
	return body, nil
}

func (h *Handler) completeChat(r *http.Request) (*chatResult, error) {
	body, err := readBody(r)
	if err != nil {
		return nil, err
	}

	var req chatjimmy.ChatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, badRequest("Request body must be JSON")
	}

	completion, err := h.completeFromChatRequest(r, req, "openai")
	if err != nil {
		return nil, err
	}
	includeUsage := req.StreamOptions != nil && req.StreamOptions.IncludeUsage
	return &chatResult{completion: completion, stream: req.Stream, includeUsage: includeUsage}, nil
}

func (h *Handler) completeFromChatRequest(r *http.Request, req chatjimmy.ChatRequest, kind string) (chatjimmy.Completion, error) {
	if kind == "" {
		kind = "adapter"
	}
	start := time.Now()
	translated, err := chatjimmy.TranslateRequest(req, h.translateOptions())
	if err != nil {
		return chatjimmy.Completion{}, badRequest(err.Error())
	}

	raw, err := h.chatUpstream(r, translated.Payload)
	if err != nil {
		return chatjimmy.Completion{}, err
	}

	tools := translated.Tools
	if translated.SkipToolParse {
		tools = nil
	}
	parsed := chatjimmy.ParseUpstream(raw)
	completion := chatjimmy.BuildCompletion(
		translated.Model,
		parsed.Text,
		parsed.Usage,
		tools,
		parsed.Stats,
	)
	h.logChat(kind, translated.Model, start, translated.SystemTruncated, translated.ToolsCompacted, parsed.Usage)
	return completion, nil
}

func (h *Handler) handleNativeChat(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r)
	if err != nil {
		var pe *proxyError
		if errors.As(err, &pe) {
			h.writeOpenAIError(w, pe.message, pe.typ, pe.code, pe.status)
			return
		}
		h.writeOpenAIError(w, "failed to read request body", "invalid_request_error", "invalid_request", http.StatusBadRequest)
		return
	}

	var payload chatjimmy.UpstreamPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		h.writeOpenAIError(w, "Request body must be JSON", "invalid_request_error", "invalid_request", http.StatusBadRequest)
		return
	}
	if len(payload.Messages) == 0 {
		h.writeOpenAIError(w, "messages must be a non-empty array", "invalid_request_error", "invalid_request", http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(payload.ChatOptions.SelectedModel) == "" {
		payload.ChatOptions.SelectedModel = h.defaultModel()
	}
	if payload.ChatOptions.TopK <= 0 {
		payload.ChatOptions.TopK = chatjimmy.DefaultTopK
	}

	start := time.Now()
	raw, err := h.chatUpstream(r, payload)
	if err != nil {
		var pe *proxyError
		if errors.As(err, &pe) {
			h.writeOpenAIError(w, pe.message, pe.typ, pe.code, pe.status)
			return
		}
		h.writeOpenAIError(w, err.Error(), "api_error", "upstream_error", http.StatusBadGateway)
		return
	}
	h.logChat("native", payload.ChatOptions.SelectedModel, start, false, false, chatjimmy.Usage{})
	h.writeRaw(w, http.StatusOK, "text/plain; charset=utf-8", []byte(raw))
}

func (h *Handler) handleAnthropicMessages(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r)
	if err != nil {
		var pe *proxyError
		if errors.As(err, &pe) {
			h.writeAnthropicError(w, pe.message, pe.typ, pe.status)
			return
		}
		h.writeAnthropicError(w, "failed to read request body", "invalid_request_error", http.StatusBadRequest)
		return
	}

	req, err := chatjimmy.AnthropicToChatRequest(body)
	if err != nil {
		h.writeAnthropicError(w, err.Error(), "invalid_request_error", http.StatusBadRequest)
		return
	}

	completion, err := h.completeFromChatRequest(r, req, "adapter")
	if err != nil {
		var pe *proxyError
		if errors.As(err, &pe) {
			typ := "api_error"
			if pe.status < 500 {
				typ = "invalid_request_error"
			}
			h.writeAnthropicError(w, pe.message, typ, pe.status)
			return
		}
		h.writeAnthropicError(w, err.Error(), "api_error", http.StatusBadGateway)
		return
	}

	msg := chatjimmy.CompletionToAnthropic(completion)
	if req.Stream {
		h.writeSSE(w, chatjimmy.EncodeAnthropicSSE(msg))
		return
	}
	h.writeJSON(w, http.StatusOK, msg)
}

func (h *Handler) handleGeminiGenerate(w http.ResponseWriter, r *http.Request, model string, stream bool) {
	body, err := readBody(r)
	if err != nil {
		var pe *proxyError
		if errors.As(err, &pe) {
			h.writeGeminiError(w, pe.message, pe.status)
			return
		}
		h.writeGeminiError(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	req, err := chatjimmy.GeminiToChatRequest(model, body)
	if err != nil {
		h.writeGeminiError(w, err.Error(), http.StatusBadRequest)
		return
	}
	if stream {
		req.Stream = true
	}

	completion, err := h.completeFromChatRequest(r, req, "adapter")
	if err != nil {
		var pe *proxyError
		if errors.As(err, &pe) {
			h.writeGeminiError(w, pe.message, pe.status)
			return
		}
		h.writeGeminiError(w, err.Error(), http.StatusBadGateway)
		return
	}

	resp := chatjimmy.CompletionToGemini(completion)
	if stream {
		h.writeSSE(w, chatjimmy.EncodeGeminiSSE(resp))
		return
	}
	h.writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) chatUpstream(r *http.Request, payload chatjimmy.UpstreamPayload) (string, error) {
	if h.client == nil {
		return "", upstreamError("upstream client is nil", "upstream_error")
	}
	raw, err := h.client.Chat(r.Context(), payload)
	if err != nil {
		return "", fmtUpstreamError(err)
	}
	return raw, nil
}

func (h *Handler) translateOptions() chatjimmy.TranslateOptions {
	return chatjimmy.TranslateOptions{DefaultModel: h.defaultModel()}
}

func (h *Handler) defaultModel() string {
	if h.settings != nil {
		if model := strings.TrimSpace(h.settings.ChatJimmyModel); model != "" {
			return model
		}
	}
	return chatjimmy.DefaultModel
}

func (h *Handler) logChat(kind, model string, start time.Time, truncated, compacted bool, usage chatjimmy.Usage) {
	if h == nil || h.logger == nil {
		return
	}
	if kind == "" {
		kind = "unknown"
	}
	if model == "" {
		model = "-"
	}
	if truncated {
		h.logger.Printf("system prompt truncated to %d chars", chatjimmy.MaxSystemPrompt)
	}
	if compacted {
		h.logger.Printf("dropped tools JSON from oversized system prompt")
	}
	// upstream= is chat-path latency (translate+upstream+parse), not full HTTP
	// time — access logs own the end-to-end request duration.
	durationMS := time.Since(start).Milliseconds()
	if durationMS < 0 {
		durationMS = 0
	}
	if h.settings != nil && h.settings.Verbose {
		h.logger.Printf(
			"chat kind=%s model=%s upstream=%dms truncated=%t compacted=%t prompt_tokens=%d completion_tokens=%d",
			kind,
			model,
			durationMS,
			truncated,
			compacted,
			usage.PromptTokens,
			usage.CompletionTokens,
		)
		return
	}
	h.logger.Printf("chat kind=%s model=%s upstream=%dms", kind, model, durationMS)
}
