package limits

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseMaxParallelChunks(t *testing.T) {
	testCases := []struct {
		name         string
		body         string
		want         int
		wantAccepted bool
	}{
		{"integer", `{"MAX_PARALLEL_CHUNKS": 5}`, 5, true},
		{"string", `{"MAX_PARALLEL_CHUNKS": "5"}`, 0, false},
		{"decimal", `{"MAX_PARALLEL_CHUNKS": 5.5}`, 0, false},
		{"zero", `{"MAX_PARALLEL_CHUNKS": 0}`, 0, false},
		{"negative", `{"MAX_PARALLEL_CHUNKS": -1}`, 0, false},
		{"null", `{"MAX_PARALLEL_CHUNKS": null}`, 0, false},
		{"missing_key", `{"OTHER_SETTING": 5}`, 0, false},
		{"json_array", `[5]`, 0, false},
		{"malformed_json", `{"MAX_PARALLEL_CHUNKS": `, 0, false},
		{"null_body", `null`, 0, false},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := parseMaxParallelChunks([]byte(testCase.body))

			accepted := err == nil
			if got != testCase.want || accepted != testCase.wantAccepted {
				t.Errorf("parseMaxParallelChunks(%s): got %d, accepted %t (error %v), want %d, accepted %t",
					testCase.body, got, accepted, err, testCase.want, testCase.wantAccepted)
			}
		})
	}
}

const realisticPayload = `{
	"API_URL": "https://api.example.test",
	"ANALYTICS_TOKEN": "fake-token-for-tests",
	"MAINTENANCE": false,
	"UPLOAD": {"CHUNK_SIZE": 4194304, "RETRIES": 3},
	"MAX_PARALLEL_CHUNKS": 5
}`

func TestFetcher_ParsesValue(t *testing.T) {
	source := newSource(t, http.StatusOK, realisticPayload)
	fetcher := newLimitFetcher(source.URL, time.Second)

	got, err := fetcher.fetchMaxParallelChunks(t.Context())

	if err != nil || got != 5 {
		t.Errorf("fetchMaxParallelChunks: got %d, error %v, want 5 and no error", got, err)
	}
}

func TestFetcher_RejectsInvalidResponses(t *testing.T) {
	testCases := []struct {
		name   string
		status int
		body   string
	}{
		{"server_error", http.StatusInternalServerError, `{"MAX_PARALLEL_CHUNKS": 5}`},
		{"not_found", http.StatusNotFound, `{"MAX_PARALLEL_CHUNKS": 5}`},
		{"body_over_1_mib", http.StatusOK, `{"MAX_PARALLEL_CHUNKS": 5, "PADDING": "` + strings.Repeat("x", 1<<20) + `"}`},
		{"invalid_value", http.StatusOK, `{"MAX_PARALLEL_CHUNKS": 0}`},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			source := newSource(t, testCase.status, testCase.body)
			fetcher := newLimitFetcher(source.URL, time.Second)

			got, err := fetcher.fetchMaxParallelChunks(t.Context())

			if err == nil || got != 0 {
				t.Errorf("fetchMaxParallelChunks with status %d: got %d, error %v, want 0 and an error", testCase.status, got, err)
			}
		})
	}
}

func TestFetcher_TimesOut(t *testing.T) {
	source := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
	}))
	defer source.Close()
	fetcher := newLimitFetcher(source.URL, 50*time.Millisecond)

	got, err := fetcher.fetchMaxParallelChunks(t.Context())

	if err == nil || got != 0 {
		t.Errorf("fetchMaxParallelChunks from a source that never answers: got %d, error %v, want 0 and an error", got, err)
	}
}

func TestFetcher_FailsOnTruncatedBody(t *testing.T) {
	source := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Length", "100")
		_, _ = writer.Write([]byte(`{"MAX_PARALLEL_CHUNKS": 5`))
	}))
	defer source.Close()
	fetcher := newLimitFetcher(source.URL, time.Second)

	got, err := fetcher.fetchMaxParallelChunks(t.Context())

	if err == nil || got != 0 {
		t.Errorf("fetchMaxParallelChunks of a truncated body: got %d, error %v, want 0 and an error", got, err)
	}
}

func newSource(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	source := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(status)
		_, _ = writer.Write([]byte(body))
	}))
	t.Cleanup(source.Close)
	return source
}
