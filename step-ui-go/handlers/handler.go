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
	"step-ui/models"
	"step-ui/security"
)

var StartedAt time.Time

// Версионирование — переопределяется через ldflags при сборке
var (
	Version   = "1.2.0"
	BuildDate = "2026-04-18"
	GitCommit = "3f02595"
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
		"dashboard", "certificates", "issue", "import",
		"provisioners", "history", "users", "user_profile",
		"profile", "security_log",
		"le_dashboard", "le_issue", "le_settings", "le_logs",
	}
	for _, page := range pages {
		t, err := template.New("base.html").Funcs(funcs).ParseFiles(
			"templates/base.html",
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
			return t.Format("2006-01-02 15:04")
		},
		"fmtLog": func(t time.Time) string {
			return t.Format("2006-01-02 15:04:05")
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
		"add":  func(a, b int) int { return a + b },
		"sub":  func(a, b int) int { return a - b },
		"deref": func(s *string) string {
			if s == nil { return "—" }
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
	return &models.SessionInfo{UserID: id, Username: username, Role: role}
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
	}
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("render %s: %v", page, err)
	}
}
