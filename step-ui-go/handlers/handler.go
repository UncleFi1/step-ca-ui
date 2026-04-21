package handlers

import (
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/sessions"
	"step-ui/config"
	appdb "step-ui/db"
	"step-ui/models"
	"step-ui/security"
)

var StartedAt time.Time

// Версионирование — переопределяется через ldflags при сборке
var (
	Version   = "1.4.4"
	BuildDate = "2026-04-21"
	GitCommit = "4bf0aa0"
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
	if t, err := template.New("login.html").Funcs(funcs).ParseFiles("templates/login.html"); err == nil {
		h.tmpls["login"] = t
	} else {
		log.Printf("login template error: %v", err)
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
	}
}

func (h *Handler) sess(r *http.Request) *sessions.Session {
	s, _ := h.store.Get(r, "step-ui")
	return s
}

func (h *Handler) sessionInfo(r *http.Request) *models.SessionInfo {
	s := h.sess(r)
	id, _ := s.Values["user_id"].(int)
	username, _ := s.Values["username"].(string)
	role, _ := s.Values["role"].(string)
	theme := "dark"
	if id > 0 {
		if u, err := appdb.GetUserByID(h.db, id); err == nil && u != nil && u.Theme != "" {
			theme = u.Theme
		}
	}
	return &models.SessionInfo{UserID: id, Username: username, Role: role, Theme: theme}
}

func (h *Handler) flash(w http.ResponseWriter, r *http.Request, t, text string) {
	s := h.sess(r)
	s.AddFlash(models.FlashMsg{Type: t, Text: text})
	s.Save(r, w)
}

func (h *Handler) popFlash(w http.ResponseWriter, r *http.Request) []models.FlashMsg {
	s := h.sess(r)
	flashes := s.Flashes()
	s.Save(r, w)
	var msgs []models.FlashMsg
	for _, f := range flashes {
		if m, ok := f.(models.FlashMsg); ok {
			msgs = append(msgs, m)
		}
	}
	return msgs
}

func (h *Handler) csrf(w http.ResponseWriter, r *http.Request) string {
	s := h.sess(r)
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

func (h *Handler) base(w http.ResponseWriter, r *http.Request, activePage string) map[string]interface{} {
	return map[string]interface{}{
		"Session":    h.sessionInfo(r),
		"Msgs":       h.popFlash(w, r),
		"ActivePage": activePage,
		"CSRFToken":  h.csrf(w, r),
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
	if page == "login" {
		name = "login.html"
	} else if page == "admin" || (len(page) >= 6 && page[:6] == "admin_") {
		name = "admin_layout"
	}
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("render %s: %v", page, err)
	}
}
