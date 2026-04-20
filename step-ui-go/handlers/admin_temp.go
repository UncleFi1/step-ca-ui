package handlers

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"net/http"
	appdb "step-ui/db"
	"step-ui/security"
	"strconv"
	"strings"
	"time"
)

// AdminUsersTempGet — страница списка временных пользователей.
func (h *Handler) AdminUsersTempGet(w http.ResponseWriter, r *http.Request) {
	users, _ := appdb.ListTempUsers(h.db)

	// Формируем view-model: предрассчитанный статус и отформатированные даты
	type tempUserVM struct {
		ID        int
		Username  string
		Role      string
		Note      string
		CreatedAt string
		ExpiresAt string
		Status    string // "active" | "expired" | "blocked"
	}
	now := time.Now()
	var vms []tempUserVM
	for _, u := range users {
		vm := tempUserVM{
			ID:        u.ID,
			Username:  u.Username,
			Role:      u.Role,
			Note:      u.Note,
			CreatedAt: u.CreatedAt.Local().Format("2006-01-02 15:04"),
		}
		if u.ExpiresAt != nil {
			vm.ExpiresAt = u.ExpiresAt.Local().Format("2006-01-02 15:04")
		} else {
			vm.ExpiresAt = ""
		}
		switch {
		case u.IsActive:
			vm.Status = "active"
		case u.ExpiresAt != nil && now.After(*u.ExpiresAt):
			vm.Status = "expired"
		default:
			vm.Status = "blocked"
		}
		vms = append(vms, vm)
	}
	data := h.base(w, r, "admin_users_temp")
	scheme := "https"
	if r.TLS == nil && r.Header.Get("X-Forwarded-Proto") != "https" {
		scheme = "http"
		if p := r.Header.Get("X-Forwarded-Proto"); p != "" {
			scheme = p
		}
	}
	data["LoginURL"] = scheme + "://" + r.Host + "/login"
	data["Flashes"] = h.popFlash(w, r)
	data["Users"] = vms
	data["Now"] = time.Now()

	// Одноразовый показ свежесгенерированных credentials — через flash в сессии
	if fl := r.URL.Query().Get("new_id"); fl != "" {
		// Пароль подтягиваем из cookie-заглушки (мы положили туда в POST)
		if c, err := r.Cookie("new_temp_cred"); err == nil {
			// формат: "username|password"
			val := c.Value
			for i := 0; i < len(val); i++ {
				if val[i] == '|' {
					data["NewUsername"] = val[:i]
					data["NewPassword"] = val[i+1:]
					break
				}
			}
			// Удалим cookie сразу после показа
			http.SetCookie(w, &http.Cookie{
				Name:    "new_temp_cred",
				Value:   "",
				Path:    "/",
				Expires: time.Unix(0, 0),
				MaxAge:  -1,
			})
		}
	}
	h.render(w, "admin_users_temp", data)
}

// AdminUsersTempPost — создание временного пользователя.
func (h *Handler) AdminUsersTempPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	role := r.FormValue("role")
	if role != "admin" && role != "manager" && role != "viewer" {
		role = "viewer"
	}
	note := r.FormValue("note")

	// Срок действия: либо custom_datetime (формат "2006-01-02 15:04"),
	// либо preset ("30m"|"1h"|"4h"|"24h"|"7d"|"30d").
	var expiresAt time.Time
	if custom := strings.TrimSpace(r.FormValue("custom_datetime")); custom != "" {
		if t, err := time.ParseInLocation("2006-01-02 15:04", custom, time.Local); err == nil {
			expiresAt = t
		} else {
			h.flash(w, r, "err", "Неверный формат даты/времени")
			http.Redirect(w, r, "/admin/users-temp", http.StatusSeeOther)
			return
		}
	}
	if expiresAt.IsZero() {
		preset := r.FormValue("preset")
		if preset == "" {
			// Совместимость со старой формой
			if hrs, _ := strconv.Atoi(r.FormValue("preset_hours")); hrs > 0 {
				preset = fmt.Sprintf("%dh", hrs)
			}
		}
		dur := presetToDuration(preset)
		if dur <= 0 {
			dur = 24 * time.Hour
		}
		expiresAt = time.Now().Add(dur)
	}

	if !expiresAt.After(time.Now().Add(1 * time.Minute)) {
		h.flash(w, r, "err", "Срок действия должен быть в будущем (хотя бы через минуту)")
		http.Redirect(w, r, "/admin/users-temp", http.StatusSeeOther)
		return
	}

	// Генерация логина и пароля
	username := generateTempUsername()
	password := generateTempPassword(16)

	hash := security.HashPassword(password)
	id, err := appdb.CreateTempUser(h.db, username, hash, role, expiresAt, note)
	if err != nil {
		h.flash(w, r, "err", "Не удалось создать пользователя: "+err.Error())
		http.Redirect(w, r, "/admin/users-temp", http.StatusSeeOther)
		return
	}

	// Кладём свежие credentials в короткоживущий cookie — чтобы GET показал их 1 раз
	http.SetCookie(w, &http.Cookie{
		Name:     "new_temp_cred",
		Value:    username + "|" + password,
		Path:     "/",
		MaxAge:   120, // 2 минуты
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	h.flash(w, r, "ok", "Временный пользователь создан")
	http.Redirect(w, r, fmt.Sprintf("/admin/users-temp?new_id=%d", id), http.StatusSeeOther)
}

// generateTempUsername → "guest-ab12cd"
func generateTempUsername() string {
	const alphabet = "abcdefghijkmnopqrstuvwxyz23456789" // без 0,1,l,o
	b := make([]byte, 6)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		b[i] = alphabet[n.Int64()]
	}
	return "guest-" + string(b)
}

// generateTempPassword — безопасный пароль длины n, исключая похожие символы
func generateTempPassword(n int) string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghjkmnpqrstuvwxyz23456789!@#$%&*+-=?"
	b := make([]byte, n)
	for i := range b {
		idx, _ := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		b[i] = alphabet[idx.Int64()]
	}
	return string(b)
}

// presetToDuration — маппинг строки пресета в Duration.
// Поддерживает: 30m, 1h, 4h, 24h, 7d, 30d
func presetToDuration(p string) time.Duration {
	switch p {
	case "30m":
		return 30 * time.Minute
	case "1h":
		return 1 * time.Hour
	case "4h":
		return 4 * time.Hour
	case "24h":
		return 24 * time.Hour
	case "7d":
		return 7 * 24 * time.Hour
	case "30d":
		return 30 * 24 * time.Hour
	default:
		return 0
	}
}
