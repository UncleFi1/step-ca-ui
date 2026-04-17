package models

import "time"

type User struct {
	ID           int
	Username     string
	PasswordHash string
	Role         string
	IsActive     bool
	CreatedAt    *time.Time
	LastLogin    *time.Time
	LastIP       *string
}

type AuthLog struct {
	ID        int
	Username  string
	IP        string
	Success   bool
	Reason    string
	CreatedAt time.Time
}

type Certificate struct {
	ID        int
	Name      string
	Domain    string
	CertPath  string
	KeyPath   string
	IssuedAt  *time.Time
	ExpiresAt *time.Time
	Serial    string
	Status    string
	KeyType   string
	CreatedAt *time.Time
}

type CertHistory struct {
	ID        int
	Action    string
	CertName  string
	Domain    string
	Details   string
	Username  string
	Role      string
	CreatedAt time.Time
}

type FlashMsg struct {
	Type string // "ok" or "err"
	Text string
}

type SessionInfo struct {
	UserID   int
	Username string
	Role     string
}
