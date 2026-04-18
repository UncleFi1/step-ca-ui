package main

import (
	"time"
	"encoding/gob"
	"crypto/sha256"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/sessions"

	"step-ui/config"
	"step-ui/db"
	"step-ui/handlers"
	"step-ui/le"
	mw "step-ui/middleware"
)

func main() {
	handlers.StartedAt = time.Now()
	// Регистрируем типы для gob (gorilla/sessions)
	gob.Register(int(0))
	gob.Register(int64(0))
	gob.Register("")
	cfg := config.Load()

	// ─── Database ────────────────────────────────────────────────────────────
	conn, err := db.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Cannot connect to database: %v", err)
	}
	defer conn.Close()

	if err := db.InitSchema(conn); err != nil {
		log.Fatalf("Cannot init DB schema: %v", err)
	}
	if err := db.InitLESchema(conn); err != nil {
		log.Fatalf("Cannot init LE schema: %v", err)
	}

	// ─── Sessions ────────────────────────────────────────────────────────────
	hashKey := sha256.Sum256([]byte(cfg.SecretKey))
	blockKey := sha256.Sum256([]byte(cfg.SecretKey + "_block"))
	store := sessions.NewCookieStore(hashKey[:], blockKey[:16])
	store.Options = &sessions.Options{
		MaxAge:   28800,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   false,
	}

	// ─── Handlers ────────────────────────────────────────────────────────────
	h := handlers.New(conn, cfg, store)

	// ─── Let's Encrypt auto-renewer ──────────────────────────────────────────
	le.StartRenewer(conn)

	// ─── Router ──────────────────────────────────────────────────────────────
	r := chi.NewRouter()
	r.Use(chiMiddleware.Recoverer)
	r.Use(chiMiddleware.RealIP)
	r.Use(mw.SecurityHeaders)

	// Публичные маршруты
	r.Get("/login",  h.LoginGet)
	r.Post("/login", h.LoginPost)
	r.Get("/logout", h.Logout)

	// Авторизованные маршруты
	r.Group(func(r chi.Router) {
		r.Use(mw.RequireLogin(store))

		r.Get("/",            h.Dashboard)
		r.Get("/api/status",  h.APIStatus)

		// Сертификаты (viewer+)
		r.Get("/certificates", h.Certificates)
		r.Get("/history",      h.History)
		r.Get("/provisioners", h.Provisioners)

		// Скачать CA cert (admin)
		r.Group(func(r chi.Router) {
			r.Use(mw.RequireRole("admin", store))
			r.Get("/download/ca", h.DownloadCA)
		})

		// Операции с сертификатами (manager+)
		r.Group(func(r chi.Router) {
			r.Use(mw.RequireRole("manager", store))
			r.Get("/issue",              h.IssueGet)
			r.Post("/issue",             h.IssuePost)
			r.Get("/renew/{id}",         h.Renew)
			r.Get("/import",             h.ImportGet)
			r.Post("/import",            h.ImportPost)
			r.Get("/download/cert/{id}", h.DownloadCert)
			r.Get("/download/key/{id}",  h.DownloadKey)
		})

		// Отзыв (admin)
		r.Group(func(r chi.Router) {
			r.Use(mw.RequireRole("admin", store))
			r.Get("/revoke/{id}", h.Revoke)
		})

		// Управление пользователями (admin)
		r.Group(func(r chi.Router) {
			r.Use(mw.RequireRole("admin", store))
			r.Get("/users",        h.Users)
			r.Post("/users",       h.UsersPost)
			r.Get("/users/{id}",   h.UserProfile)
			r.Get("/security",     h.SecurityLog)
		})

		// Let's Encrypt (manager+)
		r.Group(func(r chi.Router) {
			r.Use(mw.RequireRole("manager", store))
			r.Get("/le",                      h.LEDashboard)
			r.Get("/le/issue",                h.LEIssueGet)
			r.Post("/le/issue",               h.LEIssuePost)
			r.Post("/le/{id}/renew",          h.LERenew)
			r.Post("/le/{id}/delete",         h.LEDelete)
			r.Post("/le/{id}/autorenew",      h.LEToggleAutoRenew)
			r.Get("/le/download/cert/{id}",   h.LEDownloadCert)
			r.Get("/le/download/key/{id}",    h.LEDownloadKey)
			r.Get("/le/settings",             h.LESettingsGet)
			r.Post("/le/settings",            h.LESettingsPost)
			r.Get("/le/logs",                 h.LELogs)
		})

		// Профиль (любой авторизованный)
		r.Get("/profile",  h.ProfileGet)
		r.Post("/profile", h.ProfilePost)
	})

	// ─── Static files ─────────────────────────────────────────────────────────
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// ─── Start server ─────────────────────────────────────────────────────────
	for _, dir := range []string{cfg.CertsDir, cfg.UploadDir, "/opt/step-ui/ssl", "/opt/step-ui/data"} {
		os.MkdirAll(dir, 0755)
	}

	addr := fmt.Sprintf("0.0.0.0:%d", cfg.Port)
	if _, err := os.Stat(cfg.SSLCert); err == nil {
		tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
		srv := &http.Server{Addr: addr, Handler: r, TLSConfig: tlsCfg}
		fmt.Printf("[*] Starting Step-CA UI (HTTPS) on port %d\n", cfg.Port)
		log.Fatal(srv.ListenAndServeTLS(cfg.SSLCert, cfg.SSLKey))
	} else {
		fmt.Printf("[!] SSL cert not found, starting HTTP on port %d\n", cfg.Port)
		log.Fatal(http.ListenAndServe(addr, r))
	}
}
