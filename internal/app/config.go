package app

import (
	"fmt"
	"log/slog"
	"net"
)

// Config is the service configuration, read from environment variables.
type Config struct {
	HTTPAddr string
	LogLevel slog.Level
}

func loadConfig(getenv func(string) string) (Config, error) {
	config := Config{HTTPAddr: ":8080", LogLevel: slog.LevelInfo}

	if value := getenv("HTTP_ADDR"); value != "" {
		config.HTTPAddr = value
	}
	if _, _, err := net.SplitHostPort(config.HTTPAddr); err != nil {
		return Config{}, fmt.Errorf("HTTP_ADDR: %w", err)
	}

	if value := getenv("LOG_LEVEL"); value != "" {
		if err := config.LogLevel.UnmarshalText([]byte(value)); err != nil {
			return Config{}, fmt.Errorf("LOG_LEVEL: %w", err)
		}
	}

	return config, nil
}
