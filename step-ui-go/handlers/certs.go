package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	appdb "step-ui/db"
	"step-ui/models"
)

func (h *Handler) Home(w http.ResponseWriter, r *http.Request) {
	// Статус CA: проверяем через step ca health
	caOnline := true
	_, err := exec.Command("step", "ca", "health",
		"--ca-url", h.cfg.CAURL,
		"--root", h.cfg.RootCert).Output()
	if err != nil {
		caOnline = false
	}

	// Быстрая статистика по активным сертификатам
	certs, _ := appdb.GetCerts(h.db, "active")
	var activeCount, expiringCount int
	for _, c := range certs {
		d := daysLeftVal(c.ExpiresAt)
		if d > 0 && d <= 30 {
			expiringCount++
		}
		if d > 0 {
			activeCount++
		}
	}

	var leCount int
	h.db.QueryRow("SELECT COUNT(*) FROM le_certificates WHERE status='active'").Scan(&leCount)

	data := h.base(w, r, "home")
	data["CAOnline"] = caOnline
	data["Uptime"] = fmtUptime(time.Since(StartedAt))
	data["ActiveCerts"] = activeCount
	data["ExpiringCerts"] = expiringCount
	data["LECerts"] = leCount
	data["Version"] = Version
	h.render(w, "home", data)
}

func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	certs, _ := appdb.GetCerts(h.db, "active")
	total := len(certs)
	okC, warnC, expC := 0, 0, 0
	for _, c := range certs {
		d := daysLeftVal(c.ExpiresAt)
		if d <= 0 {
			expC++
		} else if d <= 30 {
			warnC++
		} else {
			okC++
		}
	}

	// ── Активность CA за периоды ──
	act := map[string]map[string]int{
		"24h": dashCountActions(h.db, 24*time.Hour),
		"7d":  dashCountActions(h.db, 7*24*time.Hour),
		"30d": dashCountActions(h.db, 30*24*time.Hour),
	}

	// ── Общая статистика ──
	var allCerts, leCerts, usersCount int
	h.db.QueryRow("SELECT COUNT(*) FROM certificates").Scan(&allCerts)
	h.db.QueryRow("SELECT COUNT(*) FROM le_certificates").Scan(&leCerts)
	h.db.QueryRow("SELECT COUNT(*) FROM users WHERE is_active = true").Scan(&usersCount)

	// ── Аптайм сервера ──
	uptime := time.Since(StartedAt)

	data := h.base(w, r, "dash")
	data["Certs"] = certs
	data["Total"] = total
	data["OkC"] = okC
	data["WarnC"] = warnC
	data["ExpC"] = expC
	data["Activity"] = act
	data["AllCerts"] = allCerts
	data["LECerts"] = leCerts
	data["UsersCount"] = usersCount
	data["Uptime"] = fmtUptime(uptime)
	data["StartedAt"] = StartedAt.Format("2006-01-02 15:04")
	data["Version"] = Version
	data["BuildDate"] = BuildDate
	data["GitCommit"] = GitCommit
	h.render(w, "dashboard", data)
}

// ─── helper: считает действия по типам за последний период ──────────────────
func dashCountActions(db *sql.DB, since time.Duration) map[string]int {
	result := map[string]int{"issue": 0, "renew": 0, "revoke": 0, "import": 0, "total": 0}
	rows, err := db.Query(
		`SELECT action, COUNT(*) FROM cert_history WHERE created_at >= $1 GROUP BY action`,
		time.Now().Add(-since),
	)
	if err != nil {
		return result
	}
	defer rows.Close()
	for rows.Next() {
		var action string
		var count int
		if err := rows.Scan(&action, &count); err == nil {
			if _, ok := result[action]; ok {
				result[action] = count
			}
			result["total"] += count
		}
	}
	return result
}

// ─── helper: форматирует длительность ───────────────────────────────────────
func fmtUptime(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%dд %dч %dм", days, hours, mins)
	}
	if hours > 0 {
		return fmt.Sprintf("%dч %dм", hours, mins)
	}
	return fmt.Sprintf("%dм", mins)
}

func (h *Handler) Certificates(w http.ResponseWriter, r *http.Request) {
	certs, _ := appdb.GetCerts(h.db, "")
	data := h.base(w, r, "certs")
	data["Certs"] = certs
	h.render(w, "certificates", data)
}

func (h *Handler) IssueGet(w http.ResponseWriter, r *http.Request) {
	h.render(w, "issue", h.base(w, r, "issue"))
}

