package service

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/CoreUnit-NET/jimmy-gateway/internal/config"
	"github.com/CoreUnit-NET/jimmy-gateway/internal/settings"
	"github.com/CoreUnit-NET/jimmy-gateway/lib/chatjimmy"
)

func testSettings() *settings.Settings {
	s, err := settings.FromAppConfig(&config.AppConfig{
		Host:             "0.0.0.0",
		Port:             8080,
		AllowedOrigin:    "*",
		ChatJimmyURL:     "https://chatjimmy.ai/api/chat",
		ChatJimmyTimeout: 120,
	})
	if err != nil {
		panic(err)
	}
	return s
}

func testConfig() *settings.Settings {
	s := testSettings()
	s.APIKey = "secret"
	return s
}

func TestLogChatCompactionAlways(t *testing.T) {
	var buf bytes.Buffer
	h := NewHandler(log.New(&buf, "", 0), testSettings(), &chatjimmy.Client{})
	h.logChat("openai", chatjimmy.DefaultModel, time.Now(), true, true, chatjimmy.Usage{
		PromptTokens:     11,
		CompletionTokens: 7,
	})
	got := buf.String()
	if !strings.Contains(got, "dropped tools JSON") {
		t.Fatalf("log = %q, want compaction message", got)
	}
	if !strings.Contains(got, "system prompt truncated") {
		t.Fatalf("log = %q, want truncate message", got)
	}
	if !strings.Contains(got, "chat kind=openai model="+chatjimmy.DefaultModel+" upstream=") {
		t.Fatalf("log = %q, want always-on chat summary", got)
	}
	if strings.Contains(got, "prompt_tokens=") || strings.Contains(got, "truncated=") {
		t.Fatalf("verbose chat detail leaked without verbose: %q", got)
	}
}

func TestLogChatVerboseDetails(t *testing.T) {
	var buf bytes.Buffer
	s := testSettings()
	s.Verbose = true
	h := NewHandler(log.New(&buf, "", 0), s, &chatjimmy.Client{})
	h.logChat("openai", chatjimmy.DefaultModel, time.Now(), false, false, chatjimmy.Usage{
		PromptTokens:     11,
		CompletionTokens: 7,
	})
	got := buf.String()
	if !strings.Contains(got, "prompt_tokens=11") || !strings.Contains(got, "completion_tokens=7") {
		t.Fatalf("log = %q, want verbose token fields", got)
	}
	if !strings.Contains(got, "upstream=") || !strings.Contains(got, "ms") {
		t.Fatalf("log = %q, want upstream latency in ms", got)
	}
}

func TestLogChatEmptyFields(t *testing.T) {
	var buf bytes.Buffer
	h := NewHandler(log.New(&buf, "", 0), testSettings(), &chatjimmy.Client{})
	h.logChat("", "", time.Now(), false, false, chatjimmy.Usage{})
	got := buf.String()
	if !strings.Contains(got, "chat kind=unknown model=- upstream=") {
		t.Fatalf("log = %q, want empty-field defaults", got)
	}
}

func TestLogRequestNilSafe(t *testing.T) {
	var buf bytes.Buffer
	h := NewHandler(log.New(&buf, "", 0), testSettings(), &chatjimmy.Client{})
	h.logRequest(nil, http.StatusOK, time.Now())
	got := buf.String()
	if !strings.Contains(got, "UNKNOWN / status=200 duration=") || !strings.Contains(got, "ms remote=-") {
		t.Fatalf("log = %q, want nil-safe access log", got)
	}

	// Nil handler / nil logger must not panic.
	var nilH *Handler
	nilH.logRequest(nil, http.StatusOK, time.Now())
	nilH.logChat("openai", "m", time.Now(), false, false, chatjimmy.Usage{})
	hNilLog := &Handler{logger: nil, settings: testSettings()}
	hNilLog.logRequest(nil, http.StatusOK, time.Now())
	hNilLog.logChat("openai", "m", time.Now(), true, true, chatjimmy.Usage{})
}

