package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	appdb "step-ui/db"
	"step-ui/models"
)

type notificationPayload struct {
	Type      string            `json:"type"`
	Severity  string            `json:"severity"`
	Title     string            `json:"title"`
	Message   string            `json:"message"`
	Timestamp time.Time         `json:"timestamp"`
	Source    string            `json:"source"`
	Version   string            `json:"version"`
	Meta      map[string]string `json:"meta,omitempty"`
}

func (h *Handler) AdminNotificationsGet(w http.ResponseWriter, r *http.Request) {
	settings, err := appdb.GetNotificationSettings(h.db)
	if err != nil {
		h.flash(w, r, "err", "Не удалось загрузить настройки уведомлений: "+err.Error())
		settings = &models.NotificationSettings{NotifyExpiry: true, ExpiryDays: 30, NotifyFailures: true, NotifyAuthBurst: true}
	}
	logs, _ := appdb.GetNotificationLogs(h.db, 25)

	data := h.base(w, r, "admin_notifications")
	data["Settings"] = settings
	data["Logs"] = logs
	h.render(w, "admin_notifications", data)
}

func (h *Handler) AdminNotificationsPost(w http.ResponseWriter, r *http.Request) {
	if !h.requireCSRF(w, r, "/admin/notifications") {
		return
	}
	expiryDays, _ := strconv.Atoi(r.FormValue("expiry_days"))
	if expiryDays < 1 {
		expiryDays = 1
	}
	if expiryDays > 365 {
		expiryDays = 365
	}
	settings := &models.NotificationSettings{
		WebhookEnabled:  r.FormValue("webhook_enabled") == "on",
		WebhookURL:      strings.TrimSpace(r.FormValue("webhook_url")),
		NotifyExpiry:    r.FormValue("notify_expiry") == "on",
		ExpiryDays:      expiryDays,
		NotifyFailures:  r.FormValue("notify_failures") == "on",
		NotifyAuthBurst: r.FormValue("notify_auth_burst") == "on",
	}
	if settings.WebhookEnabled {
		if _, err := url.ParseRequestURI(settings.WebhookURL); err != nil {
			h.flash(w, r, "err", "Webhook URL некорректен")
			http.Redirect(w, r, "/admin/notifications", http.StatusSeeOther)
			return
		}
	}
	if err := appdb.SaveNotificationSettings(h.db, settings); err != nil {
		h.flash(w, r, "err", "Не удалось сохранить настройки: "+err.Error())
	} else {
		h.auditSecurity(r, fmt.Sprintf("notifications.save webhook_enabled=%t notify_expiry=%t notify_failures=%t notify_auth_burst=%t expiry_days=%d",
			settings.WebhookEnabled, settings.NotifyExpiry, settings.NotifyFailures, settings.NotifyAuthBurst, settings.ExpiryDays))
		h.flash(w, r, "ok", "Настройки уведомлений сохранены")
	}
	http.Redirect(w, r, "/admin/notifications", http.StatusSeeOther)
}

func (h *Handler) AdminNotificationsTest(w http.ResponseWriter, r *http.Request) {
	if !h.requireCSRF(w, r, "/admin/notifications") {
		return
	}
	settings, err := appdb.GetNotificationSettings(h.db)
	if err != nil {
		h.flash(w, r, "err", "Не удалось загрузить настройки: "+err.Error())
		http.Redirect(w, r, "/admin/notifications", http.StatusSeeOther)
		return
	}
	if !settings.WebhookEnabled || strings.TrimSpace(settings.WebhookURL) == "" {
		h.flash(w, r, "err", "Сначала включите webhook и укажите URL")
		http.Redirect(w, r, "/admin/notifications", http.StatusSeeOther)
		return
	}
	err = h.sendNotification(r.Context(), "", "system.test", "info", "Step-CA UI test notification", "Тестовая отправка webhook из админ-панели", map[string]string{
		"remote_addr": r.RemoteAddr,
	})
	if err != nil {
		h.auditSecurity(r, "notifications.test status=failed")
		h.flash(w, r, "err", "Webhook test failed: "+err.Error())
	} else {
		h.auditSecurity(r, "notifications.test status=sent")
		h.flash(w, r, "ok", "Webhook test отправлен")
	}
	http.Redirect(w, r, "/admin/notifications", http.StatusSeeOther)
}

