package app

import (
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/guillaumebarbas/reserve-file/internal/limits"
)

type Config struct {
	HTTPAddress  string
	LogLevel     slog.Level
	DatabasePath string
	SeedAccounts map[string]int64
	LimitSource  limits.Config
}

func loadConfig(getEnvironmentVariable func(string) string) (Config, error) {
	config := Config{
		HTTPAddress:  ":8080",
		LogLevel:     slog.LevelInfo,
		DatabasePath: "/data/reserve.db",
		SeedAccounts: map[string]int64{"large_account": 10737418240, "small_account": 104857600},
	}

	if httpAddress := getEnvironmentVariable("HTTP_ADDR"); httpAddress != "" {
		config.HTTPAddress = httpAddress
	}
	if _, _, err := net.SplitHostPort(config.HTTPAddress); err != nil {
		return Config{}, fmt.Errorf("HTTP_ADDR: %w", err)
	}

	if logLevel := getEnvironmentVariable("LOG_LEVEL"); logLevel != "" {
		if err := config.LogLevel.UnmarshalText([]byte(logLevel)); err != nil {
			return Config{}, fmt.Errorf("LOG_LEVEL: %w", err)
		}
	}

	if databasePath := getEnvironmentVariable("DB_PATH"); databasePath != "" {
		config.DatabasePath = databasePath
	}

	if seedAccounts := getEnvironmentVariable("SEED_ACCOUNTS"); seedAccounts != "" {
		accounts, err := parseSeedAccounts(seedAccounts)
		if err != nil {
			return Config{}, fmt.Errorf("SEED_ACCOUNTS: %w", err)
		}
		config.SeedAccounts = accounts
	}

	limitSource, err := loadLimitSourceConfig(getEnvironmentVariable)
	if err != nil {
		return Config{}, err
	}
	config.LimitSource = limitSource

	return config, nil
}

func loadLimitSourceConfig(getEnvironmentVariable func(string) string) (limits.Config, error) {
	limitSource := limits.Config{
		SourceURL:       "https://env.hivenet.com/dev",
		FetchTimeout:    3 * time.Second,
		RefreshInterval: 60 * time.Second,
	}
	if sourceURL := getEnvironmentVariable("ENV_CONFIG_URL"); sourceURL != "" {
		if !isAbsoluteHTTPURL(sourceURL) {
			return limits.Config{}, fmt.Errorf("ENV_CONFIG_URL: %q is not an absolute http or https URL", sourceURL)
		}
		limitSource.SourceURL = sourceURL
	}
	if refreshInterval := getEnvironmentVariable("ENV_CONFIG_REFRESH"); refreshInterval != "" {
		duration, err := parsePositiveDuration(refreshInterval)
		if err != nil {
			return limits.Config{}, fmt.Errorf("ENV_CONFIG_REFRESH: %w", err)
		}
		limitSource.RefreshInterval = duration
	}
	if fetchTimeout := getEnvironmentVariable("ENV_CONFIG_TIMEOUT"); fetchTimeout != "" {
		duration, err := parsePositiveDuration(fetchTimeout)
		if err != nil {
			return limits.Config{}, fmt.Errorf("ENV_CONFIG_TIMEOUT: %w", err)
		}
		limitSource.FetchTimeout = duration
	}
	return limitSource, nil
}

func isAbsoluteHTTPURL(text string) bool {
	parsedURL, err := url.Parse(text)
	return err == nil && (parsedURL.Scheme == "http" || parsedURL.Scheme == "https") && parsedURL.Host != ""
}

func parsePositiveDuration(text string) (time.Duration, error) {
	duration, err := time.ParseDuration(text)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("%q is not a positive duration such as 60s", text)
	}
	return duration, nil
}

func parseSeedAccounts(seedAccounts string) (map[string]int64, error) {
	accounts := make(map[string]int64)
	for _, entry := range strings.Split(seedAccounts, ",") {
		accountID, limitText, found := strings.Cut(entry, ":")
		accountID = strings.TrimSpace(accountID)
		if !found || accountID == "" {
			return nil, fmt.Errorf("entry %q: want <account id>:<limit in bytes>", entry)
		}
		limitBytes, err := strconv.ParseInt(strings.TrimSpace(limitText), 10, 64)
		if err != nil || limitBytes < 0 {
			return nil, fmt.Errorf("entry %q: the limit must be a whole number of bytes, 0 or more", entry)
		}
		if _, listed := accounts[accountID]; listed {
			return nil, fmt.Errorf("account %q is listed twice", accountID)
		}
		accounts[accountID] = limitBytes
	}
	return accounts, nil
}
