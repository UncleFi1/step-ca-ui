package le

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge/http01"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/providers/dns/cloudflare"
	"github.com/go-acme/lego/v4/registration"
)

const (
	LEDirectory    = "/opt/step-ui/le-certs"
	LEAccountFile  = "/opt/step-ui/le-certs/account.json"
	LEKeyFile      = "/opt/step-ui/le-certs/account.key"
	LEProductionCA = "https://acme-v02.api.letsencrypt.org/directory"
	LEStagingCA    = "https://acme-staging-v02.api.letsencrypt.org/directory"
)

// LEUser реализует интерфейс registration.User
type LEUser struct {
	Email        string
	Registration *registration.Resource
	key          crypto.PrivateKey
}

func (u *LEUser) GetEmail() string                        { return u.Email }
func (u *LEUser) GetRegistration() *registration.Resource { return u.Registration }
func (u *LEUser) GetPrivateKey() crypto.PrivateKey        { return u.key }

// LEConfig конфигурация для выпуска
type LEConfig struct {
	Email     string
	Domain    string
	Provider  string // http01, cloudflare, route53, manual
	CFToken   string
	CFZoneID  string
	R53KeyID  string
	R53Secret string
	R53Region string
	Staging   bool
}

// LEResult результат выпуска
type LEResult struct {
	CertPath  string
	KeyPath   string
	IssuedAt  *time.Time
	ExpiresAt *time.Time
}

// IssueCert выпускает сертификат Let's Encrypt
func IssueCert(cfg LEConfig) (*LEResult, error) {
	os.MkdirAll(filepath.Join(LEDirectory, cfg.Domain), 0700)

	// Загружаем или создаём ключ аккаунта
	privateKey, err := loadOrCreateKey(LEKeyFile)
	if err != nil {
		return nil, fmt.Errorf("ключ аккаунта: %w", err)
	}

	user := &LEUser{Email: cfg.Email, key: privateKey}

	// Загружаем регистрацию если есть
	if reg, err := loadRegistration(LEAccountFile); err == nil {
		user.Registration = reg
	}

	// Выбираем CA
	caURL := LEProductionCA
	if cfg.Staging {
		caURL = LEStagingCA
	}

	// Создаём LEGO клиент
	legoConfig := lego.NewConfig(user)
	legoConfig.CADirURL = caURL
	legoConfig.Certificate.KeyType = certcrypto.EC256

	client, err := lego.NewClient(legoConfig)
	if err != nil {
		return nil, fmt.Errorf("lego client: %w", err)
	}

	// Настраиваем challenge провайдер
	switch cfg.Provider {
	case "http01":
		client.Challenge.SetHTTP01Provider(http01.NewProviderServer("", "80"))
	case "cloudflare":
		if cfg.CFToken == "" {
			return nil, fmt.Errorf("Cloudflare API token не задан")
		}
		os.Setenv("CF_DNS_API_TOKEN", cfg.CFToken)
		provider := cloudflare.NewDefaultConfig()
		cp, err := cloudflare.NewDNSProviderConfig(provider)
		if err != nil {
			return nil, fmt.Errorf("cloudflare provider: %w", err)
		}
		client.Challenge.SetDNS01Provider(cp)
	default:
		return nil, fmt.Errorf("неизвестный провайдер: %s", cfg.Provider)
	}

	// Регистрация если нет
	if user.Registration == nil {
		reg, err := client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
		if err != nil {
			return nil, fmt.Errorf("регистрация: %w", err)
		}
		user.Registration = reg
		saveRegistration(LEAccountFile, reg)
	}

	// Запрашиваем сертификат
	request := certificate.ObtainRequest{
		Domains: []string{cfg.Domain},
		Bundle:  true,
	}
	certs, err := client.Certificate.Obtain(request)
	if err != nil {
		return nil, fmt.Errorf("выпуск сертификата: %w", err)
	}

	// Сохраняем файлы
	certDir := filepath.Join(LEDirectory, cfg.Domain)
	certPath := filepath.Join(certDir, "certificate.crt")
	keyPath := filepath.Join(certDir, "private.key")

	if err := os.WriteFile(certPath, certs.Certificate, 0644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(keyPath, certs.PrivateKey, 0600); err != nil {
		return nil, err
	}

	// Парсим даты
	issued, expires := parseCertDates(certs.Certificate)

	return &LEResult{
		CertPath:  certPath,
		KeyPath:   keyPath,
		IssuedAt:  issued,
		ExpiresAt: expires,
	}, nil
}

func parseCertDates(certPEM []byte) (issued, expires *time.Time) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return
	}
	i := cert.NotBefore
	e := cert.NotAfter
	return &i, &e
}

func loadOrCreateKey(path string) (crypto.PrivateKey, error) {
	if data, err := os.ReadFile(path); err == nil {
		block, _ := pem.Decode(data)
		if block != nil {
			return x509.ParseECPrivateKey(block.Bytes)
		}
	}
	// Создаём новый ключ
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	data, _ := x509.MarshalECPrivateKey(key)
	os.MkdirAll(filepath.Dir(path), 0700)
	os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: data}), 0600)
	return key, nil
}

type savedRegistration struct {
	Body *registration.Resource
}

func saveRegistration(path string, reg *registration.Resource) {
	data, _ := json.Marshal(&savedRegistration{Body: reg})
	os.WriteFile(path, data, 0600)
}

func loadRegistration(path string) (*registration.Resource, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s savedRegistration
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return s.Body, nil
}
