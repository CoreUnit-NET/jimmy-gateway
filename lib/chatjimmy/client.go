package chatjimmy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	HTTP    *http.Client
	URL     string
	Timeout time.Duration
}

func (c *Client) Chat(ctx context.Context, payload UpstreamPayload) (string, error) {
	return c.chat(ctx, payload, true)
}

func (c *Client) chat(ctx context.Context, payload UpstreamPayload, retryEmpty bool) (string, error) {
	if c == nil {
		return "", fmt.Errorf("client is nil")
	}
	url := c.URL
	if url == "" {
		url = DefaultUpstreamURL
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal upstream payload: %w", err)
	}

	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create upstream request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Origin", "https://chatjimmy.ai")
	req.Header.Set("Referer", "https://chatjimmy.ai/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("upstream request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read upstream response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("upstream returned %d: %s", resp.StatusCode, truncate(string(raw), 500))
	}
	text := string(raw)
	if retryEmpty && strings.TrimSpace(text) == "" {
		return c.chat(ctx, payload, false)
	}
	return text, nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
