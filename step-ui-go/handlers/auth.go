package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/pquerna/otp/totp"
	appdb "step-ui/db"
	"step-ui/models"
	"step-ui/security"
)

func (h *Handler) LoginGet(w http.ResponseWriter, r *http.Request) {
	ip := r.RemoteAddr
	data := h.base(w, r, "")
	if h.pending2FAUserID(r) > 0 {
		data["NeedTOTP"] = true
	}
	if security.RL.IsBlocked(ip) {
		data["Error"] = h.T(r, "Слишком много попыток. Подождите 15 минут.")
		data["Blocked"] = true
	}
	h.render(w, "login", data)
}

func (h *Handler) LoginPost(w http.ResponseWriter, r *http.Request) {
	ip := r.RemoteAddr

	if security.RL.IsBlocked(ip) {
		data := h.base(w, r, "")
		data["Error"] = h.T(r, "Слишком много попыток. Подождите 15 минут.")
		data["Blocked"] = true
		h.render(w, "login", data)
		return
	}

	if !h.csrfOK(r) {
		data := h.base(w, r, "")
		data["Error"] = h.T(r, "Ошибка сессии. Обновите страницу.")
		h.render(w, "login", data)
		return
	}

	if uid := h.pending2FAUserID(r); uid > 0 {
		h.loginPost2FA(w, r, uid)
		return
	}

	username := trimStr(r.FormValue("username"))
	password := r.FormValue("password")

	user, _ := appdb.GetUserByUsername(h.db, username)
	if user == nil || !security.VerifyPassword(password, user.PasswordHash) {
		security.RL.Register(ip)
		left := security.RL.Left(ip)
		appdb.LogAuth(h.db, username, ip, false, fmt.Sprintf("Неверный пароль (%d попыток осталось)", left))
		if left > 0 {
			h.flash(w, r, "err", h.Tf(r, "Неверный логин или пароль. Осталось попыток: %d", left))
		} else {
			h.notifyAsync("auth-burst:"+ip+":"+time.Now().Format("2006-01-02T15:04"), "auth.failed_burst", "warn",
				"Failed login burst",
				fmt.Sprintf("IP %s заблокирован после серии неудачных входов", ip),
				map[string]string{"username": username, "ip": ip})
			h.flash(w, r, "err", h.T(r, "Слишком много попыток. Подождите 15 минут."))
		}
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	if !user.IsActive {
		appdb.LogAuth(h.db, username, ip, false, "Аккаунт заблокирован")
		h.flash(w, r, "err", h.T(r, "Аккаунт заблокирован. Обратитесь к администратору."))
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	if security.NeedsPasswordRehash(user.PasswordHash) {
		appdb.UpdateUserPassword(h.db, user.ID, security.HashPassword(password))
	}

	if user.TOTPEnabled {
		s := h.sess(r)
		s.Values["pending_2fa_user_id"] = user.ID
		s.Values["pending_2fa_expires"] = time.Now().Add(totpPendingTTL).Unix()
		s.Save(r, w)
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	h.completeLogin(w, r, user, "")
	http.Redirect(w, r, "/", http.StatusFound)
}

func (h *Handler) loginPost2FA(w http.ResponseWriter, r *http.Request, uid int) {
	ip := r.RemoteAddr
	user, _ := appdb.GetUserByID(h.db, uid)
	if user == nil || !user.IsActive || !user.TOTPEnabled {
		h.clearPending2FA(w, r)
		h.flash(w, r, "err", h.T(r, "2FA сессия недействительна"))
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	code := r.FormValue("totp_code")
	recovery := r.FormValue("recovery_code")
	ok := totp.Validate(code, user.TOTPSecret)
	recoveryUsed := false
	if !ok && recovery != "" {
		ok = h.verifyRecoveryCode(user.ID, recovery)
		recoveryUsed = ok
	}
	if !ok {
		security.RL.Register(ip)
		left := security.RL.Left(ip)
		appdb.LogAuth(h.db, user.Username, ip, false, fmt.Sprintf("Неверный 2FA код (%d попыток осталось)", left))
		h.flash(w, r, "err", h.T(r, "Неверный 2FA или recovery код"))
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	reason := ""
	if recoveryUsed {
		reason = "Login with recovery code"
	}
	h.completeLogin(w, r, user, reason)
	http.Redirect(w, r, "/", http.StatusFound)
}

func (h *Handler) pending2FAUserID(r *http.Request) int {
	s := h.sess(r)
	rawID, ok := s.Values["pending_2fa_user_id"].(int)
	if !ok || rawID <= 0 {
		return 0
	}
	exp, _ := s.Values["pending_2fa_expires"].(int64)
	if exp == 0 {
		if expStr, ok := s.Values["pending_2fa_expires"].(string); ok {
			exp, _ = strconv.ParseInt(expStr, 10, 64)
		}
	}
	if exp > 0 && time.Now().Unix() > exp {
		return 0
	}
	return rawID
}

func (h *Handler) clearPending2FA(w http.ResponseWriter, r *http.Request) {
	s := h.sess(r)
	delete(s.Values, "pending_2fa_user_id")
	delete(s.Values, "pending_2fa_expires")
	s.Save(r, w)
}

func (h *Handler) completeLogin(w http.ResponseWriter, r *http.Request, user *models.User, reason string) {
	security.RL.Clear(r.RemoteAddr)
	s := h.sess(r)
	s.Values = map[interface{}]interface{}{}
	s.Values["user_id"] = user.ID
	s.Values["username"] = user.Username
	s.Values["role"] = user.Role
	s.Values["last_activity"] = time.Now().Unix()
	s.Values["csrf_token"] = security.GenerateToken()
	s.Save(r, w)
	appdb.LogAuth(h.db, user.Username, r.RemoteAddr, true, reason)
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
