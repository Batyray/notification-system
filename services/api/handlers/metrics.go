package handlers

import (
	"net/http"
)

func (h *Handler) GetMetrics(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
}
