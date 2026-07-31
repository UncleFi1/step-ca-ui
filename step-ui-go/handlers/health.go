package handlers

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type HealthCheck struct {
	Name     string
	Status   string
	Detail   string
	Critical bool
}

type HealthSummary struct {
	OK       int
	Warnings int
	Errors   int
}

type SystemInfo struct {
	Version       string
	BuildDate     string
	GitCommit     string
	StartedAt     time.Time
	Uptime        string
	CAURL         string
	RootCert      string
	Provisioner   string
	PasswordFile  string
	StepCAImage   string
	CertsDir      string
	UploadDir     string
	SSLCert       string
	SSLKey        string
	SessionSecure bool
}

func (h *Handler) systemInfo(lang string) SystemInfo {
	return SystemInfo{
		Version:       Version,
		BuildDate:     BuildDate,
		GitCommit:     GitCommit,
		StartedAt:     StartedAt,
		Uptime:        fmtUptime(lang, time.Since(StartedAt)),
		CAURL:         h.cfg.CAURL,
		RootCert:      h.cfg.RootCert,
		Provisioner:   h.cfg.Provisioner,
		PasswordFile:  h.cfg.PasswordFile,
		StepCAImage:   h.cfg.StepCAImage,
		CertsDir:      h.cfg.CertsDir,
		UploadDir:     h.cfg.UploadDir,
		SSLCert:       h.cfg.SSLCert,
		SSLKey:        h.cfg.SSLKey,
		SessionSecure: h.cfg.SessionSecure,
	}
}

func (h *Handler) preflight(ctx context.Context) ([]HealthCheck, HealthSummary) {
	var checks []HealthCheck
	add := func(name, status, detail string, critical bool) {
		checks = append(checks, HealthCheck{Name: name, Status: status, Detail: detail, Critical: critical})
	}

	if err := h.db.PingContext(ctx); err != nil {
		add("PostgreSQL", "err", err.Error(), true)
	} else {
		add("PostgreSQL", "ok", "database connection is alive", true)
	}

	if h.caIsOnline() {
		add("Step-CA API", "ok", "CA health endpoint is reachable", true)
	} else {
		add("Step-CA API", "err", "CA health endpoint is not reachable", true)
	}

	h.checkFile(&checks, "Root CA certificate", h.cfg.RootCert, true)
	h.checkFile(&checks, "Intermediate CA certificate", h.intermediateCertPath(), true)
	h.checkFile(&checks, "Provisioner password file", h.cfg.PasswordFile, true)
	h.checkFile(&checks, "UI TLS certificate", h.cfg.SSLCert, false)
	h.checkFile(&checks, "UI TLS private key", h.cfg.SSLKey, false)
	h.checkDir(&checks, "Issued certificates directory", h.cfg.CertsDir, true)
	h.checkDir(&checks, "Upload directory", h.cfg.UploadDir, false)

	h.checkCAConfig(&checks)
	h.checkCAChain(&checks)
	h.checkProvisionerPasswordSync(&checks)
	h.checkStepCAImagePin(&checks)
	h.checkDisk(&checks, h.cfg.CertsDir)
	h.checkDisk(&checks, filepath.Dir(h.cfg.RootCert))
	h.checkDisk(&checks, filepath.Dir(h.cfg.PasswordFile))

	if h.cfg.SessionSecure {
		add("Session cookie", "ok", "SESSION_SECURE=true", true)
	} else {
		add("Session cookie", "warn", "SESSION_SECURE=false; use only for local HTTP development", true)
	}

	summary := summarizeHealth(checks)
	return checks, summary
}

func (h *Handler) caIntegrity(ctx context.Context) ([]HealthCheck, HealthSummary) {
	var checks []HealthCheck

	if h.caIsOnline() {
		checks = append(checks, HealthCheck{Name: "Step-CA API", Status: "ok", Detail: "CA health endpoint is reachable", Critical: true})
	} else {
		checks = append(checks, HealthCheck{Name: "Step-CA API", Status: "err", Detail: "CA health endpoint is not reachable", Critical: true})
	}

	h.checkCAChain(&checks)
	h.checkCAConfig(&checks)
	h.checkProvisionerPasswordSync(&checks)
	h.checkStepCAImagePin(&checks)

	return checks, summarizeHealth(checks)
}

