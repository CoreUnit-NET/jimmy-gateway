package settings

import (
	"testing"
	"time"

	"github.com/CoreUnit-NET/jimmy-gateway/internal/config"
)

func validConfig() *config.AppConfig {
	return &config.AppConfig{
		Host:             "0.0.0.0",
		Port:             8080,
		AllowedOrigin:    "*",
		ChatJimmyURL:     "https://chatjimmy.ai/api/chat",
		ChatJimmyTimeout: 120,
	}
}

func TestFromAppConfigOK(t *testing.T) {
	cfg := validConfig()
	cfg.Verbose = true
	cfg.APIKey = " secret "
	cfg.ChatJimmyAPIKey = " up "
	cfg.ChatJimmyModel = " custom "
	cfg.ChatJimmyModels = "extra-a, extra-b"
	s, err := FromAppConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Verbose {
		t.Fatal("expected Verbose true")
	}
	if s.APIKey != "secret" {
		t.Fatalf("APIKey = %q", s.APIKey)
	}
	if s.ChatJimmyAPIKey != "up" {
		t.Fatalf("ChatJimmyAPIKey = %q", s.ChatJimmyAPIKey)
	}
	if s.ChatJimmyModel != "custom" {
		t.Fatalf("ChatJimmyModel = %q", s.ChatJimmyModel)
	}
	if len(s.ChatJimmyModels) != 2 || s.ChatJimmyModels[0] != "extra-a" || s.ChatJimmyModels[1] != "extra-b" {
		t.Fatalf("ChatJimmyModels = %#v", s.ChatJimmyModels)
	}
	if s.UpstreamTimeout() != 120*time.Second {
		t.Fatalf("UpstreamTimeout = %s", s.UpstreamTimeout())
	}
}

func TestFromAppConfigNil(t *testing.T) {
	if _, err := FromAppConfig(nil); err == nil {
		t.Fatal("expected nil config error")
	}
}

func TestFromAppConfigInvalidPort(t *testing.T) {
	cfg := validConfig()
	cfg.Port = 0
	if _, err := FromAppConfig(cfg); err == nil {
		t.Fatal("expected port error")
	}
}

func TestFromAppConfigEmptyHost(t *testing.T) {
	cfg := validConfig()
	cfg.Host = "   "
	if _, err := FromAppConfig(cfg); err == nil {
		t.Fatal("expected host error")
	}
}

func TestFromAppConfigInvalidURL(t *testing.T) {
	cfg := validConfig()
	cfg.ChatJimmyURL = "not-a-url"
	if _, err := FromAppConfig(cfg); err == nil {
		t.Fatal("expected url error")
	}
}

func TestFromAppConfigEmptyURL(t *testing.T) {
	cfg := validConfig()
	cfg.ChatJimmyURL = "  "
	if _, err := FromAppConfig(cfg); err == nil {
		t.Fatal("expected empty url error")
	}
}

func TestFromAppConfigTimeoutBounds(t *testing.T) {
	cfg := validConfig()
	cfg.ChatJimmyTimeout = 0
	if _, err := FromAppConfig(cfg); err == nil {
		t.Fatal("expected timeout 0 error")
	}
	cfg = validConfig()
	cfg.ChatJimmyTimeout = 301
	if _, err := FromAppConfig(cfg); err == nil {
		t.Fatal("expected timeout 301 error")
	}
}

func TestFromAppConfigEmptyOriginDefault(t *testing.T) {
	cfg := validConfig()
	cfg.AllowedOrigin = "  "
	s, err := FromAppConfig(cfg)
	if err != nil {
		t.Fatalf("FromAppConfig: %v", err)
	}
	if s.AllowedOrigin != "*" {
		t.Fatalf("AllowedOrigin = %q, want *", s.AllowedOrigin)
	}
}

func TestFromAppConfigCommaOriginRejected(t *testing.T) {
	cfg := validConfig()
	cfg.AllowedOrigin = "https://a.example,https://b.example"
	if _, err := FromAppConfig(cfg); err == nil {
		t.Fatal("expected comma origin error")
	}
}

func TestFromAppConfigWhitespaceOriginRejected(t *testing.T) {
	cfg := validConfig()
	cfg.AllowedOrigin = "https://a.example https://b.example"
	if _, err := FromAppConfig(cfg); err == nil {
		t.Fatal("expected whitespace origin error")
	}
}

func TestFromAppConfigTrailingSlashTrimmed(t *testing.T) {
	cfg := validConfig()
	cfg.AllowedOrigin = "https://app.example/"
	s, err := FromAppConfig(cfg)
	if err != nil {
		t.Fatalf("FromAppConfig: %v", err)
	}
	if s.AllowedOrigin != "https://app.example" {
		t.Fatalf("AllowedOrigin = %q", s.AllowedOrigin)
	}
}

func TestFromAppConfigStarOriginKept(t *testing.T) {
	cfg := validConfig()
	cfg.AllowedOrigin = "*"
	s, err := FromAppConfig(cfg)
	if err != nil {
		t.Fatalf("FromAppConfig: %v", err)
	}
	if s.AllowedOrigin != "*" {
		t.Fatalf("AllowedOrigin = %q", s.AllowedOrigin)
	}
}
