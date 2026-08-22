package service

import "testing"

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", "/"},
		{"/", "/"},
		{"/health/", "/health"},
		{"/v1/models/", "/v1/models"},
		{"/api/v1-chat-completions/", "/api/v1-chat-completions"},
		{"/unknown/path/", "/unknown/path"},
	}
	for _, tc := range tests {
		if got := normalizePath(tc.in); got != tc.want {
			t.Fatalf("normalizePath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestMapAliasPath(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"/api", "/"},
		{"/api/health", "/health"},
		{"/healthz", "/health"},
		{"/api/v1-models", "/v1/models"},
		{"/models", "/v1/models"},
		{"/api/v1-chat-completions", "/v1/chat/completions"},
		{"/chat/completions", "/v1/chat/completions"},
		{"/api/v1-completions", "/v1/completions"},
		{"/completions", "/v1/completions"},
		{"/v1/chat/completions", "/v1/chat/completions"},
	}
	for _, tc := range tests {
		if got := mapAliasPath(tc.in); got != tc.want {
			t.Fatalf("mapAliasPath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestResolvePathAliases(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"/api/", "/"},
		{"/api/health/", "/health"},
		{"/healthz/", "/health"},
		{"/api/v1-models", "/v1/models"},
		{"/models/", "/v1/models"},
		{"/api/v1-chat-completions", "/v1/chat/completions"},
		{"/chat/completions/", "/v1/chat/completions"},
		{"/api/v1-completions", "/v1/completions"},
		{"/completions/", "/v1/completions"},
	}
	for _, tc := range tests {
		if got := resolvePath(tc.in); got != tc.want {
			t.Fatalf("resolvePath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
