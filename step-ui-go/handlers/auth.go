package handlers

import (
	"fmt"
	"net/http"
	"time"

	appdb "step-ui/db"
	"step-ui/security"
)

func (h *Handler) LoginGet(w http.ResponseWriter, r *http.Request) {
	ip := r.RemoteAddr
	data := h.base(w, r, "")
	if security.RL.IsBlocked(ip) {
		data["Error"] = "Слишком много попыток. Подождите 15 минут."
		data["Blocked"] = true
	}
	h.render(w, "login", data)
}

func (h *Handler) LoginPost(w http.ResponseWriter, r *http.Request) {
	ip := r.RemoteAddr

	if security.RL.IsBlocked(ip) {
		data := h.base(w, r, "")
		data["Error"] = "Слишком много попыток. Подождите 15 минут."
		data["Blocked"] = true
		h.render(w, "login", data)
		return
	}

	if !h.csrfOK(r) {
		data := h.base(w, r, "")
		data["Error"] = "Ошибка сессии. Обновите страницу."
		h.render(w, "login", data)
		return
	}

	username := trimStr(r.FormValue("username"))
	password := r.FormValue("password")

	user, _ := appdb.GetUserByUsername(h.db, username)
	if user == nil || user.PasswordHash != security.HashPassword(password) {
		security.RL.Register(ip)
		left := security.RL.Left(ip)
		appdb.LogAuth(h.db, username, ip, false, fmt.Sprintf("Неверный пароль (%d попыток осталось)", left))
		if left > 0 {
			h.flash(w, r, "err", fmt.Sprintf("Неверный логин или пароль. Осталось попыток: %d", left))
		} else {
			h.flash(w, r, "err", "Слишком много попыток. Подождите 15 минут.")
		}
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	if !user.IsActive {
		appdb.LogAuth(h.db, username, ip, false, "Аккаунт заблокирован")
		h.flash(w, r, "err", "Аккаунт заблокирован. Обратитесь к администратору.")
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	security.RL.Clear(ip)
	s := h.sess(r)
	s.Values = map[interface{}]interface{}{}
	s.Values["user_id"] = user.ID
	s.Values["username"] = user.Username
	s.Values["role"] = user.Role
	s.Values["last_activity"] = time.Now().Unix()
	s.Values["csrf_token"] = security.GenerateToken()
	s.Save(r, w)
	appdb.LogAuth(h.db, username, ip, true, "")
	http.Redirect(w, r, "/", http.StatusFound)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	si := h.sessionInfo(r)
	if si.UserID != 0 {
		appdb.LogAuth(h.db, si.Username, r.RemoteAddr, true, "Выход")
	}
	s := h.sess(r)
	s.Values = map[interface{}]interface{}{}
	s.Options.MaxAge = -1
	s.Save(r, w)
	http.Redirect(w, r, "/login", http.StatusFound)
}
