package service

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/CoreUnit-NET/jimmy-gateway/internal/settings"
	"github.com/CoreUnit-NET/jimmy-gateway/lib/chatjimmy"
)

// Handler implements the OpenAI-compatible proxy routes from cj2api proxy.js.
type Handler struct {
	logger   *log.Logger
	settings *settings.Settings
	client   *chatjimmy.Client
}

// NewHandler returns an HTTP handler for the ChatJimmy proxy.
func NewHandler(logger *log.Logger, s *settings.Settings, client *chatjimmy.Client) *Handler {
	if logger == nil {
		logger = log.New(os.Stdout, "", log.LstdFlags)
	}
	return &Handler{logger: logger, settings: s, client: client}
}

// ServeHTTP routes requests through path normalization, aliases, and handlers.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
	// Exactly one access log per request, even when handlers return early.
	defer func() { h.logRequest(r, sw.status, start) }()

	if r == nil || r.URL == nil {
		h.writeOpenAIError(sw, "Invalid request", "invalid_request_error", "invalid_request", http.StatusBadRequest)
		return
	}

	path := resolvePath(r.URL.Path)

	if r.Method == http.MethodOptions {
		h.writeCORS(sw)
		sw.WriteHeader(http.StatusNoContent)
		return
	}

	switch {
	case path == "/" || path == "/health":
		if r.Method != http.MethodGet {
			h.writeOpenAIError(sw, "Method not allowed", "invalid_request_error", "method_not_allowed", http.StatusMethodNotAllowed)
			return
		}
		h.writeJSON(sw, http.StatusOK, map[string]string{"status": "ok"})
	case path == "/v1/models":
		if r.Method != http.MethodGet {
			h.writeOpenAIError(sw, "Method not allowed", "invalid_request_error", "method_not_allowed", http.StatusMethodNotAllowed)
			return
		}
		h.handleModels(sw)
	case path == "/v1/chat/completions":
		if r.Method != http.MethodPost {
			h.writeOpenAIError(sw, "Method not allowed", "invalid_request_error", "method_not_allowed", http.StatusMethodNotAllowed)
			return
		}
		if !h.authorize(r) {
			h.writeOpenAIError(sw, "Invalid API key", "invalid_api_key", "invalid_api_key", http.StatusUnauthorized)
			return
		}
		h.handleChatCompletions(sw, r)
	case path == "/api/chat":
		if r.Method != http.MethodPost {
			h.writeOpenAIError(sw, "Method not allowed", "invalid_request_error", "method_not_allowed", http.StatusMethodNotAllowed)
			return
		}
		if !h.authorize(r) {
			h.writeOpenAIError(sw, "Invalid API key", "invalid_api_key", "invalid_api_key", http.StatusUnauthorized)
			return
		}
		h.handleNativeChat(sw, r)
	case path == "/v1/messages":
		if r.Method != http.MethodPost {
			h.writeAnthropicError(sw, "Method not allowed", "invalid_request_error", http.StatusMethodNotAllowed)
			return
		}
		if !h.authorize(r) {
			h.writeAnthropicError(sw, "Invalid API key", "authentication_error", http.StatusUnauthorized)
			return
		}
		h.handleAnthropicMessages(sw, r)
	default:
		if model, stream, ok := parseGeminiPath(path); ok {
			if r.Method != http.MethodPost {
				h.writeGeminiError(sw, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			if !h.authorize(r) {
				h.writeGeminiError(sw, "Invalid API key", http.StatusUnauthorized)
				return
			}
			h.handleGeminiGenerate(sw, r, model, stream)
			return
		}
		h.writeOpenAIError(sw, "Not found", "invalid_request_error", "not_found", http.StatusNotFound)
	}
}

func (h *Handler) handleModels(w http.ResponseWriter) {
	now := time.Now().Unix()
	defaultModel := h.defaultModel()
	extras := []string(nil)
	if h.settings != nil {
		extras = h.settings.ChatJimmyModels
	}
	models := chatjimmy.ListModels(defaultModel, extras)
	data := make([]map[string]any, 0, len(models))
	for _, id := range models {
		data = append(data, map[string]any{
			"id":       id,
			"object":   "model",
			"created":  now,
			"owned_by": "chatjimmy",
		})
	}
	h.writeJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data":   data,
	})
}