func (h *Handler) checkFile(checks *[]HealthCheck, name, path string, critical bool) {
	info, err := os.Stat(path)
	if err != nil {
		*checks = append(*checks, HealthCheck{Name: name, Status: "err", Detail: path + " is not readable: " + err.Error(), Critical: critical})
		return
	}
	if info.IsDir() {
		*checks = append(*checks, HealthCheck{Name: name, Status: "err", Detail: path + " is a directory", Critical: critical})
		return
	}
	if info.Size() == 0 {
		*checks = append(*checks, HealthCheck{Name: name, Status: "err", Detail: path + " is empty", Critical: critical})
		return
	}
	*checks = append(*checks, HealthCheck{Name: name, Status: "ok", Detail: fmt.Sprintf("%s (%d bytes)", path, info.Size()), Critical: critical})
}

func (h *Handler) checkDir(checks *[]HealthCheck, name, path string, critical bool) {
	info, err := os.Stat(path)
	if err != nil {
		*checks = append(*checks, HealthCheck{Name: name, Status: "err", Detail: path + " is not readable: " + err.Error(), Critical: critical})
		return
	}
	if !info.IsDir() {
		*checks = append(*checks, HealthCheck{Name: name, Status: "err", Detail: path + " is not a directory", Critical: critical})
		return
	}
	f, err := os.CreateTemp(path, ".preflight-*")
	if err != nil {
		*checks = append(*checks, HealthCheck{Name: name, Status: "warn", Detail: path + " is readable but not writable: " + err.Error(), Critical: critical})
		return
	}
	tmp := f.Name()
	f.Close()
	os.Remove(tmp)
	*checks = append(*checks, HealthCheck{Name: name, Status: "ok", Detail: path + " is writable", Critical: critical})
}

