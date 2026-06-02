package handlers

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/smtp"
	"net/url"
	"strings"
	"sync"
	"time"

	appdb "step-ui/db"
	"step-ui/security"
)

const (
	passwordResetTTL         = 30 * time.Minute
	passwordResetLimitCount  = 3
	passwordResetLimitWindow = 15 * time.Minute
)

var passwordResetRL = struct {
	sync.Mutex
	attempts map[string][]time.Time
}{attempts: make(map[string][]time.Time)}

func (h *Handler) ForgotPasswordGet(w http.ResponseWriter, r *http.Request) {
	h.render(w, "forgot_password", h.base(w, r, ""))
}

func (h *Handler) ForgotPasswordPost(w http.ResponseWriter, r *http.Request) {
	data := h.base(w, r, "")
	if !h.csrfOK(r) {
		data["Error"] = "Ошибка сессии. Обновите страницу."
		h.render(w, "forgot_password", data)
		return
	}
	ip := clientIP(r)
	if !passwordResetAllowed(ip) {
		data["Info"] = "Если аккаунт найден и email настроен, ссылка для сброса будет отправлена."
		_ = appdb.LogAuth(h.db, "password-reset", ip, false, "Password reset rate limited")
		h.render(w, "forgot_password", data)
		return
	}
	identifier := trimStr(r.FormValue("identifier"))
	if identifier == "" {
		data["Error"] = "Укажите логин или email."
		h.render(w, "forgot_password", data)
		return
	}

	generic := "Если аккаунт найден и email настроен, ссылка для сброса будет отправлена."
	user, err := appdb.GetUserByLoginOrEmail(h.db, identifier)
	if err != nil || user == nil {
		_ = appdb.LogAuth(h.db, identifier, ip, false, "Password reset requested for unknown account")
		data["Info"] = generic
		h.render(w, "forgot_password", data)
		return
	}
	if !user.IsActive {
		_ = appdb.LogAuth(h.db, user.Username, ip, false, "Password reset requested for inactive account")
		data["Info"] = generic
		h.render(w, "forgot_password", data)
		return
	}
	if strings.TrimSpace(user.Email) == "" {
		_ = appdb.LogAuth(h.db, user.Username, ip, false, "Password reset requested but user email is empty")
		data["Info"] = generic
		h.render(w, "forgot_password", data)
		return
	}

	settings, err := appdb.GetNotificationSettings(h.db)
	if err != nil || !settings.SMTPEnabled || strings.TrimSpace(settings.SMTPHost) == "" || strings.TrimSpace(settings.SMTPFrom) == "" {
		_ = appdb.LogAuth(h.db, user.Username, ip, false, "Password reset requested but SMTP is not configured")
		data["Info"] = generic
		h.render(w, "forgot_password", data)
		return
	}

	rawToken := security.GenerateToken()
	tokenHash := passwordResetTokenHash(rawToken)
	_ = appdb.InvalidatePasswordResetTokens(h.db, user.ID)
	if err := appdb.CreatePasswordResetToken(h.db, user.ID, tokenHash, ip, time.Now().Add(passwordResetTTL)); err != nil {
		_ = appdb.LogAuth(h.db, user.Username, ip, false, "Password reset token creation failed")
		data["Info"] = generic
		h.render(w, "forgot_password", data)
		return
	}
	link := h.absoluteURL(r, "/reset-password?token="+url.QueryEscape(rawToken))
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := sendPasswordResetMail(ctx, settings.SMTPHost, settings.SMTPPort, settings.SMTPSecurity,
		settings.SMTPUsername, settings.SMTPPassword, settings.SMTPFrom, user.Email, link); err != nil {
		_ = appdb.LogAuth(h.db, user.Username, ip, false, "Password reset email failed: "+err.Error())
	} else {
		_ = appdb.LogAuth(h.db, user.Username, ip, true, "Password reset email sent")
	}
	data["Info"] = generic
	h.render(w, "forgot_password", data)
}

func (h *Handler) ResetPasswordGet(w http.ResponseWriter, r *http.Request) {
	data := h.base(w, r, "")
	token := trimStr(r.URL.Query().Get("token"))
	if !h.passwordResetTokenOK(token) {
		data["Error"] = "Ссылка сброса недействительна или истекла."
	} else {
		data["Token"] = token
	}
	h.render(w, "reset_password", data)
}

