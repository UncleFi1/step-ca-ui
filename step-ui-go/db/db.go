package db

import (
	"strings"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
	"step-ui/models"
	"step-ui/security"
)

func Connect(dsn string) (*sql.DB, error) {
	conn, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(25)
	conn.SetMaxIdleConns(5)
	conn.SetConnMaxLifetime(5 * time.Minute)
	if err = conn.Ping(); err != nil {
		return nil, fmt.Errorf("db ping failed: %w", err)
	}
	return conn, nil
}

func InitSchema(d *sql.DB) error {
	// migration: users.theme
	if _, err := d.Exec(`ALTER TABLE users ADD COLUMN IF NOT EXISTS theme VARCHAR(10) DEFAULT 'dark'`); err != nil {
		return err
	}

	// -- migration: user profile fields
	if _, err := d.Exec(`ALTER TABLE users ADD COLUMN IF NOT EXISTS display_name VARCHAR(100) DEFAULT ''`); err != nil {
		return err
	}
	if _, err := d.Exec(`ALTER TABLE users ADD COLUMN IF NOT EXISTS email VARCHAR(200) DEFAULT ''`); err != nil {
		return err
	}

	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id           SERIAL PRIMARY KEY,
		username     VARCHAR(100) UNIQUE NOT NULL,
		password_hash VARCHAR(255) NOT NULL,
		role         VARCHAR(20) DEFAULT 'viewer',
		is_active    BOOLEAN DEFAULT TRUE,
		created_at   TIMESTAMPTZ DEFAULT NOW(),
		last_login   TIMESTAMPTZ,
		last_ip      VARCHAR(45)
	);

	CREATE TABLE IF NOT EXISTS auth_log (
		id         SERIAL PRIMARY KEY,
		username   VARCHAR(100),
		ip         VARCHAR(45),
		success    BOOLEAN,
		reason     TEXT,
		created_at TIMESTAMPTZ DEFAULT NOW()
	);
	CREATE INDEX IF NOT EXISTS idx_auth_log_username ON auth_log(username);
	CREATE INDEX IF NOT EXISTS idx_auth_log_created  ON auth_log(created_at);

	CREATE TABLE IF NOT EXISTS certificates (
		id         SERIAL PRIMARY KEY,
		name       VARCHAR(255) NOT NULL,
		domain     VARCHAR(255) NOT NULL,
		cert_path  TEXT,
		key_path   TEXT,
		issued_at  TIMESTAMPTZ,
		expires_at TIMESTAMPTZ,
		serial     VARCHAR(100) UNIQUE,
		status     VARCHAR(20) DEFAULT 'active',
		key_type   VARCHAR(50),
		created_at TIMESTAMPTZ DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS cert_history (
		id         SERIAL PRIMARY KEY,
		action     VARCHAR(50) NOT NULL,
		cert_name  VARCHAR(255) NOT NULL,
		domain     VARCHAR(255),
		details    TEXT,
		username   VARCHAR(100),
		role       VARCHAR(20),
		created_at TIMESTAMPTZ DEFAULT NOW()
	);
	CREATE INDEX IF NOT EXISTS idx_hist_created ON cert_history(created_at);
	`
	if _, err := d.Exec(schema); err != nil {
		return err
	}
	// Создаём admin если нет пользователей
	var count int
	d.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count)
	if count == 0 {
		d.Exec(`INSERT INTO users (username,password_hash,role,is_active) VALUES ($1,$2,'admin',true)`,
			"admin", security.HashPassword("Admin123!"))
		fmt.Println("[!] Default user created: admin / Admin123!")
	}
	return nil
}

// ─── Users ────────────────────────────────────────────────────────────────────

func GetUserByUsername(d *sql.DB, username string) (*models.User, error) {
	u := &models.User{}
	err := d.QueryRow(`SELECT id,username,password_hash,role,is_active,created_at,last_login,last_ip FROM users WHERE username=$1`, username).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.IsActive, &u.CreatedAt, &u.LastLogin, &u.LastIP)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return u, err
}

func GetUserByID(d *sql.DB, id int) (*models.User, error) {
	u := &models.User{}
	var displayName, email, theme sql.NullString
	err := d.QueryRow(`SELECT id, username, password_hash, role, is_active, created_at, last_login, last_ip,
		COALESCE(display_name,''), COALESCE(email,''), COALESCE(theme,'dark') FROM users WHERE id=$1`, id).Scan(
		&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.IsActive,
		&u.CreatedAt, &u.LastLogin, &u.LastIP, &displayName, &email, &theme,
	)
	if err != nil {
		return nil, err
	}
	u.DisplayName = displayName.String
	u.Email = email.String
	u.Theme = theme.String
	if u.Theme == "" {
		u.Theme = "dark"
	}
	return u, nil
}

func GetAllUsers(d *sql.DB) ([]*models.User, error) {
	rows, err := d.Query(`SELECT id,username,password_hash,role,is_active,created_at,last_login,last_ip FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []*models.User
	for rows.Next() {
		u := &models.User{}
		rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.IsActive, &u.CreatedAt, &u.LastLogin, &u.LastIP)
		users = append(users, u)
	}
	return users, nil
}

