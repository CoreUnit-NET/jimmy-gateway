package main

/*
jimmy-gateway entrypoint.

Load .env, parse config, dispatch CLI commands, then start the ChatJimmy
OpenAI-compatible HTTP proxy when serving.
*/

import (
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/CoreUnit-NET/jimmy-gateway/internal/config"
	"github.com/CoreUnit-NET/jimmy-gateway/internal/service"
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

	if cfg.ShowVersion {
		printVersion()
		return 0
	}

	if cfg.Verbose {
		logger = newLogger(true)
	}

	switch cfg.Command {
	case config.CommandServe:
		return runServe(logger, cfg)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", cfg.Command)
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

func runServe(logger *log.Logger, cfg *config.AppConfig) int {
	if err := cfg.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	if err := service.Run(service.RunOptions{
		Logger:      logger,
		ShortName:   ShortName,
		DisplayName: DisplayName,
		Version:     Version,
		Config:      cfg,
	}); err != nil {
		logger.Printf("error: %v", err)
		return 1
	}
	return 0
}
