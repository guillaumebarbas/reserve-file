package app

import (
	"log/slog"
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/guillaumebarbas/reserve-file/internal/limits"
)

func TestLoadConfig_Defaults(t *testing.T) {
	noVariables := func(name string) string { return "" }

	got, err := loadConfig(noVariables)

	if err != nil {
		t.Fatalf("loadConfig: unexpected error: %v", err)
	}
	if got.HTTPAddress != ":8080" {
		t.Errorf("HTTPAddress: got %q, want %q", got.HTTPAddress, ":8080")
	}
	if got.LogLevel != slog.LevelInfo {
		t.Errorf("LogLevel: got %v, want %v", got.LogLevel, slog.LevelInfo)
	}
	if got.DatabasePath != "/data/reserve.db" {
		t.Errorf("DatabasePath: got %q, want %q", got.DatabasePath, "/data/reserve.db")
	}
	wantAccounts := map[string]int64{"large_account": 10737418240, "small_account": 104857600}
	if !maps.Equal(got.SeedAccounts, wantAccounts) {
		t.Errorf("SeedAccounts: got %v, want %v", got.SeedAccounts, wantAccounts)
	}
}

func TestLoadConfig_SeedAccounts(t *testing.T) {
	environment := map[string]string{"SEED_ACCOUNTS": "first_account:100, second_account : 200"}

	got, err := loadConfig(fakeEnvironment(environment))

	if err != nil {
		t.Fatalf("loadConfig(%v): unexpected error: %v", environment, err)
	}
	want := map[string]int64{"first_account": 100, "second_account": 200}
	if !maps.Equal(got.SeedAccounts, want) {
		t.Errorf("loadConfig(%v): SeedAccounts: got %v, want %v", environment, got.SeedAccounts, want)
	}
}

func TestLoadConfig_EnvConfig(t *testing.T) {
	testCases := []struct {
		name        string
		environment map[string]string
		want        limits.Config
	}{
		{
			name:        "defaults",
			environment: map[string]string{},
			want:        limits.Config{SourceURL: "https://env.hivenet.com/dev", FetchTimeout: 3 * time.Second, RefreshInterval: 60 * time.Second},
		},
		{
			name:        "overrides",
			environment: map[string]string{"ENV_CONFIG_URL": "http://localhost:9000/staging", "ENV_CONFIG_TIMEOUT": "500ms", "ENV_CONFIG_REFRESH": "5m"},
			want:        limits.Config{SourceURL: "http://localhost:9000/staging", FetchTimeout: 500 * time.Millisecond, RefreshInterval: 5 * time.Minute},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := loadConfig(fakeEnvironment(testCase.environment))

			if err != nil {
				t.Fatalf("loadConfig(%v): unexpected error: %v", testCase.environment, err)
			}
			if got.LimitSource != testCase.want {
				t.Errorf("loadConfig(%v): LimitSource: got %+v, want %+v", testCase.environment, got.LimitSource, testCase.want)
			}
		})
	}
}

