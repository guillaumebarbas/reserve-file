package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
)

const maxIdempotencyKeyLength = 255

func validateIdempotencyKey(idempotencyKey string) error {
	if idempotencyKey == "" {
		return invalidIdempotencyKey("The Idempotency-Key header is required. Generate one key per logical request and reuse it on every retry.")
	}
	if !isVisibleASCII(idempotencyKey) {
		return invalidIdempotencyKey("The Idempotency-Key header may only contain visible ASCII characters, without spaces.")
	}
	if len(idempotencyKey) > maxIdempotencyKeyLength {
		return invalidIdempotencyKey(fmt.Sprintf("The Idempotency-Key header must not exceed %d characters, got %d.", maxIdempotencyKeyLength, len(idempotencyKey)))
	}
	return nil
}

func parseBytes(rawBytes json.RawMessage) (int64, error) {
	if rawBytes == nil || string(rawBytes) == "null" {
		return 0, invalidRequest("bytes is required: the number of bytes to reserve, as a positive integer.")
	}
	bytes, err := strconv.ParseInt(string(rawBytes), 10, 64)
	if errors.Is(err, strconv.ErrRange) {
		return 0, invalidRequest(fmt.Sprintf("bytes must be a positive integer of at most %d, got %s.", int64(math.MaxInt64), rawBytes))
	}
	if err != nil || bytes <= 0 {
		return 0, invalidRequest(fmt.Sprintf("bytes must be a positive integer, got %s.", rawBytes))
	}
	return bytes, nil
}

func isVisibleASCII(text string) bool {
	for index := range len(text) {
		if text[index] < '!' || text[index] > '~' {
			return false
		}
	}
	return true
}
