package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	appdb "step-ui/db"
	"step-ui/models"
	"step-ui/security"
)

func (h *Handler) Users(w http.ResponseWriter, r *http.Request) {
	users, _ := appdb.GetAllUsers(h.db)
	since := time.Now().Add(-24 * time.Hour)
	failCounts := map[int]int{}
	for _, u := range users {
		failCounts[u.ID] = appdb.GetFailCount(h.db, u.Username, since)
	}
	data := h.base(w, r, "users")
	data["Users"] = users
	data["FailCounts"] = failCounts
	h.render(w, "users", data)
}

func (h *Handler) UsersPost(w http.ResponseWriter, r *http.Request) {
	si := h.sessionInfo(r)
	action := r.FormValue("action")
	switch action {
	case "create":
		username := trimStr(r.FormValue("username"))
		password := trimStr(r.FormValue("password"))
		role := r.FormValue("role")
		if username == "" || password == "" {
			h.flash(w, r, "err", "Заполните все поля")
			break
		}
		if ok, msg := security.ValidatePassword(password); !ok {
			h.flash(w, r, "err", msg)
			break
		}
		if err := appdb.CreateUser(h.db, username, security.HashPassword(password), role); err != nil {
			h.flash(w, r, "err", "Пользователь уже существует")
		} else {
			h.flash(w, r, "ok", "Пользователь "+username+" создан")
		}

	case "delete":
		uid, _ := strconv.Atoi(r.FormValue("uid"))
		if uid == si.UserID {
			h.flash(w, r, "err", "Нельзя удалить себя")
			break
		}
		appdb.DeleteUser(h.db, uid)
		h.flash(w, r, "ok", "Пользователь удалён")

	case "change_role":
		uid, _ := strconv.Atoi(r.FormValue("uid"))
		role := r.FormValue("role")
		if uid == si.UserID {
			h.flash(w, r, "err", "Нельзя изменить свою роль")
			break
		}
		if role == "viewer" || role == "manager" || role == "admin" {
			appdb.UpdateUserRole(h.db, uid, role)
			h.flash(w, r, "ok", "Роль обновлена")
		}

	case "toggle_active":
		uid, _ := strconv.Atoi(r.FormValue("uid"))
		if uid == si.UserID {
			h.flash(w, r, "err", "Нельзя заблокировать себя")
			break
		}
		u, _ := appdb.GetUserByID(h.db, uid)
		if u != nil {
			newState := !u.IsActive
			appdb.UpdateUserActive(h.db, uid, newState)
			if newState {
				h.flash(w, r, "ok", "Пользователь разблокирован")
			} else {
				h.flash(w, r, "ok", "Пользователь заблокирован")
			}
		}

	case "unblock_ip":
		ip := r.FormValue("target_ip")
		if ip != "" {
			security.RL.Clear(ip)
			h.flash(w, r, "ok", fmt.Sprintf("IP %s разблокирован", ip))
		}

	case "reset_password":
		uid, _ := strconv.Atoi(r.FormValue("uid"))
		newPW := trimStr(r.FormValue("new_password"))
		if ok, msg := security.ValidatePassword(newPW); !ok {
			h.flash(w, r, "err", msg)
			break
		}
		appdb.UpdateUserPassword(h.db, uid, security.HashPassword(newPW))
		h.flash(w, r, "ok", "Пароль сброшен")
	}
	http.Redirect(w, r, "/users", http.StatusFound)
}

func (h *Handler) UserProfile(w http.ResponseWriter, r *http.Request) {
	uid, _ := strconv.Atoi(chi.URLParam(r, "id"))
	u, _ := appdb.GetUserByID(h.db, uid)
	if u == nil {
		http.Redirect(w, r, "/users", http.StatusFound)
		return
	}
	logs, _ := appdb.GetUserAuthLogs(h.db, u.Username, 50)
	ok, _ := appdb.GetAuthStats(h.db)
	totalOK := 0
	totalFail := 0
	for _, l := range logs {
		if l.Success {
			totalOK++
		} else {
			totalFail++
		}
	}
	_ = ok
	ipBlocked := false
	if u.LastIP != nil && *u.LastIP != "" {
		ipBlocked = security.RL.IsBlocked(*u.LastIP)
	}
	data := h.base(w, r, "users")
	data["U"] = u
	data["Logs"] = logs
	data["TotalOK"] = totalOK
	data["TotalFail"] = totalFail
	data["IPBlocked"] = ipBlocked
	h.render(w, "user_profile", data)
}

func (h *Handler) ProfileGet(w http.ResponseWriter, r *http.Request) {
	h.render(w, "profile", h.base(w, r, "profile"))
}

func (h *Handler) ProfilePost(w http.ResponseWriter, r *http.Request) {
	si := h.sessionInfo(r)
	current := r.FormValue("current_password")
	newPW := trimStr(r.FormValue("new_password"))
	confirm := trimStr(r.FormValue("confirm_password"))

	u, _ := appdb.GetUserByID(h.db, si.UserID)
	data := h.base(w, r, "profile")

	if u == nil || u.PasswordHash != security.HashPassword(current) {
		data["Msgs"] = []models.FlashMsg{{Type: "err", Text: "Неверный текущий пароль"}}
		h.render(w, "profile", data)
		return
	}
	if newPW != confirm {
		data["Msgs"] = []models.FlashMsg{{Type: "err", Text: "Пароли не совпадают"}}
		h.render(w, "profile", data)
		return
	}
	if ok, msg := security.ValidatePassword(newPW); !ok {
		data["Msgs"] = []models.FlashMsg{{Type: "err", Text: msg}}
		h.render(w, "profile", data)
		return
	}
	appdb.UpdateUserPassword(h.db, si.UserID, security.HashPassword(newPW))
	h.flash(w, r, "ok", "Пароль успешно изменён")
	http.Redirect(w, r, "/profile", http.StatusFound)
}
