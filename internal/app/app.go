// Package app reads the configuration, wires the service together and runs it.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/guillaumebarbas/reserve-file/internal/handler"
)

const shutdownTimeout = 10 * time.Second

// Run starts the service and blocks until ctx is cancelled, then shuts the
// HTTP server down gracefully. It returns an error if the service cannot
// start, stops on its own, or does not finish its requests in time.
//
// With "healthcheck" as first argument, Run instead checks that the instance
// listening on HTTP_ADDR answers /healthz, and returns an error if it does not.
func Run(ctx context.Context, args []string, getenv func(string) string, stdout io.Writer) error {
	config, err := loadConfig(getenv)
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	if len(args) > 0 && args[0] == "healthcheck" {
		return healthcheck(config.HTTPAddr)
	}
	logger := slog.New(slog.NewJSONHandler(stdout, &slog.HandlerOptions{Level: config.LogLevel}))
	return serve(ctx, config.HTTPAddr, logger)
}

func serve(ctx context.Context, httpAddr string, logger *slog.Logger) error {
	listener, err := net.Listen("tcp", httpAddr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", httpAddr, err)
	}
	server := newServer(handler.New())

	// If Serve stops on its own, cancelling ctx shuts everything down as a
	// signal would.
	ctx, stop := context.WithCancel(ctx)
	defer stop()
	var serveErr error
	var wg sync.WaitGroup
	wg.Go(func() {
		serveErr = server.Serve(listener)
		stop()
	})
	logger.Info("server started", "addr", listener.Addr().String())

	<-ctx.Done()
	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	shutdownErr := server.Shutdown(shutdownCtx)
	wg.Wait()
	logger.Info("server stopped")

	if errors.Is(serveErr, http.ErrServerClosed) {
		serveErr = nil
	}
	return errors.Join(serveErr, shutdownErr)
}

func healthcheck(httpAddr string) error {
	// loadConfig has already checked that the address splits into host and port.
	_, port, _ := net.SplitHostPort(httpAddr)
	client := &http.Client{Timeout: 2 * time.Second}

	// Docker runs the check inside the container, next to the server, so the
	// loopback address always reaches it.
	response, err := client.Get("http://" + net.JoinHostPort("127.0.0.1", port) + "/healthz")
	if err != nil {
		return fmt.Errorf("healthcheck: %w", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("healthcheck: /healthz answered %d", response.StatusCode)
	}
	return nil
}

func newServer(routes http.Handler) *http.Server {
	return &http.Server{
		Handler:           routes,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		// Longer than SQLite's 5 s busy timeout, so that a 503 sent after a
		// lock wait still reaches the client.
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}
