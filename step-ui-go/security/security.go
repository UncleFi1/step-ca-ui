package security

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// ─── Password ─────────────────────────────────────────────────────────────────

func HashPassword(pw string) string {
	hash, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		panic("bcrypt password hash failed: " + err.Error())
	}
	return string(hash)
}

func legacySHA256(pw string) string {
	h := sha256.Sum256([]byte(pw))
	return hex.EncodeToString(h[:])
}

func isLegacySHA256Hash(hash string) bool {
	if len(hash) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(hash)
	return err == nil
}

func VerifyPassword(pw, hash string) bool {
	if strings.HasPrefix(hash, "$2a$") || strings.HasPrefix(hash, "$2b$") || strings.HasPrefix(hash, "$2y$") {
		return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
	}
	if isLegacySHA256Hash(hash) {
		expected := legacySHA256(pw)
		return subtle.ConstantTimeCompare([]byte(expected), []byte(hash)) == 1
	}
	return false
}

func NeedsPasswordRehash(hash string) bool {
	if isLegacySHA256Hash(hash) {
		return true
	}
	cost, err := bcrypt.Cost([]byte(hash))
	return err != nil || cost < bcrypt.DefaultCost
}

func ValidatePassword(pw string) (bool, string) {
	// Rückgabe-Strings sind i18n-Quellschlüssel (Russisch) und müssen mit i18n.T übersetzt werden.
	if len(pw) < 8 {
		return false, "Минимум 8 символов"
	}
	if len(pw) > 72 {
		return false, "Максимум 72 символа"
	}
	hasDigit, hasLetter, hasSpecial := false, false, false
	for _, c := range pw {
		if c >= '0' && c <= '9' {
			hasDigit = true
		} else if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			hasLetter = true
		} else if strings.ContainsRune(`+!@#$%^&*()_-=[]{}|;:,.<>?`, c) {
			hasSpecial = true
		}
	}
	if !hasDigit {
		return false, "Нужна хотя бы одна цифра"
	}
	if !hasLetter {
		return false, "Нужна хотя бы одна буква"
	}
	if !hasSpecial {
		return false, "Нужен хотя бы один спецсимвол"
	}
	return true, ""
}

// ─── CSRF token ───────────────────────────────────────────────────────────────

func GenerateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// ─── Rate Limiting ────────────────────────────────────────────────────────────

const (
	LimitCount  = 5
	LimitWindow = 5 * time.Minute
	BlockTime   = 15 * time.Minute
)

type RateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
}

var RL = &RateLimiter{attempts: make(map[string][]time.Time)}

func (r *RateLimiter) clean(ip string) {
	now := time.Now()
	var v []time.Time
	for _, t := range r.attempts[ip] {
		if now.Sub(t) < LimitWindow {
			v = append(v, t)
		}
	}
	r.attempts[ip] = v
}

func (r *RateLimiter) IsBlocked(ip string) bool {
	if ip == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clean(ip)
	return len(r.attempts[ip]) >= LimitCount
}

func (r *RateLimiter) Register(ip string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clean(ip)
	r.attempts[ip] = append(r.attempts[ip], time.Now())
}

func (r *RateLimiter) Clear(ip string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.attempts, ip)
}

func (r *RateLimiter) Left(ip string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clean(ip)
	n := LimitCount - len(r.attempts[ip])
	if n < 0 {
		return 0
	}
	return n
}