func (h *Handler) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	result, err := h.completeChat(r)
	if err != nil {
		var pe *proxyError
		if errors.As(err, &pe) {
			h.writeOpenAIError(w, pe.message, pe.typ, pe.code, pe.status)
			return
		}
		h.writeOpenAIError(w, err.Error(), "api_error", "upstream_error", http.StatusBadGateway)
		return
	}

	if result.stream {
		h.writeSSE(w, chatjimmy.EncodeSSEChunks(chatjimmy.BuildStreamChunks(result.completion)))
		return
	}
	h.writeJSON(w, http.StatusOK, result.completion)
}

func (h *Handler) authorize(r *http.Request) bool {
	if h.settings == nil {
		return true
	}
	expected := strings.TrimSpace(h.settings.APIKey)
	if expected == "" {
		return true
	}
	candidates := []string{
		bearerToken(r.Header.Get("Authorization")),
		strings.TrimSpace(r.Header.Get("x-api-key")),
		strings.TrimSpace(r.Header.Get("X-Api-Key")),
		strings.TrimSpace(r.Header.Get("x-goog-api-key")),
		strings.TrimSpace(r.Header.Get("X-Goog-Api-Key")),
	}
	for _, actual := range candidates {
		if actual != "" && actual == expected {
			return true
		}
	}
	return false
}

func bearerToken(auth string) string {
	const prefix = "Bearer "
	auth = strings.TrimSpace(auth)
	if len(auth) < len(prefix) || !strings.EqualFold(auth[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(auth[len(prefix):])
}

func (h *Handler) allowedOrigin() string {
	if h.settings == nil {
		return "*"
	}
	origin := strings.TrimSpace(h.settings.AllowedOrigin)
	if origin == "" {
		return "*"
	}
	return origin
}

func (h *Handler) writeCORS(w http.ResponseWriter) {
	origin := h.allowedOrigin()
	w.Header().Set("Access-Control-Allow-Origin", origin)
	if origin != "*" {
		w.Header().Set("Vary", "Origin")
	}
	w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization,Content-Type,x-api-key,x-goog-api-key,anthropic-version")
	w.Header().Set("Access-Control-Max-Age", "86400")
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, data any) {
	h.writeCORS(w)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func (h *Handler) writeRaw(w http.ResponseWriter, status int, contentType string, body []byte) {
	h.writeCORS(w)
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func (h *Handler) writeOpenAIError(w http.ResponseWriter, message, typ, code string, status int) {
	h.writeJSON(w, status, chatjimmy.NewOpenAIError(message, typ, code))
}

func (h *Handler) writeAnthropicError(w http.ResponseWriter, message, typ string, status int) {
	h.writeJSON(w, status, chatjimmy.NewAnthropicError(message, typ))
}

func (h *Handler) writeGeminiError(w http.ResponseWriter, message string, status int) {
	h.writeJSON(w, status, chatjimmy.NewGeminiError(message, status))
}

func (h *Handler) writeSSE(w http.ResponseWriter, payload []byte) {
	h.writeCORS(w)
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (h *Handler) logRequest(r *http.Request, status int, start time.Time) {
	if h == nil || h.logger == nil {
		return
	}
	// Client-visible path only (aliases preserved; query string omitted to avoid
	// leaking api keys). Millisecond duration so sub-second checks are not "0s".
	method := "UNKNOWN"
	path := "/"
	remote := "-"
	if r != nil {
		if r.Method != "" {
			method = r.Method
		}
		path = clientRequestPath(r)
		if r.RemoteAddr != "" {
			remote = r.RemoteAddr
		}
	}
	durationMS := time.Since(start).Milliseconds()
	if durationMS < 0 {
		durationMS = 0
	}
	h.logger.Printf("%s %s status=%d duration=%dms remote=%s", method, path, status, durationMS, remote)
}

// clientRequestPath returns the path the client sent (no query), defaulting to "/".
func clientRequestPath(r *http.Request) string {
	if r == nil || r.URL == nil || r.URL.Path == "" {
		return "/"
	}
	return r.URL.Path
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// Flush preserves streaming support when the underlying ResponseWriter is a Flusher.
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

type proxyError struct {
	message string
	typ     string
	code    string
	status  int
}

func (e *proxyError) Error() string {
	return e.message
}

func badRequest(message string) error {
	return &proxyError{message: message, typ: "invalid_request_error", code: "invalid_request", status: http.StatusBadRequest}
}

func upstreamError(message, code string) error {
	return &proxyError{message: message, typ: "api_error", code: code, status: http.StatusBadGateway}
}

func fmtUpstreamError(err error) error {
	msg := err.Error()
	code := "upstream_error"
	if strings.HasPrefix(msg, "upstream returned") {
		code = "upstream_status_error"
	}
	return upstreamError(msg, code)
}
