package config

/*
Package config owns raw CLI flag and environment parsing (cobra).

It maps flags/env into a plain AppConfig struct only — no validation
beyond parse. Flags override env; missing .env is ignored at main.

User-facing options match the README: PORT and API_KEY (plus OPENAI_API_KEY).
ChatJimmy upstream URL, model, and topK are fixed in lib/chatjimmy.
*/

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

const helpURL = "https://github.com/CoreUnit-NET/jimmy-gateway"

const (
	DefaultHost = "0.0.0.0"
	DefaultPort = 8080
)

// Known subcommand names captured in AppConfig.Command after ParseConfig.
const (
	CommandVersion = "version"
	CommandServe   = "serve"
)

type AppConfig struct {
	Verbose     bool
	ShowVersion bool

	// Command is the selected subcommand name (see Command* constants).
	// Bare root defaults to CommandServe.
	Command string
	// Args are positional args passed to the selected subcommand.
	Args []string

	Host   string
	Port   int
	APIKey string
}

// DefaultAppConfigForTest returns a fresh config with package defaults (tests only).
func DefaultAppConfigForTest() *AppConfig {
	return defaultAppConfig()
}

func defaultAppConfig() *AppConfig {
	return &AppConfig{
		Verbose:     false,
		ShowVersion: false,

		Host: DefaultHost,
		Port: DefaultPort,
	}
}

func versionCommand(appConfig *AppConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			appConfig.ShowVersion = true
			appConfig.Command = CommandVersion
			appConfig.Args = args
		},
	}
}

func serveCommand(appConfig *AppConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start the OpenAI-compatible proxy HTTP server",
		Run: func(cmd *cobra.Command, args []string) {
			appConfig.Command = CommandServe
			appConfig.Args = args
		},
	}
}

func loadEnvVars(appConfig *AppConfig) error {
	if err := envIsBool("VERBOSE", func(value bool) {
		appConfig.Verbose = value
	}); err != nil {
		return err
	}
	if err := envIsString("HOST", func(value string) {
		appConfig.Host = value
	}); err != nil {
		return err
	}
	if err := envIsInt("PORT", func(value int) {
		appConfig.Port = value
	}); err != nil {
		return err
	}
	if err := envIsString("API_KEY", func(value string) {
		appConfig.APIKey = value
	}); err != nil {
		return err
	}
	if err := envIsString("OPENAI_API_KEY", func(value string) {
		if appConfig.APIKey == "" {
			appConfig.APIKey = value
		}
	}); err != nil {
		return err
	}
	return nil
}

func applyServeFlags(appConfig *AppConfig, cmd *cobra.Command) {
	// Persistent so bare root and subcommands share one definition (caddy-forward-auth style).
	cmd.PersistentFlags().StringVar(&appConfig.Host, "host", appConfig.Host, "bind host (HOST)")
	cmd.PersistentFlags().IntVarP(&appConfig.Port, "port", "p", appConfig.Port, "bind port (PORT)")
	cmd.PersistentFlags().StringVar(&appConfig.APIKey, "api-key", appConfig.APIKey, "optional Bearer key for /v1/chat/completions (API_KEY)")
}

func (c *AppConfig) Validate() error {
	if c == nil {
		return fmt.Errorf("app config is nil")
	}
	if strings.TrimSpace(c.Host) == "" {
		return fmt.Errorf("host must not be empty")
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("invalid port %d: must be between 1 and 65535", c.Port)
	}
	return nil
}

// ParseConfig loads env defaults, parses CLI flags/subcommands, and returns the app config.
// It returns ErrHelpRequested when the user asked for help (cobra has already printed it).
// Callers should handle ShowVersion and process exit themselves.
func ParseConfig(displayName, shortName string) (*AppConfig, error) {
	appConfig := defaultAppConfig()

	short := displayName + " is an OpenAI-compatible HTTP proxy for ChatJimmy.\n" +
		"For more help, visit " + helpURL
	rootCmd := &cobra.Command{
		Use:   shortName,
		Short: short,
		Long: short + "\n" +
			"Running without a subcommand starts the HTTP server (same as '" + shortName + " serve').",
		Run: func(cmd *cobra.Command, args []string) {
			appConfig.Command = CommandServe
			appConfig.Args = args
		},
	}

	rootCmd.PersistentFlags().BoolVarP(&appConfig.Verbose, "verbose", "b", appConfig.Verbose, "enable verbose logs (VERBOSE)")
	rootCmd.Flags().BoolVarP(&appConfig.ShowVersion, "version", "v", appConfig.ShowVersion, "print version")

	applyServeFlags(appConfig, rootCmd)

	if err := loadEnvVars(appConfig); err != nil {
		return nil, err
	}

	rootCmd.AddCommand(
		versionCommand(appConfig),
		serveCommand(appConfig),
	)

	rootCmd.SetHelpCommand(&cobra.Command{
		Use:    "",
		Hidden: true,
	})

	cmd, err := rootCmd.ExecuteC()
	if err != nil {
		return nil, err
	}

	if commandHelpRequested(cmd) {
		return nil, ErrHelpRequested
	}

	if appConfig.Verbose {
		fmt.Fprintln(os.Stderr, "Verbose mode enabled")
	}

	return appConfig, nil
}

func commandHelpRequested(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		if helpFlag := c.Flags().Lookup("help"); helpFlag != nil && helpFlag.Changed {
			return true
		}
	}
	return false
}
