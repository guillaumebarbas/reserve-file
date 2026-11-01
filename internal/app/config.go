package app

import (
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
)

type Config struct {
	HTTPAddr     string
	LogLevel     slog.Level
	DBPath       string
	SeedAccounts map[string]int64
}

func loadConfig(getenv func(string) string) (Config, error) {
	config := Config{
		HTTPAddr:     ":8080",
		LogLevel:     slog.LevelInfo,
		DBPath:       "/data/reserve.db",
		SeedAccounts: map[string]int64{"acc_demo": 10737418240, "acc_small": 104857600},
	}

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

	if value := getenv("DB_PATH"); value != "" {
		config.DBPath = value
	}

	if value := getenv("SEED_ACCOUNTS"); value != "" {
		accounts, err := parseSeedAccounts(value)
		if err != nil {
			return Config{}, fmt.Errorf("SEED_ACCOUNTS: %w", err)
		}
		config.SeedAccounts = accounts
	}

	return config, nil
}

func parseSeedAccounts(value string) (map[string]int64, error) {
	accounts := make(map[string]int64)
	for _, entry := range strings.Split(value, ",") {
		id, limit, found := strings.Cut(entry, ":")
		id = strings.TrimSpace(id)
		if !found || id == "" {
			return nil, fmt.Errorf("entry %q: want <account id>:<limit in bytes>", entry)
		}
		limitBytes, err := strconv.ParseInt(strings.TrimSpace(limit), 10, 64)
		if err != nil || limitBytes < 0 {
			return nil, fmt.Errorf("entry %q: the limit must be a whole number of bytes, 0 or more", entry)
		}
		if _, listed := accounts[id]; listed {
			return nil, fmt.Errorf("account %q is listed twice", id)
		}
		accounts[id] = limitBytes
	}
	return accounts, nil
}
