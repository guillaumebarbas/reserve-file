package handler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/guillaumebarbas/reserve-file/internal/domain"
)

type reserveRequestBody struct {
	Bytes *int64 `json:"bytes"`
}

type reservationBody struct {
	ID         string     `json:"id"`
	AccountID  string     `json:"account_id"`
	Bytes      int64      `json:"bytes"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	ReleasedAt *time.Time `json:"released_at"`
}

func (handlers *apiHandlers) reserve(writer http.ResponseWriter, request *http.Request) {
	idempotencyKey, bytes, err := readReserveRequest(writer, request)
	if err != nil {
		handlers.writeError(writer, request, err)
		return
	}
	reservation, err := handlers.storage.Reserve(request.Context(), request.PathValue("accountID"), idempotencyKey, bytes)
	if err != nil {
		handlers.writeError(writer, request, err)
		return
	}
	writer.Header().Set("Location", "/accounts/"+reservation.AccountID+"/reservations/"+reservation.ID)
	handlers.writeJSON(request.Context(), writer, http.StatusCreated, jsonContentType, newReservationBody(reservation))
}

func readReserveRequest(writer http.ResponseWriter, request *http.Request) (string, int64, error) {
	idempotencyKey := request.Header.Get("Idempotency-Key")
	if idempotencyKey == "" {
		return "", 0, invalidIdempotencyKey("The Idempotency-Key header is required. Generate one key per logical request and reuse it on every retry.")
	}
	var body reserveRequestBody
	if err := decodeJSON(writer, request, &body); err != nil {
		return "", 0, err
	}
	if body.Bytes == nil {
		return "", 0, invalidRequest("bytes is required: the number of bytes to reserve, as a positive integer.")
	}
	if *body.Bytes <= 0 {
		return "", 0, invalidRequest(fmt.Sprintf("bytes must be a positive integer, got %d.", *body.Bytes))
	}
	return idempotencyKey, *body.Bytes, nil
}

func newReservationBody(reservation domain.Reservation) reservationBody {
	return reservationBody{
		ID:        reservation.ID,
		AccountID: reservation.AccountID,
		Bytes:     reservation.Bytes,
		Status:    string(reservation.Status),
		CreatedAt: reservation.CreatedAt,
	}
}
