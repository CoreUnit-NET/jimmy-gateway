package config

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func clearConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"VERBOSE", "HOST", "PORT", "API_KEY", "OPENAI_API_KEY",
		"ALLOWED_ORIGIN", "CHATJIMMY_URL", "CHATJIMMY_TIMEOUT",
		"CHATJIMMY_API_KEY", "CHATJIMMY_MODEL", "CHATJIMMY_MODELS",
	} {
		t.Setenv(key, "")
	}
}

func TestParseConfigDefaults(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	clearConfigEnv(t)

	os.Args = []string{"jimmy-gateway", "serve"}
	cfg, err := ParseConfig("Demo", "demo")
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}

	if cfg.Command != CommandServe {
		t.Fatalf("Command = %q, want %q", cfg.Command, CommandServe)
	}
	if cfg.Verbose {
		t.Fatal("expected Verbose false by default")
	}
	if cfg.Host != "0.0.0.0" {
		t.Fatalf("Host = %q, want 0.0.0.0", cfg.Host)
	}
	if cfg.Port != 8080 {
		t.Fatalf("Port = %d, want 8080", cfg.Port)
	}
	if cfg.APIKey != "" {
		t.Fatalf("APIKey = %q, want empty", cfg.APIKey)
	}
	if cfg.AllowedOrigin != "*" {
		t.Fatalf("AllowedOrigin = %q, want *", cfg.AllowedOrigin)
	}
	if cfg.ChatJimmyURL != "https://chatjimmy.ai/api/chat" {
		t.Fatalf("ChatJimmyURL = %q, want default", cfg.ChatJimmyURL)
	}
	if cfg.ChatJimmyTimeout != 120 {
		t.Fatalf("ChatJimmyTimeout = %d, want 120", cfg.ChatJimmyTimeout)
	}
	if cfg.ChatJimmyAPIKey != "" || cfg.ChatJimmyModel != "" || cfg.ChatJimmyModels != "" {
		t.Fatalf("unexpected chatjimmy extras: key=%q model=%q models=%q", cfg.ChatJimmyAPIKey, cfg.ChatJimmyModel, cfg.ChatJimmyModels)
	}
}

func TestParseConfigBareRootStartsServer(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	clearConfigEnv(t)

	os.Args = []string{"jimmy-gateway"}
	cfg, err := ParseConfig("Demo", "demo")
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.Command != CommandServe {
		t.Fatalf("Command = %q, want %q", cfg.Command, CommandServe)
	}
	if cfg.ShowVersion {
		t.Fatal("bare root must not set ShowVersion")
	}
}

func TestParseConfigFlagsOverrideEnv(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	clearConfigEnv(t)

	t.Setenv("HOST", "127.0.0.1")
	t.Setenv("PORT", "9090")
	t.Setenv("API_KEY", "env-key")
	t.Setenv("VERBOSE", "true")

	os.Args = []string{
		"jimmy-gateway", "serve",
		"--host", "10.0.0.1",
		"-p", "8081",
		"--api-key", "flag-key",
	}
	cfg, err := ParseConfig("Demo", "demo")
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}

	if !cfg.Verbose {
		t.Fatal("expected Verbose true from env")
	}
	if cfg.Host != "10.0.0.1" {
		t.Fatalf("Host = %q, want flag", cfg.Host)
	}
	if cfg.Port != 8081 {
		t.Fatalf("Port = %d, want flag", cfg.Port)
	}
	if cfg.APIKey != "flag-key" {
		t.Fatalf("APIKey = %q, want flag", cfg.APIKey)
	}
}

