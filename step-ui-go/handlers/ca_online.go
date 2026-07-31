package handlers

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	caOnlineMu     sync.Mutex
	caOnlineCached bool
	caOnlineAt     time.Time
)

const caOnlineCacheTTL = 15 * time.Second

// caIsOnline prüft /health der CA. Ergebnis wird kurz gecacht.
// Docker-Hostname (step-ca) steht oft nicht im Zertifikat-SAN; deshalb
// ServerName=localhost (übliches SAN der Step-CA). Die Kette wird gegen
// RootCert verifiziert inkl. Intermediate aus dem Handshake.
func (h *Handler) caIsOnline() bool {
	caOnlineMu.Lock()
	defer caOnlineMu.Unlock()
	if time.Since(caOnlineAt) < caOnlineCacheTTL {
		return caOnlineCached
	}
	online := h.probeCAHealth()
	caOnlineCached = online
	caOnlineAt = time.Now()
	return online
}

func (h *Handler) probeCAHealth() bool {
	rootPEM, err := os.ReadFile(h.cfg.RootCert)
	if err != nil {
		log.Printf("ca health: root lesen: %v", err)
		return false
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(rootPEM) {
		log.Printf("ca health: root PEM ungültig")
		return false
	}

	client := &http.Client{
		Timeout: 3 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
				RootCAs:    pool,
				// Hostname in CA_URL (z.B. step-ca) weicht vom SAN ab
				ServerName: "localhost",
			},
		},
	}

	url := strings.TrimRight(h.cfg.CAURL, "/") + "/health"
	resp, err := client.Get(url)
	if err != nil {
		log.Printf("ca health: request: %v", err)
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("ca health: status %d", resp.StatusCode)
		return false
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512))
	if err != nil {
		log.Printf("ca health: body: %v", err)
		return false
	}
	var payload struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		log.Printf("ca health: json: %v", err)
		return false
	}
	ok := strings.EqualFold(payload.Status, "ok")
	if !ok {
		log.Printf("ca health: unerwarteter status %q", payload.Status)
	}
	return ok
}
