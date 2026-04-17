package handlers

import (
	"crypto/x509"
	"database/sql"
	"encoding/pem"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	appdb "step-ui/db"
	"step-ui/config"
	"step-ui/models"
)

func issueCert(domain, certPath, keyPath, duration, keyType string, cfg *config.Config) error {
	args := []string{
		"ca", "certificate",
		"--ca-url", cfg.CAURL,
		"--root", cfg.RootCert,
		"--provisioner", cfg.Provisioner,
		"--provisioner-password-file", cfg.PasswordFile,
		"--not-after", duration,
		"--force",
	}
	if strings.HasPrefix(keyType, "EC:") {
		args = append(args, "--kty", "EC", "--curve", strings.TrimPrefix(keyType, "EC:"))
	} else if strings.HasPrefix(keyType, "RSA:") {
		args = append(args, "--kty", "RSA", "--size", strings.TrimPrefix(keyType, "RSA:"))
	}
	args = append(args, domain, certPath, "--key", keyPath)
	cmd := exec.Command("step", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(out))
	}
	return nil
}

func revokeStep(certPath, keyPath string, cfg *config.Config) {
	exec.Command("step", "ca", "revoke",
		"--cert", certPath,
		"--key", keyPath,
		"--ca-url", cfg.CAURL,
		"--root", cfg.RootCert,
	).Run()
}

func parseCertDates(certPath string) (issued, expires *time.Time, serial string, err error) {
	data, err := os.ReadFile(certPath)
	if err != nil {
		return
	}
	block, _ := pem.Decode(data)
	if block == nil {
		err = fmt.Errorf("no PEM block found")
		return
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return
	}
	i := cert.NotBefore
	e := cert.NotAfter
	issued = &i
	expires = &e
	serial = cert.SerialNumber.String()
	return
}

func getCertKeyType(certPath string) string {
	data, err := os.ReadFile(certPath)
	if err != nil {
		return ""
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return ""
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return ""
	}
	switch cert.PublicKeyAlgorithm {
	case x509.ECDSA:
		return "EC"
	case x509.RSA:
		return "RSA"
	default:
		return "Unknown"
	}
}

func scanExistingCerts(certsDir string, d *sql.DB) []map[string]string {
	var found []map[string]string
	filepath.WalkDir(certsDir, func(path string, de os.DirEntry, err error) error {
		if err != nil || de.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, "certificate.crt") {
			dir := filepath.Dir(path)
			name := filepath.Base(dir)
			keyPath := filepath.Join(dir, "private.key")
			if _, e := os.Stat(keyPath); e != nil {
				keyPath = ""
			}
			// Проверяем не в базе ли уже
			_, _, serial, e := parseCertDates(path)
			if e != nil || serial == "" {
				return nil
			}
			c, _ := appdb.GetCertBySerial(d, serial)
			if c == nil {
				found = append(found, map[string]string{
					"name": name, "cert_path": path, "key_path": keyPath,
				})
			}
		}
		return nil
	})
	return found
}

func sanitizeName(name string) string {
	replacer := strings.NewReplacer(
		" ", "_", "/", "_", "\\", "_",
		"..", "_", "<", "_", ">", "_",
	)
	return replacer.Replace(name)
}

func saveUploadedFile(file multipart.File, dst string) error {
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, file)
	return err
}

func trimStr(s string) string {
	return strings.TrimSpace(s)
}

func daysLeftVal(t *time.Time) int {
	if t == nil {
		return 999
	}
	return int(time.Until(*t).Hours() / 24)
}

// GetCertBySerial wrapper needed in db
var _ = (*models.Certificate)(nil)