func TestLoadConfig_InvalidValues(t *testing.T) {
	testCases := []struct {
		name         string
		environment  map[string]string
		wantVariable string
	}{
		{
			name:         "unknown_log_level",
			environment:  map[string]string{"LOG_LEVEL": "verbose"},
			wantVariable: "LOG_LEVEL",
		},
		{
			name:         "address_without_port",
			environment:  map[string]string{"HTTP_ADDR": "localhost"},
			wantVariable: "HTTP_ADDR",
		},
		{
			name:         "seed_entry_without_colon",
			environment:  map[string]string{"SEED_ACCOUNTS": "large_account"},
			wantVariable: "SEED_ACCOUNTS",
		},
		{
			name:         "seed_limit_not_a_number",
			environment:  map[string]string{"SEED_ACCOUNTS": "large_account:ten"},
			wantVariable: "SEED_ACCOUNTS",
		},
		{
			name:         "seed_negative_limit",
			environment:  map[string]string{"SEED_ACCOUNTS": "large_account:-1"},
			wantVariable: "SEED_ACCOUNTS",
		},
		{
			name:         "seed_duplicate_account",
			environment:  map[string]string{"SEED_ACCOUNTS": "large_account:1,large_account:2"},
			wantVariable: "SEED_ACCOUNTS",
		},
		{
			name:         "seed_empty_entry",
			environment:  map[string]string{"SEED_ACCOUNTS": "large_account:1,,small_account:2"},
			wantVariable: "SEED_ACCOUNTS",
		},
		{
			name:         "seed_empty_account_id",
			environment:  map[string]string{"SEED_ACCOUNTS": ":100"},
			wantVariable: "SEED_ACCOUNTS",
		},
		{
			name:         "limit_source_url_relative",
			environment:  map[string]string{"ENV_CONFIG_URL": "env.hivenet.com/dev"},
			wantVariable: "ENV_CONFIG_URL",
		},
		{
			name:         "limit_source_url_not_http",
			environment:  map[string]string{"ENV_CONFIG_URL": "ftp://env.hivenet.com/dev"},
			wantVariable: "ENV_CONFIG_URL",
		},
		{
			name:         "limit_source_url_unparsable",
			environment:  map[string]string{"ENV_CONFIG_URL": "http://[::1"},
			wantVariable: "ENV_CONFIG_URL",
		},
		{
			name:         "limit_refresh_not_a_duration",
			environment:  map[string]string{"ENV_CONFIG_REFRESH": "sixty"},
			wantVariable: "ENV_CONFIG_REFRESH",
		},
		{
			name:         "limit_refresh_zero",
			environment:  map[string]string{"ENV_CONFIG_REFRESH": "0s"},
			wantVariable: "ENV_CONFIG_REFRESH",
		},
		{
			name:         "limit_timeout_without_unit",
			environment:  map[string]string{"ENV_CONFIG_TIMEOUT": "3"},
			wantVariable: "ENV_CONFIG_TIMEOUT",
		},
		{
			name:         "limit_timeout_negative",
			environment:  map[string]string{"ENV_CONFIG_TIMEOUT": "-1s"},
			wantVariable: "ENV_CONFIG_TIMEOUT",
		},
		{
			name:         "cors_origin_without_scheme",
			environment:  map[string]string{"CORS_ALLOWED_ORIGIN": "localhost:8081"},
			wantVariable: "CORS_ALLOWED_ORIGIN",
		},
		{
			name:         "cors_origin_with_path",
			environment:  map[string]string{"CORS_ALLOWED_ORIGIN": "http://localhost:8081/docs"},
			wantVariable: "CORS_ALLOWED_ORIGIN",
		},
		{
			name:         "cors_origin_with_trailing_slash",
			environment:  map[string]string{"CORS_ALLOWED_ORIGIN": "http://localhost:8081/"},
			wantVariable: "CORS_ALLOWED_ORIGIN",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := loadConfig(fakeEnvironment(testCase.environment))

			if err == nil {
				t.Fatalf("loadConfig(%v): got nil error, want an error", testCase.environment)
			}
			if !strings.Contains(err.Error(), testCase.wantVariable) {
				t.Errorf("loadConfig(%v): error %q does not name %s", testCase.environment, err, testCase.wantVariable)
			}
		})
	}
}

func TestLoadConfig_CORSAllowedOrigin(t *testing.T) {
	testCases := []struct {
		name        string
		environment map[string]string
		want        string
	}{
		{name: "off_by_default", environment: map[string]string{}, want: ""},
		{name: "swagger_ui_origin", environment: map[string]string{"CORS_ALLOWED_ORIGIN": "http://localhost:8081"}, want: "http://localhost:8081"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := loadConfig(fakeEnvironment(testCase.environment))

			if err != nil {
				t.Fatalf("loadConfig(%v): unexpected error: %v", testCase.environment, err)
			}
			if got.CORSAllowedOrigin != testCase.want {
				t.Errorf("loadConfig(%v): CORSAllowedOrigin: got %q, want %q", testCase.environment, got.CORSAllowedOrigin, testCase.want)
			}
		})
	}
}

func fakeEnvironment(variables map[string]string) func(string) string {
	return func(name string) string { return variables[name] }
}
