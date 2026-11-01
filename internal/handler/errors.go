package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/guillaumebarbas/reserve-file/internal/domain"
)

const (
	kibibyte = 1024
	mebibyte = 1024 * kibibyte
	gibibyte = 1024 * mebibyte
)

const (
	problemContentType = "application/problem+json"
	problemTypeBaseURL = "https://github.com/guillaumebarbas/reserve-file/blob/main/docs/api.md#"
)

type problemBody struct {
	Type                  string `json:"type"`
	Title                 string `json:"title"`
	Status                int    `json:"status"`
	Code                  string `json:"code"`
	Detail                string `json:"detail"`
	RequestedBytes        *int64 `json:"requested_bytes,omitempty"`
	AvailableBytes        *int64 `json:"available_bytes,omitempty"`
	LimitBytes            *int64 `json:"limit_bytes,omitempty"`
	ActiveReservations    *int   `json:"active_reservations,omitempty"`
	MaxActiveReservations *int   `json:"max_active_reservations,omitempty"`
	OriginalBytes         *int64 `json:"original_bytes,omitempty"`
	RequestID             string `json:"request_id"`
	RetryAfterSeconds     int    `json:"-"`
}

type invalidInputError struct {
	problem problemBody
}

func (invalidInput invalidInputError) Error() string {
	return invalidInput.problem.Code + ": " + invalidInput.problem.Detail
}

func invalidRequest(detail string) invalidInputError {
	return invalidInputError{problem: problemBody{
		Status: http.StatusBadRequest,
		Code:   "invalid_request",
		Title:  "Invalid request",
		Detail: detail,
	}}
}

func invalidIdempotencyKey(detail string) invalidInputError {
	return invalidInputError{problem: problemBody{
		Status: http.StatusBadRequest,
		Code:   "invalid_idempotency_key",
		Title:  "Invalid idempotency key",
		Detail: detail,
	}}
}

func requestTooLarge() invalidInputError {
	return invalidInputError{problem: problemBody{
		Status: http.StatusRequestEntityTooLarge,
		Code:   "request_too_large",
		Title:  "Request too large",
		Detail: fmt.Sprintf("The request body must not exceed %d bytes.", maxRequestBodyBytes),
	}}
}

func (handlers *apiHandlers) writeError(writer http.ResponseWriter, request *http.Request, err error) {
	problem := problemFor(request, err)
	problem.Type = problemTypeBaseURL + problem.Code
	problem.RequestID = requestIDFrom(request.Context())
	if problem.Status >= http.StatusInternalServerError {
		handlers.logger.Error("request failed", "request_id", problem.RequestID, "error", err)
	}
	if problem.RetryAfterSeconds > 0 {
		writer.Header().Set("Retry-After", strconv.Itoa(problem.RetryAfterSeconds))
	}
	handlers.writeJSON(request.Context(), writer, problem.Status, problemContentType, problem)
}

func problemFor(request *http.Request, err error) problemBody {
	var invalidInput invalidInputError
	var tooManyActive domain.TooManyActiveReservationsError
	var insufficientStorage domain.InsufficientStorageError
	var keyReused domain.IdempotencyKeyReusedError
	switch {
	case errors.As(err, &invalidInput):
		return invalidInput.problem
	case errors.Is(err, domain.ErrAccountNotFound):
		return accountNotFoundProblem(request.PathValue("accountID"))
	case errors.Is(err, domain.ErrReservationNotFound):
		return reservationNotFoundProblem(request.PathValue("accountID"), request.PathValue("reservationID"))
	case errors.Is(err, domain.ErrRequestInProgress):
		return requestInProgressProblem()
	case errors.As(err, &tooManyActive):
		return tooManyActiveReservationsProblem(tooManyActive)
	case errors.As(err, &insufficientStorage):
		return insufficientStorageProblem(insufficientStorage)
	case errors.As(err, &keyReused):
		return idempotencyKeyReusedProblem(keyReused)
	case errors.Is(err, domain.ErrLimitUnavailable):
		return limitUnavailableProblem()
	case errors.Is(err, domain.ErrServiceUnavailable):
		return serviceUnavailableProblem()
	default:
		return internalErrorProblem(requestIDFrom(request.Context()))
	}
}

