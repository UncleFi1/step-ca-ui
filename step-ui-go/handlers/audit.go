package handlers

import (
	"net/http"
	"strings"

	appdb "step-ui/db"
)

const adminAuditPrefix = "Audit: "

func (h *Handler) auditSecurity(r *http.Request, reason string) {
	si := h.sessionInfo(r)
	if si.UserID == 0 || si.Username == "" || reason == "" {
		return
	}
	_ = appdb.LogAuth(h.db, si.Username, r.RemoteAddr, true, adminAuditPrefix+reason)
}

func securityEventLabel(success bool, reason string) string {
	if !success {
		return "Отказ"
	}
	switch {
	case strings.HasPrefix(reason, adminAuditPrefix):
		return "Аудит"
	case strings.HasPrefix(reason, "2FA"):
		return "2FA"
	case strings.Contains(strings.ToLower(reason), "recovery code"):
		return "2FA"
	case reason == "Выход":
		return "Выход"
	default:
		return "Вход"
	}
}

func securityEventBadge(success bool, reason string) string {
	if !success {
		return "danger"
	}
	if strings.HasPrefix(reason, adminAuditPrefix) {
		return "warn"
	}
	return "ok"
}
