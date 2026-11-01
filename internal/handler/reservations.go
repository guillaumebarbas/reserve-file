package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/guillaumebarbas/reserve-file/internal/domain"
)

type reserveRequestBody struct {
	Bytes json.RawMessage `json:"bytes"`
}

type reservationBody struct {
	ID         string     `json:"id"`
	AccountID  string     `json:"account_id"`
	Bytes      int64      `json:"bytes"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	ReleasedAt *time.Time `json:"released_at"`
}

type reservationListBody struct {
	Reservations []reservationBody `json:"reservations"`
}

func (handlers *apiHandlers) reserve(writer http.ResponseWriter, request *http.Request) {
	idempotencyKey, bytes, err := readReserveRequest(writer, request)
	if err != nil {
		handlers.writeError(writer, request, err)
		return
	}
	reservation, outcome, err := handlers.storage.Reserve(request.Context(), request.PathValue("accountID"), idempotencyKey, bytes)
	if err != nil {
		handlers.writeError(writer, request, err)
		return
	}
	if outcome == domain.Replayed {
		writer.Header().Set("Idempotent-Replayed", "true")
		handlers.writeJSON(request.Context(), writer, http.StatusOK, jsonContentType, newReservationBody(reservation))
		return
	}
	writer.Header().Set("Location", "/accounts/"+reservation.AccountID+"/reservations/"+reservation.ID)
	handlers.writeJSON(request.Context(), writer, http.StatusCreated, jsonContentType, newReservationBody(reservation))
}

func (handlers *apiHandlers) listActiveReservations(writer http.ResponseWriter, request *http.Request) {
	reservations, err := handlers.storage.ListActiveReservations(request.Context(), request.PathValue("accountID"))
	if err != nil {
		handlers.writeError(writer, request, err)
		return
	}
	body := reservationListBody{Reservations: make([]reservationBody, 0, len(reservations))}
	for _, reservation := range reservations {
		body.Reservations = append(body.Reservations, newReservationBody(reservation))
	}
	handlers.writeJSON(request.Context(), writer, http.StatusOK, jsonContentType, body)
}

func readReserveRequest(writer http.ResponseWriter, request *http.Request) (string, int64, error) {
	idempotencyKey := request.Header.Get("Idempotency-Key")
	if err := validateIdempotencyKey(idempotencyKey); err != nil {
		return "", 0, err
	}
	var body reserveRequestBody
	if err := decodeJSON(writer, request, &body); err != nil {
		return "", 0, err
	}
	bytes, err := parseBytes(body.Bytes)
	if err != nil {
		return "", 0, err
	}
	return idempotencyKey, bytes, nil
}

func newReservationBody(reservation domain.Reservation) reservationBody {
	body := reservationBody{
		ID:        reservation.ID,
		AccountID: reservation.AccountID,
		Bytes:     reservation.Bytes,
		Status:    string(reservation.Status),
		CreatedAt: reservation.CreatedAt,
	}
	if reservation.Status == domain.StatusReleased {
		body.ReleasedAt = &reservation.ReleasedAt
	}
	return body
}
