package api

import (
	"net/http"
	"strconv"
)

const (
	maxPageLimit = 100
	defaultPageLimit = 30
)

// parsePagination extracts limit and offset from request query parameters.
// limit defaults to 30, max 100. offset defaults to 0.
func parsePagination(r *http.Request) (limit, offset int) {
	limit = defaultPageLimit
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= maxPageLimit {
			limit = n
		}
	}
	if s := r.URL.Query().Get("offset"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			offset = n
		}
	}
	return
}