func TestParseConfigChatJimmyEnvAndFlags(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	clearConfigEnv(t)

	t.Setenv("ALLOWED_ORIGIN", "https://env.example")
	t.Setenv("CHATJIMMY_URL", "https://env.example/api/chat")
	t.Setenv("CHATJIMMY_TIMEOUT", "90")
	t.Setenv("CHATJIMMY_API_KEY", "env-up")
	t.Setenv("CHATJIMMY_MODEL", "env-model")
	t.Setenv("CHATJIMMY_MODELS", "extra-a,extra-b")

	os.Args = []string{
		"jimmy-gateway", "serve",
		"--allowed-origin", "https://flag.example",
		"--chatjimmy-url", "https://flag.example/api/chat",
		"--chatjimmy-timeout", "45",
		"--chatjimmy-api-key", "flag-up",
		"--chatjimmy-model", "flag-model",
		"--chatjimmy-models", "flag-extra",
	}
	cfg, err := ParseConfig("Demo", "demo")
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.AllowedOrigin != "https://flag.example" {
		t.Fatalf("AllowedOrigin = %q, want flag", cfg.AllowedOrigin)
	}
	if cfg.ChatJimmyURL != "https://flag.example/api/chat" {
		t.Fatalf("ChatJimmyURL = %q, want flag", cfg.ChatJimmyURL)
	}
	if cfg.ChatJimmyTimeout != 45 {
		t.Fatalf("ChatJimmyTimeout = %d, want 45", cfg.ChatJimmyTimeout)
	}
	if cfg.ChatJimmyAPIKey != "flag-up" {
		t.Fatalf("ChatJimmyAPIKey = %q, want flag", cfg.ChatJimmyAPIKey)
	}
	if cfg.ChatJimmyModel != "flag-model" {
		t.Fatalf("ChatJimmyModel = %q, want flag", cfg.ChatJimmyModel)
	}
	if cfg.ChatJimmyModels != "flag-extra" {
		t.Fatalf("ChatJimmyModels = %q, want flag", cfg.ChatJimmyModels)
	}
}

func TestParseConfigOpenAIAPIKeyFallback(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	clearConfigEnv(t)

	t.Setenv("OPENAI_API_KEY", "openai-key")
	os.Args = []string{"jimmy-gateway", "serve"}
	cfg, err := ParseConfig("Demo", "demo")
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.APIKey != "openai-key" {
		t.Fatalf("APIKey = %q, want openai-key", cfg.APIKey)
	}
}

func TestParseConfigAPIKeyPrecedenceOverOpenAIAPIKey(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	clearConfigEnv(t)

	t.Setenv("API_KEY", "primary")
	t.Setenv("OPENAI_API_KEY", "fallback")
	os.Args = []string{"jimmy-gateway", "serve"}
	cfg, err := ParseConfig("Demo", "demo")
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.APIKey != "primary" {
		t.Fatalf("APIKey = %q, want primary", cfg.APIKey)
	}
}

func TestParseConfigInvalidPort(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	clearConfigEnv(t)

	t.Setenv("PORT", "nope")
	os.Args = []string{"jimmy-gateway", "serve"}
	_, err := ParseConfig("Demo", "demo")
	if err == nil {
		t.Fatal("expected error for invalid PORT")
	}
	if !strings.Contains(err.Error(), "PORT") {
		t.Fatalf("expected PORT in error, got: %v", err)
	}
}

func TestParseConfigVersionFlag(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	clearConfigEnv(t)

	os.Args = []string{"jimmy-gateway", "--version"}
	cfg, err := ParseConfig("Demo", "demo")
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if !cfg.ShowVersion {
		t.Fatal("expected ShowVersion true")
	}
}

func TestParseConfigVersionSubcommand(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	clearConfigEnv(t)

	os.Args = []string{"jimmy-gateway", "version"}
	cfg, err := ParseConfig("Demo", "demo")
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.Command != CommandVersion {
		t.Fatalf("Command = %q, want %q", cfg.Command, CommandVersion)
	}
	if !cfg.ShowVersion {
		t.Fatal("expected ShowVersion true")
	}
}

func TestParseConfigHelpRequested(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	clearConfigEnv(t)

	os.Args = []string{"jimmy-gateway", "--help"}
	_, err := ParseConfig("Demo", "demo")
	if !errors.Is(err, ErrHelpRequested) {
		t.Fatalf("err = %v, want ErrHelpRequested", err)
	}
}