func (h *Handler) ResetPasswordPost(w http.ResponseWriter, r *http.Request) {
	data := h.base(w, r, "")
	if !h.csrfOK(r) {
		data["Error"] = "Ошибка сессии. Обновите страницу."
		h.render(w, "reset_password", data)
		return
	}
	token := trimStr(r.FormValue("token"))
	resetToken, err := appdb.GetValidPasswordResetToken(h.db, passwordResetTokenHash(token))
	if err != nil || resetToken == nil {
		data["Error"] = "Ссылка сброса недействительна или истекла."
		h.render(w, "reset_password", data)
		return
	}
	newPW := trimStr(r.FormValue("new_password"))
	confirm := trimStr(r.FormValue("confirm_password"))
	if newPW != confirm {
		data["Error"] = "Пароли не совпадают."
		data["Token"] = token
		h.render(w, "reset_password", data)
		return
	}
	if ok, msg := security.ValidatePassword(newPW); !ok {
		data["Error"] = msg
		data["Token"] = token
		h.render(w, "reset_password", data)
		return
	}
	user, err := appdb.GetUserByID(h.db, resetToken.UserID)
	if err != nil || user == nil || !user.IsActive {
		data["Error"] = "Аккаунт недоступен."
		h.render(w, "reset_password", data)
		return
	}
	if err := appdb.UpdateUserPassword(h.db, user.ID, security.HashPassword(newPW)); err != nil {
		data["Error"] = "Не удалось обновить пароль."
		data["Token"] = token
		h.render(w, "reset_password", data)
		return
	}
	_ = appdb.MarkPasswordResetTokenUsed(h.db, resetToken.ID)
	_ = appdb.InvalidatePasswordResetTokens(h.db, user.ID)
	_ = appdb.LogAuth(h.db, user.Username, clientIP(r), true, "Password reset completed")
	h.flash(w, r, "ok", "Пароль обновлён. Войдите с новым паролем.")
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (h *Handler) passwordResetTokenOK(token string) bool {
	if token == "" {
		return false
	}
	t, err := appdb.GetValidPasswordResetToken(h.db, passwordResetTokenHash(token))
	return err == nil && t != nil
}

func passwordResetTokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func passwordResetAllowed(ip string) bool {
	passwordResetRL.Lock()
	defer passwordResetRL.Unlock()
	now := time.Now()
	var fresh []time.Time
	for _, t := range passwordResetRL.attempts[ip] {
		if now.Sub(t) < passwordResetLimitWindow {
			fresh = append(fresh, t)
		}
	}
	if len(fresh) >= passwordResetLimitCount {
		passwordResetRL.attempts[ip] = fresh
		return false
	}
	fresh = append(fresh, now)
	passwordResetRL.attempts[ip] = fresh
	return true
}

func clientIP(r *http.Request) string {
	if ip := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); ip != "" {
		return strings.TrimSpace(strings.Split(ip, ",")[0])
	}
	if ip := strings.TrimSpace(r.Header.Get("X-Real-IP")); ip != "" {
		return ip
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func (h *Handler) absoluteURL(r *http.Request, path string) string {
	scheme := "https"
	if proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); proto != "" {
		scheme = proto
	} else if r.TLS == nil {
		scheme = "http"
	}
	return scheme + "://" + r.Host + path
}

func sendPasswordResetMail(ctx context.Context, host string, port int, securityMode, username, password, from, to, link string) error {
	if port <= 0 {
		port = 587
	}
	addr := fmt.Sprintf("%s:%d", host, port)
	subject := "Step-CA UI password reset"
	body := fmt.Sprintf("Password reset was requested for your Step-CA UI account.\r\n\r\nOpen this link within 30 minutes:\r\n%s\r\n\r\nIf you did not request this, ignore this email.\r\n", link)
	msg := []byte("From: " + from + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"\r\n" + body)

	var auth smtp.Auth
	if username != "" {
		auth = smtp.PlainAuth("", username, password, host)
	}
	mode := strings.ToLower(strings.TrimSpace(securityMode))
	if mode == "" {
		mode = "starttls"
	}
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	if mode == "tls" {
		tlsConn := tls.Client(conn, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return err
		}
		return sendSMTP(tlsConn, host, from, []string{to}, msg, auth)
	}
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer client.Close()
	if mode == "starttls" {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("server does not advertise STARTTLS")
		}
	}
	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		_ = w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func sendSMTP(conn net.Conn, host, from string, to []string, msg []byte, auth smtp.Auth) error {
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer client.Close()
	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	for _, rcpt := range to {
		if err := client.Rcpt(rcpt); err != nil {
			return err
		}
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		_ = w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}
