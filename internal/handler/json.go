package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

const (
	jsonContentType     = "application/json"
	maxRequestBodyBytes = 4096
)

func (handlers *apiHandlers) writeJSON(ctx context.Context, writer http.ResponseWriter, status int, contentType string, body any) {
	writer.Header().Set("Content-Type", contentType)
	writer.WriteHeader(status)
	if err := json.NewEncoder(writer).Encode(body); err != nil {
		handlers.logger.Warn("response not written", "request_id", requestIDFrom(ctx), "error", err)
	}
}

func decodeJSON(writer http.ResponseWriter, request *http.Request, destination any) error {
	request.Body = http.MaxBytesReader(writer, request.Body, maxRequestBodyBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return invalidBodyError(err)
	}
	var trailingData json.RawMessage
	if err := decoder.Decode(&trailingData); !errors.Is(err, io.EOF) {
		return invalidBodyError(err)
	}
	return nil
}

func invalidBodyError(decodeErr error) error {
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(decodeErr, &tooLarge):
		return requestTooLarge()
	case errors.Is(decodeErr, io.EOF):
		return invalidRequest("The request body is empty. Send a JSON object.")
	default:
		return invalidRequest("The request body must be a single JSON object, without unknown fields.")
	}
}
