package chatjimmy

import (
	"encoding/json"
	"testing"
)

func TestContentToTextString(t *testing.T) {
	got := ContentToText(json.RawMessage(`"hello world"`))
	if got != "hello world" {
		t.Fatalf("got %q", got)
	}
}

func TestContentToTextNullAndEmpty(t *testing.T) {
	if got := ContentToText(nil); got != "" {
		t.Fatalf("nil = %q", got)
	}
	if got := ContentToText(json.RawMessage(`null`)); got != "" {
		t.Fatalf("null = %q", got)
	}
}

func TestContentToTextArrayParts(t *testing.T) {
	raw := json.RawMessage(`[
		{"type":"text","text":"line one"},
		{"type":"text","text":"line two"},
		"plain"
	]`)
	got := ContentToText(raw)
	want := "line one\nline two\nplain"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestContentToTextObjectText(t *testing.T) {
	got := ContentToText(json.RawMessage(`{"text":"from text field"}`))
	if got != "from text field" {
		t.Fatalf("got %q", got)
	}
}

func TestContentToTextObjectContent(t *testing.T) {
	got := ContentToText(json.RawMessage(`{"content":"nested content"}`))
	if got != "nested content" {
		t.Fatalf("got %q", got)
	}
}
