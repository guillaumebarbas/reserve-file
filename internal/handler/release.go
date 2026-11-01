package handler

import "net/http"

func (handlers *apiHandlers) readReservation(writer http.ResponseWriter, request *http.Request) {
	reservation, err := handlers.storage.Reservation(request.Context(), request.PathValue("accountID"), request.PathValue("reservationID"))
	if err != nil {
		handlers.writeError(writer, request, err)
		return
	}
	handlers.writeJSON(request.Context(), writer, http.StatusOK, jsonContentType, newReservationBody(reservation))
}

func (handlers *apiHandlers) releaseReservation(writer http.ResponseWriter, request *http.Request) {
	reservation, err := handlers.storage.Release(request.Context(), request.PathValue("accountID"), request.PathValue("reservationID"))
	if err != nil {
		handlers.writeError(writer, request, err)
		return
	}
	handlers.writeJSON(request.Context(), writer, http.StatusOK, jsonContentType, newReservationBody(reservation))
}
