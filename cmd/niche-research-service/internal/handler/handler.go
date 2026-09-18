package handler

import (
	"encoding/json"
	"net/http"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/niche-research-service/internal/service"
)

type Handler struct {
	Service *service.Service
}

// createRequest mirrors what web/niche.jsx already posts to
// /api/niche/sessions in its mock API.
type createRequest struct {
	RawKeyword   string   `json:"raw_keyword"`
	CountryCodes []string `json:"country_codes"`
}

func (h *Handler) Sessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		sessions, err := h.Service.ListSessions(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, sessions)

	case http.MethodPost:
		var req createRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		sess, err := h.Service.CreateAndRun(r.Context(), req.RawKeyword, req.CountryCodes)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, sess)

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

// WithCORS allows the standalone web/ dashboard (served from a different
// origin/port during development) to call this API directly from the
// browser.
func WithCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
