package handlers

import (
	"net/http"
	"time"
)

// AdminStats — сводные счётчики для /admin.
type AdminStats struct {
	TotalUsers    int
	ActiveUsers   int
	BlockedUsers  int
	AdminsCount   int
	ManagersCount int
	ViewersCount  int
	LoginsToday   int
	FailedLogins  int
	ActiveCerts   int
	LeCerts       int
}

// AdminLogin — строка последних входов.
type AdminLogin struct {
	Username  string
	IP        string
	CreatedAt time.Time
}

// AdminGet — обзорная страница админа.
func (h *Handler) AdminGet(w http.ResponseWriter, r *http.Request) {
	var s AdminStats

	_ = h.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&s.TotalUsers)
	_ = h.db.QueryRow(`SELECT COUNT(*) FROM users WHERE is_active = true`).Scan(&s.ActiveUsers)
	_ = h.db.QueryRow(`SELECT COUNT(*) FROM users WHERE is_active = false`).Scan(&s.BlockedUsers)
	_ = h.db.QueryRow(`SELECT COUNT(*) FROM users WHERE role = 'admin'`).Scan(&s.AdminsCount)
	_ = h.db.QueryRow(`SELECT COUNT(*) FROM users WHERE role = 'manager'`).Scan(&s.ManagersCount)
	_ = h.db.QueryRow(`SELECT COUNT(*) FROM users WHERE role = 'viewer'`).Scan(&s.ViewersCount)

	_ = h.db.QueryRow(`
		SELECT COUNT(*) FROM certs
		WHERE (revoked IS NULL OR revoked = false)
		  AND not_after > NOW()
	`).Scan(&s.ActiveCerts)
	_ = h.db.QueryRow(`SELECT COUNT(*) FROM le_certificates`).Scan(&s.LeCerts)

	_ = h.db.QueryRow(`
		SELECT COUNT(*) FROM security_log
		WHERE event_type = 'login_success'
		  AND created_at > NOW() - INTERVAL '24 hours'
	`).Scan(&s.LoginsToday)
	_ = h.db.QueryRow(`
		SELECT COUNT(*) FROM security_log
		WHERE event_type = 'login_failed'
		  AND created_at > NOW() - INTERVAL '24 hours'
	`).Scan(&s.FailedLogins)

	var recent []AdminLogin
	rows, err := h.db.Query(`
		SELECT COALESCE(u.username, '—') AS username,
		       COALESCE(sl.ip, '—')      AS ip,
		       sl.created_at
		FROM security_log sl
		LEFT JOIN users u ON u.id = sl.user_id
		WHERE sl.event_type = 'login_success'
		ORDER BY sl.created_at DESC
		LIMIT 10
	`)
	if err == nil && rows != nil {
		defer rows.Close()
		for rows.Next() {
			var l AdminLogin
			if err := rows.Scan(&l.Username, &l.IP, &l.CreatedAt); err == nil {
				recent = append(recent, l)
			}
		}
	}

	data := h.base(w, r, "admin")
	data["Stats"] = s
	data["RecentLogins"] = recent
	h.render(w, "admin", data)
}

// AdminActivityGet — заглушка.
func (h *Handler) AdminActivityGet(w http.ResponseWriter, r *http.Request) {
	h.render(w, "admin_activity", h.base(w, r, "admin_activity"))
}

// AdminConsoleGet — заглушка (2FA).
func (h *Handler) AdminConsoleGet(w http.ResponseWriter, r *http.Request) {
	h.render(w, "admin_console", h.base(w, r, "admin_console"))
}

// AdminAboutGet — о системе.
func (h *Handler) AdminAboutGet(w http.ResponseWriter, r *http.Request) {
	data := h.base(w, r, "admin_about")
	data["AppVersion"] = Version
	data["AppBuildDate"] = BuildDate
	data["AppGitCommit"] = GitCommit
	h.render(w, "admin_about", data)
}
