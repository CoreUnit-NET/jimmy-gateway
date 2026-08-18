package chatjimmy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientRetriesOn502(t *testing.T) {
	var attempts int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}))
	t.Cleanup(upstream.Close)

	c := &Client{URL: upstream.URL, MaxRetries: 2, RetryBackoff: 0}
	_, err := c.Chat(context.Background(), UpstreamPayload{
		Messages: []UpstreamMessage{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected upstream error")
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
}

func TestClientNoRetryWhenMaxRetriesZero(t *testing.T) {
	var attempts int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}))
	t.Cleanup(upstream.Close)

	c := &Client{URL: upstream.URL}
	_, err := c.Chat(context.Background(), UpstreamPayload{
		Messages: []UpstreamMessage{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected upstream error")
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

func TestClientRetriesEmptyBodyOnce(t *testing.T) {
	var attempts int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(upstream.Close)

	c := &Client{URL: upstream.URL}
	got, err := c.Chat(context.Background(), UpstreamPayload{
		Messages: []UpstreamMessage{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if got != "ok" {
		t.Fatalf("got %q, want ok", got)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

func TestClientHonorsRetryAfterWithoutSleep(t *testing.T) {
	var attempts int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "0")
			http.Error(w, "slow down", http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(upstream.Close)

	c := &Client{URL: upstream.URL, MaxRetries: 1, RetryBackoff: 0}
	got, err := c.Chat(context.Background(), UpstreamPayload{
		Messages: []UpstreamMessage{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if got != "ok" {
		t.Fatalf("got %q", got)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

func TestClientEmptyBodyRetryDoesNotRecycleMaxRetries(t *testing.T) {
	var attempts int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			return
		}
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}))
	t.Cleanup(upstream.Close)

	c := &Client{URL: upstream.URL, MaxRetries: 3, RetryBackoff: 0}
	_, err := c.Chat(context.Background(), UpstreamPayload{
		Messages: []UpstreamMessage{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected upstream error")
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2 (empty-body extra=0, not MaxRetries)", attempts)
	}
}

func TestClientNoRetryOnCanceledContext(t *testing.T) {
	var attempts int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(upstream.Close)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c := &Client{URL: upstream.URL, MaxRetries: 3, RetryBackoff: 0}
	_, err := c.Chat(ctx, UpstreamPayload{
		Messages: []UpstreamMessage{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if attempts > 1 {
		t.Fatalf("attempts = %d, want 0 or 1", attempts)
	}
}

func TestShouldRetrySkipsCancelAndDeadline(t *testing.T) {
	if shouldRetry(0, context.Canceled) {
		t.Fatal("canceled should not retry")
	}
	if shouldRetry(0, context.DeadlineExceeded) {
		t.Fatal("deadline should not retry")
	}
	if shouldRetry(0, errors.Join(context.Canceled)) {
		t.Fatal("wrapped canceled should not retry")
	}
	if !shouldRetry(0, io.EOF) {
		t.Fatal("network error should retry")
	}
	if !shouldRetry(http.StatusBadGateway, nil) {
		t.Fatal("502 should retry")
	}
	if shouldRetry(http.StatusBadRequest, nil) {
		t.Fatal("400 should not retry")
	}
}

func TestClientSetsBearerAndOrigin(t *testing.T) {
	var gotAuth, gotOrigin, gotReferer string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotOrigin = r.Header.Get("Origin")
		gotReferer = r.Header.Get("Referer")
		_, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(upstream.Close)

	c := &Client{URL: upstream.URL, APIKey: "up-secret"}
	if _, err := c.Chat(context.Background(), UpstreamPayload{
		Messages: []UpstreamMessage{{Role: "user", Content: "hi"}},
	}); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if gotAuth != "Bearer up-secret" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if !strings.HasPrefix(gotOrigin, "http://127.0.0.1:") && !strings.HasPrefix(gotOrigin, "http://localhost:") {
		t.Fatalf("Origin = %q", gotOrigin)
	}
	if !strings.HasPrefix(gotReferer, gotOrigin+"/") {
		t.Fatalf("Referer = %q origin = %q", gotReferer, gotOrigin)
	}
}
