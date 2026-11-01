package handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidateIdempotencyKey_RejectsInvalidKeys(t *testing.T) {
	testCases := []struct {
		name           string
		idempotencyKey string
		wantDetail     string
	}{
		{"missing_or_empty", "", "The Idempotency-Key header is required. Generate one key per logical request and reuse it on every retry."},
		{"longer_than_255_characters", strings.Repeat("k", 256), "The Idempotency-Key header must not exceed 255 characters, got 256."},
		{"space", "upload 42", "The Idempotency-Key header may only contain visible ASCII characters, without spaces."},
		{"tab", "upload\t42", "The Idempotency-Key header may only contain visible ASCII characters, without spaces."},
		{"delete_character", "upload\x7f42", "The Idempotency-Key header may only contain visible ASCII characters, without spaces."},
		{"non_ascii", "clé-42", "The Idempotency-Key header may only contain visible ASCII characters, without spaces."},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			err := validateIdempotencyKey(testCase.idempotencyKey)

			var invalidInput invalidInputError
			if !errors.As(err, &invalidInput) {
				t.Fatalf("validateIdempotencyKey(%q): got error %v, want an invalid input error", testCase.idempotencyKey, err)
			}
			if invalidInput.problem.Code != "invalid_idempotency_key" || invalidInput.problem.Detail != testCase.wantDetail {
				t.Errorf("validateIdempotencyKey(%q): got %s %q, want invalid_idempotency_key %q", testCase.idempotencyKey, invalidInput.problem.Code, invalidInput.problem.Detail, testCase.wantDetail)
			}
		})
	}
}

func TestValidateIdempotencyKey_AcceptsVisibleASCII(t *testing.T) {
	var everyVisibleCharacter strings.Builder
	for character := byte('!'); character <= '~'; character++ {
		everyVisibleCharacter.WriteByte(character)
	}
	testCases := []struct {
		name           string
		idempotencyKey string
	}{
		{"one_character", "k"},
		{"exactly_255_characters", strings.Repeat("k", 255)},
		{"every_visible_character", everyVisibleCharacter.String()},
		{"random_uuid", "9b2f4c1e-5a43-4f7e-9d0a-2f7c1b8e4d21"},
		{"derived_from_the_upload", "file-42:chunk-7"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			err := validateIdempotencyKey(testCase.idempotencyKey)

			if err != nil {
				t.Errorf("validateIdempotencyKey(%q): got error %v, want nil", testCase.idempotencyKey, err)
			}
		})
	}
}

func TestReadReserveRequest_RejectsInvalidBytes(t *testing.T) {
	testCases := []struct {
		name       string
		body       string
		wantDetail string
	}{
		{"null", `{"bytes": null}`, "bytes is required: the number of bytes to reserve, as a positive integer."},
		{"decimal", `{"bytes": 5.5}`, "bytes must be a positive integer, got 5.5."},
		{"decimal_with_zero_fraction", `{"bytes": 5.0}`, "bytes must be a positive integer, got 5.0."},
		{"exponent", `{"bytes": 1e3}`, "bytes must be a positive integer, got 1e3."},
		{"string", `{"bytes": "5"}`, `bytes must be a positive integer, got "5".`},
		{"boolean", `{"bytes": true}`, "bytes must be a positive integer, got true."},
		{"above_the_largest_integer", `{"bytes": 9223372036854775808}`, "bytes must be a positive integer of at most 9223372036854775807, got 9223372036854775808."},
		{"below_the_smallest_integer", `{"bytes": -9223372036854775809}`, "bytes must be a positive integer of at most 9223372036854775807, got -9223372036854775809."},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			request := newReserveRequest("key_1", testCase.body)

			_, _, err := readReserveRequest(httptest.NewRecorder(), request)

			var invalidInput invalidInputError
			if !errors.As(err, &invalidInput) {
				t.Fatalf("readReserveRequest(%s): got error %v, want an invalid input error", testCase.body, err)
			}
			if invalidInput.problem.Code != "invalid_request" || invalidInput.problem.Detail != testCase.wantDetail {
				t.Errorf("readReserveRequest(%s): got %s %q, want invalid_request %q", testCase.body, invalidInput.problem.Code, invalidInput.problem.Detail, testCase.wantDetail)
			}
		})
	}
}

