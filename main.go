package main

/*
jimmy-gateway entrypoint.

Load .env, parse config, convert to settings, dispatch CLI commands,
then start the ChatJimmy OpenAI-compatible HTTP proxy when serving.
*/

import (
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/CoreUnit-NET/jimmy-gateway/internal/config"
	"github.com/CoreUnit-NET/jimmy-gateway/internal/service"
	"github.com/CoreUnit-NET/jimmy-gateway/internal/settings"
	"github.com/joho/godotenv"
)

var DisplayName string = "Unset"
var ShortName string = "unset"
var Version string = "?.?.?"
var Commit string = "???????"

func main() {
	os.Exit(run())
}

func run() int {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "warning: .env: %v\n", err)
	}

	logger := newLogger(false)

	cfg, err := config.ParseConfig(DisplayName, ShortName)
	if errors.Is(err, config.ErrHelpRequested) {
		return 0
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	if cfg.ShowVersion || cfg.Command == config.CommandVersion {
		printVersion()
		return 0
	}

	s, err := settings.FromAppConfig(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	if s.Verbose {
		logger = newLogger(true)
	}

	switch s.Command {
	case config.CommandServe, "":
		return runServe(logger, s)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", s.Command)
		return 1
	}
}

func newLogger(verbose bool) *log.Logger {
	flags := log.LstdFlags
	if verbose {
		flags |= log.Lmicroseconds
	}
	return log.New(os.Stdout, "", flags)
}

func printVersion() {
	fmt.Printf("%s version %s, build %s\n", DisplayName, Version, Commit)
}

func runServe(logger *log.Logger, s *settings.Settings) int {
	if err := service.Run(service.RunOptions{
		Logger:      logger,
		ShortName:   ShortName,
		DisplayName: DisplayName,
		Version:     Version,
		Settings:    s,
	}); err != nil {
		logger.Printf("error: %v", err)
		return 1
	}
	return 0
}