func (h *Handler) checkCAConfig(checks *[]HealthCheck) {
	caConfig := filepath.Join(filepath.Dir(filepath.Dir(h.cfg.RootCert)), "config", "ca.json")
	raw, err := os.ReadFile(caConfig)
	if err != nil {
		*checks = append(*checks, HealthCheck{Name: "CA config", Status: "warn", Detail: caConfig + " is not readable: " + err.Error(), Critical: false})
		return
	}

	var cfg struct {
		Authority struct {
			Provisioners []struct {
				Name   string                 `json:"name"`
				Type   string                 `json:"type"`
				Claims map[string]interface{} `json:"claims"`
			} `json:"provisioners"`
		} `json:"authority"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		*checks = append(*checks, HealthCheck{Name: "CA config", Status: "warn", Detail: "cannot parse ca.json: " + err.Error(), Critical: false})
		return
	}

	for _, p := range cfg.Authority.Provisioners {
		if p.Name != h.cfg.Provisioner {
			continue
		}
		*checks = append(*checks, HealthCheck{Name: "Provisioner", Status: "ok", Detail: fmt.Sprintf("%s (%s)", p.Name, p.Type), Critical: true})
		h.checkDurationClaim(checks, "Default TLS duration", claimString(p.Claims, "defaultTLSCertDuration"), 8760*time.Hour, false)
		h.checkDurationClaim(checks, "Max TLS duration", claimString(p.Claims, "maxTLSCertDuration"), 87600*time.Hour, true)
		return
	}

	*checks = append(*checks, HealthCheck{Name: "Provisioner", Status: "err", Detail: "provisioner " + h.cfg.Provisioner + " not found in ca.json", Critical: true})
}

func (h *Handler) checkCAChain(checks *[]HealthCheck) {
	root, err := readPEMCert(h.cfg.RootCert)
	if err != nil {
		*checks = append(*checks, HealthCheck{Name: "Root CA integrity", Status: "err", Detail: err.Error(), Critical: true})
		return
	}
	intermediate, err := readPEMCert(h.intermediateCertPath())
	if err != nil {
		*checks = append(*checks, HealthCheck{Name: "Intermediate CA integrity", Status: "err", Detail: err.Error(), Critical: true})
		return
	}

	now := time.Now()
	if !root.IsCA {
		*checks = append(*checks, HealthCheck{Name: "Root CA integrity", Status: "err", Detail: "root certificate is not marked as CA", Critical: true})
	} else if now.Before(root.NotBefore) || now.After(root.NotAfter) {
		*checks = append(*checks, HealthCheck{Name: "Root CA integrity", Status: "err", Detail: fmt.Sprintf("root certificate is outside validity window: %s - %s", root.NotBefore.Format(time.RFC3339), root.NotAfter.Format(time.RFC3339)), Critical: true})
	} else if err := root.CheckSignatureFrom(root); err != nil {
		*checks = append(*checks, HealthCheck{Name: "Root CA integrity", Status: "warn", Detail: "root certificate is not self-signed: " + err.Error(), Critical: true})
	} else {
		*checks = append(*checks, HealthCheck{Name: "Root CA integrity", Status: "ok", Detail: fmt.Sprintf("CN=%s, expires=%s", root.Subject.CommonName, root.NotAfter.Format("2006-01-02")), Critical: true})
	}

	if !intermediate.IsCA {
		*checks = append(*checks, HealthCheck{Name: "Intermediate CA integrity", Status: "err", Detail: "intermediate certificate is not marked as CA", Critical: true})
	} else if now.Before(intermediate.NotBefore) || now.After(intermediate.NotAfter) {
		*checks = append(*checks, HealthCheck{Name: "Intermediate CA integrity", Status: "err", Detail: fmt.Sprintf("intermediate certificate is outside validity window: %s - %s", intermediate.NotBefore.Format(time.RFC3339), intermediate.NotAfter.Format(time.RFC3339)), Critical: true})
	} else if err := intermediate.CheckSignatureFrom(root); err != nil {
		*checks = append(*checks, HealthCheck{Name: "Intermediate CA integrity", Status: "err", Detail: "intermediate is not signed by root: " + err.Error(), Critical: true})
	} else {
		*checks = append(*checks, HealthCheck{Name: "Intermediate CA integrity", Status: "ok", Detail: fmt.Sprintf("CN=%s, signed by root, expires=%s", intermediate.Subject.CommonName, intermediate.NotAfter.Format("2006-01-02")), Critical: true})
	}

	roots := x509.NewCertPool()
	roots.AddCert(root)
	if _, err := intermediate.Verify(x509.VerifyOptions{Roots: roots, CurrentTime: now, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}}); err != nil {
		*checks = append(*checks, HealthCheck{Name: "Full chain", Status: "err", Detail: "chain verification failed: " + err.Error(), Critical: true})
		return
	}
	*checks = append(*checks, HealthCheck{Name: "Full chain", Status: "ok", Detail: "intermediate verifies against root", Critical: true})
}

func readPEMCert(path string) (*x509.Certificate, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%s is not readable: %w", path, err)
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("%s does not contain a PEM block", path)
	}
	if block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("%s contains PEM block %q, expected CERTIFICATE", path, block.Type)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%s is not a valid certificate: %w", path, err)
	}
	return cert, nil
}

func (h *Handler) checkProvisionerPasswordSync(checks *[]HealthCheck) {
	uiPassword, err := readSecretLine(h.cfg.PasswordFile)
	if err != nil {
		*checks = append(*checks, HealthCheck{Name: "Provisioner password sync", Status: "err", Detail: err.Error(), Critical: true})
		return
	}
	caPassword, err := readSecretLine("/home/step/secrets/password")
	if err != nil {
		*checks = append(*checks, HealthCheck{Name: "Provisioner password sync", Status: "warn", Detail: "cannot read step-ca secret for comparison: " + err.Error(), Critical: true})
		return
	}
	if uiPassword != caPassword {
		*checks = append(*checks, HealthCheck{Name: "Provisioner password sync", Status: "err", Detail: "UI provisioner password file differs from step-ca secret", Critical: true})
		return
	}
	*checks = append(*checks, HealthCheck{Name: "Provisioner password sync", Status: "ok", Detail: "UI password file matches step-ca secret", Critical: true})
}

func readSecretLine(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("%s is not readable: %w", path, err)
	}
	secret := strings.TrimSpace(string(raw))
	if secret == "" {
		return "", fmt.Errorf("%s is empty", path)
	}
	return secret, nil
}

func (h *Handler) checkStepCAImagePin(checks *[]HealthCheck) {
	image := strings.TrimSpace(h.cfg.StepCAImage)
	if image == "" {
		*checks = append(*checks, HealthCheck{Name: "step-ca image pin", Status: "warn", Detail: "STEP_CA_IMAGE is empty; compose fallback will be used", Critical: false})
		return
	}
	if strings.HasSuffix(image, ":latest") || !strings.Contains(image, ":") {
		*checks = append(*checks, HealthCheck{Name: "step-ca image pin", Status: "warn", Detail: image + " is not pinned to a fixed version", Critical: false})
		return
	}
	*checks = append(*checks, HealthCheck{Name: "step-ca image pin", Status: "ok", Detail: image, Critical: false})
}

func claimString(claims map[string]interface{}, key string) string {
	if claims == nil {
		return ""
	}
	v, ok := claims[key]
	if !ok || v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	default:
		return fmt.Sprint(x)
	}
}

func (h *Handler) checkDurationClaim(checks *[]HealthCheck, name, value string, min time.Duration, critical bool) {
	if value == "" {
		*checks = append(*checks, HealthCheck{Name: name, Status: "warn", Detail: "claim is not set", Critical: critical})
		return
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		*checks = append(*checks, HealthCheck{Name: name, Status: "warn", Detail: value + " is not parseable: " + err.Error(), Critical: critical})
		return
	}
	if d < min {
		*checks = append(*checks, HealthCheck{Name: name, Status: "warn", Detail: fmt.Sprintf("%s is below expected %s", value, min), Critical: critical})
		return
	}
	*checks = append(*checks, HealthCheck{Name: name, Status: "ok", Detail: value, Critical: critical})
}

func (h *Handler) checkDisk(checks *[]HealthCheck, path string) {
	if path == "" {
		return
	}
	out, err := runCheck(context.Background(), 3*time.Second, "df", "-Pk", path)
	if err != nil {
		*checks = append(*checks, HealthCheck{Name: "Disk space", Status: "warn", Detail: path + ": df unavailable", Critical: false})
		return
	}
	lines := strings.Fields(string(out))
	if len(lines) < 11 {
		*checks = append(*checks, HealthCheck{Name: "Disk space", Status: "warn", Detail: path + ": cannot parse df output", Critical: false})
		return
	}
	availableKB, err := strconv.ParseInt(lines[len(lines)-3], 10, 64)
	if err != nil {
		*checks = append(*checks, HealthCheck{Name: "Disk space", Status: "warn", Detail: path + ": cannot parse available space", Critical: false})
		return
	}
	availableMB := availableKB / 1024
	status := "ok"
	if availableMB < 256 {
		status = "err"
	} else if availableMB < 1024 {
		status = "warn"
	}
	*checks = append(*checks, HealthCheck{Name: "Disk space", Status: status, Detail: fmt.Sprintf("%s: %d MB available", path, availableMB), Critical: status == "err"})
}

func summarizeHealth(checks []HealthCheck) HealthSummary {
	var s HealthSummary
	for _, c := range checks {
		switch c.Status {
		case "ok":
			s.OK++
		case "warn":
			s.Warnings++
		case "err":
			s.Errors++
		}
	}
	return s
}

func runCheck(ctx context.Context, timeout time.Duration, name string, args ...string) ([]byte, error) {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, name, args...)
	out, err := cmd.CombinedOutput()
	if cctx.Err() == context.DeadlineExceeded {
		return out, fmt.Errorf("timeout after %s", timeout)
	}
	return out, err
}

func cleanCheckOutput(out []byte, err error) string {
	text := strings.TrimSpace(string(out))
	if text == "" {
		return err.Error()
	}
	return text
}
