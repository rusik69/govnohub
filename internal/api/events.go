package api

import (
	"net/http"
	"strconv"

	"github.com/rusik69/govnohub/internal/events"
)

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit := 30
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	evts, err := s.events.List(r.Context(), limit)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if evts == nil {
		evts = []events.Event{}
	}
	jsonOK(w, evts)
}
