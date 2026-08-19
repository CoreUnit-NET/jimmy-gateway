package settings

/*
Package settings turns raw config into a validated, app-ready Settings
struct.

Convert/parse values into usable types. Every field must have a validator
that runs before Settings is returned.
*/

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/CoreUnit-NET/jimmy-gateway/internal/config"
)

const (
	defaultAllowedOrigin    = "*"
	maxChatJimmyTimeoutSecs = 300
)

type Settings struct {
	Command     string
	Args        []string
	ShowVersion bool
	Verbose     bool

	Host          string
	Port          int
	APIKey        string
	AllowedOrigin string

	ChatJimmyURL     string
	ChatJimmyTimeout int
	ChatJimmyAPIKey  string
	ChatJimmyModel   string
	ChatJimmyModels  []string
}

// FromAppConfig validates cfg and returns Settings.
func FromAppConfig(cfg *config.AppConfig) (*Settings, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}
	s := &Settings{
		Command:          cfg.Command,
		Args:             append([]string(nil), cfg.Args...),
		ShowVersion:      cfg.ShowVersion,
		Verbose:          cfg.Verbose,
		Host:             strings.TrimSpace(cfg.Host),
		Port:             cfg.Port,
		APIKey:           strings.TrimSpace(cfg.APIKey),
		AllowedOrigin:    strings.TrimSpace(cfg.AllowedOrigin),
		ChatJimmyURL:     strings.TrimSpace(cfg.ChatJimmyURL),
		ChatJimmyTimeout: cfg.ChatJimmyTimeout,
		ChatJimmyAPIKey:  strings.TrimSpace(cfg.ChatJimmyAPIKey),
		ChatJimmyModel:   strings.TrimSpace(cfg.ChatJimmyModel),
		ChatJimmyModels:  splitCommaList(cfg.ChatJimmyModels),
	}
	if err := s.validate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Settings) validate() error {
	if s.Host == "" {
		return fmt.Errorf("host must not be empty")
	}
	if s.Port < 1 || s.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535, got %d", s.Port)
	}
	if s.AllowedOrigin == "" {
		s.AllowedOrigin = defaultAllowedOrigin
	} else {
		if strings.Contains(s.AllowedOrigin, ",") || strings.ContainsAny(s.AllowedOrigin, " \t") {
			return fmt.Errorf("allowed origin must be a single origin")
		}
		if s.AllowedOrigin != "*" {
			s.AllowedOrigin = strings.TrimRight(s.AllowedOrigin, "/")
		}
	}
	if s.ChatJimmyURL == "" {
		return fmt.Errorf("chatjimmy url must not be empty")
	}
	parsed, err := url.Parse(s.ChatJimmyURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("invalid chatjimmy url %q: want http(s) URL", s.ChatJimmyURL)
	}
	if s.ChatJimmyTimeout < 1 || s.ChatJimmyTimeout > maxChatJimmyTimeoutSecs {
		return fmt.Errorf("invalid chatjimmy timeout %d: must be between 1 and %d seconds", s.ChatJimmyTimeout, maxChatJimmyTimeoutSecs)
	}
	return nil
}

// UpstreamTimeout is the ChatJimmy HTTP client timeout.
func (s *Settings) UpstreamTimeout() time.Duration {
	secs := 120
	if s != nil && s.ChatJimmyTimeout > 0 {
		secs = s.ChatJimmyTimeout
	}
	return time.Duration(secs) * time.Second
}

func splitCommaList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
