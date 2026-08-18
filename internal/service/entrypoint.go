package service

/*
Package service is the HTTP glue layer for the ChatJimmy OpenAI-compatible proxy.

Routing and request handling follow context/cj2api/src/proxy.js.
Upstream translation uses lib/chatjimmy for the HTTP client and response parsing.
*/

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/CoreUnit-NET/jimmy-gateway/internal/config"
	"github.com/CoreUnit-NET/jimmy-gateway/lib/chatjimmy"
)

const upstreamTimeout = 120 * time.Second

// RunOptions configures the HTTP proxy server started by Run.
type RunOptions struct {
	Logger      *log.Logger
	ShortName   string
	DisplayName string
	Version     string
	Config      *config.AppConfig
}

// Run starts the OpenAI-compatible ChatJimmy HTTP proxy until interrupted.
func Run(opts RunOptions) error {
	cfg := opts.Config
	if cfg == nil {
		return fmt.Errorf("app config is nil")
	}

	logger := opts.Logger
	if logger == nil {
		logger = log.New(os.Stdout, "", log.LstdFlags)
	}

	displayName := strings.TrimSpace(opts.DisplayName)
	if displayName == "" {
		displayName = strings.TrimSpace(opts.ShortName)
	}
	if displayName == "" {
		displayName = "jimmy-gateway"
	}

	client := &chatjimmy.Client{
		URL:     chatjimmy.DefaultUpstreamURL,
		Timeout: upstreamTimeout,
	}
	handler := NewHandler(logger, cfg, client)

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      upstreamTimeout + 10*time.Second,
		IdleTimeout:       60 * time.Second,
	}

	authMode := "off"
	if strings.TrimSpace(cfg.APIKey) != "" {
		authMode = "bearer"
	}

	version := strings.TrimSpace(opts.Version)
	if version == "" {
		version = "dev"
	}
	logger.Printf(
		"%s %s listening on http://%s (gateway auth: %s, upstream: %s)",
		displayName,
		version,
		addr,
		authMode,
		chatjimmy.DefaultUpstreamURL,
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		logger.Printf("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Printf("http shutdown: %v", err)
		}
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
