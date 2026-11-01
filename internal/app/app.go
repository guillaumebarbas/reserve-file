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
	"github.com/guillaumebarbas/reserve-file/internal/repository/sqlite"
)

const shutdownTimeout = 10 * time.Second

func Run(ctx context.Context, args []string, getenv func(string) string, stdout io.Writer) error {
	config, err := loadConfig(getenv)
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	if len(args) > 0 && args[0] == "healthcheck" {
		return healthcheck(config.HTTPAddr)
	}
	logger := slog.New(slog.NewJSONHandler(stdout, &slog.HandlerOptions{Level: config.LogLevel}))
	store, err := sqlite.Open(ctx, config.DBPath, config.SeedAccounts)
	if err != nil {
		return err
	}
	serveErr := serve(ctx, config.HTTPAddr, logger)
	return errors.Join(serveErr, store.Close())
}

func serve(ctx context.Context, httpAddr string, logger *slog.Logger) error {
	listener, err := net.Listen("tcp", httpAddr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", httpAddr, err)
	}
	server := newServer(handler.New())

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
	host, port, _ := net.SplitHostPort(httpAddr)
	listensOnEveryInterface := host == "" || net.ParseIP(host).IsUnspecified()
	if listensOnEveryInterface {
		host = "127.0.0.1"
	}
	client := &http.Client{Timeout: 2 * time.Second}

	response, err := client.Get("http://" + net.JoinHostPort(host, port) + "/healthz")
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
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}
