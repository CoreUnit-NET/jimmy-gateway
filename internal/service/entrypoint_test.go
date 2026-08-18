package service

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CoreUnit-NET/jimmy-gateway/internal/config"
	"github.com/CoreUnit-NET/jimmy-gateway/lib/chatjimmy"
)

func testConfig() *config.AppConfig {
	cfg := config.DefaultAppConfigForTest()
	cfg.APIKey = "secret"
	return cfg
}

func TestHandlerHealth(t *testing.T) {
	h := NewHandler(nil, testConfig(), &chatjimmy.Client{})
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestHandlerHealthAlias(t *testing.T) {
	h := NewHandler(nil, testConfig(), &chatjimmy.Client{})
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestHandlerOptions(t *testing.T) {
	h := NewHandler(nil, testConfig(), &chatjimmy.Client{})
	req := httptest.NewRequest(http.MethodOptions, "/v1/chat/completions", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("expected CORS header")
	}
}

func TestHandlerNotFound(t *testing.T) {
	h := NewHandler(nil, testConfig(), &chatjimmy.Client{})
	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestHandlerModels(t *testing.T) {
	h := NewHandler(nil, testConfig(), &chatjimmy.Client{})
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	data, ok := body["data"].([]any)
	if !ok || len(data) != len(chatjimmy.ListedModels()) {
		t.Fatalf("data len = %d, want %d (%#v)", len(data), len(chatjimmy.ListedModels()), body["data"])
	}
}

func TestHandlerModelsAlias(t *testing.T) {
	h := NewHandler(nil, testConfig(), &chatjimmy.Client{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1-models", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestHandlerAuthRequired(t *testing.T) {
	h := NewHandler(nil, testConfig(), &chatjimmy.Client{})
	body := strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestHandlerAuthValid(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`ok`))
	}))
	t.Cleanup(upstream.Close)

	h := NewHandler(nil, testConfig(), &chatjimmy.Client{URL: upstream.URL})
	body := strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandlerChatCompletionsMergesSystemMessages(t *testing.T) {
	var upstreamBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &upstreamBody)
		_, _ = w.Write([]byte(`done`))
	}))
	t.Cleanup(upstream.Close)

	cfg := config.DefaultAppConfigForTest()
	h := NewHandler(nil, cfg, &chatjimmy.Client{URL: upstream.URL})
	body := strings.NewReader(`{
		"messages":[
			{"role":"system","content":"You are helpful."},
			{"role":"system","content":"Be concise."},
			{"role":"user","content":"hi"}
		]
	}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}

	opts, ok := upstreamBody["chatOptions"].(map[string]any)
	if !ok {
		t.Fatalf("chatOptions = %#v", upstreamBody["chatOptions"])
	}
	if opts["systemPrompt"] != "You are helpful.\nBe concise." {
		t.Fatalf("systemPrompt = %#v", opts["systemPrompt"])
	}

	msgs, ok := upstreamBody["messages"].([]any)
	if !ok || len(msgs) != 1 {
		t.Fatalf("messages = %#v", upstreamBody["messages"])
	}
	first, ok := msgs[0].(map[string]any)
	if !ok || first["role"] != "user" {
		t.Fatalf("first message = %#v", msgs[0])
	}
}

func TestHandlerChatCompletions(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Fatalf("content-type = %q", ct)
		}
		_, _ = w.Write([]byte(`Hello<|stats|>{"prefill_tokens":1,"decode_tokens":2,"total_tokens":3}<|/stats|>`))
	}))
	t.Cleanup(upstream.Close)

	cfg := testConfig()
	cfg.APIKey = ""
	client := &chatjimmy.Client{URL: upstream.URL}

	h := NewHandler(nil, cfg, client)
	body := strings.NewReader(`{"model":"llama3.1-8B","messages":[{"role":"user","content":"hi"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}

	var completion chatjimmy.Completion
	if err := json.Unmarshal(rec.Body.Bytes(), &completion); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if completion.Usage.TotalTokens != 3 {
		t.Fatalf("usage = %+v", completion.Usage)
	}
	if completion.ChatJimmyStats == nil {
		t.Fatal("expected chatjimmy_stats")
	}

	if len(completion.Choices) == 0 {
		t.Fatalf("choices = %#v", completion.Choices)
	}
	msg := completion.Choices[0].Message
	if msg.Content == nil || *msg.Content != "Hello" {
		t.Fatalf("content = %#v", msg.Content)
	}
}

func TestHandlerChatCompletionsStream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`Hi there`))
	}))
	t.Cleanup(upstream.Close)

	cfg := config.DefaultAppConfigForTest()
	client := &chatjimmy.Client{URL: upstream.URL}
	h := NewHandler(nil, cfg, client)

	body := strings.NewReader(`{"stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1-chat-completions", body)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "data: [DONE]") {
		t.Fatalf("body = %q", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Hi there") {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestHandlerChatCompletionsInvalidJSON(t *testing.T) {
	cfg := config.DefaultAppConfigForTest()
	h := NewHandler(nil, cfg, &chatjimmy.Client{})
	body := strings.NewReader(`not-json`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandlerChatCompletionsEmptyMessages(t *testing.T) {
	cfg := config.DefaultAppConfigForTest()
	h := NewHandler(nil, cfg, &chatjimmy.Client{})
	body := strings.NewReader(`{"messages":[]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandlerChatCompletionsUpstreamError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}))
	t.Cleanup(upstream.Close)

	cfg := config.DefaultAppConfigForTest()
	h := NewHandler(nil, cfg, &chatjimmy.Client{URL: upstream.URL})
	body := strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}

	var errBody chatjimmy.OpenAIError
	if err := json.Unmarshal(rec.Body.Bytes(), &errBody); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if errBody.Error.Code != "upstream_status_error" {
		t.Fatalf("code = %q", errBody.Error.Code)
	}
}

