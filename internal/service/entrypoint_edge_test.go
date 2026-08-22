package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CoreUnit-NET/jimmy-gateway/lib/chatjimmy"
)

func mockUpstreamOK(t *testing.T) *httptest.Server {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`ok`))
	}))
	t.Cleanup(upstream.Close)
	return upstream
}

func TestHandlerHealthMethodNotAllowed(t *testing.T) {
	h := NewHandler(nil, testConfig(), &chatjimmy.Client{})
	req := httptest.NewRequest(http.MethodPost, "/health", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
	var errBody chatjimmy.OpenAIError
	if err := json.Unmarshal(rec.Body.Bytes(), &errBody); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if errBody.Error.Code != "method_not_allowed" {
		t.Fatalf("code = %q", errBody.Error.Code)
	}
}

func TestHandlerChatCompletionsMethodNotAllowed(t *testing.T) {
	h := NewHandler(nil, testConfig(), &chatjimmy.Client{})
	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

func TestHandlerAuthInvalidKey(t *testing.T) {
	h := NewHandler(nil, testConfig(), &chatjimmy.Client{})
	body := strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	req.Header.Set("Authorization", "Bearer wrong")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	var errBody chatjimmy.OpenAIError
	if err := json.Unmarshal(rec.Body.Bytes(), &errBody); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if errBody.Error.Code != "invalid_api_key" {
		t.Fatalf("code = %q", errBody.Error.Code)
	}
}

func TestHandlerAuthXGoogAPIKey(t *testing.T) {
	upstream := mockUpstreamOK(t)
	h := NewHandler(nil, testConfig(), &chatjimmy.Client{URL: upstream.URL})
	body := strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	req.Header.Set("x-goog-api-key", "secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandlerNotFoundErrorShape(t *testing.T) {
	h := NewHandler(nil, testConfig(), &chatjimmy.Client{})
	req := httptest.NewRequest(http.MethodGet, "/nope", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
	var errBody chatjimmy.OpenAIError
	if err := json.Unmarshal(rec.Body.Bytes(), &errBody); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if errBody.Error.Code != "not_found" {
		t.Fatalf("code = %q", errBody.Error.Code)
	}
}

func TestHandlerOptionsOnHealthAndModels(t *testing.T) {
	h := NewHandler(nil, testConfig(), &chatjimmy.Client{})
	for _, path := range []string{"/health", "/v1/models", "/v1/messages"} {
		req := httptest.NewRequest(http.MethodOptions, path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("%s status = %d, want 204", path, rec.Code)
		}
		if rec.Header().Get("Access-Control-Allow-Methods") == "" {
			t.Fatalf("%s missing Allow-Methods", path)
		}
	}
}

func TestHandlerNativeChatAuthRequired(t *testing.T) {
	h := NewHandler(nil, testConfig(), &chatjimmy.Client{})
	body := strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/chat", body)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestHandlerNativeChatMethodNotAllowed(t *testing.T) {
	h := NewHandler(nil, testConfig(), &chatjimmy.Client{})
	req := httptest.NewRequest(http.MethodGet, "/api/chat", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

func TestHandlerNativeChatInvalidJSON(t *testing.T) {
	h := NewHandler(nil, testConfig(), &chatjimmy.Client{})
	body := strings.NewReader(`not-json`)
	req := httptest.NewRequest(http.MethodPost, "/api/chat", body)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandlerNativeChatBodyTooLarge(t *testing.T) {
	h := NewHandler(nil, testConfig(), &chatjimmy.Client{})
	body := strings.NewReader(strings.Repeat("x", maxRequestBytes+1))
	req := httptest.NewRequest(http.MethodPost, "/api/chat", body)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", rec.Code)
	}
}

func TestHandlerNativeChatUpstreamError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream down", http.StatusBadGateway)
	}))
	t.Cleanup(upstream.Close)

	h := NewHandler(nil, testConfig(), &chatjimmy.Client{URL: upstream.URL})
	body := strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/chat", body)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandlerAnthropicAuthRequired(t *testing.T) {
	h := NewHandler(nil, testConfig(), &chatjimmy.Client{})
	body := strings.NewReader(`{"model":"claude-3-haiku-20240307","max_tokens":32,"messages":[{"role":"user","content":"hi"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", body)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	var errBody chatjimmy.AnthropicErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &errBody); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if errBody.Error.Type != "authentication_error" {
		t.Fatalf("type = %q", errBody.Error.Type)
	}
}

func TestHandlerAnthropicMethodNotAllowed(t *testing.T) {
	h := NewHandler(nil, testConfig(), &chatjimmy.Client{})
	req := httptest.NewRequest(http.MethodGet, "/v1/messages", nil)
	req.Header.Set("x-api-key", "secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

func TestHandlerAnthropicInvalidJSON(t *testing.T) {
	h := NewHandler(nil, testConfig(), &chatjimmy.Client{})
	body := strings.NewReader(`{bad`)
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", body)
	req.Header.Set("x-api-key", "secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandlerAnthropicEmptyMessages(t *testing.T) {
	h := NewHandler(nil, testConfig(), &chatjimmy.Client{})
	body := strings.NewReader(`{"model":"claude-3-haiku-20240307","max_tokens":32,"messages":[]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", body)
	req.Header.Set("x-api-key", "secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandlerAnthropicStream(t *testing.T) {
	upstream := mockUpstreamOK(t)
	h := NewHandler(nil, testConfig(), &chatjimmy.Client{URL: upstream.URL})
	body := strings.NewReader(`{"model":"claude-3-haiku-20240307","max_tokens":32,"stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", body)
	req.Header.Set("x-api-key", "secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("content-type = %q", rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(rec.Body.String(), "event: message_stop") {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestHandlerAnthropicBodyTooLarge(t *testing.T) {
	h := NewHandler(nil, testConfig(), &chatjimmy.Client{})
	body := strings.NewReader(strings.Repeat("x", maxRequestBytes+1))
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", body)
	req.Header.Set("x-api-key", "secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", rec.Code)
	}
}

func TestHandlerGeminiAuthRequired(t *testing.T) {
	h := NewHandler(nil, testConfig(), &chatjimmy.Client{})
	body := strings.NewReader(`{"contents":[{"parts":[{"text":"hi"}]}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-1.5-flash:generateContent", body)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	var errBody chatjimmy.GeminiErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &errBody); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if errBody.Error.Status != "UNAUTHENTICATED" {
		t.Fatalf("status = %q", errBody.Error.Status)
	}
}

func TestHandlerGeminiMethodNotAllowed(t *testing.T) {
	h := NewHandler(nil, testConfig(), &chatjimmy.Client{})
	req := httptest.NewRequest(http.MethodGet, "/v1beta/models/gemini-1.5-flash:generateContent", nil)
	req.Header.Set("x-goog-api-key", "secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

func TestHandlerGeminiInvalidJSON(t *testing.T) {
	h := NewHandler(nil, testConfig(), &chatjimmy.Client{})
	body := strings.NewReader(`not-json`)
	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-1.5-flash:generateContent", body)
	req.Header.Set("x-goog-api-key", "secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandlerGeminiEmptyContents(t *testing.T) {
	h := NewHandler(nil, testConfig(), &chatjimmy.Client{})
	body := strings.NewReader(`{"contents":[]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-1.5-flash:generateContent", body)
	req.Header.Set("x-goog-api-key", "secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandlerGeminiInvalidModelPathNotFound(t *testing.T) {
	h := NewHandler(nil, testConfig(), &chatjimmy.Client{})
	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/:generateContent", nil)
	req.Header.Set("x-goog-api-key", "secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestHandlerGeminiBodyTooLarge(t *testing.T) {
	h := NewHandler(nil, testConfig(), &chatjimmy.Client{})
	body := strings.NewReader(strings.Repeat("x", maxRequestBytes+1))
	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-1.5-flash:generateContent", body)
	req.Header.Set("x-goog-api-key", "secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", rec.Code)
	}
}

func TestHandlerGeminiMalformedSuffixNotFound(t *testing.T) {
	h := NewHandler(nil, testConfig(), &chatjimmy.Client{})
	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-1.5-flash:unknownAction", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestHandlerChatCompletionsMissingContentTypeStillWorks(t *testing.T) {
	upstream := mockUpstreamOK(t)
	cfg := testSettings()
	h := NewHandler(nil, cfg, &chatjimmy.Client{URL: upstream.URL})
	body := strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandlerChatCompletionsToolRoundTripWithoutToolName(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`done`))
	}))
	t.Cleanup(upstream.Close)

	cfg := testSettings()
	h := NewHandler(nil, cfg, &chatjimmy.Client{URL: upstream.URL})
	body := strings.NewReader(`{
		"messages":[
			{"role":"user","content":"run"},
			{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"read","arguments":"{\"path\":\"x\"}"}}]},
			{"role":"tool","tool_call_id":"call_1","content":"file body"}
		]
	}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandlerTrailingSlashHealthAndModels(t *testing.T) {
	h := NewHandler(nil, testConfig(), &chatjimmy.Client{})
	for _, path := range []string{"/health/", "/v1/models/", "/api/health/", "/api/v1-models/"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200", path, rec.Code)
		}
	}
}

func TestHandlerChatAliasMethodNotAllowed(t *testing.T) {
	h := NewHandler(nil, testConfig(), &chatjimmy.Client{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1-chat-completions", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

func TestHandlerAnthropicUpstreamError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	t.Cleanup(upstream.Close)

	h := NewHandler(nil, testConfig(), &chatjimmy.Client{URL: upstream.URL})
	body := strings.NewReader(`{"model":"claude-3-haiku-20240307","max_tokens":32,"messages":[{"role":"user","content":"hi"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", body)
	req.Header.Set("x-api-key", "secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var errBody chatjimmy.AnthropicErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &errBody); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if errBody.Error.Type == "" {
		t.Fatalf("error = %+v", errBody.Error)
	}
}

func TestHandlerGeminiUpstreamError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	t.Cleanup(upstream.Close)

	h := NewHandler(nil, testConfig(), &chatjimmy.Client{URL: upstream.URL})
	body := strings.NewReader(`{"contents":[{"parts":[{"text":"hi"}]}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-1.5-flash:generateContent", body)
	req.Header.Set("x-goog-api-key", "secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var errBody chatjimmy.GeminiErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &errBody); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if errBody.Error.Status == "" {
		t.Fatalf("error = %+v", errBody.Error)
	}
}

func TestHandlerAuthOpenWhenAPIKeyEmpty(t *testing.T) {
	upstream := mockUpstreamOK(t)
	h := NewHandler(nil, testSettings(), &chatjimmy.Client{URL: upstream.URL})
	body := strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandlerCompletionsEdgeCases(t *testing.T) {
	t.Run("method not allowed", func(t *testing.T) {
		h := NewHandler(nil, testConfig(), &chatjimmy.Client{})
		req := httptest.NewRequest(http.MethodGet, "/v1/completions", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status = %d, want 405", rec.Code)
		}
	})

	t.Run("auth required", func(t *testing.T) {
		h := NewHandler(nil, testConfig(), &chatjimmy.Client{})
		body := strings.NewReader(`{"prompt":"hi"}`)
		req := httptest.NewRequest(http.MethodPost, "/v1/completions", body)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("rejects n greater than 1", func(t *testing.T) {
		h := NewHandler(nil, testSettings(), &chatjimmy.Client{})
		body := strings.NewReader(`{"prompt":"hi","n":2}`)
		req := httptest.NewRequest(http.MethodPost, "/v1/completions", body)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("whitespace prompt rejected", func(t *testing.T) {
		h := NewHandler(nil, testSettings(), &chatjimmy.Client{})
		body := strings.NewReader(`{"prompt":"   "}`)
		req := httptest.NewRequest(http.MethodPost, "/v1/completions", body)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("strips thinking from upstream", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`<think>secret</think>visible text`))
		}))
		t.Cleanup(upstream.Close)
		h := NewHandler(nil, testSettings(), &chatjimmy.Client{URL: upstream.URL})
		body := strings.NewReader(`{"prompt":"hi"}`)
		req := httptest.NewRequest(http.MethodPost, "/v1/completions", body)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
		}
		var out chatjimmy.TextCompletion
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(out.Choices) != 1 || out.Choices[0].Text != "visible text" {
			t.Fatalf("choices = %+v", out.Choices)
		}
		if strings.Contains(out.Choices[0].Text, "secret") || strings.Contains(out.Choices[0].Text, "<think>") {
			t.Fatalf("thinking leaked: %q", out.Choices[0].Text)
		}
	})
}

func TestHandlerChatStreamIncludeUsageFalse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`Hi<|stats|>{"prefill_tokens":2,"decode_tokens":3,"total_tokens":5}<|/stats|>`))
	}))
	t.Cleanup(upstream.Close)

	h := NewHandler(nil, testSettings(), &chatjimmy.Client{URL: upstream.URL})
	body := strings.NewReader(`{"stream":true,"stream_options":{"include_usage":false},"messages":[{"role":"user","content":"hi"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	out := rec.Body.String()
	if strings.Contains(out, `"prompt_tokens"`) {
		t.Fatalf("usage leaked with include_usage false: %q", out)
	}
	if !strings.Contains(out, "data: [DONE]") {
		t.Fatalf("missing DONE: %q", out)
	}
}
