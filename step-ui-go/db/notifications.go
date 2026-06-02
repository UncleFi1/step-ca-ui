package db

import (
	"database/sql"

	"step-ui/models"
)

func InitNotificationSchema(d *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS notification_settings (
		id                INT PRIMARY KEY DEFAULT 1,
		webhook_enabled   BOOLEAN DEFAULT FALSE,
		webhook_url       TEXT DEFAULT '',
		notify_expiry     BOOLEAN DEFAULT TRUE,
		expiry_days       INT DEFAULT 30,
		notify_failures   BOOLEAN DEFAULT TRUE,
		notify_auth_burst BOOLEAN DEFAULT TRUE,
		smtp_enabled      BOOLEAN DEFAULT FALSE,
		smtp_host         TEXT DEFAULT '',
		smtp_port         INT DEFAULT 587,
		smtp_security     VARCHAR(20) DEFAULT 'starttls',
		smtp_username     TEXT DEFAULT '',
		smtp_password     TEXT DEFAULT '',
		smtp_from         TEXT DEFAULT '',
		updated_at        TIMESTAMPTZ DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS notification_log (
		id         SERIAL PRIMARY KEY,
		event_key  TEXT UNIQUE,
		event_type VARCHAR(80),
		severity   VARCHAR(20),
		title      TEXT,
		message    TEXT,
		success    BOOLEAN DEFAULT FALSE,
		error      TEXT DEFAULT '',
		created_at TIMESTAMPTZ DEFAULT NOW()
	);
	CREATE INDEX IF NOT EXISTS idx_notification_log_created ON notification_log(created_at);

	INSERT INTO notification_settings (id) VALUES (1) ON CONFLICT (id) DO NOTHING;
	`
	if _, err := d.Exec(schema); err != nil {
		return err
	}
	for _, stmt := range []string{
		`ALTER TABLE notification_settings ADD COLUMN IF NOT EXISTS smtp_enabled BOOLEAN DEFAULT FALSE`,
		`ALTER TABLE notification_settings ADD COLUMN IF NOT EXISTS smtp_host TEXT DEFAULT ''`,
		`ALTER TABLE notification_settings ADD COLUMN IF NOT EXISTS smtp_port INT DEFAULT 587`,
		`ALTER TABLE notification_settings ADD COLUMN IF NOT EXISTS smtp_security VARCHAR(20) DEFAULT 'starttls'`,
		`ALTER TABLE notification_settings ADD COLUMN IF NOT EXISTS smtp_username TEXT DEFAULT ''`,
		`ALTER TABLE notification_settings ADD COLUMN IF NOT EXISTS smtp_password TEXT DEFAULT ''`,
		`ALTER TABLE notification_settings ADD COLUMN IF NOT EXISTS smtp_from TEXT DEFAULT ''`,
	} {
		if _, err := d.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func GetNotificationSettings(d *sql.DB) (*models.NotificationSettings, error) {
	s := &models.NotificationSettings{}
	err := d.QueryRow(`SELECT id,webhook_enabled,COALESCE(webhook_url,''),notify_expiry,
		COALESCE(expiry_days,30),notify_failures,notify_auth_burst,
		COALESCE(smtp_enabled,false),COALESCE(smtp_host,''),COALESCE(smtp_port,587),
		COALESCE(smtp_security,'starttls'),COALESCE(smtp_username,''),COALESCE(smtp_password,''),
		COALESCE(smtp_from,''),updated_at
		FROM notification_settings WHERE id=1`).
		Scan(&s.ID, &s.WebhookEnabled, &s.WebhookURL, &s.NotifyExpiry,
			&s.ExpiryDays, &s.NotifyFailures, &s.NotifyAuthBurst,
			&s.SMTPEnabled, &s.SMTPHost, &s.SMTPPort, &s.SMTPSecurity,
			&s.SMTPUsername, &s.SMTPPassword, &s.SMTPFrom, &s.UpdatedAt)
	if err == sql.ErrNoRows {
		return &models.NotificationSettings{
			ID:              1,
			NotifyExpiry:    true,
			ExpiryDays:      30,
			NotifyFailures:  true,
			NotifyAuthBurst: true,
			SMTPPort:        587,
			SMTPSecurity:    "starttls",
		}, nil
	}
	if s.ExpiryDays <= 0 {
		s.ExpiryDays = 30
	}
	if s.SMTPPort <= 0 {
		s.SMTPPort = 587
	}
	if s.SMTPSecurity == "" {
		s.SMTPSecurity = "starttls"
	}
	return s, err
}

func SaveNotificationSettings(d *sql.DB, s *models.NotificationSettings) error {
	_, err := d.Exec(`INSERT INTO notification_settings
		(id,webhook_enabled,webhook_url,notify_expiry,expiry_days,notify_failures,notify_auth_burst,
		 smtp_enabled,smtp_host,smtp_port,smtp_security,smtp_username,smtp_password,smtp_from,updated_at)
		VALUES (1,$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,NOW())
		ON CONFLICT (id) DO UPDATE SET
			webhook_enabled=$1,
			webhook_url=$2,
			notify_expiry=$3,
			expiry_days=$4,
			notify_failures=$5,
			notify_auth_burst=$6,
			smtp_enabled=$7,
			smtp_host=$8,
			smtp_port=$9,
			smtp_security=$10,
			smtp_username=$11,
			smtp_password=$12,
			smtp_from=$13,
			updated_at=NOW()`,
		s.WebhookEnabled, s.WebhookURL, s.NotifyExpiry, s.ExpiryDays, s.NotifyFailures, s.NotifyAuthBurst,
		s.SMTPEnabled, s.SMTPHost, s.SMTPPort, s.SMTPSecurity, s.SMTPUsername, s.SMTPPassword, s.SMTPFrom)
	return err
}

func AddNotificationLog(d *sql.DB, l *models.NotificationLog) error {
	_, err := d.Exec(`INSERT INTO notification_log
		(event_key,event_type,severity,title,message,success,error)
		VALUES (NULLIF($1,''),$2,$3,$4,$5,$6,$7)
		ON CONFLICT (event_key) DO NOTHING`,
		l.EventKey, l.EventType, l.Severity, l.Title, l.Message, l.Success, l.Error)
	return err
}

func NotificationEventExists(d *sql.DB, eventKey string) bool {
	if eventKey == "" {
		return false
	}
	var exists bool
	_ = d.QueryRow(`SELECT EXISTS(SELECT 1 FROM notification_log WHERE event_key=$1)`, eventKey).Scan(&exists)
	return exists
}

func GetNotificationLogs(d *sql.DB, limit int) ([]*models.NotificationLog, error) {
	rows, err := d.Query(`SELECT id,COALESCE(event_key,''),COALESCE(event_type,''),COALESCE(severity,''),
		COALESCE(title,''),COALESCE(message,''),success,COALESCE(error,''),created_at
		FROM notification_log ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var logs []*models.NotificationLog
	for rows.Next() {
		l := &models.NotificationLog{}
		if err := rows.Scan(&l.ID, &l.EventKey, &l.EventType, &l.Severity,
			&l.Title, &l.Message, &l.Success, &l.Error, &l.CreatedAt); err == nil {
			logs = append(logs, l)
		}
	}
	return logs, nil
}
