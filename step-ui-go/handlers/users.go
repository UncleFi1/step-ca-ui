package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	appdb "step-ui/db"
	"step-ui/security"
)

func (h *Handler) Users(w http.ResponseWriter, r *http.Request) {
	users, _ := appdb.GetAllUsers(h.db)
	since := time.Now().Add(-24 * time.Hour)
	failCounts := map[int]int{}
	for _, u := range users {
		failCounts[u.ID] = appdb.GetFailCount(h.db, u.Username, since)
	}
	data := h.base(w, r, "admin_users")
	data["Users"] = users
	data["FailCounts"] = failCounts
	h.render(w, "admin_users", data)
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
	returnTo := r.FormValue("return_to")
	if returnTo == "" {
		returnTo = "/users"
	}
	http.Redirect(w, r, returnTo, http.StatusFound)
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
	data := h.base(w, r, "admin_users")
	data["U"] = u
	data["Logs"] = logs
	data["TotalOK"] = totalOK
	data["TotalFail"] = totalFail
	data["IPBlocked"] = ipBlocked
	h.render(w, "admin_user_profile", data)
}

func (h *Handler) ProfileGet(w http.ResponseWriter, r *http.Request) {
	si := h.sessionInfo(r)
	u, _ := appdb.GetUserByID(h.db, si.UserID)
	data := h.base(w, r, "profile")
	data["U"] = u
	h.render(w, "profile", data)
}

func (h *Handler) ProfilePost(w http.ResponseWriter, r *http.Request) {
	si := h.sessionInfo(r)
	action := r.FormValue("action")

	switch action {
	case "theme":
		theme := trimStr(r.FormValue("theme"))
		valid := map[string]bool{"dark": true, "light": true, "blue": true, "auto": true}
		if !valid[theme] {
			theme = "dark"
		}
		if err := appdb.UpdateUserTheme(h.db, si.UserID, theme); err != nil {
			h.flash(w, r, "err", "Ошибка сохранения темы")
		} else {
			h.flash(w, r, "ok", "Тема обновлена")
		}
		http.Redirect(w, r, "/profile", http.StatusFound)
		return

	case "update_info":
		username := trimStr(r.FormValue("username"))
		displayName := trimStr(r.FormValue("display_name"))
		email := trimStr(r.FormValue("email"))
		if username == "" {
			h.flash(w, r, "err", "Логин не может быть пустым")
			http.Redirect(w, r, "/profile", http.StatusFound)
			return
		}
		// Проверим что логин не занят другим пользователем
		exists, _ := appdb.UsernameExistsExceptID(h.db, username, si.UserID)
		if exists {
			h.flash(w, r, "err", "Пользователь с таким логином уже существует")
			http.Redirect(w, r, "/profile", http.StatusFound)
			return
		}
		if err := appdb.UpdateUserInfo(h.db, si.UserID, username, displayName, email); err != nil {
			h.flash(w, r, "err", "Ошибка при обновлении: "+err.Error())
			http.Redirect(w, r, "/profile", http.StatusFound)
			return
		}
		// Обновляем username в сессии
		s := h.sess(r)
		s.Values["username"] = username
		s.Save(r, w)
		h.flash(w, r, "ok", "Профиль обновлён")
		http.Redirect(w, r, "/profile", http.StatusFound)
		return

	case "change_password", "":
		current := r.FormValue("current_password")
		newPW := trimStr(r.FormValue("new_password"))
		confirm := trimStr(r.FormValue("confirm_password"))

		u, _ := appdb.GetUserByID(h.db, si.UserID)
		if u == nil || u.PasswordHash != security.HashPassword(current) {
			h.flash(w, r, "err", "Неверный текущий пароль")
			http.Redirect(w, r, "/profile", http.StatusFound)
			return
		}
		if newPW != confirm {
			h.flash(w, r, "err", "Пароли не совпадают")
			http.Redirect(w, r, "/profile", http.StatusFound)
			return
		}
		if ok, msg := security.ValidatePassword(newPW); !ok {
			h.flash(w, r, "err", msg)
			http.Redirect(w, r, "/profile", http.StatusFound)
			return
		}
		appdb.UpdateUserPassword(h.db, si.UserID, security.HashPassword(newPW))
		h.flash(w, r, "ok", "Пароль успешно изменён")
		http.Redirect(w, r, "/profile", http.StatusFound)
		return
	}

	http.Redirect(w, r, "/profile", http.StatusFound)
}
