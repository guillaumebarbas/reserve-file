package limits

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

const maxPayloadBytes = 1 << 20

var (
	errPayloadNotObject         = errors.New("the limit payload is not a JSON object")
	errMaxParallelChunksInvalid = errors.New("MAX_PARALLEL_CHUNKS is missing or is not a JSON integer of 1 or more")
)

type limitFetcher struct {
	sourceURL string
	client    *http.Client
}

func newLimitFetcher(sourceURL string, timeout time.Duration) limitFetcher {
	return limitFetcher{sourceURL: sourceURL, client: &http.Client{Timeout: timeout}}
}

func (fetcher limitFetcher) fetchMaxParallelChunks(ctx context.Context) (int, error) {
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, fetcher.sourceURL, nil)
	response, err := fetcher.client.Do(request)
	if err != nil {
		return 0, fmt.Errorf("fetch the limit: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("fetch the limit: the source answered %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxPayloadBytes+1))
	if err != nil {
		return 0, fmt.Errorf("read the limit payload: %v", err)
	}
	if len(body) > maxPayloadBytes {
		return 0, fmt.Errorf("read the limit payload: larger than %d bytes", maxPayloadBytes)
	}
	return parseMaxParallelChunks(body)
}

func parseMaxParallelChunks(body []byte) (int, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil || fields == nil {
		return 0, errPayloadNotObject
	}
	var maxParallelChunks int
	if err := json.Unmarshal(fields["MAX_PARALLEL_CHUNKS"], &maxParallelChunks); err != nil || maxParallelChunks < 1 {
		return 0, errMaxParallelChunksInvalid
	}
	return maxParallelChunks, nil
}
