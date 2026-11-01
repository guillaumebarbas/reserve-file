package handler

import (
	"net/http"

	"github.com/guillaumebarbas/reserve-file/internal/service"
)

type usageBody struct {
	AccountID             string `json:"account_id"`
	LimitBytes            int64  `json:"limit_bytes"`
	ReservedBytes         int64  `json:"reserved_bytes"`
	AvailableBytes        int64  `json:"available_bytes"`
	ActiveReservations    int    `json:"active_reservations"`
	MaxActiveReservations *int   `json:"max_active_reservations"`
}

func (handlers *apiHandlers) reportUsage(writer http.ResponseWriter, request *http.Request) {
	usage, err := handlers.storage.Usage(request.Context(), request.PathValue("accountID"))
	if err != nil {
		handlers.writeError(writer, request, err)
		return
	}
	handlers.writeJSON(request.Context(), writer, http.StatusOK, jsonContentType, newUsageBody(usage))
}

func newUsageBody(usage service.Usage) usageBody {
	body := usageBody{
		AccountID:          usage.Account.ID,
		LimitBytes:         usage.Account.LimitBytes,
		ReservedBytes:      usage.Account.ReservedBytes(),
		AvailableBytes:     usage.Account.AvailableBytes(),
		ActiveReservations: usage.Account.ActiveReservations(),
	}
	if usage.MaxActive.Known {
		body.MaxActiveReservations = &usage.MaxActive.Value
	}
	return body
}
