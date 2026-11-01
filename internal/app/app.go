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
	"github.com/guillaumebarbas/reserve-file/internal/limits"
	"github.com/guillaumebarbas/reserve-file/internal/repository/sqlite"
)

const shutdownTimeout = 10 * time.Second

func Run(ctx context.Context, arguments []string, getEnvironmentVariable func(string) string, logOutput io.Writer) error {
	config, err := loadConfig(getEnvironmentVariable)
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	if len(arguments) > 0 && arguments[0] == "healthcheck" {
		return checkHealth(config.HTTPAddress)
	}
	logger := slog.New(slog.NewJSONHandler(logOutput, &slog.HandlerOptions{Level: config.LogLevel}))
	store, err := sqlite.Open(ctx, config.DatabasePath, config.SeedAccounts)
	if err != nil {
		return err
	}
	limitCache := limits.New(config.LimitSource, store, logger)
	limitCache.Init(ctx)
	refresherCtx, stopRefresher := context.WithCancel(ctx)
	var refreshing sync.WaitGroup
	refreshing.Go(func() { limitCache.Run(refresherCtx) })
	routes := handler.New(logger, store, limitCache)
	serveErr := serveUntilCancelled(ctx, config.HTTPAddress, routes, logger)
	stopRefresher()
	refreshing.Wait()
	return errors.Join(serveErr, store.Close())
}

func serveUntilCancelled(ctx context.Context, httpAddress string, routes http.Handler, logger *slog.Logger) error {
	listener, err := net.Listen("tcp", httpAddress)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", httpAddress, err)
	}
	server := newHTTPServer(routes)

	ctx, stop := context.WithCancel(ctx)
	defer stop()
	var serveErr error
	var serving sync.WaitGroup
	serving.Go(func() {
		serveErr = server.Serve(listener)
		stop()
	})
	logger.Info("server started", "address", listener.Addr().String())

	<-ctx.Done()
	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	shutdownErr := server.Shutdown(shutdownCtx)
	serving.Wait()
	logger.Info("server stopped")

	if errors.Is(serveErr, http.ErrServerClosed) {
		serveErr = nil
	}
	return errors.Join(serveErr, shutdownErr)
}

func checkHealth(httpAddress string) error {
	host, port, _ := net.SplitHostPort(httpAddress)
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

func newHTTPServer(routes http.Handler) *http.Server {
	return &http.Server{
		Handler:           routes,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}
