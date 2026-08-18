package service

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/CoreUnit-NET/jimmy-gateway/lib/chatjimmy"
)

const maxRequestBytes = 2 << 20

type chatResult struct {
	completion chatjimmy.Completion
	stream     bool
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

	start := time.Now()
	translated, err := chatjimmy.TranslateRequest(req, h.translateOptions())
	if err != nil {
		return nil, badRequest(err.Error())
	}

	raw, err := h.chatUpstream(r, translated.Payload)
	if err != nil {
		return nil, err
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
	h.logChat("openai", translated.Model, start, translated.SystemTruncated, translated.ToolsCompacted, parsed.Usage)

	return &chatResult{completion: completion, stream: req.Stream}, nil
}

func (h *Handler) completeFromChatRequest(r *http.Request, req chatjimmy.ChatRequest) (chatjimmy.Completion, error) {
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
	h.logChat("adapter", translated.Model, start, translated.SystemTruncated, translated.ToolsCompacted, parsed.Usage)
	return completion, nil
}

func (h *Handler) handleNativeChat(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r)
	if err != nil {
		var pe *proxyError
		if asProxyError(err, &pe) {
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
		if ok := asProxyError(err, &pe); ok {
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
		if asProxyError(err, &pe) {
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

	completion, err := h.completeFromChatRequest(r, req)
	if err != nil {
		var pe *proxyError
		if ok := asProxyError(err, &pe); ok {
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
		if asProxyError(err, &pe) {
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

	completion, err := h.completeFromChatRequest(r, req)
	if err != nil {
		var pe *proxyError
		if ok := asProxyError(err, &pe); ok {
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
	if h.cfg != nil {
		if model := strings.TrimSpace(h.cfg.ChatJimmyModel); model != "" {
			return model
		}
	}
	return chatjimmy.DefaultModel
}

func (h *Handler) logChat(kind, model string, start time.Time, truncated, compacted bool, usage chatjimmy.Usage) {
	if h == nil || h.logger == nil {
		return
	}
	if truncated {
		h.logger.Printf("system prompt truncated to %d chars", chatjimmy.MaxSystemPrompt)
	}
	if compacted {
		h.logger.Printf("dropped tools JSON from oversized system prompt")
	}
	if h.cfg == nil || !h.cfg.Verbose {
		return
	}
	h.logger.Printf(
		"chat kind=%s model=%s duration=%s truncated=%t compacted=%t prompt_tokens=%d completion_tokens=%d",
		kind,
		model,
		time.Since(start).Round(time.Millisecond),
		truncated,
		compacted,
		usage.PromptTokens,
		usage.CompletionTokens,
	)
}
