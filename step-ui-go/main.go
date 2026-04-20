package main

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/gob"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/sessions"

	"io"
	"mime"
	"path/filepath"
	"step-ui/config"
	appdb "step-ui/db"
	"step-ui/handlers"
	"step-ui/le"
	mw "step-ui/middleware"
	"strings"
)

// staticHandlerWithMIME раздаёт статические файлы с ПРАВИЛЬНЫМ Content-Type.
// Не использует http.FileServer/ServeContent, чтобы те не перезаписали MIME
// значениями из системного /etc/mime.types (где .css может быть text/plain).
func staticHandlerWithMIME(rootDir string) http.Handler {
	mimeByExt := map[string]string{
		".css":   "text/css; charset=utf-8",
		".js":    "application/javascript; charset=utf-8",
		".mjs":   "application/javascript; charset=utf-8",
		".json":  "application/json; charset=utf-8",
		".svg":   "image/svg+xml",
		".png":   "image/png",
		".jpg":   "image/jpeg",
		".jpeg":  "image/jpeg",
		".gif":   "image/gif",
		".webp":  "image/webp",
		".ico":   "image/x-icon",
		".woff":  "font/woff",
		".woff2": "font/woff2",
		".ttf":   "font/ttf",
		".otf":   "font/otf",
		".map":   "application/json; charset=utf-8",
		".html":  "text/html; charset=utf-8",
		".txt":   "text/plain; charset=utf-8",
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// безопасная склейка пути
		clean := filepath.Clean("/" + r.URL.Path)
		full := filepath.Join(rootDir, clean)
		// защита от path traversal
		absRoot, _ := filepath.Abs(rootDir)
		absFile, _ := filepath.Abs(full)
		if !strings.HasPrefix(absFile, absRoot) {
			http.NotFound(w, r)
			return
		}
		f, err := os.Open(full)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		st, err := f.Stat()
		if err != nil || st.IsDir() {
			http.NotFound(w, r)
			return
		}
		ext := strings.ToLower(filepath.Ext(full))
		if mt, ok := mimeByExt[ext]; ok {
			w.Header().Set("Content-Type", mt)
		} else {
			w.Header().Set("Content-Type", "application/octet-stream")
		}
		w.Header().Set("Cache-Control", "public, max-age=3600")
		io.Copy(w, f)
	})
}

func init() {
	// Принудительно регистрируем корректные MIME-типы для статики.
	// http.ServeContent использует mime.TypeByExtension() и перезаписывает
	// любой ранее установленный Content-Type, поэтому только этот способ работает.
	mime.AddExtensionType(".css", "text/css; charset=utf-8")
	mime.AddExtensionType(".js", "application/javascript; charset=utf-8")
	mime.AddExtensionType(".mjs", "application/javascript; charset=utf-8")
	mime.AddExtensionType(".json", "application/json; charset=utf-8")
	mime.AddExtensionType(".svg", "image/svg+xml")
	mime.AddExtensionType(".webp", "image/webp")
	mime.AddExtensionType(".woff", "font/woff")
	mime.AddExtensionType(".woff2", "font/woff2")
	mime.AddExtensionType(".ttf", "font/ttf")
	mime.AddExtensionType(".otf", "font/otf")
	mime.AddExtensionType(".map", "application/json; charset=utf-8")
}

