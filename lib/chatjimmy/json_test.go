package chatjimmy

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestIsEmptyJSON(t *testing.T) {
	tests := []struct {
		name string
		raw  json.RawMessage
		want bool
	}{
		{name: "nil", raw: nil, want: true},
		{name: "empty", raw: json.RawMessage{}, want: true},
		{name: "whitespace", raw: json.RawMessage("  \n\t"), want: true},
		{name: "null", raw: json.RawMessage("null"), want: true},
		{name: "padded null", raw: json.RawMessage("  null  "), want: true},
		{name: "string null", raw: json.RawMessage(`"null"`), want: false},
		{name: "empty object", raw: json.RawMessage(`{}`), want: false},
		{name: "empty array", raw: json.RawMessage(`[]`), want: false},
		{name: "false", raw: json.RawMessage(`false`), want: false},
		{name: "zero", raw: json.RawMessage(`0`), want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isEmptyJSON(tc.raw); got != tc.want {
				t.Fatalf("isEmptyJSON(%q) = %t, want %t", tc.raw, got, tc.want)
			}
		})
	}
}

func TestMustJSON(t *testing.T) {
	raw := mustJSON("hello")
	if string(raw) != `"hello"` {
		t.Fatalf("mustJSON string = %s", raw)
	}
	raw = mustJSON(map[string]any{})
	if string(raw) != `{}` {
		t.Fatalf("mustJSON object = %s", raw)
	}
}

func TestMarshalJSONNoHTMLEscape(t *testing.T) {
	raw, err := MarshalJSON(map[string]string{
		"reason": "<|eot_id|> & more>",
	})
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	got := string(raw)
	if !strings.Contains(got, `"reason":"<|eot_id|> & more>"`) {
		t.Fatalf("MarshalJSON = %s, want literal < > &", got)
	}
	if strings.Contains(got, `\u003c`) || strings.Contains(got, `\u003e`) || strings.Contains(got, `\u0026`) {
		t.Fatalf("MarshalJSON HTML-escaped: %s", got)
	}
}
