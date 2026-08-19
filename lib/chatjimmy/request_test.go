package chatjimmy

import (
	"encoding/json"
	"testing"
)

func TestChatRequestUnmarshalStreamBoolish(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    bool
		wantErr bool
	}{
		{name: "bool true", raw: `{"stream":true,"messages":[{"role":"user","content":"hi"}]}`, want: true},
		{name: "bool false", raw: `{"stream":false,"messages":[{"role":"user","content":"hi"}]}`, want: false},
		{name: "string true", raw: `{"stream":"true","messages":[{"role":"user","content":"hi"}]}`, want: true},
		{name: "string TRUE", raw: `{"stream":"TRUE","messages":[{"role":"user","content":"hi"}]}`, want: true},
		{name: "string 1", raw: `{"stream":"1","messages":[{"role":"user","content":"hi"}]}`, want: true},
		{name: "string yes", raw: `{"stream":"yes","messages":[{"role":"user","content":"hi"}]}`, want: true},
		{name: "string false", raw: `{"stream":"false","messages":[{"role":"user","content":"hi"}]}`, want: false},
		{name: "number 1", raw: `{"stream":1,"messages":[{"role":"user","content":"hi"}]}`, want: true},
		{name: "number 2", raw: `{"stream":2,"messages":[{"role":"user","content":"hi"}]}`, want: true},
		{name: "number 0", raw: `{"stream":0,"messages":[{"role":"user","content":"hi"}]}`, want: false},
		{name: "null", raw: `{"stream":null,"messages":[{"role":"user","content":"hi"}]}`, want: false},
		{name: "omitted", raw: `{"messages":[{"role":"user","content":"hi"}]}`, want: false},
		{name: "object", raw: `{"stream":{"ok":true},"messages":[{"role":"user","content":"hi"}]}`, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var req ChatRequest
			err := json.Unmarshal([]byte(tc.raw), &req)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if req.Stream != tc.want {
				t.Fatalf("stream = %t, want %t", req.Stream, tc.want)
			}
		})
	}
}

func TestChatRequestUnmarshalMessages(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantLen int
		wantErr bool
	}{
		{name: "array", raw: `{"messages":[{"role":"user","content":"hi"}]}`, wantLen: 1},
		{name: "null", raw: `{"messages":null}`, wantLen: 0},
		{name: "omitted", raw: `{"model":"x"}`, wantLen: 0},
		{name: "empty array", raw: `{"messages":[]}`, wantLen: 0},
		{name: "string", raw: `{"messages":"hi"}`, wantErr: true},
		{name: "object", raw: `{"messages":{"role":"user"}}`, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var req ChatRequest
			err := json.Unmarshal([]byte(tc.raw), &req)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if len(req.Messages) != tc.wantLen {
				t.Fatalf("messages len = %d, want %d", len(req.Messages), tc.wantLen)
			}
		})
	}
}

func TestChatRequestUnmarshalNAndMaxCompletionTokens(t *testing.T) {
	raw := `{"n":1,"max_completion_tokens":64,"stop":null,"messages":[{"role":"user","content":"hi"}]}`
	var req ChatRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.N == nil || *req.N != 1 {
		t.Fatalf("n = %#v", req.N)
	}
	if req.MaxCompletionTokens == nil || *req.MaxCompletionTokens != 64 {
		t.Fatalf("max_completion_tokens = %#v", req.MaxCompletionTokens)
	}
	if !isEmptyJSON(req.Stop) {
		t.Fatalf("stop = %s, want empty JSON", req.Stop)
	}
}
