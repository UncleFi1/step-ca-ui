package handlers

import (
	"net/http"
	"strconv"

	appdb "step-ui/db"
)

const pageSize = 30

func (h *Handler) History(w http.ResponseWriter, r *http.Request) {
	action := r.URL.Query().Get("action")
	cert := r.URL.Query().Get("cert")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	entries, total, _ := appdb.GetHistory(h.db, action, cert, page, pageSize)
	totalPages := (total + pageSize - 1) / pageSize
	if totalPages < 1 {
		totalPages = 1
	}
	data := h.base(w, r, "history")
	data["Entries"] = entries
	data["FilterAction"] = action
	data["FilterCert"] = cert
	data["CurrentPage"] = page
	data["TotalPages"] = totalPages
	data["Total"] = total
	h.render(w, "history", data)
}
