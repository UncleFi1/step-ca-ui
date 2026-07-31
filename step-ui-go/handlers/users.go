package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	appdb "step-ui/db"
	"step-ui/i18n"
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
	if !h.requireCSRF(w, r, "/admin/users") {
		return
	}
	si := h.sessionInfo(r)
	action := r.FormValue("action")
	switch action {
	case "create":
		username := trimStr(r.FormValue("username"))
		password := trimStr(r.FormValue("password"))
		role := r.FormValue("role")
		if username == "" || password == "" {
			h.flash(w, r, "err", h.T(r, "Заполните все поля"))
			break
		}
		if ok, msg := security.ValidatePassword(password); !ok {
			h.flash(w, r, "err", h.T(r, msg))
			break
		}
		if err := appdb.CreateUser(h.db, username, security.HashPassword(password), role); err != nil {
			h.flash(w, r, "err", h.T(r, "Пользователь уже существует"))
		} else {
			h.auditSecurity(r, fmt.Sprintf("user.create target=%s role=%s", username, role))
			h.flash(w, r, "ok", h.T(r, "Пользователь ")+username+h.T(r, " создан"))
		}

	case "delete":
		uid, _ := strconv.Atoi(r.FormValue("uid"))
		if uid == si.UserID {
			h.flash(w, r, "err", h.T(r, "Нельзя удалить себя"))
			break
		}
		target, _ := appdb.GetUserByID(h.db, uid)
		appdb.DeleteUser(h.db, uid)
		if target != nil {
			h.auditSecurity(r, fmt.Sprintf("user.delete target=%s uid=%d", target.Username, uid))
		} else {
			h.auditSecurity(r, fmt.Sprintf("user.delete uid=%d", uid))
		}
		h.flash(w, r, "ok", h.T(r, "Пользователь удалён"))

	case "change_role":
		uid, _ := strconv.Atoi(r.FormValue("uid"))
		role := r.FormValue("role")
		if uid == si.UserID {
			h.flash(w, r, "err", h.T(r, "Нельзя изменить свою роль"))
			break
		}
		if role == "viewer" || role == "manager" || role == "admin" {
			appdb.UpdateUserRole(h.db, uid, role)
			target, _ := appdb.GetUserByID(h.db, uid)
			if target != nil {
				h.auditSecurity(r, fmt.Sprintf("user.change_role target=%s uid=%d role=%s", target.Username, uid, role))
			} else {
				h.auditSecurity(r, fmt.Sprintf("user.change_role uid=%d role=%s", uid, role))
			}
			h.flash(w, r, "ok", h.T(r, "Роль обновлена"))
		}

	case "toggle_active":
		uid, _ := strconv.Atoi(r.FormValue("uid"))
		if uid == si.UserID {
			h.flash(w, r, "err", h.T(r, "Нельзя заблокировать себя"))
			break
		}
		u, _ := appdb.GetUserByID(h.db, uid)
		if u != nil {
			newState := !u.IsActive
			appdb.UpdateUserActive(h.db, uid, newState)
			if newState {
				h.auditSecurity(r, fmt.Sprintf("user.unblock target=%s uid=%d", u.Username, uid))
				h.flash(w, r, "ok", h.T(r, "Пользователь разблокирован"))
			} else {
				h.auditSecurity(r, fmt.Sprintf("user.block target=%s uid=%d", u.Username, uid))
				h.flash(w, r, "ok", h.T(r, "Пользователь заблокирован"))
			}
		}

	case "unblock_ip":
		ip := r.FormValue("target_ip")
		if ip != "" {
			security.RL.Clear(ip)
			h.auditSecurity(r, fmt.Sprintf("ip.unblock target=%s", ip))
			h.flash(w, r, "ok", h.Tf(r, "IP %s разблокирован", ip))
		}

	case "reset_password":
		uid, _ := strconv.Atoi(r.FormValue("uid"))
		newPW := trimStr(r.FormValue("new_password"))
		if ok, msg := security.ValidatePassword(newPW); !ok {
			h.flash(w, r, "err", h.T(r, msg))
			break
		}
		appdb.UpdateUserPassword(h.db, uid, security.HashPassword(newPW))
		target, _ := appdb.GetUserByID(h.db, uid)
		if target != nil {
			h.auditSecurity(r, fmt.Sprintf("user.reset_password target=%s uid=%d", target.Username, uid))
		} else {
			h.auditSecurity(r, fmt.Sprintf("user.reset_password uid=%d", uid))
		}
		h.flash(w, r, "ok", h.T(r, "Пароль сброшен"))
	}
	returnTo := r.FormValue("return_to")
	if returnTo == "" {
		returnTo = "/admin/users"
	}
	http.Redirect(w, r, returnTo, http.StatusFound)
}

