package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/sessions"
	"step-ui/config"
	appdb "step-ui/db"
	"step-ui/i18n"
	"step-ui/models"
	"step-ui/security"
)

var StartedAt time.Time

// Версионирование — переопределяется через ldflags при сборке
var (
	Version   = "1.7.0"
	BuildDate = "2026-06-02"
	GitCommit = "unknown"
)

type Handler struct {
	db    *sql.DB
	cfg   *config.Config
	store *sessions.CookieStore
	tmpls map[string]*template.Template
}

func New(db *sql.DB, cfg *config.Config, store *sessions.CookieStore) *Handler {
	h := &Handler{db: db, cfg: cfg, store: store, tmpls: make(map[string]*template.Template)}
	h.loadTemplates()
	return h
}

func (h *Handler) loadTemplates() {
	funcs := h.templateFuncs()
	pages := []string{
		"home",
		"dashboard",
		"certificates",
		"certificate_detail",
		"issue",
		"import",
		"provisioners",
		"history",
		"admin",
		"profile",
		"le_dashboard",
		"le_issue",
		"le_settings",
		"le_logs",
		"admin_users",
		"admin_user_profile",
		"admin_users_temp",
		"admin_activity",
		"admin_security",
		"admin_console",
		"admin_about",
		"admin_integrity",
		"admin_backup",
		"admin_notifications",
		"profile_2fa",
	}
	for _, page := range pages {
		baseFile := "templates/base.html"
		if len(page) >= 6 && page[:6] == "admin_" || page == "admin" {
			baseFile = "templates/admin_base.html"
		}
		t, err := template.New("base.html").Funcs(funcs).ParseFiles(
			baseFile,
			fmt.Sprintf("templates/%s.html", page),
		)
		if err != nil {
			log.Printf("template error (%s): %v", page, err)
			continue
		}
		h.tmpls[page] = t
	}
	for _, page := range []string{"login", "forgot_password", "reset_password"} {
		file := fmt.Sprintf("templates/%s.html", page)
		if t, err := template.New(fmt.Sprintf("%s.html", page)).Funcs(funcs).ParseFiles(file); err == nil {
			h.tmpls[page] = t
		} else {
			log.Printf("%s template error: %v", page, err)
		}
	}
}

func (h *Handler) templateFuncs() template.FuncMap {
	return template.FuncMap{
		"daysLeft": func(t *time.Time) int {
			if t == nil {
				return 999
			}
			return int(time.Until(*t).Hours() / 24)
		},
		"badgeClass": func(t *time.Time) string {
			if t == nil {
				return "ok"
			}
			d := int(time.Until(*t).Hours() / 24)
			if d <= 0 {
				return "danger"
			}
			if d <= 30 {
				return "warn"
			}
			return "ok"
		},
		"fmtTime": func(t *time.Time) string {
			if t == nil {
				return "—"
			}
			return t.Local().Format("2006-01-02 15:04")
		},
		"fmtLog": func(t time.Time) string {
			return t.Local().Format("2006-01-02 15:04:05")
		},
		"hasRole": func(role, minRole string) bool {
			levels := map[string]int{"viewer": 1, "manager": 2, "admin": 3}
			return levels[role] >= levels[minRole]
		},
		"isActive": func(page, current string) string {
			if page == current {
				return "active"
			}
			return ""
		},
		"add": func(a, b int) int { return a + b },
		"sub": func(a, b int) int { return a - b },
		"deref": func(s *string) string {
			if s == nil {
				return "—"
			}
			return *s
		},
		"contains": func(arr []string, v string) bool {
			for _, s := range arr {
				if s == v {
					return true
				}
			}
			return false
		},
		"seq": func(start, end int) []int {
			var s []int
			for i := start; i <= end; i++ {
				s = append(s, i)
			}
			return s
		},
		"tr": func(lang, msg string) string {
			return i18n.T(lang, msg)
		},
		"trf": func(lang, format string, args ...interface{}) string {
			return i18n.Tf(lang, format, args...)
		},
		"securityEventLabel": securityEventLabel,
		"securityEventBadge": securityEventBadge,
	}
}

func (h *Handler) sess(r *http.Request) *sessions.Session {
	s, err := h.store.Get(r, "step-ui")
	if err != nil {
		log.Printf("session decode failed: remote=%s host=%s path=%s err=%v", r.RemoteAddr, r.Host, r.URL.Path, err)
	}
	return s
}

func (h *Handler) sessionInfo(r *http.Request) *models.SessionInfo {
	s := h.sess(r)
	id, _ := s.Values["user_id"].(int)
	username, _ := s.Values["username"].(string)
	role, _ := s.Values["role"].(string)
	theme := "dark"
	userLang := ""
	if id > 0 {
		if u, err := appdb.GetUserByID(h.db, id); err == nil && u != nil {
			if u.Theme != "" {
				theme = u.Theme
			}
			userLang = u.Language
		}
	}
	lang := i18n.Resolve(r, userLang)
	return &models.SessionInfo{UserID: id, Username: username, Role: role, Theme: theme, Language: lang}
}

