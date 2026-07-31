package i18n

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	LangEN     = "en"
	LangDE     = "de"
	LangRU     = "ru"
	CookieName = "step-ui-lang"
	Default    = LangRU
)

var supported = map[string]bool{
	LangEN: true,
	LangDE: true,
	LangRU: true,
}

var (
	mu       sync.RWMutex
	catalogs = map[string]map[string]string{}
)

// Load liest Locale-JSON aus dir (en.json, de.json, ru.json).
func Load(dir string) error {
	mu.Lock()
	defer mu.Unlock()
	next := map[string]map[string]string{}
	for _, lang := range []string{LangEN, LangDE, LangRU} {
		path := filepath.Join(dir, lang+".json")
		b, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("i18n: %s: %w", path, err)
		}
		var m map[string]string
		if err := json.Unmarshal(b, &m); err != nil {
			return fmt.Errorf("i18n: parse %s: %w", path, err)
		}
		next[lang] = m
	}
	catalogs = next
	return nil
}

// Normalize liefert eine unterstützte Sprachkennung.
func Normalize(lang string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if i := strings.IndexByte(lang, '-'); i > 0 {
		lang = lang[:i]
	}
	if i := strings.IndexByte(lang, '_'); i > 0 {
		lang = lang[:i]
	}
	if supported[lang] {
		return lang
	}
	return Default
}

// IsValid prüft, ob lang eine bekannte Kennung ist (ohne Fallback).
func IsValid(lang string) bool {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if i := strings.IndexAny(lang, "-_"); i > 0 {
		lang = lang[:i]
	}
	return supported[lang]
}

// T übersetzt msg (russischer Quelltext als Schlüssel).
func T(lang, msg string) string {
	if msg == "" {
		return ""
	}
	lang = Normalize(lang)
	if lang == LangRU {
		return msg
	}
	mu.RLock()
	defer mu.RUnlock()
	if cat := catalogs[lang]; cat != nil {
		if tr, ok := cat[msg]; ok && tr != "" {
			return tr
		}
	}
	return msg
}

// Tf wie T, danach fmt.Sprintf.
func Tf(lang, format string, args ...interface{}) string {
	return fmt.Sprintf(T(lang, format), args...)
}

// FromAcceptLanguage wertet den Accept-Language-Header aus.
func FromAcceptLanguage(header string) string {
	if header == "" {
		return Default
	}
	best := Default
	bestQ := -1.0
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		tag := part
		q := 1.0
		if i := strings.IndexByte(part, ';'); i >= 0 {
			tag = strings.TrimSpace(part[:i])
			rest := part[i+1:]
			if j := strings.Index(rest, "q="); j >= 0 {
				var parsed float64
				if _, err := fmt.Sscanf(strings.TrimSpace(rest[j+2:]), "%f", &parsed); err == nil {
					q = parsed
				}
			}
		}
		base := strings.ToLower(strings.TrimSpace(tag))
		if i := strings.IndexAny(base, "-_"); i > 0 {
			base = base[:i]
		}
		if !supported[base] {
			continue
		}
		if q > bestQ {
			bestQ = q
			best = base
		}
	}
	return best
}

// Resolve bestimmt die Sprache: userPref > Cookie > Accept-Language > Default.
func Resolve(r *http.Request, userPref string) string {
	if userPref != "" {
		return Normalize(userPref)
	}
	if c, err := r.Cookie(CookieName); err == nil && c.Value != "" {
		return Normalize(c.Value)
	}
	return FromAcceptLanguage(r.Header.Get("Accept-Language"))
}

// SetCookie speichert die Sprachwahl (1 Jahr).
func SetCookie(w http.ResponseWriter, lang string) {
	lang = Normalize(lang)
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    lang,
		Path:     "/",
		MaxAge:   365 * 24 * 3600,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
	})
}

// CatalogForJS liefert ein kleines Wörterbuch für Client-Skripte.
func CatalogForJS(lang string, keys []string) map[string]string {
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		out[k] = T(lang, k)
	}
	return out
}
