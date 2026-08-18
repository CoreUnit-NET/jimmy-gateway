package chatjimmy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	DefaultMaxRetries = 3
	maxRetryBackoff   = 10 * time.Second
	defaultOrigin     = "https://chatjimmy.ai"
	defaultReferer    = "https://chatjimmy.ai/"
	defaultUserAgent  = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36"
)

type Client struct {
	HTTP    *http.Client
	URL     string
	Timeout time.Duration
	APIKey  string

	// MaxRetries is extra attempts after the first. Zero means no extra retries
	// (tests leave this unset so 502 mapping stays fast). Production sets 3.
	MaxRetries int
	// RetryBackoff is the base delay for exponential backoff (1x/2x/4x, cap 10s).
	// Zero skips sleeping between retries.
	RetryBackoff time.Duration

	// TrueSSE is gated: ChatJimmy currently returns one body. Do not stream
	// tokens to clients from this flag — XML tool_call parse needs the full body.
	TrueSSE bool

	httpOnce   sync.Once
	httpClient *http.Client
}

func (c *Client) Chat(ctx context.Context, payload UpstreamPayload) (string, error) {
	text, err := c.roundTrip(ctx, payload, c.maxRetries())
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(text) != "" {
		return text, nil
	}
	// One extra empty-body attempt. Does not recycle MaxRetries.
	return c.roundTrip(ctx, payload, 0)
}

// ChatStream is the gated true-SSE hook. ChatJimmy still returns one blob, so
// this currently buffers the same way Chat does.
func (c *Client) ChatStream(ctx context.Context, payload UpstreamPayload) (string, error) {
	return c.Chat(ctx, payload)
}

func (c *Client) maxRetries() int {
	if c == nil || c.MaxRetries < 0 {
		return 0
	}
	return c.MaxRetries
}

func (c *Client) roundTrip(ctx context.Context, payload UpstreamPayload, extra int) (string, error) {
	if c == nil {
		return "", fmt.Errorf("client is nil")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal upstream payload: %w", err)
	}

	if extra < 0 {
		extra = 0
	}

	var lastText string
	var lastStatus int
	var lastErr error
	var lastHeader http.Header
	for attempt := 0; attempt <= extra; attempt++ {
		if attempt > 0 {
			delay := retryAfterDelay(lastHeader, c.backoffDelay(attempt-1))
			if err := c.sleep(ctx, delay); err != nil {
				return "", err
			}
		}
		text, status, hdr, err := c.doOnce(ctx, body)
		lastText, lastStatus, lastHeader, lastErr = text, status, hdr, err
		if err == nil && status >= 200 && status < 300 {
			return text, nil
		}
		if !shouldRetry(status, err) {
			break
		}
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("upstream returned %d: %s", lastStatus, truncate(lastText, 500))
}

func (c *Client) doOnce(ctx context.Context, body []byte) (string, int, http.Header, error) {
	rawURL := c.URL
	if rawURL == "" {
		rawURL = DefaultUpstreamURL
	}

	httpClient := c.transport()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(body))
	if err != nil {
		return "", 0, nil, fmt.Errorf("create upstream request: %w", err)
	}
	origin, referer := originHeaders(rawURL)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Origin", origin)
	req.Header.Set("Referer", referer)
	req.Header.Set("User-Agent", defaultUserAgent)
	if key := strings.TrimSpace(c.APIKey); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", 0, nil, fmt.Errorf("upstream request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", resp.StatusCode, resp.Header.Clone(), fmt.Errorf("read upstream response: %w", err)
	}
	return string(raw), resp.StatusCode, resp.Header.Clone(), nil
}

func (c *Client) transport() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	c.httpOnce.Do(func() {
		timeout := c.Timeout
		if timeout <= 0 {
			timeout = 120 * time.Second
		}
		c.httpClient = &http.Client{Timeout: timeout}
	})
	return c.httpClient
}

func (c *Client) backoffDelay(failIndex int) time.Duration {
	base := c.RetryBackoff
	if base <= 0 {
		return 0
	}
	if failIndex < 0 {
		failIndex = 0
	}
	if failIndex > 8 {
		failIndex = 8
	}
	d := base * time.Duration(1<<failIndex)
	if d > maxRetryBackoff {
		d = maxRetryBackoff
	}
	return d
}

func (c *Client) sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func shouldRetry(status int, err error) bool {
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return false
		}
		return true
	}
	return status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= 500
}

func retryAfterDelay(header http.Header, fallback time.Duration) time.Duration {
	if header == nil {
		return fallback
	}
	raw := strings.TrimSpace(header.Get("Retry-After"))
	if raw == "" {
		return fallback
	}
	if secs, err := strconv.Atoi(raw); err == nil {
		if secs < 0 {
			return fallback
		}
		d := time.Duration(secs) * time.Second
		if d > maxRetryBackoff {
			d = maxRetryBackoff
		}
		return d
	}
	if t, err := http.ParseTime(raw); err == nil {
		d := time.Until(t)
		if d < 0 {
			return 0
		}
		if d > maxRetryBackoff {
			d = maxRetryBackoff
		}
		return d
	}
	return fallback
}

func originHeaders(rawURL string) (origin, referer string) {
	origin, referer = defaultOrigin, defaultReferer
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return
	}
	origin = parsed.Scheme + "://" + parsed.Host
	referer = origin + "/"
	return
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