func CreateUser(d *sql.DB, username, passwordHash, role string) error {
	_, err := d.Exec(`INSERT INTO users (username,password_hash,role,is_active) VALUES ($1,$2,$3,true)`,
		username, passwordHash, role)
	return err
}

func UpdateUserRole(d *sql.DB, id int, role string) error {
	_, err := d.Exec(`UPDATE users SET role=$1 WHERE id=$2`, role, id)
	return err
}

func UpdateUserActive(d *sql.DB, id int, active bool) error {
	_, err := d.Exec(`UPDATE users SET is_active=$1 WHERE id=$2`, active, id)
	return err
}

func UpdateUserPassword(d *sql.DB, id int, hash string) error {
	_, err := d.Exec(`UPDATE users SET password_hash=$1 WHERE id=$2`, hash, id)
	return err
}

func UpdateUserInfo(d *sql.DB, id int, username, displayName, email string) error {
	_, err := d.Exec(`UPDATE users SET username=$1, display_name=$2, email=$3 WHERE id=$4`,
		username, displayName, email, id)
	return err
}

func UpdateUserTheme(d *sql.DB, id int, theme string) error {
	_, err := d.Exec(`UPDATE users SET theme=$1 WHERE id=$2`, theme, id)
	return err
}

func UsernameExistsExceptID(d *sql.DB, username string, id int) (bool, error) {
	var exists bool
	err := d.QueryRow(`SELECT EXISTS(SELECT 1 FROM users WHERE username=$1 AND id<>$2)`, username, id).Scan(&exists)
	return exists, err
}

func UpdateUserLogin(d *sql.DB, username, ip string) error {
	_, err := d.Exec(`UPDATE users SET last_login=NOW(),last_ip=$1 WHERE username=$2`, ip, username)
	return err
}

func DeleteUser(d *sql.DB, id int) error {
	var username string
	d.QueryRow(`SELECT username FROM users WHERE id=$1`, id).Scan(&username)
	d.Exec(`DELETE FROM auth_log WHERE username=$1`, username)
	_, err := d.Exec(`DELETE FROM users WHERE id=$1`, id)
	return err
}

// ─── Auth Log ─────────────────────────────────────────────────────────────────

func LogAuth(d *sql.DB, username, ip string, success bool, reason string) error {
	_, err := d.Exec(`INSERT INTO auth_log (username,ip,success,reason) VALUES ($1,$2,$3,$4)`,
		username, ip, success, reason)
	if success {
		UpdateUserLogin(d, username, ip)
	}
	return err
}

