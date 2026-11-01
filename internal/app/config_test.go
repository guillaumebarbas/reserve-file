package app

import (
	"log/slog"
	"maps"
	"strings"
	"testing"
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

func fakeEnvironment(variables map[string]string) func(string) string {
	return func(name string) string { return variables[name] }
}