func (h *Handler) notifyAsync(eventKey, eventType, severity, title, message string, meta map[string]string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
		defer cancel()
		if err := h.sendNotification(ctx, eventKey, eventType, severity, title, message, meta); err != nil {
			log.Printf("notification %s failed: %v", eventType, err)
		}
	}()
}

func (h *Handler) sendNotification(ctx context.Context, eventKey, eventType, severity, title, message string, meta map[string]string) error {
	settings, err := appdb.GetNotificationSettings(h.db)
	if err != nil {
		return err
	}
	if !settings.WebhookEnabled || strings.TrimSpace(settings.WebhookURL) == "" {
		return nil
	}
	if !notificationAllowed(settings, eventType) {
		return nil
	}
	if eventKey != "" && appdb.NotificationEventExists(h.db, eventKey) {
		return nil
	}

	payload := notificationPayload{
		Type:      eventType,
		Severity:  severity,
		Title:     title,
		Message:   message,
		Timestamp: time.Now(),
		Source:    "step-ca-ui",
		Version:   Version,
		Meta:      meta,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, settings.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "step-ca-ui/"+Version)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	logEntry := &models.NotificationLog{
		EventKey:  eventKey,
		EventType: eventType,
		Severity:  severity,
		Title:     title,
		Message:   message,
	}
	if err != nil {
		logEntry.Success = false
		logEntry.Error = err.Error()
		_ = appdb.AddNotificationLog(h.db, logEntry)
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err = fmt.Errorf("webhook returned HTTP %d", resp.StatusCode)
		logEntry.Success = false
		logEntry.Error = err.Error()
		_ = appdb.AddNotificationLog(h.db, logEntry)
		return err
	}
	logEntry.Success = true
	_ = appdb.AddNotificationLog(h.db, logEntry)
	return nil
}

func notificationAllowed(settings *models.NotificationSettings, eventType string) bool {
	switch {
	case strings.HasPrefix(eventType, "certificate.expir"):
		return settings.NotifyExpiry
	case strings.HasPrefix(eventType, "certificate.") && strings.HasSuffix(eventType, "_failed"):
		return settings.NotifyFailures
	case strings.HasPrefix(eventType, "auth."):
		return settings.NotifyAuthBurst
	default:
		return true
	}
}

func (h *Handler) StartNotificationWorker() {
	go func() {
		h.checkExpiringCertificates(context.Background())
		t := time.NewTicker(24 * time.Hour)
		defer t.Stop()
		for range t.C {
			h.checkExpiringCertificates(context.Background())
		}
	}()
}

func (h *Handler) checkExpiringCertificates(ctx context.Context) {
	settings, err := appdb.GetNotificationSettings(h.db)
	if err != nil || !settings.NotifyExpiry || settings.ExpiryDays <= 0 {
		return
	}
	certs, err := appdb.GetCerts(h.db, "active")
	if err != nil {
		return
	}
	now := time.Now()
	limit := now.Add(time.Duration(settings.ExpiryDays) * 24 * time.Hour)
	for _, c := range certs {
		if c.ExpiresAt == nil || c.ExpiresAt.Before(now) || c.ExpiresAt.After(limit) {
			continue
		}
		eventKey := fmt.Sprintf("cert-expiry:%d:%s", c.ID, c.ExpiresAt.Format("2006-01-02"))
		days := int(time.Until(*c.ExpiresAt).Hours() / 24)
		_ = h.sendNotification(ctx, eventKey, "certificate.expiring", "warn",
			"Certificate expires soon",
			fmt.Sprintf("Сертификат %s (%s) истекает через %d дн.", c.Name, c.Domain, days),
			map[string]string{
				"id":      strconv.Itoa(c.ID),
				"name":    c.Name,
				"domain":  c.Domain,
				"expires": c.ExpiresAt.Format(time.RFC3339),
			})
	}
}