func (h *Handler) IssuePost(w http.ResponseWriter, r *http.Request) {
	si := h.sessionInfo(r)
	name := trimStr(r.FormValue("name"))
	domain := trimStr(r.FormValue("domain"))
	duration := r.FormValue("duration")
	keyType := r.FormValue("key_type")
	if duration == "" {
		duration = "8760h"
	}
	if keyType == "" {
		keyType = "EC:P-256"
	}
	data := h.base(w, r, "issue")
	if name == "" || domain == "" {
		data["Msgs"] = []models.FlashMsg{{Type: "err", Text: "Заполните все поля"}}
		h.render(w, "issue", data)
		return
	}
	certDir := filepath.Join(h.cfg.CertsDir, sanitizeName(name))
	os.MkdirAll(certDir, 0755)
	certPath := filepath.Join(certDir, "certificate.crt")
	keyPath := filepath.Join(certDir, "private.key")
	if err := issueCert(domain, certPath, keyPath, duration, keyType, h.cfg); err != nil {
		data["Msgs"] = []models.FlashMsg{{Type: "err", Text: "Ошибка: " + err.Error()}}
		h.render(w, "issue", data)
		return
	}
	issued, expires, serial, _ := parseCertDates(certPath)
	appdb.InsertCert(h.db, &models.Certificate{
		Name: name, Domain: domain, CertPath: certPath, KeyPath: keyPath,
		IssuedAt: issued, ExpiresAt: expires, Serial: serial, KeyType: keyType,
	})
	appdb.InsertHistory(h.db, "issue", name, domain, fmt.Sprintf("Тип: %s, срок: %s", keyType, duration), si.Username, si.Role)
	h.flash(w, r, "ok", fmt.Sprintf("Сертификат %s для %s выпущен (%s)!", name, domain, keyType))
	http.Redirect(w, r, "/issue", http.StatusFound)
}

func (h *Handler) Renew(w http.ResponseWriter, r *http.Request) {
	si := h.sessionInfo(r)
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	c, _ := appdb.GetCert(h.db, id)
	if c != nil {
		keyType := c.KeyType
		if keyType == "" {
			keyType = "EC:P-256"
		}
		if err := issueCert(c.Domain, c.CertPath, c.KeyPath, "8760h", keyType, h.cfg); err == nil {
			issued, expires, serial, _ := parseCertDates(c.CertPath)
			appdb.InsertCert(h.db, &models.Certificate{
				Name: c.Name, Domain: c.Domain, CertPath: c.CertPath, KeyPath: c.KeyPath,
				IssuedAt: issued, ExpiresAt: expires, Serial: serial, KeyType: keyType,
			})
			appdb.InsertHistory(h.db, "renew", c.Name, c.Domain, "Перевыпуск, тип: "+keyType, si.Username, si.Role)
			h.flash(w, r, "ok", "Сертификат перевыпущен")
		} else {
			h.flash(w, r, "err", "Ошибка: "+err.Error())
		}
	}
	http.Redirect(w, r, "/certificates", http.StatusFound)
}

func (h *Handler) Revoke(w http.ResponseWriter, r *http.Request) {
	si := h.sessionInfo(r)
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	c, _ := appdb.GetCert(h.db, id)
	if c != nil {
		revokeStep(c.CertPath, c.KeyPath, h.cfg)
		appdb.UpdateCertStatus(h.db, id, "revoked")
		appdb.InsertHistory(h.db, "revoke", c.Name, c.Domain, "Отозван (CRL)", si.Username, si.Role)
		h.flash(w, r, "ok", "Сертификат отозван")
	}
	http.Redirect(w, r, "/certificates", http.StatusFound)
}

func (h *Handler) DownloadCA(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Disposition", "attachment; filename=home-ca.crt")
	http.ServeFile(w, r, h.cfg.RootCert)
}

func (h *Handler) DownloadCert(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	c, _ := appdb.GetCert(h.db, id)
	if c == nil || c.CertPath == "" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.crt", sanitizeName(c.Name)))
	http.ServeFile(w, r, c.CertPath)
}

func (h *Handler) DownloadKey(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	c, _ := appdb.GetCert(h.db, id)
	if c == nil || c.KeyPath == "" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.key", sanitizeName(c.Name)))
	http.ServeFile(w, r, c.KeyPath)
}

func (h *Handler) ImportGet(w http.ResponseWriter, r *http.Request) {
	data := h.base(w, r, "import")
	data["Unregistered"] = scanExistingCerts(h.cfg.CertsDir, h.db)
	data["ActiveTab"] = r.URL.Query().Get("tab")
	h.render(w, "import", data)
}

func (h *Handler) ImportPost(w http.ResponseWriter, r *http.Request) {
	si := h.sessionInfo(r)
	switch r.FormValue("action") {
	case "upload":
		h.importUpload(w, r, si)
	case "scan":
		h.importScan(w, r, si)
	case "manual":
		h.importManual(w, r, si)
	default:
		http.Redirect(w, r, "/import", http.StatusFound)
	}
}

