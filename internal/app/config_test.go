package app

import (
	"log/slog"
	"strings"
	"testing"
)

func TestLoadConfig_Defaults(t *testing.T) {
	noVariables := func(name string) string { return "" }

	got, err := loadConfig(noVariables)

	if err != nil {
		t.Fatalf("loadConfig: unexpected error: %v", err)
	}
	want := Config{HTTPAddr: ":8080", LogLevel: slog.LevelInfo}
	if got != want {
		t.Errorf("config: got %+v, want %+v", got, want)
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
