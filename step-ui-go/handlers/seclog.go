package handlers

import (
	"net/http"
	"strconv"

	appdb "step-ui/db"
)

func (h *Handler) SecurityLog(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("q")
	filter := r.URL.Query().Get("filter")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	entries, total, _ := appdb.GetAuthLogs(h.db, search, filter, page, pageSize)
	totalPages := (total + pageSize - 1) / pageSize
	if totalPages < 1 {
		totalPages = 1
	}
	okCount, failCount := appdb.GetAuthStats(h.db)
	data := h.base(w, r, "admin_security")
	data["Entries"] = entries
	data["SearchQ"] = search
	data["Filter"] = filter
	data["Total"] = total
	data["TotalOK"] = okCount
	data["TotalFail"] = failCount
	data["CurrentPage"] = page
	data["TotalPages"] = totalPages
	h.render(w, "admin_security", data)
}