func accountNotFoundProblem(accountID string) problemBody {
	return problemBody{
		Status: http.StatusNotFound,
		Code:   "account_not_found",
		Title:  "Account not found",
		Detail: fmt.Sprintf("Account '%s' does not exist.", accountID),
	}
}

func reservationNotFoundProblem(accountID, reservationID string) problemBody {
	return problemBody{
		Status: http.StatusNotFound,
		Code:   "reservation_not_found",
		Title:  "Reservation not found",
		Detail: fmt.Sprintf("Reservation '%s' does not exist for account '%s'.", reservationID, accountID),
	}
}

func requestInProgressProblem() problemBody {
	return problemBody{
		Status:            http.StatusConflict,
		Code:              "request_in_progress",
		Title:             "Request in progress",
		Detail:            "A request with this Idempotency-Key is still being processed. Retry in a moment to get its result.",
		RetryAfterSeconds: 1,
	}
}

func tooManyActiveReservationsProblem(refusal domain.TooManyActiveReservationsError) problemBody {
	return problemBody{
		Status: http.StatusConflict,
		Code:   "too_many_active_reservations",
		Title:  "Too many active reservations",
		Detail: fmt.Sprintf("You have %d active reservations and the maximum is %d. Release a reservation before creating a new one.",
			refusal.ActiveReservations, refusal.MaxActiveReservations),
		ActiveReservations:    &refusal.ActiveReservations,
		MaxActiveReservations: &refusal.MaxActiveReservations,
	}
}

func insufficientStorageProblem(refusal domain.InsufficientStorageError) problemBody {
	return problemBody{
		Status: http.StatusConflict,
		Code:   "insufficient_storage",
		Title:  "Insufficient storage",
		Detail: fmt.Sprintf("Requested %s but only %s is available (limit %s). Release a reservation or request less space.",
			formatBytes(refusal.RequestedBytes), formatBytes(refusal.AvailableBytes), formatBytes(refusal.LimitBytes)),
		RequestedBytes: &refusal.RequestedBytes,
		AvailableBytes: &refusal.AvailableBytes,
		LimitBytes:     &refusal.LimitBytes,
	}
}

func idempotencyKeyReusedProblem(refusal domain.IdempotencyKeyReusedError) problemBody {
	return problemBody{
		Status:        http.StatusUnprocessableEntity,
		Code:          "idempotency_key_reused",
		Title:         "Idempotency key reused",
		Detail:        fmt.Sprintf("This Idempotency-Key was already used to reserve %d bytes. Use a new key for a new request.", refusal.OriginalBytes),
		OriginalBytes: &refusal.OriginalBytes,
	}
}

func limitUnavailableProblem() problemBody {
	return problemBody{
		Status:            http.StatusServiceUnavailable,
		Code:              "limit_unavailable",
		Title:             "Limit unavailable",
		Detail:            "Reservation limits are temporarily unavailable. Retry later; releases are not affected.",
		RetryAfterSeconds: 5,
	}
}

func serviceUnavailableProblem() problemBody {
	return problemBody{
		Status:            http.StatusServiceUnavailable,
		Code:              "service_unavailable",
		Title:             "Service unavailable",
		Detail:            "The service is temporarily unavailable. Retry with the same Idempotency-Key.",
		RetryAfterSeconds: 1,
	}
}

func internalErrorProblem(requestID string) problemBody {
	return problemBody{
		Status: http.StatusInternalServerError,
		Code:   "internal_error",
		Title:  "Internal error",
		Detail: fmt.Sprintf("An unexpected error occurred. Reference: %s.", requestID),
	}
}

func formatBytes(byteCount int64) string {
	switch {
	case byteCount >= gibibyte:
		return fmt.Sprintf("%.2f GiB", float64(byteCount)/gibibyte)
	case byteCount >= mebibyte:
		return fmt.Sprintf("%.2f MiB", float64(byteCount)/mebibyte)
	case byteCount >= kibibyte:
		return fmt.Sprintf("%.2f KiB", float64(byteCount)/kibibyte)
	default:
		return fmt.Sprintf("%d B", byteCount)
	}
}
