package db

import (
	"database/sql"
	"time"

	"step-ui/models"
)

func InitPasswordResetSchema(d *sql.DB) error {
	_, err := d.Exec(`
	CREATE TABLE IF NOT EXISTS password_reset_tokens (
		id         SERIAL PRIMARY KEY,
		user_id    INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		token_hash TEXT UNIQUE NOT NULL,
		request_ip VARCHAR(80) DEFAULT '',
		expires_at TIMESTAMPTZ NOT NULL,
		used_at    TIMESTAMPTZ,
		created_at TIMESTAMPTZ DEFAULT NOW()
	);
	CREATE INDEX IF NOT EXISTS idx_password_reset_token_hash ON password_reset_tokens(token_hash);
	CREATE INDEX IF NOT EXISTS idx_password_reset_user ON password_reset_tokens(user_id);
	CREATE INDEX IF NOT EXISTS idx_password_reset_created ON password_reset_tokens(created_at);
	`)
	return err
}

func CreatePasswordResetToken(d *sql.DB, userID int, tokenHash, requestIP string, expiresAt time.Time) error {
	_, err := d.Exec(`INSERT INTO password_reset_tokens (user_id,token_hash,request_ip,expires_at)
		VALUES ($1,$2,$3,$4)`, userID, tokenHash, requestIP, expiresAt)
	return err
}

func InvalidatePasswordResetTokens(d *sql.DB, userID int) error {
	_, err := d.Exec(`UPDATE password_reset_tokens SET used_at=NOW()
		WHERE user_id=$1 AND used_at IS NULL`, userID)
	return err
}

func GetValidPasswordResetToken(d *sql.DB, tokenHash string) (*models.PasswordResetToken, error) {
	t := &models.PasswordResetToken{}
	var usedAt sql.NullTime
	err := d.QueryRow(`SELECT id,user_id,token_hash,expires_at,used_at,created_at
		FROM password_reset_tokens
		WHERE token_hash=$1 AND used_at IS NULL AND expires_at>NOW()`, tokenHash).
		Scan(&t.ID, &t.UserID, &t.TokenHash, &t.ExpiresAt, &usedAt, &t.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if usedAt.Valid {
		t.UsedAt = &usedAt.Time
	}
	return t, err
}

func MarkPasswordResetTokenUsed(d *sql.DB, id int) error {
	_, err := d.Exec(`UPDATE password_reset_tokens SET used_at=NOW()
		WHERE id=$1 AND used_at IS NULL`, id)
	return err
}

func GetUserByLoginOrEmail(d *sql.DB, identifier string) (*models.User, error) {
	u := &models.User{}
	err := d.QueryRow(`SELECT id,username,password_hash,role,is_active,created_at,last_login,last_ip,
		COALESCE(display_name,''),COALESCE(email,''),COALESCE(theme,'dark'),
		COALESCE(totp_enabled,false),COALESCE(totp_secret,''),COALESCE(totp_pending_secret,'')
		FROM users WHERE username=$1 OR lower(email)=lower($1) LIMIT 1`, identifier).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.IsActive, &u.CreatedAt, &u.LastLogin, &u.LastIP,
			&u.DisplayName, &u.Email, &u.Theme, &u.TOTPEnabled, &u.TOTPSecret, &u.TOTPPendingSecret)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return u, err
}