func GetAuthLogs(d *sql.DB, search, filter string, page, limit int) ([]*models.AuthLog, int, error) {
	where := "WHERE 1=1"
	args := []interface{}{}
	i := 1
	if search != "" {
		where += fmt.Sprintf(" AND (username ILIKE $%d OR ip ILIKE $%d)", i, i+1)
		args = append(args, "%"+search+"%", "%"+search+"%")
		i += 2
	}
	if filter == "fail" {
		where += fmt.Sprintf(" AND success=$%d", i)
		args = append(args, false); i++
	} else if filter == "ok" {
		where += fmt.Sprintf(" AND success=$%d", i)
		args = append(args, true); i++
	}
	var total int
	d.QueryRow(`SELECT COUNT(*) FROM auth_log `+where, args...).Scan(&total)
	offset := (page - 1) * limit
	rows, err := d.Query(`SELECT id,username,ip,success,COALESCE(reason,''),created_at FROM auth_log `+where+
		fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d OFFSET $%d`, i, i+1),
		append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var logs []*models.AuthLog
	for rows.Next() {
		l := &models.AuthLog{}
		rows.Scan(&l.ID, &l.Username, &l.IP, &l.Success, &l.Reason, &l.CreatedAt)
		logs = append(logs, l)
	}
	return logs, total, nil
}

func GetUserAuthLogs(d *sql.DB, username string, limit int) ([]*models.AuthLog, error) {
	rows, err := d.Query(`SELECT id,username,ip,success,COALESCE(reason,''),created_at FROM auth_log WHERE username=$1 ORDER BY created_at DESC LIMIT $2`,
		username, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var logs []*models.AuthLog
	for rows.Next() {
		l := &models.AuthLog{}
		rows.Scan(&l.ID, &l.Username, &l.IP, &l.Success, &l.Reason, &l.CreatedAt)
		logs = append(logs, l)
	}
	return logs, nil
}

func GetFailCount(d *sql.DB, username string, since time.Time) int {
	var n int
	d.QueryRow(`SELECT COUNT(*) FROM auth_log WHERE username=$1 AND success=false AND created_at>$2`, username, since).Scan(&n)
	return n
}

func GetAuthStats(d *sql.DB) (ok, fail int) {
	d.QueryRow(`SELECT COUNT(*) FROM auth_log WHERE success=true`).Scan(&ok)
	d.QueryRow(`SELECT COUNT(*) FROM auth_log WHERE success=false`).Scan(&fail)
	return
}

// ─── Certificates ─────────────────────────────────────────────────────────────

func GetCerts(d *sql.DB, statusFilter string) ([]*models.Certificate, error) {
	q := `SELECT id,name,domain,cert_path,key_path,issued_at,expires_at,COALESCE(serial,''),status,COALESCE(key_type,''),created_at FROM certificates`
	var args []interface{}
	if statusFilter != "" {
		q += ` WHERE status=$1`
		args = append(args, statusFilter)
	}
	q += ` ORDER BY expires_at ASC`
	rows, err := d.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var certs []*models.Certificate
	for rows.Next() {
		c := &models.Certificate{}
		rows.Scan(&c.ID, &c.Name, &c.Domain, &c.CertPath, &c.KeyPath, &c.IssuedAt, &c.ExpiresAt, &c.Serial, &c.Status, &c.KeyType, &c.CreatedAt)
		certs = append(certs, c)
	}
	return certs, nil
}

func GetCert(d *sql.DB, id int) (*models.Certificate, error) {
	c := &models.Certificate{}
	err := d.QueryRow(`SELECT id,name,domain,cert_path,key_path,issued_at,expires_at,COALESCE(serial,''),status,COALESCE(key_type,''),created_at FROM certificates WHERE id=$1`, id).
		Scan(&c.ID, &c.Name, &c.Domain, &c.CertPath, &c.KeyPath, &c.IssuedAt, &c.ExpiresAt, &c.Serial, &c.Status, &c.KeyType, &c.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return c, err
}

func InsertCert(d *sql.DB, c *models.Certificate) error {
	_, err := d.Exec(`INSERT INTO certificates (name,domain,cert_path,key_path,issued_at,expires_at,serial,status,key_type)
		VALUES ($1,$2,$3,$4,$5,$6,$7,'active',$8)
		ON CONFLICT (serial) DO UPDATE SET
			name=$1,domain=$2,cert_path=$3,key_path=$4,issued_at=$5,expires_at=$6,key_type=$8,status='active'`,
		c.Name, c.Domain, c.CertPath, c.KeyPath, c.IssuedAt, c.ExpiresAt, c.Serial, c.KeyType)
	return err
}

func UpdateCertStatus(d *sql.DB, id int, status string) error {
	_, err := d.Exec(`UPDATE certificates SET status=$1 WHERE id=$2`, status, id)
	return err
}

// ─── Cert History ─────────────────────────────────────────────────────────────

func InsertHistory(d *sql.DB, action, certName, domain, details, username, role string) error {
	_, err := d.Exec(`INSERT INTO cert_history (action,cert_name,domain,details,username,role) VALUES ($1,$2,$3,$4,$5,$6)`,
		action, certName, domain, details, username, role)
	return err
}

func GetHistory(d *sql.DB, actions []string, cert string, page, limit int) ([]*models.CertHistory, int, error) {
	where := "WHERE 1=1"
	args := []interface{}{}
	i := 1
	if len(actions) > 0 {
		placeholders := make([]string, 0, len(actions))
		for _, a := range actions {
			placeholders = append(placeholders, fmt.Sprintf("$%d", i))
			args = append(args, a)
			i++
		}
		where += " AND action IN (" + strings.Join(placeholders, ",") + ")"
	}
	if cert != "" {
		where += fmt.Sprintf(" AND cert_name=$%d", i)
		args = append(args, cert)
		i++
	}
	var total int
	d.QueryRow(`SELECT COUNT(*) FROM cert_history `+where, args...).Scan(&total)
	offset := (page - 1) * limit
	rows, err := d.Query(`SELECT id,action,cert_name,COALESCE(domain,''),COALESCE(details,''),COALESCE(username,''),COALESCE(role,''),created_at FROM cert_history `+where+
		fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d OFFSET $%d`, i, i+1),
		append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var list []*models.CertHistory
	for rows.Next() {
		h := &models.CertHistory{}
		if err := rows.Scan(&h.ID, &h.Action, &h.CertName, &h.Domain, &h.Details, &h.Username, &h.Role, &h.CreatedAt); err == nil {
			list = append(list, h)
		}
	}
	return list, total, nil
}

func GetCertBySerial(d *sql.DB, serial string) (*models.Certificate, error) {
	c := &models.Certificate{}
	err := d.QueryRow(`SELECT id,name,domain,cert_path,key_path,issued_at,expires_at,COALESCE(serial,''),status,COALESCE(key_type,''),created_at FROM certificates WHERE serial=$1`, serial).
		Scan(&c.ID, &c.Name, &c.Domain, &c.CertPath, &c.KeyPath, &c.IssuedAt, &c.ExpiresAt, &c.Serial, &c.Status, &c.KeyType, &c.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return c, err
}