func TestReadReserveRequest_AcceptsPositiveIntegers(t *testing.T) {
	testCases := []struct {
		name      string
		body      string
		wantBytes int64
	}{
		{"one_byte", `{"bytes": 1}`, 1},
		{"largest_integer", `{"bytes": 9223372036854775807}`, 9223372036854775807},
		{"spaces_around_the_value", `{ "bytes" :  4194304 }`, 4194304},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			request := newReserveRequest("key_1", testCase.body)

			idempotencyKey, bytes, err := readReserveRequest(httptest.NewRecorder(), request)

			if err != nil {
				t.Fatalf("readReserveRequest(%s): got error %v, want nil", testCase.body, err)
			}
			if idempotencyKey != "key_1" || bytes != testCase.wantBytes {
				t.Errorf("readReserveRequest(%s): got key %q and %d bytes, want key_1 and %d", testCase.body, idempotencyKey, bytes, testCase.wantBytes)
			}
		})
	}
}

func TestAPI_ReserveValidation(t *testing.T) {
	validBody := `{"bytes": 1048576}`
	validKey := http.Header{"Idempotency-Key": {"key_1"}}
	testCases := []struct {
		name       string
		header     http.Header
		body       string
		wantStatus int
		wantCode   string
	}{
		{"key_longer_than_255_characters", http.Header{"Idempotency-Key": {strings.Repeat("k", 256)}}, validBody, http.StatusBadRequest, "invalid_idempotency_key"},
		{"key_with_a_space", http.Header{"Idempotency-Key": {"upload 42"}}, validBody, http.StatusBadRequest, "invalid_idempotency_key"},
		{"key_not_ascii", http.Header{"Idempotency-Key": {"clé-42"}}, validBody, http.StatusBadRequest, "invalid_idempotency_key"},
		{"decimal_bytes", validKey, `{"bytes": 5.5}`, http.StatusBadRequest, "invalid_request"},
		{"exponent_bytes", validKey, `{"bytes": 1e3}`, http.StatusBadRequest, "invalid_request"},
		{"string_bytes", validKey, `{"bytes": "5"}`, http.StatusBadRequest, "invalid_request"},
		{"bytes_above_the_largest_integer", validKey, `{"bytes": 9223372036854775808}`, http.StatusBadRequest, "invalid_request"},
		{"unknown_field", validKey, `{"bytes": 5, "size": 5}`, http.StatusBadRequest, "invalid_request"},
		{"trailing_data", validKey, `{"bytes": 5} {"bytes": 5}`, http.StatusBadRequest, "invalid_request"},
		{"body_over_4_kib", validKey, `{"bytes": 5` + strings.Repeat(" ", 4096) + `}`, http.StatusRequestEntityTooLarge, "request_too_large"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			routes := newRoutesWithAccounts(t, map[string]int64{"small_account": 104857600}, fakeLimitProvider{maxActive: 5, known: true})

			recorder := postReservation(routes, "small_account", testCase.header, testCase.body)

			body := decodeObject(t, recorder)
			if recorder.Code != testCase.wantStatus || body["code"] != testCase.wantCode {
				t.Errorf("got %d %v, want %d %s", recorder.Code, body["code"], testCase.wantStatus, testCase.wantCode)
			}
			assertUsage(t, routes, "small_account", 0, 0)
		})
	}
}

func TestAPI_ReserveStoresNothingForAnInvalidRequest(t *testing.T) {
	routes := newRoutesWithAccounts(t, map[string]int64{"small_account": 104857600}, fakeLimitProvider{maxActive: 5, known: true})
	invalidRecorder := postReservation(routes, "small_account", http.Header{"Idempotency-Key": {"key_1"}}, `{"bytes": 5.5}`)

	validRecorder := reserve(routes, "small_account", "key_1", 1048576)

	if invalidRecorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid request: got %d, want %d", invalidRecorder.Code, http.StatusBadRequest)
	}
	if validRecorder.Code != http.StatusCreated {
		t.Errorf("valid request with the same key after the invalid one: got %d %s, want %d: the key must be unused", validRecorder.Code, validRecorder.Body.String(), http.StatusCreated)
	}
	assertUsage(t, routes, "small_account", 1048576, 1)
}

func newReserveRequest(idempotencyKey, body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/accounts/small_account/reservations", strings.NewReader(body))
	request.Header.Set("Idempotency-Key", idempotencyKey)
	return request
}