func (h *Handler) T(r *http.Request, msg string) string {
	return i18n.T(h.sessionInfo(r).Language, msg)
}

func (h *Handler) Tf(r *http.Request, format string, args ...interface{}) string {
	return i18n.Tf(h.sessionInfo(r).Language, format, args...)
}

// SetLanguage speichert Sprache in Cookie (und optional in User-Profil).
func (h *Handler) SetLanguage(w http.ResponseWriter, r *http.Request) {
	lang := i18n.Normalize(r.FormValue("lang"))
	if lang == "" {
		lang = i18n.Normalize(r.URL.Query().Get("lang"))
	}
	i18n.SetCookie(w, lang)
	si := h.sessionInfo(r)
	if si.UserID > 0 {
		_ = appdb.UpdateUserLanguage(h.db, si.UserID, lang)
	}
	next := r.FormValue("next")
	if next == "" {
		next = r.Header.Get("Referer")
	}
	if next == "" {
		next = "/"
	}
	if strings.HasPrefix(next, "http://") || strings.HasPrefix(next, "https://") {
		if u, err := url.Parse(next); err == nil && u.Path != "" {
			next = u.Path
			if u.RawQuery != "" {
				next += "?" + u.RawQuery
			}
		} else {
			next = "/"
		}
	}
	if !strings.HasPrefix(next, "/") {
		next = "/"
	}
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func (h *Handler) flash(w http.ResponseWriter, r *http.Request, t, text string) {
	s := h.sess(r)
	s.AddFlash(models.FlashMsg{Type: t, Text: text})
	if err := s.Save(r, w); err != nil {
		log.Printf("flash save failed: %v", err)
	}
}

func (h *Handler) popFlash(w http.ResponseWriter, r *http.Request) []models.FlashMsg {
	s := h.sess(r)
	flashes := s.Flashes()
	if err := s.Save(r, w); err != nil {
		log.Printf("flash pop save failed: %v", err)
	}
	var msgs []models.FlashMsg
	for _, f := range flashes {
		if m, ok := f.(models.FlashMsg); ok {
			msgs = append(msgs, m)
		} else {
			log.Printf("flash type assert failed: %T", f)
		}
	}
	return msgs
}

func (h *Handler) csrf(w http.ResponseWriter, r *http.Request) string {
	s, err := h.store.Get(r, "step-ui")
	if err != nil {
		log.Printf("session reset after decode failure: remote=%s host=%s path=%s err=%v", r.RemoteAddr, r.Host, r.URL.Path, err)
		s, _ = h.store.New(r, "step-ui")
	}
	token, ok := s.Values["csrf_token"].(string)
	if !ok || token == "" {
		token = security.GenerateToken()
		s.Values["csrf_token"] = token
		s.Save(r, w)
	}
	return token
}

func (h *Handler) csrfOK(r *http.Request) bool {
	s := h.sess(r)
	token := r.FormValue("csrf_token")
	sess, _ := s.Values["csrf_token"].(string)
	return token != "" && token == sess
}

func (h *Handler) requireCSRF(w http.ResponseWriter, r *http.Request, redirectTo string) bool {
	if h.csrfOK(r) {
		return true
	}
	h.flash(w, r, "err", h.T(r, "Ошибка сессии. Обновите страницу."))
	http.Redirect(w, r, redirectTo, http.StatusSeeOther)
	return false
}

func (h *Handler) base(w http.ResponseWriter, r *http.Request, activePage string) map[string]interface{} {
	si := h.sessionInfo(r)
	jsKeys := []string{
		"Январь", "Февраль", "Март", "Апрель", "Май", "Июнь",
		"Июль", "Август", "Сентябрь", "Октябрь", "Ноябрь", "Декабрь",
		"пн", "вт", "ср", "чт", "пт", "сб", "вс",
		"Выберите дату и время", "Предыдущий месяц", "Следующий месяц",
		"Время:", "Очистить", "Готово", "Укажите часы и минуты",
		"чч", "мм",
		"Подтвердите действие", "Отмена", "Подтвердить",
		"Тёмная", "Светлая", "Синяя", "Авто (системная)",
	}
	jsJSON, err := json.Marshal(i18n.CatalogForJS(si.Language, jsKeys))
	if err != nil || len(jsJSON) == 0 {
		jsJSON = []byte("{}")
	}
	return map[string]interface{}{
		"Session":    si,
		"Lang":       si.Language,
		"Msgs":       h.popFlash(w, r),
		"ActivePage": activePage,
		"CSRFToken":  h.csrf(w, r),
		"I18NJSON":   template.JS(jsJSON),
		"CAOnline":   h.caIsOnline(),
	}
}

func (h *Handler) render(w http.ResponseWriter, page string, data map[string]interface{}) {
	tmpl, ok := h.tmpls[page]
	if !ok {
		http.Error(w, "template not found: "+page, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	name := "layout"
	if page == "login" || page == "forgot_password" || page == "reset_password" {
		name = page + ".html"
	} else if page == "admin" || (len(page) >= 6 && page[:6] == "admin_") {
		name = "admin_layout"
	}
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("render %s: %v", page, err)
	}
}
