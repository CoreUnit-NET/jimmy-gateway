package service

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/CoreUnit-NET/jimmy-gateway/internal/config"
	"github.com/CoreUnit-NET/jimmy-gateway/lib/chatjimmy"
)

// Handler implements the OpenAI-compatible proxy routes from cj2api proxy.js.
type Handler struct {
	logger *log.Logger
	cfg    *config.AppConfig
	client *chatjimmy.Client
}

// NewHandler returns an HTTP handler for the ChatJimmy proxy.
func NewHandler(logger *log.Logger, cfg *config.AppConfig, client *chatjimmy.Client) *Handler {
	if logger == nil {
		logger = log.New(os.Stdout, "", log.LstdFlags)
	}
	return &Handler{logger: logger, cfg: cfg, client: client}
}

// ServeHTTP routes requests through path normalization, aliases, and handlers.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.cfg != nil && h.cfg.Verbose {
		h.logger.Printf("%s %s", r.Method, resolvePath(r.URL.Path))
	}

	if r.Method == http.MethodOptions {
		writeCORS(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	path := resolvePath(r.URL.Path)
	switch path {
	case "/", "/health":
		if r.Method != http.MethodGet {
			writeOpenAIError(w, "Method not allowed", "invalid_request_error", "method_not_allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	case "/v1/models":
		if r.Method != http.MethodGet {
			writeOpenAIError(w, "Method not allowed", "invalid_request_error", "method_not_allowed", http.StatusMethodNotAllowed)
			return
		}
		h.handleModels(w)
	case "/v1/chat/completions":
		if r.Method != http.MethodPost {
			writeOpenAIError(w, "Method not allowed", "invalid_request_error", "method_not_allowed", http.StatusMethodNotAllowed)
			return
		}
		if !h.authorize(r) {
			writeOpenAIError(w, "Invalid API key", "invalid_api_key", "invalid_api_key", http.StatusUnauthorized)
			return
		}
		h.handleChatCompletions(w, r)
	default:
		writeOpenAIError(w, "Not found", "invalid_request_error", "not_found", http.StatusNotFound)
	}
}

func (h *Handler) handleModels(w http.ResponseWriter) {
	now := time.Now().Unix()
	writeJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data": []map[string]any{{
			"id":       chatjimmy.DefaultModel,
			"object":   "model",
			"created":  now,
			"owned_by": "chatjimmy",
		}},
	})
}

func (h *Handler) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	result, err := h.completeChat(r)
	if err != nil {
		var pe *proxyError
		if ok := asProxyError(err, &pe); ok {
			writeOpenAIError(w, pe.message, pe.typ, pe.code, pe.status)
			return
		}
		writeOpenAIError(w, err.Error(), "api_error", "upstream_error", http.StatusBadGateway)
		return
	}

	if result.stream {
		writeSSE(w, encodeStream(result.completion, result.tools))
		return
	}
	writeJSON(w, http.StatusOK, result.completion)
}

func (h *Handler) authorize(r *http.Request) bool {
	expected := strings.TrimSpace(h.cfg.APIKey)
	if expected == "" {
		return true
	}
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return false
	}
	actual := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	return actual != "" && actual == expected
}

func writeCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization,Content-Type")
	w.Header().Set("Access-Control-Max-Age", "86400")
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	writeCORS(w)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeOpenAIError(w http.ResponseWriter, message, typ, code string, status int) {
	body, _ := chatjimmy.NewOpenAIError(message, typ, code, status)
	writeJSON(w, status, body)
}

func writeSSE(w http.ResponseWriter, payload []byte) {
	writeCORS(w)
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
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

func asProxyError(err error, target **proxyError) bool {
	if err == nil {
		return false
	}
	if pe, ok := err.(*proxyError); ok {
		*target = pe
		return true
	}
	return false
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
