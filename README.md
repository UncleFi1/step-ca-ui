<div align="center">

# Step-CA UI

**Self-hosted web interface for [Smallstep step-ca](https://smallstep.com/docs/step-ca/) — manage your private PKI from a browser.**

[![License: GPL v3](https://img.shields.io/badge/License-GPLv3-blue.svg)](https://www.gnu.org/licenses/gpl-3.0)
[![Made with Go](https://img.shields.io/badge/Made%20with-Go%201.22-00ADD8.svg)](https://go.dev)
[![Docker](https://img.shields.io/badge/Docker-Compose-2496ED.svg)](https://docs.docker.com/compose/)
[![Latest release](https://img.shields.io/badge/release-v1.4.0-success.svg)](https://github.com/UncleFi1/step-ca-ui/releases/latest)

🇬🇧 **English** · [🇷🇺 Русский](README.ru.md)

</div>

---

> A small-team-friendly web UI on top of `smallstep/step-ca`. No SaaS, no telemetry, no vendor lock-in — runs entirely on your own server in three Docker containers.

## Features

- 📋 **Certificate management** — issue, renew, revoke and import X.509 certificates
- 👥 **Role-based access** — `admin` / `manager` / `viewer`
- ⏱️ **Temporary users** — short-lived guest accounts with automatic expiration *(new in v1.4.0)*
- 📅 **Custom date picker** — site-themed, no native browser widget *(new in v1.4.0)*
- 🌍 **Timezone-aware** — Moscow time by default, easily configurable
- 🎨 **4 themes** — dark, light, blue, auto (follows OS)
- 🛡️ **Built-in security** — CSRF tokens, rate limiting, IP blocking, security log
- 🌐 **Provisioner inspection** — list and edit step-ca provisioners

## Quick Start

```bash
git clone https://github.com/UncleFi1/step-ca-ui.git
cd step-ca-ui
./install.sh
```

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

- ✅ **CSRF tokens** on every form
- ✅ **Rate limiting** — 5 failed login attempts → 15-minute IP block
- ✅ **Security headers** — HSTS, CSP, X-Frame-Options, X-Content-Type-Options, Referrer-Policy
- ✅ **Session timeout** — 8 hours, sliding
- ✅ **Login audit log** — every login attempt is recorded with IP and User-Agent
- ✅ **Self-signed TLS** — auto-generated on first boot, 10-year validity
- ⚠️ **Password hashing** — currently SHA-256 *(legacy from v1.0; bcrypt migration tracked in [#1](https://github.com/UncleFi1/step-ca-ui/issues))*

> 🔒 **Production tip:** put step-ui behind a reverse proxy (Caddy/nginx) with a real TLS certificate, restrict access via VPN/Tailscale, and back up the `step-ca-data` volume regularly.

## Configuration

All configuration lives in `.env`. The installer creates this file for you, but you can edit it manually:

```env
HOST_IP=192.168.1.100              # SAN in self-signed cert; step-ca DNS
PROVISIONER=admin@example.com      # step-ca provisioner identifier
CA_PASSWORD=<generated>            # step-ca provisioner password
SECRET_KEY=<generated>             # session/CSRF signing key
POSTGRES_PASSWORD=<generated>      # database password
```

After changing `.env`, recreate the containers:

```bash
docker compose up -d --force-recreate
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
Then restart: `docker compose up -d --force-recreate step-ui`.
</details>

<details>
<summary><b>How do I back up the data?</b></summary>

Two volumes contain everything stateful:
- `step-ca-data` — root CA private keys, intermediate certs, provisioners
- PostgreSQL volume — users, certificates metadata, logs

```bash
# Backup
docker compose exec postgres pg_dump -U stepui stepui > backup.sql
docker run --rm -v step-ca-data:/src -v "$PWD":/dst alpine tar czf /dst/ca-data.tgz -C /src .

# Restore
docker compose exec -T postgres psql -U stepui stepui < backup.sql
docker run --rm -v step-ca-data:/dst -v "$PWD":/src alpine tar xzf /src/ca-data.tgz -C /dst
```
</details>

<details>
<summary><b>How do I reset the admin password?</b></summary>

```bash
docker compose exec postgres psql -U stepui -d stepui -c \
  "UPDATE users SET password_hash = encode(sha256('newpass'::bytea), 'hex') WHERE username='admin';"
```
Then log in with `admin` / `newpass` and change it from the UI.
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
git pull
docker compose up -d --build
```
Migrations run automatically on startup. Always check the [release notes](https://github.com/UncleFi1/step-ca-ui/releases) first — major versions may have breaking changes.
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