func TestResolvePath(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"/v1/models/", "/v1/models"},
		{"/api/v1-models/", "/v1/models"},
		{"/api", "/"},
	}
	for _, tc := range tests {
		if got := resolvePath(tc.in); got != tc.want {
			t.Fatalf("resolvePath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestHandlerRootHealth(t *testing.T) {
	h := NewHandler(nil, testConfig(), &chatjimmy.Client{})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestHandlerMethodNotAllowedModels(t *testing.T) {
	h := NewHandler(nil, testConfig(), &chatjimmy.Client{})
	req := httptest.NewRequest(http.MethodPost, "/v1/models", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

func TestHandlerChatCompletionsOnlySystemMessages(t *testing.T) {
	cfg := config.DefaultAppConfigForTest()
	h := NewHandler(nil, cfg, &chatjimmy.Client{})
	body := strings.NewReader(`{"messages":[{"role":"system","content":"rules"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandlerChatCompletionsChatOptionsSystemPrompt(t *testing.T) {
	var upstreamBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &upstreamBody)
		_, _ = w.Write([]byte(`ok`))
	}))
	t.Cleanup(upstream.Close)

	cfg := config.DefaultAppConfigForTest()
	h := NewHandler(nil, cfg, &chatjimmy.Client{URL: upstream.URL})
	body := strings.NewReader(`{
		"messages":[{"role":"user","content":"hi"}],
		"chatOptions":{"systemPrompt":"fallback prompt"}
	}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	opts := upstreamBody["chatOptions"].(map[string]any)
	if opts["systemPrompt"] != "fallback prompt" {
		t.Fatalf("systemPrompt = %#v", opts["systemPrompt"])
	}
}

func TestHandlerChatCompletionsToolResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`<tool_call>{"name":"read","arguments":{"path":"x"}}</tool_call>`))
	}))
	t.Cleanup(upstream.Close)

	cfg := config.DefaultAppConfigForTest()
	h := NewHandler(nil, cfg, &chatjimmy.Client{URL: upstream.URL})
	body := strings.NewReader(`{
		"messages":[{"role":"user","content":"read file"}],
		"tools":[{"type":"function","function":{"name":"read","parameters":{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}}}]
	}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}

	var completion map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &completion); err != nil {
		t.Fatalf("decode: %v", err)
	}
	choices := completion["choices"].([]any)
	choice := choices[0].(map[string]any)
	if choice["finish_reason"] != "tool_calls" {
		t.Fatalf("finish_reason = %#v", choice["finish_reason"])
	}
}
