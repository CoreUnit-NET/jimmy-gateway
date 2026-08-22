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

func TestParseUpstreamStripsThinking(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "think plus answer",
			raw:  "<think>plan</think>Hello",
			want: "Hello",
		},
		{
			name: "thinking plus tool xml",
			raw:  `<thinking>x</thinking><tool_call>{"name":"bash","arguments":{}}</tool_call>`,
			want: `<tool_call>{"name":"bash","arguments":{}}</tool_call>`,
		},
		{
			name: "reasoning only",
			raw:  "<reasoning>secret</reasoning>",
			want: "",
		},
		{
			name: "reflection wrapper",
			raw:  "<reflection>notes</reflection>Done",
			want: "Done",
		},
		{
			name: "prose with think word",
			raw:  "I think this is fine",
			want: "I think this is fine",
		},
		{
			name: "stats and thinking",
			raw:  `<think>plan</think>Answer<|stats|>{"prefill_tokens":1,"decode_tokens":2,"total_tokens":3}<|/stats|>`,
			want: "Answer",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parsed := ParseUpstream(tc.raw)
			if parsed.Text != tc.want {
				t.Fatalf("text = %q, want %q", parsed.Text, tc.want)
			}
		})
	}

	parsed := ParseUpstream(`<think>plan</think>Answer<|stats|>{"prefill_tokens":1,"decode_tokens":2,"total_tokens":3}<|/stats|>`)
	if parsed.Usage.TotalTokens != 3 {
		t.Fatalf("usage total = %d, want 3", parsed.Usage.TotalTokens)
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

func TestParseUpstreamThinkingEdgeCases(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "attributes on think tag",
			raw:  `<think class="x">hidden</think>Visible`,
			want: "Visible",
		},
		{
			name: "multiple thinking blocks",
			raw:  `<think>a</think>mid<thinking>b</thinking>end`,
			want: "midend",
		},
		{
			name: "nested-looking but sequential",
			raw:  `<reasoning>outer</reasoning><reflection>inner</reflection>ok`,
			want: "ok",
		},
		{
			name: "unclosed think stripped to eof",
			raw:  `<think>oops still open`,
			want: "",
		},
		{
			name: "thinking only whitespace after strip",
			raw:  `<think>x</think>   `,
			want: "",
		},
		{
			name: "unclosed think after prose stripped",
			raw:  `use <think> as prose? no close`,
			want: "use",
		},
		{
			name: "closed think then unclosed thinking",
			raw:  `<think>a</think>keep<thinking>leaked`,
			want: "keep",
		},
		{
			name: "stats then thinking order",
			raw:  `Hi<|stats|>{"prefill_tokens":1,"decode_tokens":1,"total_tokens":2}<|/stats|><think>x</think>`,
			want: "Hi",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parsed := ParseUpstream(tc.raw)
			if parsed.Text != tc.want {
				t.Fatalf("text = %q, want %q", parsed.Text, tc.want)
			}
		})
	}

	parsed := ParseUpstream(`Hi<|stats|>{"prefill_tokens":1,"decode_tokens":1,"total_tokens":2}<|/stats|><think>x</think>`)
	if parsed.Usage.TotalTokens != 2 {
		t.Fatalf("usage = %+v", parsed.Usage)
	}
}
