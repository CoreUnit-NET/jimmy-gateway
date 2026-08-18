package chatjimmy

import "testing"

func TestParseUpstreamAlternateStatsTag(t *testing.T) {
	raw := `Answer<stats>{"prefill_tokens":3,"decode_tokens":7,"total_tokens":10}</stats>`
	parsed := ParseUpstream(raw)
	if parsed.Text != "Answer" {
		t.Fatalf("text = %q", parsed.Text)
	}
	if parsed.Usage.PromptTokens != 3 || parsed.Usage.CompletionTokens != 7 {
		t.Fatalf("usage = %+v", parsed.Usage)
	}
}

func TestParseUpstreamNoStats(t *testing.T) {
	parsed := ParseUpstream("plain response")
	if parsed.Text != "plain response" {
		t.Fatalf("text = %q", parsed.Text)
	}
	if parsed.Usage.TotalTokens != 0 {
		t.Fatalf("usage = %+v", parsed.Usage)
	}
	if len(parsed.Stats) != 0 {
		t.Fatalf("stats = %#v", parsed.Stats)
	}
}

func TestParseUpstreamInvalidStatsJSON(t *testing.T) {
	raw := `text<|stats|>not-json<|/stats|>`
	parsed := ParseUpstream(raw)
	if parsed.Text != "text" {
		t.Fatalf("text = %q", parsed.Text)
	}
	rawVal, ok := parsed.Stats["raw"].(string)
	if !ok || rawVal != "not-json" {
		t.Fatalf("stats = %#v", parsed.Stats)
	}
}

func TestParseUpstreamTotalTokensFallback(t *testing.T) {
	raw := `ok<|stats|>{"prefill_tokens":4,"decode_tokens":6}<|/stats|>`
	parsed := ParseUpstream(raw)
	if parsed.Usage.TotalTokens != 10 {
		t.Fatalf("total = %d, want 10", parsed.Usage.TotalTokens)
	}
}

func TestSafeInt(t *testing.T) {
	tests := []struct {
		in   any
		want int
	}{
		{int(5), 5},
		{int64(9), 9},
		{float64(3), 3},
		{"12", 12},
		{"bad", 0},
		{nil, 0},
	}
	for _, tc := range tests {
		if got := safeInt(tc.in); got != tc.want {
			t.Fatalf("safeInt(%#v) = %d, want %d", tc.in, got, tc.want)
		}
	}
}
