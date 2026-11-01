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
	if got.HTTPAddr != ":8080" {
		t.Errorf("HTTPAddr: got %q, want %q", got.HTTPAddr, ":8080")
	}
	if got.LogLevel != slog.LevelInfo {
		t.Errorf("LogLevel: got %v, want %v", got.LogLevel, slog.LevelInfo)
	}
	if got.DBPath != "/data/reserve.db" {
		t.Errorf("DBPath: got %q, want %q", got.DBPath, "/data/reserve.db")
	}
	wantAccounts := map[string]int64{"acc_demo": 10737418240, "acc_small": 104857600}
	if !maps.Equal(got.SeedAccounts, wantAccounts) {
		t.Errorf("SeedAccounts: got %v, want %v", got.SeedAccounts, wantAccounts)
	}
}

func TestLoadConfig_SeedAccounts(t *testing.T) {
	environment := map[string]string{"SEED_ACCOUNTS": "acc_one:100, acc_two:200"}

	got, err := loadConfig(fakeGetenv(environment))

	if err != nil {
		t.Fatalf("loadConfig(%v): unexpected error: %v", environment, err)
	}
	want := map[string]int64{"acc_one": 100, "acc_two": 200}
	if !maps.Equal(got.SeedAccounts, want) {
		t.Errorf("loadConfig(%v): SeedAccounts: got %v, want %v", environment, got.SeedAccounts, want)
	}
}

func TestLoadConfig_InvalidValues(t *testing.T) {
	tests := []struct {
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
			environment:  map[string]string{"SEED_ACCOUNTS": "acc_demo"},
			wantVariable: "SEED_ACCOUNTS",
		},
		{
			name:         "seed_limit_not_a_number",
			environment:  map[string]string{"SEED_ACCOUNTS": "acc_demo:ten"},
			wantVariable: "SEED_ACCOUNTS",
		},
		{
			name:         "seed_negative_limit",
			environment:  map[string]string{"SEED_ACCOUNTS": "acc_demo:-1"},
			wantVariable: "SEED_ACCOUNTS",
		},
		{
			name:         "seed_duplicate_account",
			environment:  map[string]string{"SEED_ACCOUNTS": "acc_demo:1,acc_demo:2"},
			wantVariable: "SEED_ACCOUNTS",
		},
		{
			name:         "seed_empty_entry",
			environment:  map[string]string{"SEED_ACCOUNTS": "acc_demo:1,,acc_small:2"},
			wantVariable: "SEED_ACCOUNTS",
		},
		{
			name:         "seed_empty_account_id",
			environment:  map[string]string{"SEED_ACCOUNTS": ":100"},
			wantVariable: "SEED_ACCOUNTS",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := loadConfig(fakeGetenv(tc.environment))

			if err == nil {
				t.Fatalf("loadConfig(%v): got nil error, want an error", tc.environment)
			}
			if !strings.Contains(err.Error(), tc.wantVariable) {
				t.Errorf("loadConfig(%v): error %q does not name %s", tc.environment, err, tc.wantVariable)
			}
		})
	}
}

func fakeGetenv(environment map[string]string) func(string) string {
	return func(name string) string { return environment[name] }
}
