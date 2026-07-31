package handlers

import (
	"net/http"
	"strings"

	appdb "step-ui/db"
	"step-ui/i18n"
)

const adminAuditPrefix = "Audit: "

func (h *Handler) auditSecurity(r *http.Request, reason string) {
	si := h.sessionInfo(r)
	if si.UserID == 0 || si.Username == "" || reason == "" {
		return
	}
	_ = appdb.LogAuth(h.db, si.Username, r.RemoteAddr, true, adminAuditPrefix+reason)
}

func securityEventLabel(lang string, success bool, reason string) string {
	if !success {
		return i18n.T(lang, "Отказ")
	}
	switch {
	case strings.HasPrefix(reason, adminAuditPrefix):
		return i18n.T(lang, "Аудит")
	case strings.HasPrefix(reason, "2FA"):
		return "2FA"
	case strings.Contains(strings.ToLower(reason), "recovery code"):
		return "2FA"
	case strings.HasPrefix(reason, "Password reset"):
		return "Reset"
	case reason == "Выход":
		return i18n.T(lang, "Выход")
	default:
		return i18n.T(lang, "Вход")
	}
}

func securityEventBadge(success bool, reason string) string {
	if !success {
		return "danger"
	}
	if strings.HasPrefix(reason, adminAuditPrefix) {
		return "warn"
	}
	if strings.HasPrefix(reason, "Password reset") {
		return "warn"
	}
	return "ok"
}