func main() {
	handlers.StartedAt = time.Now()
	// Регистрируем типы для gob (gorilla/sessions)
	gob.Register(int(0))
	gob.Register(int64(0))
	gob.Register("")
	cfg := config.Load()

	// ─── Database ────────────────────────────────────────────────────────────
	conn, err := appdb.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Cannot connect to database: %v", err)
	}
	defer conn.Close()

	if err := appdb.InitSchema(conn); err != nil {
		log.Fatalf("Cannot init DB schema: %v", err)
	}
	if err := appdb.InitLESchema(conn); err != nil {
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
	r.Get("/login", h.LoginGet)
	r.Post("/login", h.LoginPost)
	r.Get("/logout", h.Logout)

	// Авторизованные маршруты
	r.Group(func(r chi.Router) {
		r.Use(mw.RequireLogin(store))

		r.Get("/", h.Home)
		r.Get("/dashboard", h.Dashboard)
		r.Get("/api/status", h.APIStatus)

		// Сертификаты (viewer+)
		r.Get("/certificates", h.Certificates)
		r.Get("/history", h.History)
		r.Get("/provisioners", h.Provisioners)

		// Скачать CA cert (admin)
		r.Group(func(r chi.Router) {
			r.Use(mw.RequireRole("admin", store))
			r.Get("/download/ca", h.DownloadCA)
		})

		// Операции с сертификатами (manager+)
		r.Group(func(r chi.Router) {
			r.Use(mw.RequireRole("manager", store))
			r.Get("/issue", h.IssueGet)
			r.Post("/issue", h.IssuePost)
			r.Get("/renew/{id}", h.Renew)
			r.Get("/import", h.ImportGet)
			r.Post("/import", h.ImportPost)
			r.Get("/download/cert/{id}", h.DownloadCert)
			r.Get("/download/key/{id}", h.DownloadKey)
		})

		// Отзыв (admin)
		r.Group(func(r chi.Router) {
			r.Use(mw.RequireRole("admin", store))
			r.Get("/revoke/{id}", h.Revoke)
		})

		// Управление пользователями (admin)
		r.Group(func(r chi.Router) {
			r.Use(mw.RequireRole("admin", store))
			// Админ-пространство
			r.Get("/admin", h.AdminGet)
			r.Get("/admin/users", h.Users)
			r.Post("/admin/users", h.UsersPost)
			r.Get("/admin/users/{id}", h.UserProfile)
			r.Get("/admin/users-temp", h.AdminUsersTempGet)
			r.Post("/admin/users-temp", h.AdminUsersTempPost)
			r.Get("/admin/activity", h.AdminActivityGet)
			r.Get("/admin/security", h.SecurityLog)
			r.Get("/admin/console", h.AdminConsoleGet)
			r.Get("/admin/about", h.AdminAboutGet)
		})

		// Let's Encrypt (manager+)
		r.Group(func(r chi.Router) {
			r.Use(mw.RequireRole("manager", store))
			r.Get("/le", h.LEDashboard)
			r.Get("/le/issue", h.LEIssueGet)
			r.Post("/le/issue", h.LEIssuePost)
			r.Post("/le/{id}/renew", h.LERenew)
			r.Post("/le/{id}/delete", h.LEDelete)
			r.Post("/le/{id}/autorenew", h.LEToggleAutoRenew)
			r.Get("/le/download/cert/{id}", h.LEDownloadCert)
			r.Get("/le/download/key/{id}", h.LEDownloadKey)
			r.Get("/le/settings", h.LESettingsGet)
			r.Post("/le/settings", h.LESettingsPost)
			r.Get("/le/logs", h.LELogs)
		})

		// Профиль (любой авторизованный)
		r.Get("/profile", h.ProfileGet)
		r.Post("/profile", h.ProfilePost)
	})

	// ─── Static files ─────────────────────────────────────────────────────────
	r.Handle("/static/*", http.StripPrefix("/static/", staticHandlerWithMIME("static")))
	// ─── Start server ─────────────────────────────────────────────────────────
	for _, dir := range []string{cfg.CertsDir, cfg.UploadDir, "/opt/step-ui/ssl", "/opt/step-ui/data"} {
		os.MkdirAll(dir, 0755)
	}

	// // temp_users_expire_ticker
	go func() {
		t := time.NewTicker(60 * time.Second)
		defer t.Stop()
		for range t.C {
			if n, err := appdb.ExpireOverdueTempUsers(conn); err == nil && n > 0 {
				log.Printf("temp-users: expired %d account(s)", n)
			}
		}
	}()

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
