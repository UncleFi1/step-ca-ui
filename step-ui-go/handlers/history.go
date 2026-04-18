package handlers

import (
	"net/http"
	"strconv"

	appdb "step-ui/db"
)

const pageSize = 30

func (h *Handler) History(w http.ResponseWriter, r *http.Request) {
	// Multi-select: берём все значения ?action=... из URL
	actions := r.URL.Query()["action"]
	// Отфильтруем пустые значения
	filtered := actions[:0]
	for _, a := range actions {
		if a != "" {
			filtered = append(filtered, a)
		}
	}
	actions = filtered

	cert := r.URL.Query().Get("cert")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	entries, total, _ := appdb.GetHistory(h.db, actions, cert, page, pageSize)
	totalPages := (total + pageSize - 1) / pageSize
	if totalPages < 1 {
		totalPages = 1
	}
	data := h.base(w, r, "history")
	data["Entries"] = entries
	data["FilterActions"] = actions
	data["FilterCert"] = cert
	data["CurrentPage"] = page
	data["TotalPages"] = totalPages
	data["Total"] = total
	h.render(w, "history", data)
}