func (h *Handler) importUpload(w http.ResponseWriter, r *http.Request, si *models.SessionInfo) {
	r.ParseMultipartForm(10 << 20)
	name := trimStr(r.FormValue("name"))
	domain := trimStr(r.FormValue("domain"))
	data := h.base(w, r, "import")
	data["ActiveTab"] = "upload"
	certFile, _, err := r.FormFile("cert_file")
	if name == "" || domain == "" || err != nil {
		data["Msgs"] = []models.FlashMsg{{Type: "err", Text: "Заполните имя, домен и загрузите .crt файл"}}
		h.render(w, "import", data)
		return
	}
	defer certFile.Close()
	certDir := filepath.Join(h.cfg.CertsDir, sanitizeName(name))
	os.MkdirAll(certDir, 0755)
	certPath := filepath.Join(certDir, "certificate.crt")
	keyPath := filepath.Join(certDir, "private.key")
	if err := saveUploadedFile(certFile, certPath); err != nil {
		data["Msgs"] = []models.FlashMsg{{Type: "err", Text: "Ошибка сохранения файла"}}
		h.render(w, "import", data)
		return
	}
	if kf, _, err := r.FormFile("key_file"); err == nil {
		saveUploadedFile(kf, keyPath)
		kf.Close()
	} else {
		keyPath = ""
	}
	issued, expires, serial, err := parseCertDates(certPath)
	if err != nil || serial == "" {
		data["Msgs"] = []models.FlashMsg{{Type: "err", Text: "Не удалось прочитать сертификат"}}
		h.render(w, "import", data)
		return
	}
	kt := getCertKeyType(certPath)
	if err := appdb.InsertCert(h.db, &models.Certificate{
		Name: name, Domain: domain, CertPath: certPath, KeyPath: keyPath,
		IssuedAt: issued, ExpiresAt: expires, Serial: serial, KeyType: kt,
	}); err != nil {
		data["Msgs"] = []models.FlashMsg{{Type: "err", Text: "Сертификат уже есть в базе"}}
		h.render(w, "import", data)
		return
	}
	appdb.InsertHistory(h.db, "import", name, domain, "Загрузка с ПК, тип: "+kt, si.Username, si.Role)
	h.flash(w, r, "ok", fmt.Sprintf("Сертификат %s загружен!", name))
	http.Redirect(w, r, "/import?tab=upload", http.StatusFound)
}

func (h *Handler) importScan(w http.ResponseWriter, r *http.Request, si *models.SessionInfo) {
	count := 0
	for _, item := range scanExistingCerts(h.cfg.CertsDir, h.db) {
		issued, expires, serial, err := parseCertDates(item["cert_path"])
		if err != nil {
			continue
		}
		kt := getCertKeyType(item["cert_path"])
		if appdb.InsertCert(h.db, &models.Certificate{
			Name: item["name"], Domain: item["name"],
			CertPath: item["cert_path"], KeyPath: item["key_path"],
			IssuedAt: issued, ExpiresAt: expires, Serial: serial, KeyType: kt,
		}) == nil {
			appdb.InsertHistory(h.db, "import", item["name"], item["name"], "Скан сервера", si.Username, si.Role)
			count++
		}
	}
	if count > 0 {
		h.flash(w, r, "ok", fmt.Sprintf("Найдено и импортировано: %d", count))
	} else {
		h.flash(w, r, "ok", "Новых сертификатов не найдено")
	}
	http.Redirect(w, r, "/import?tab=scan", http.StatusFound)
}

func (h *Handler) importManual(w http.ResponseWriter, r *http.Request, si *models.SessionInfo) {
	name := trimStr(r.FormValue("name"))
	domain := trimStr(r.FormValue("domain"))
	certPath := trimStr(r.FormValue("cert_path"))
	keyPath := trimStr(r.FormValue("key_path"))
	data := h.base(w, r, "import")
	data["ActiveTab"] = "manual"
	if name == "" || domain == "" || certPath == "" {
		data["Msgs"] = []models.FlashMsg{{Type: "err", Text: "Заполните все поля"}}
		h.render(w, "import", data)
		return
	}
	if _, err := os.Stat(certPath); err != nil {
		data["Msgs"] = []models.FlashMsg{{Type: "err", Text: "Файл не найден: " + certPath}}
		h.render(w, "import", data)
		return
	}
	issued, expires, serial, _ := parseCertDates(certPath)
	kt := getCertKeyType(certPath)
	if err := appdb.InsertCert(h.db, &models.Certificate{
		Name: name, Domain: domain, CertPath: certPath, KeyPath: keyPath,
		IssuedAt: issued, ExpiresAt: expires, Serial: serial, KeyType: kt,
	}); err != nil {
		data["Msgs"] = []models.FlashMsg{{Type: "err", Text: "Уже в базе"}}
		h.render(w, "import", data)
		return
	}
	appdb.InsertHistory(h.db, "import", name, domain, "Путь вручную", si.Username, si.Role)
	h.flash(w, r, "ok", fmt.Sprintf("Сертификат %s импортирован", name))
	http.Redirect(w, r, "/import?tab=manual", http.StatusFound)
}

func (h *Handler) APIStatus(w http.ResponseWriter, r *http.Request) {
	certs, _ := appdb.GetCerts(h.db, "active")
	type exp struct {
		Name   string `json:"name"`
		Domain string `json:"domain"`
		Days   int    `json:"days"`
	}
	var expiring []exp
	for _, c := range certs {
		if d := daysLeftVal(c.ExpiresAt); d <= 30 {
			expiring = append(expiring, exp{c.Name, c.Domain, d})
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"total": len(certs), "expiring_soon": expiring,
	})
}