func TestHandlerServeHTTPNilRequest(t *testing.T) {
	var buf bytes.Buffer
	h := NewHandler(log.New(&buf, "", 0), testSettings(), &chatjimmy.Client{})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	got := buf.String()
	if !strings.Contains(got, "UNKNOWN / status=400") {
		t.Fatalf("log = %q, want nil-request access log", got)
	}
	if strings.Count(got, "status=400") != 1 {
		t.Fatalf("log = %q, want exactly one access line", got)
	}
}

func TestHandlerAccessLogAlways(t *testing.T) {
	var buf bytes.Buffer
	h := NewHandler(log.New(&buf, "", 0), testSettings(), &chatjimmy.Client{})
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.RemoteAddr = "192.0.2.1:1234"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	got := buf.String()
	if !strings.Contains(got, "GET /health status=200 duration=") || !strings.Contains(got, "ms remote=192.0.2.1:1234") {
		t.Fatalf("log = %q, want access log with ms duration and remote", got)
	}
	if strings.Count(got, "GET /health status=200") != 1 {
		t.Fatalf("log = %q, want exactly one access line", got)
	}
}

func TestHandlerAccessLogKeepsAliasPath(t *testing.T) {
	var buf bytes.Buffer
	h := NewHandler(log.New(&buf, "", 0), testSettings(), &chatjimmy.Client{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1-models", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	got := buf.String()
	if !strings.Contains(got, "GET /api/v1-models status=200") {
		t.Fatalf("log = %q, want client alias path preserved", got)
	}
	if strings.Contains(got, "GET /v1/models status=") {
		t.Fatalf("log = %q, resolved path must not replace alias", got)
	}
}

func TestHandlerAccessLogKeepsTrailingSlashAlias(t *testing.T) {
	var buf bytes.Buffer
	h := NewHandler(log.New(&buf, "", 0), testSettings(), &chatjimmy.Client{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1-models/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	got := buf.String()
	if !strings.Contains(got, "GET /api/v1-models/ status=200") {
		t.Fatalf("log = %q, want client trailing-slash path preserved", got)
	}
}

func TestHandlerAccessLogOmitsQuery(t *testing.T) {
	var buf bytes.Buffer
	h := NewHandler(log.New(&buf, "", 0), testSettings(), &chatjimmy.Client{})
	req := httptest.NewRequest(http.MethodGet, "/health?key=secret-token", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	got := buf.String()
	if strings.Contains(got, "secret-token") || strings.Contains(got, "?key=") {
		t.Fatalf("log = %q, query must not appear in access log", got)
	}
	if !strings.Contains(got, "GET /health status=200") {
		t.Fatalf("log = %q, want path without query", got)
	}
}

func TestClientRequestPath(t *testing.T) {
	if got := clientRequestPath(nil); got != "/" {
		t.Fatalf("nil = %q", got)
	}
	empty := httptest.NewRequest(http.MethodGet, "/health", nil)
	empty.URL.Path = ""
	if got := clientRequestPath(empty); got != "/" {
		t.Fatalf("empty path = %q", got)
	}
	noURL := &http.Request{}
	if got := clientRequestPath(noURL); got != "/" {
		t.Fatalf("nil URL = %q", got)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1-models?x=1", nil)
	if got := clientRequestPath(req); got != "/api/v1-models" {
		t.Fatalf("path = %q", got)
	}
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
	if rec.Header().Get("Vary") != "" {
		t.Fatalf("Vary = %q, want empty for * origin", rec.Header().Get("Vary"))
	}
}

func TestHandlerOptionsAllowedOrigin(t *testing.T) {
	cfg := testConfig()
	cfg.AllowedOrigin = "https://app.example"
	h := NewHandler(nil, cfg, &chatjimmy.Client{})
	req := httptest.NewRequest(http.MethodOptions, "/v1/chat/completions", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "https://app.example" {
		t.Fatalf("origin = %q", rec.Header().Get("Access-Control-Allow-Origin"))
	}
	if rec.Header().Get("Vary") != "Origin" {
		t.Fatalf("Vary = %q, want Origin", rec.Header().Get("Vary"))
	}
	allow := rec.Header().Get("Access-Control-Allow-Headers")
	if !strings.Contains(allow, "x-api-key") {
		t.Fatalf("Allow-Headers = %q", allow)
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

func TestBearerToken(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{in: "Bearer secret", want: "secret"},
		{in: "bearer secret", want: "secret"},
		{in: "BEARER secret", want: "secret"},
		{in: "  Bearer   secret  ", want: "secret"},
		{in: "Bearer", want: ""},
		{in: "Token secret", want: ""},
		{in: "", want: ""},
	}
	for _, tc := range tests {
		if got := bearerToken(tc.in); got != tc.want {
			t.Fatalf("bearerToken(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestHandlerAuthBearerCaseInsensitive(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`ok`))
	}))
	t.Cleanup(upstream.Close)

	h := NewHandler(nil, testConfig(), &chatjimmy.Client{URL: upstream.URL})
	body := strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	req.Header.Set("Authorization", "bearer secret")
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

	cfg := testSettings()
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

	cfg := testSettings()
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

func TestHandlerChatCompletionsBodyTooLarge(t *testing.T) {
	cfg := testSettings()
	h := NewHandler(nil, cfg, &chatjimmy.Client{})
	body := strings.NewReader(strings.Repeat("x", maxRequestBytes+1))
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandlerChatCompletionsInvalidJSON(t *testing.T) {
	cfg := testSettings()
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
	cfg := testSettings()
	h := NewHandler(nil, cfg, &chatjimmy.Client{})
	body := strings.NewReader(`{"messages":[]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandlerChatCompletionsRejectsN(t *testing.T) {
	cfg := testSettings()
	h := NewHandler(nil, cfg, &chatjimmy.Client{})
	body := strings.NewReader(`{"n":2,"messages":[{"role":"user","content":"hi"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandlerChatCompletionsStreamBoolish(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`Hi there`))
	}))
	t.Cleanup(upstream.Close)

	cfg := testSettings()
	h := NewHandler(nil, cfg, &chatjimmy.Client{URL: upstream.URL})
	body := strings.NewReader(`{"stream":"true","messages":[{"role":"user","content":"hi"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("content-type = %q", rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(rec.Body.String(), "data: [DONE]") {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestHandlerChatCompletionsUpstreamError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}))
	t.Cleanup(upstream.Close)

	cfg := testSettings()
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
	cfg := testSettings()
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

	cfg := testSettings()
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

	cfg := testSettings()
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

func TestHandlerModelsExtras(t *testing.T) {
	s := testConfig()
	s.ChatJimmyModel = "custom-model"
	s.ChatJimmyModels = []string{"extra-a", "extra-b"}
	h := NewHandler(nil, s, &chatjimmy.Client{})
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
	data := body["data"].([]any)
	first := data[0].(map[string]any)
	if first["id"] != "custom-model" {
		t.Fatalf("first id = %#v", first["id"])
	}
	want := map[string]bool{"extra-a": false, "extra-b": false}
	for _, item := range data {
		id := item.(map[string]any)["id"].(string)
		if _, ok := want[id]; ok {
			want[id] = true
		}
	}
	for id, found := range want {
		if !found {
			t.Fatalf("missing extra model %q", id)
		}
	}
}

func TestHandlerAuthXAPIKey(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`ok`))
	}))
	t.Cleanup(upstream.Close)

	h := NewHandler(nil, testConfig(), &chatjimmy.Client{URL: upstream.URL})
	body := strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	req.Header.Set("x-api-key", "secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandlerNativeChat(t *testing.T) {
	var upstreamBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &upstreamBody)
		_, _ = w.Write([]byte("raw-ok"))
	}))
	t.Cleanup(upstream.Close)

	h := NewHandler(nil, testConfig(), &chatjimmy.Client{URL: upstream.URL})
	body := strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/chat", body)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "raw-ok" {
		t.Fatalf("body = %q", rec.Body.String())
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "text/plain") {
		t.Fatalf("content-type = %q", rec.Header().Get("Content-Type"))
	}
	opts := upstreamBody["chatOptions"].(map[string]any)
	if opts["selectedModel"] != chatjimmy.DefaultModel {
		t.Fatalf("selectedModel = %#v", opts["selectedModel"])
	}
}

func TestHandlerNativeChatEmptyMessages(t *testing.T) {
	h := NewHandler(nil, testConfig(), &chatjimmy.Client{})
	body := strings.NewReader(`{"messages":[]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/chat", body)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandlerChatCompletionsToolChoiceNoneSkipsParse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`<tool_call>{"name":"read","arguments":{"path":"x"}}</tool_call>`))
	}))
	t.Cleanup(upstream.Close)

	cfg := testSettings()
	h := NewHandler(nil, cfg, &chatjimmy.Client{URL: upstream.URL})
	body := strings.NewReader(`{
		"messages":[{"role":"user","content":"read file"}],
		"tool_choice":"none",
		"tools":[{"type":"function","function":{"name":"read"}}]
	}`)
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
	if completion.Choices[0].FinishReason != "stop" {
		t.Fatalf("finish_reason = %q", completion.Choices[0].FinishReason)
	}
	if len(completion.Choices[0].Message.ToolCalls) != 0 {
		t.Fatalf("tool_calls = %+v", completion.Choices[0].Message.ToolCalls)
	}
	if completion.Choices[0].Message.Content == nil {
		t.Fatal("content is nil")
	}
	if strings.Contains(*completion.Choices[0].Message.Content, "<tool_call>") {
		t.Fatalf("tool_call XML leaked into content: %q", *completion.Choices[0].Message.Content)
	}
}

func TestHandlerAnthropicMessages(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`Hello from jimmy`))
	}))
	t.Cleanup(upstream.Close)

	h := NewHandler(nil, testConfig(), &chatjimmy.Client{URL: upstream.URL})
	body := strings.NewReader(`{"model":"claude-3-haiku-20240307","max_tokens":32,"messages":[{"role":"user","content":"hi"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", body)
	req.Header.Set("x-api-key", "secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var msg chatjimmy.AnthropicMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &msg); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if msg.Type != "message" || msg.Role != "assistant" {
		t.Fatalf("msg = %+v", msg)
	}
	if len(msg.Content) == 0 || msg.Content[0].Text != "Hello from jimmy" {
		t.Fatalf("content = %+v", msg.Content)
	}
}

func TestHandlerGeminiGenerate(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`Hello gemini`))
	}))
	t.Cleanup(upstream.Close)

	h := NewHandler(nil, testConfig(), &chatjimmy.Client{URL: upstream.URL})
	body := strings.NewReader(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-1.5-flash:generateContent", body)
	req.Header.Set("x-goog-api-key", "secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp chatjimmy.GeminiResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Candidates) == 0 || resp.Candidates[0].Content.Parts[0].Text != "Hello gemini" {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestHandlerGeminiStream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`streamed`))
	}))
	t.Cleanup(upstream.Close)

	cfg := testSettings()
	h := NewHandler(nil, cfg, &chatjimmy.Client{URL: upstream.URL})
	body := strings.NewReader(`{"contents":[{"parts":[{"text":"hi"}]}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-1.5-flash:streamGenerateContent", body)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("content-type = %q", rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(rec.Body.String(), "streamed") {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestParseGeminiPath(t *testing.T) {
	tests := []struct {
		in     string
		model  string
		stream bool
		ok     bool
	}{
		{"/v1beta/models/gemini-1.5-flash:generateContent", "gemini-1.5-flash", false, true},
		{"/v1beta/models/gemini-1.5-flash:streamGenerateContent", "gemini-1.5-flash", true, true},
		{"/v1beta/models/:generateContent", "", false, false},
		{"/v1/chat/completions", "", false, false},
	}
	for _, tc := range tests {
		model, stream, ok := parseGeminiPath(tc.in)
		if model != tc.model || stream != tc.stream || ok != tc.ok {
			t.Fatalf("parseGeminiPath(%q) = %q %t %t, want %q %t %t", tc.in, model, stream, ok, tc.model, tc.stream, tc.ok)
		}
	}
}
