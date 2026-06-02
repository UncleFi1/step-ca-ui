<div align="center">

# Step-CA UI

**Self-hosted web interface for [Smallstep step-ca](https://smallstep.com/docs/step-ca/) — manage your private PKI from a browser.**

[![License: GPL v3](https://img.shields.io/badge/License-GPLv3-blue.svg)](https://www.gnu.org/licenses/gpl-3.0)
[![Made with Go](https://img.shields.io/badge/Made%20with-Go%201.22-00ADD8.svg)](https://go.dev)
[![Docker](https://img.shields.io/badge/Docker-Compose-2496ED.svg)](https://docs.docker.com/compose/)
[![Current version](https://img.shields.io/badge/version-v1.6.3-success.svg)](https://github.com/UncleFi1/step-ca-ui/releases/tag/v1.6.3)
[![Latest release](https://img.shields.io/badge/release-v1.6.3-success.svg)](https://github.com/UncleFi1/step-ca-ui/releases/latest)

🇬🇧 **English** · [🇷🇺 Русский](README.ru.md)

</div>

---

> A small-team-friendly web UI on top of `smallstep/step-ca`. No SaaS, no telemetry, no vendor lock-in — runs entirely on your own server in three Docker containers.

## Current Release

**Latest stable:** [v1.6.3](https://github.com/UncleFi1/step-ca-ui/releases/tag/v1.6.3)

Highlights:
- password recovery UX cleanup: neutral result message, no account enumeration and cleared identifier field after submit
- password recovery with SMTP, expiring reset tokens, rate limiting and audit events
- admin audit hardening for user actions, key downloads, backups and settings changes
- responsive top navigation with dropdown groups for narrow windows
- TOTP 2FA with authenticator app enrollment, QR code and recovery codes
- login 2FA challenge after password verification
- polished main page, 2FA page and certificate list layout
- responsive certificate list for desktop, laptop and narrow browser windows

## Features

- 📋 **Certificate management** — issue, renew, revoke and import X.509 certificates
- 👥 **Role-based access** — `admin` / `manager` / `viewer`
- ⏱️ **Temporary users** — short-lived guest accounts with automatic expiration *(new in v1.4.0)*
- 📅 **Custom date picker** — site-themed, no native browser widget *(new in v1.4.0)*
- 🌍 **Timezone-aware** — configurable with the `TZ` environment variable
- 🎨 **4 themes** — dark, light, blue, auto (follows OS)
- 🧭 **Admin workspace** — polished admin UI with matching dark, light and blue themes *(new in v1.4.11)*
- 🛡️ **Built-in security** — CSRF tokens, rate limiting, IP blocking, security log
- 🌐 **Provisioner inspection** — list and edit step-ca provisioners
- 💾 **Backup export** — admin UI and CLI backup bundles with manifest checksums *(new in v1.4.9)*
- 🔎 **CA integrity checks** — root/intermediate chain, provisioner claims, password sync and pinned step-ca image *(new in v1.5.0)*
- 🔬 **Certificate details** — SANs, fingerprints, key usage, cert/key pair and chain validation *(new in v1.5.1)*
- 🧩 **Certificate templates** — server, internal service, wildcard and client identity presets *(new in v1.5.2)*
- 🔔 **Webhook notifications** — test webhook, failed issue/renew alerts, login burst alerts and expiry watcher *(new in v1.5.3)*
- 🔐 **TOTP 2FA** — authenticator app enrollment, QR code login challenge and recovery codes *(new in v1.6.0)*
- 🔁 **Password recovery** — SMTP reset links with TTL, rate limit, audit trail and neutral public responses *(updated in v1.6.3)*

## Quick Start

```bash
git clone https://github.com/UncleFi1/step-ca-ui.git
cd step-ca-ui
sudo ./install.sh
```

The installer can run in Russian or English and supports both clean installs and
safe updates:

```bash
sudo ./install.sh --mode install --lang en
sudo ./install.sh --mode update --lang en
```

Update mode creates a backup first, preserves `.env` and Docker volumes, then
runs `docker compose up -d --build`. It does not run `docker compose down -v`.

That's it. The installer:
1. Detects your OS and installs Docker if needed
2. Auto-detects your server IP (with confirmation)
3. Generates strong passwords for everything
4. Writes `.env` and `credentials.txt` (chmod 600)
5. Builds and starts the containers
6. Prints the URL and admin password

The whole thing takes 2–4 minutes on a fresh VM.

## Requirements

|                | Minimum | Recommended | High-load |
|----------------|---------|-------------|-----------|
| **CPU**        | 1 vCPU  | 2 vCPU      | 4+ vCPU   |
| **RAM**        | 1 GB    | 2 GB        | 4+ GB     |
| **Disk**       | 5 GB    | 20 GB SSD   | 50+ GB NVMe |
| **Network**    | 10 Mbit/s | 100 Mbit/s | 1 Gbit/s  |
| **Users**      | up to 50 | up to 500  | 500+      |
| **Certificates**| up to 500 | up to 10k | 10k+     |

**Software:**
- Linux kernel 4.4+ (Ubuntu 20.04+, Debian 11+, CentOS Stream 9+, Rocky 9+, Alma 9+)
- Docker Engine 20.10+ with Compose plugin v2+
- Open ports: `443/tcp` (HTTPS UI), optionally `9000/tcp` (step-ca API)

> Untested but should work: macOS / Windows via Docker Desktop (development only). \
> **Not supported:** shared hosting without Docker, Raspberry Pi Zero (insufficient RAM).

## Stack

| Layer        | Technology                  |
|--------------|-----------------------------|
| Backend      | Go 1.22, [chi](https://github.com/go-chi/chi) router |
| Frontend     | Server-rendered HTML + vanilla JS, no build step |
| Database     | PostgreSQL 16 |
| CA           | [smallstep/step-ca](https://hub.docker.com/r/smallstep/step-ca) |
| Deploy       | Docker Compose |
| Container OS | Alpine 3.19 + tzdata        |

## Architecture

```
                          ┌────────────┐
   Browser  ─── HTTPS ───►│  step-ui   │  Go web app, port 8443
                          │  (chi)     │
                          └──┬─────┬───┘
                             │     │
                  SQL ◄──────┘     └──────► HTTPS API
                             │     │
                          ┌──▼──┐ ┌▼──────────┐
                          │ pg  │ │ step-ca   │  port 9000
                          │ 16  │ │ (PKI)     │
                          └─────┘ └───────────┘

   step-ui exposes :443  →  internally redirects to :8443
   step-ca exposes :9000 →  internal-only by default
```

## Roles

| Role    | View | Issue/Import | Revoke | Manage Users |
|---------|------|--------------|--------|--------------|
| viewer  | ✅   | ❌           | ❌     | ❌           |
| manager | ✅   | ✅           | ❌     | ❌           |
| admin   | ✅   | ✅           | ✅     | ✅           |

**Temporary users** can have any role; they're auto-blocked when `expires_at` passes (a goroutine checks every minute).

## Security

- ✅ **CSRF protection** — tokens on every form and server-side checks on POST routes
- ✅ **Rate limiting** — 5 failed login attempts → 15-minute IP block
- ✅ **Security headers** — CSP, X-Frame-Options, X-Content-Type-Options, Referrer-Policy, optional HSTS
- ✅ **Session timeout** — 8 hours, sliding
- ✅ **Login audit log** — every login attempt is recorded with IP and User-Agent
- ✅ **Self-signed TLS** — auto-generated on first boot, 10-year validity
- ✅ **Password hashing** — bcrypt for new/updated passwords, with transparent migration from legacy SHA-256 hashes on next successful login

> 🔒 **Production tip:** put step-ui behind a reverse proxy (Caddy/nginx) with a real TLS certificate, restrict access via VPN/Tailscale, and back up the `step-ca-data` volume regularly.

## Configuration

All configuration lives in `.env`. The installer creates this file for you, but you can edit it manually:

```env
HOST_IP=192.168.1.100              # SAN in self-signed cert; step-ca DNS
UI_HTTPS_PORT=443                  # external HTTPS port
PROVISIONER=admin                  # step-ca provisioner identifier
CA_PASSWORD=<generated>            # step-ca provisioner password
STEP_CA_IMAGE=smallstep/step-ca:0.30.2 # pinned step-ca image
SECRET_KEY=<generated>             # session/CSRF signing key
SESSION_SECURE=true                # secure session cookie over HTTPS
ENABLE_HSTS=false                  # enable only when using a trusted TLS certificate
POSTGRES_PASSWORD=<generated>      # database password
TZ=UTC                             # container timezone
STEPCA_DEFAULT_TLS_CERT_DURATION=8760h
STEPCA_MAX_TLS_CERT_DURATION=87600h
```

After changing `.env`, recreate the containers:

```bash
sudo docker compose up -d --force-recreate
```

## FAQ

<details>
<summary><b>How do I change the HTTPS port from 443?</b></summary>

Edit `docker-compose.yml`:
```yaml
services:
  step-ui:
    ports:
      - "8443:8443"   # was "443:8443"
```
Then restart: `sudo docker compose up -d --force-recreate step-ui`.
</details>

<details>
<summary><b>How do I back up and restore the data?</b></summary>

Use the admin UI: `Admin -> Backup -> Download backup bundle`.

CLI export is also supported:

```bash
sudo ./install.sh --mode backup --lang en
```

Backups include PostgreSQL, `step-ca-data`, Step-CA UI data/certs/uploads and
`manifest.json` with SHA-256 checksums. Restore is manual by design; follow
[BACKUP_RESTORE.md](BACKUP_RESTORE.md).
</details>

<details>
<summary><b>How do I reset the admin password?</b></summary>

```bash
sudo docker compose exec postgres psql -U stepui -d stepui -c \
  "UPDATE users SET password_hash = encode(sha256('newpass'::bytea), 'hex') WHERE username='admin';"
```
Then log in with `admin` / `newpass` and change it from the UI.
The legacy SHA-256 reset value is accepted for recovery and is rehashed to bcrypt after login.
</details>

<details>
<summary><b>The browser warns about a self-signed certificate. How do I use my own?</b></summary>

Replace `step-ui-go/ssl/server.crt` and `server.key` with your own cert + key (e.g. from Let's Encrypt or your internal CA), then restart `step-ui`. Make sure the cert covers your `HOST_IP` or hostname.
</details>

<details>
<summary><b>Can I run this behind Cloudflare / Caddy / nginx?</b></summary>

Yes. Point your reverse proxy at `step-ui:8443` (HTTPS upstream) or change step-ui to plain HTTP and put TLS termination on the proxy. Set `X-Forwarded-Proto: https` so step-ui generates correct URLs.
</details>

<details>
<summary><b>How do I update to a new version?</b></summary>

```bash
sudo ./install.sh --mode update --lang en
```
The update mode creates a backup first, keeps existing Docker volumes, optionally checks out a selected tag, and runs migrations automatically on startup. Always check the [release notes](https://github.com/UncleFi1/step-ca-ui/releases) first — major versions may have breaking changes.
</details>

## Contributing

Pull requests are welcome. For major changes, please open an issue first to discuss what you'd like to change.

```bash
git clone https://github.com/UncleFi1/step-ca-ui.git
cd step-ca-ui/step-ui-go
go mod download
go run .  # requires running postgres + step-ca
```

When submitting:
- Run `gofmt -w .` and `go vet ./...`
- Update relevant tests
- Keep commits focused and descriptive

## Project structure

```
.
├── docker-compose.yml         # 3 services: postgres, step-ca, step-ui
├── .env.example               # configuration template
├── install.sh                 # one-shot installer
├── LICENSE                    # GPL-3.0
├── README.md                  # this file (English)
├── README.ru.md               # Russian translation
└── step-ui-go/
    ├── main.go                # entry point, router setup
    ├── config/                # env-based config loader
    ├── db/                    # all SQL queries
    ├── handlers/              # HTTP handlers (one file per area)
    ├── middleware/            # auth, security headers, CSRF
    ├── models/                # data structs
    ├── security/              # password hashing, rate limiting, CSRF
    ├── templates/             # HTML templates (Go html/template)
    ├── static/                # CSS, JS, favicon, images
    ├── Dockerfile             # multi-stage Alpine build
    └── entrypoint.sh          # waits for deps, generates SSL, starts app
```

## License

This project is licensed under the **GNU General Public License v3.0** — see the [LICENSE](LICENSE) file for details.

In short: you can use, modify, and distribute this software, but any derivative work must also be released under GPLv3.