func (h *Handler) UserProfile(w http.ResponseWriter, r *http.Request) {
	uid, _ := strconv.Atoi(chi.URLParam(r, "id"))
	u, _ := appdb.GetUserByID(h.db, uid)
	if u == nil {
		http.Redirect(w, r, "/admin/users", http.StatusFound)
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
	if !h.requireCSRF(w, r, "/profile") {
		return
	}
	si := h.sessionInfo(r)
	action := r.FormValue("action")

	switch action {
	case "language":
		lang := i18n.Normalize(trimStr(r.FormValue("language")))
		i18n.SetCookie(w, lang)
		if err := appdb.UpdateUserLanguage(h.db, si.UserID, lang); err != nil {
			h.flash(w, r, "err", h.T(r, "Ошибка сохранения языка"))
		} else {
			h.flash(w, r, "ok", h.T(r, "Язык обновлён"))
		}
		http.Redirect(w, r, "/profile", http.StatusFound)
		return

	case "theme":
		theme := trimStr(r.FormValue("theme"))
		valid := map[string]bool{"dark": true, "light": true, "blue": true, "auto": true}
		if !valid[theme] {
			theme = "dark"
		}
		if err := appdb.UpdateUserTheme(h.db, si.UserID, theme); err != nil {
			h.flash(w, r, "err", h.T(r, "Ошибка сохранения темы"))
		} else {
			h.flash(w, r, "ok", h.T(r, "Тема обновлена"))
		}
		http.Redirect(w, r, "/profile", http.StatusFound)
		return

	case "update_info":
		username := trimStr(r.FormValue("username"))
		displayName := trimStr(r.FormValue("display_name"))
		email := trimStr(r.FormValue("email"))
		if username == "" {
			h.flash(w, r, "err", h.T(r, "Логин не может быть пустым"))
			http.Redirect(w, r, "/profile", http.StatusFound)
			return
		}
		// Проверим что логин не занят другим пользователем
		exists, _ := appdb.UsernameExistsExceptID(h.db, username, si.UserID)
		if exists {
			h.flash(w, r, "err", h.T(r, "Пользователь с таким логином уже существует"))
			http.Redirect(w, r, "/profile", http.StatusFound)
			return
		}
		if err := appdb.UpdateUserInfo(h.db, si.UserID, username, displayName, email); err != nil {
			h.flash(w, r, "err", h.T(r, "Ошибка при обновлении: ")+err.Error())
			http.Redirect(w, r, "/profile", http.StatusFound)
			return
		}
		// Обновляем username в сессии
		s := h.sess(r)
		s.Values["username"] = username
		s.Save(r, w)
		h.flash(w, r, "ok", h.T(r, "Профиль обновлён"))
		http.Redirect(w, r, "/profile", http.StatusFound)
		return

	case "change_password", "":
		current := r.FormValue("current_password")
		newPW := trimStr(r.FormValue("new_password"))
		confirm := trimStr(r.FormValue("confirm_password"))

		u, _ := appdb.GetUserByID(h.db, si.UserID)
		if u == nil || !security.VerifyPassword(current, u.PasswordHash) {
			h.flash(w, r, "err", h.T(r, "Неверный текущий пароль"))
			http.Redirect(w, r, "/profile", http.StatusFound)
			return
		}
		if newPW != confirm {
			h.flash(w, r, "err", h.T(r, "Пароли не совпадают"))
			http.Redirect(w, r, "/profile", http.StatusFound)
			return
		}
		if ok, msg := security.ValidatePassword(newPW); !ok {
			h.flash(w, r, "err", h.T(r, msg))
			http.Redirect(w, r, "/profile", http.StatusFound)
			return
		}
		appdb.UpdateUserPassword(h.db, si.UserID, security.HashPassword(newPW))
		h.flash(w, r, "ok", h.T(r, "Пароль успешно изменён"))
		http.Redirect(w, r, "/profile", http.StatusFound)
		return
	}

	http.Redirect(w, r, "/profile", http.StatusFound)
}
