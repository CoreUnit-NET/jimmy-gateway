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
	if cfg.Host != DefaultHost {
		t.Fatalf("Host = %q, want %q", cfg.Host, DefaultHost)
	}
	if cfg.Port != DefaultPort {
		t.Fatalf("Port = %d, want %d", cfg.Port, DefaultPort)
	}
	if cfg.APIKey != "" {
		t.Fatalf("APIKey = %q, want empty", cfg.APIKey)
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

func TestAppConfigValidate(t *testing.T) {
	t.Run("valid defaults", func(t *testing.T) {
		cfg := defaultAppConfig()
		if err := cfg.Validate(); err != nil {
			t.Fatalf("Validate: %v", err)
		}
	})

	t.Run("invalid port", func(t *testing.T) {
		cfg := defaultAppConfig()
		cfg.Port = 0
		if err := cfg.Validate(); err == nil {
			t.Fatal("expected error for port 0")
		}
	})

	t.Run("empty host", func(t *testing.T) {
		cfg := defaultAppConfig()
		cfg.Host = "   "
		if err := cfg.Validate(); err == nil {
			t.Fatal("expected error for empty host")
		}
	})
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
