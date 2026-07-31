package i18n_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"testing"

	"step-ui/i18n"
)

func loadLocales(t *testing.T) {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(file), "..", "locales")
	if err := i18n.Load(dir); err != nil {
		t.Fatalf("load: %v", err)
	}
}

func TestT(t *testing.T) {
	loadLocales(t)
	if got := i18n.T("en", "Главная"); got != "Home" {
		t.Fatalf("en Главная: %q", got)
	}
	if got := i18n.T("de", "Главная"); got != "Startseite" {
		t.Fatalf("de Главная: %q", got)
	}
	if got := i18n.T("ru", "Главная"); got != "Главная" {
		t.Fatalf("ru Главная: %q", got)
	}
	if got := i18n.Tf("en", "Сертификат %s загружен!", "demo"); got != "Certificate demo uploaded!" {
		t.Fatalf("en upload: %q", got)
	}
}

func TestResolve(t *testing.T) {
	loadLocales(t)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Accept-Language", "de-DE,de;q=0.9,en;q=0.8")
	if got := i18n.Resolve(r, ""); got != "de" {
		t.Fatalf("accept-language: got %q", got)
	}
	if got := i18n.Resolve(r, "ru"); got != "ru" {
		t.Fatalf("user pref: got %q", got)
	}
}
