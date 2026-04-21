#!/usr/bin/env bash
# ──────────────────────────────────────────────────────────────────────────────
# Step-CA UI — installer
# https://github.com/UncleFi1/step-ca-ui
#
# What it does:
#   1. Detects OS and architecture
#   2. Installs Docker + Docker Compose (if missing)
#   3. Auto-detects host IP (with manual override)
#   4. Generates strong secrets (CA / DB / Session / Admin password)
#   5. Writes .env from .env.example
#   6. Builds and starts containers
#   7. Waits for healthcheck and prints credentials
#
# Idempotent: re-running won't overwrite an existing .env unless you confirm.
# ──────────────────────────────────────────────────────────────────────────────

set -euo pipefail

# ─── colors ───────────────────────────────────────────────────────────────────
if [[ -t 1 ]]; then
  C_RESET=$'\033[0m'; C_BOLD=$'\033[1m'; C_DIM=$'\033[2m'
  C_RED=$'\033[31m'; C_GREEN=$'\033[32m'; C_YELLOW=$'\033[33m'
  C_BLUE=$'\033[34m'; C_CYAN=$'\033[36m'
else
  C_RESET=''; C_BOLD=''; C_DIM=''; C_RED=''; C_GREEN=''; C_YELLOW=''; C_BLUE=''; C_CYAN=''
fi

step()   { printf "\n${C_BOLD}${C_BLUE}▸ %s${C_RESET}\n" "$*"; }
ok()     { printf "${C_GREEN}✓${C_RESET} %s\n" "$*"; }
warn()   { printf "${C_YELLOW}⚠${C_RESET} %s\n" "$*"; }
err()    { printf "${C_RED}✗${C_RESET} %s\n" "$*" >&2; }
die()    { err "$*"; exit 1; }
ask()    { local prompt="$1" default="${2:-}" reply
           if [[ -n "$default" ]]; then
             read -r -p "$(printf "${C_CYAN}?${C_RESET} %s ${C_DIM}[%s]${C_RESET}: " "$prompt" "$default")" reply
             echo "${reply:-$default}"
           else
             read -r -p "$(printf "${C_CYAN}?${C_RESET} %s: " "$prompt")" reply
             echo "$reply"
           fi
         }
confirm() { local prompt="$1" default="${2:-N}" reply
            local hint="[y/N]"; [[ "${default^^}" == "Y" ]] && hint="[Y/n]"
            read -r -p "$(printf "${C_CYAN}?${C_RESET} %s ${C_DIM}%s${C_RESET}: " "$prompt" "$hint")" reply
            reply="${reply:-$default}"
            [[ "${reply^^}" =~ ^Y(ES)?$ ]]
          }

# ─── banner ───────────────────────────────────────────────────────────────────
cat <<'BANNER'

   ┌─────────────────────────────────────────────────┐
   │                                                 │
   │     STEP-CA UI — installer                      │
   │     Self-hosted PKI management for your LAN     │
   │                                                 │
   └─────────────────────────────────────────────────┘

BANNER

# ─── 1. environment checks ────────────────────────────────────────────────────
step "[1/7] Checking environment"

if [[ $EUID -ne 0 ]]; then
  if command -v sudo >/dev/null 2>&1; then
    SUDO="sudo"
    ok "Running as non-root, will use sudo for privileged operations"
  else
    die "This script must run as root or with sudo installed"
  fi
else
  SUDO=""
  ok "Running as root"
fi

OS_ID=""
OS_VERSION=""
if [[ -f /etc/os-release ]]; then
  # shellcheck disable=SC1091
  . /etc/os-release
  OS_ID="${ID:-unknown}"
  OS_VERSION="${VERSION_ID:-unknown}"
fi

case "$OS_ID" in
  ubuntu|debian)
    PKG_MANAGER="apt"
    ok "Detected: $OS_ID $OS_VERSION (apt-based)"
    ;;
  centos|rhel|rocky|almalinux|fedora)
    PKG_MANAGER="dnf"
    command -v dnf >/dev/null 2>&1 || PKG_MANAGER="yum"
    ok "Detected: $OS_ID $OS_VERSION ($PKG_MANAGER-based)"
    ;;
  *)
    warn "Unsupported or unknown OS: $OS_ID. Will try to continue."
    PKG_MANAGER=""
    ;;
esac

ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64) ok "Architecture: x86_64" ;;
  aarch64|arm64) ok "Architecture: ARM64" ;;
  *) warn "Untested architecture: $ARCH (Docker images are linux/amd64 + linux/arm64)" ;;
esac

# ─── 2. install Docker if missing ─────────────────────────────────────────────
step "[2/7] Checking Docker"

install_docker() {
  warn "Docker not found — installing via get.docker.com"
  if ! command -v curl >/dev/null 2>&1; then
    if [[ "$PKG_MANAGER" == "apt" ]]; then
      $SUDO apt-get update -qq && $SUDO apt-get install -y -qq curl
    elif [[ -n "$PKG_MANAGER" ]]; then
      $SUDO "$PKG_MANAGER" install -y curl
    else
      die "curl is required but not installed"
    fi
  fi
  curl -fsSL https://get.docker.com | $SUDO sh
  $SUDO systemctl enable --now docker
  ok "Docker installed and started"
}

if command -v docker >/dev/null 2>&1; then
  DOCKER_VERSION=$(docker --version | grep -oE '[0-9]+\.[0-9]+' | head -1)
  ok "Docker found: $DOCKER_VERSION"
else
  install_docker
fi

if docker compose version >/dev/null 2>&1; then
  COMPOSE_CMD="docker compose"
  ok "Docker Compose plugin available"
elif command -v docker-compose >/dev/null 2>&1; then
  COMPOSE_CMD="docker-compose"
  warn "Using legacy 'docker-compose' binary. Consider upgrading to Compose plugin."
else
  die "Docker Compose not found. Install Docker Compose plugin: https://docs.docker.com/compose/install/"
fi

if ! $SUDO docker info >/dev/null 2>&1; then
  die "Docker daemon is not running or current user lacks permission. Try: sudo systemctl start docker"
fi

# ─── 3. project files presence ────────────────────────────────────────────────
step "[3/7] Checking project files"

PROJECT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$PROJECT_DIR"

[[ -f docker-compose.yml ]] || die "docker-compose.yml not found in $PROJECT_DIR"
[[ -f .env.example ]]       || die ".env.example not found in $PROJECT_DIR"
[[ -d step-ui-go ]]         || die "step-ui-go/ directory not found in $PROJECT_DIR"
ok "All required files present"

# ─── 4. configuration ─────────────────────────────────────────────────────────
step "[4/7] Configuration"

detect_ip() {
  local ip=""
  ip=$(curl -fsS --max-time 3 https://ifconfig.me 2>/dev/null || true)
  if [[ -z "$ip" ]] || ! [[ "$ip" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    ip=$(hostname -I 2>/dev/null | awk '{print $1}')
  fi
  if [[ -z "$ip" ]] || ! [[ "$ip" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    ip=$(ip -4 route get 1.1.1.1 2>/dev/null | awk '{print $7; exit}')
  fi
  echo "$ip"
}

DETECTED_IP=$(detect_ip)
if [[ -n "$DETECTED_IP" ]]; then
  ok "Auto-detected IP: ${C_BOLD}$DETECTED_IP${C_RESET}"
  HOST_IP=$(ask "Use this IP (or enter another)" "$DETECTED_IP")
else
  warn "Could not auto-detect IP"
  HOST_IP=$(ask "Enter the IP address of this server")
fi
[[ "$HOST_IP" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "Invalid IP address: $HOST_IP"

PROVISIONER_DEFAULT="admin@$(hostname -s 2>/dev/null || echo "localhost").local"
PROVISIONER=$(ask "Provisioner email/identifier" "$PROVISIONER_DEFAULT")

# ─── 5. generate secrets ──────────────────────────────────────────────────────
step "[5/7] Generating secrets"

gen_secret() {
  openssl rand -base64 48 2>/dev/null | tr -dc 'A-Za-z0-9' | head -c "${1:-32}"
}
gen_password() {
  # Human-readable: no ambiguous 0/O/1/l/I, mixed case + digits
  openssl rand -base64 32 2>/dev/null | tr -dc 'A-HJ-NP-Za-km-z2-9' | head -c "${1:-12}"
}

CA_PASSWORD=$(gen_secret 24)
SECRET_KEY=$(gen_secret 48)
POSTGRES_PASSWORD=$(gen_secret 24)
ADMIN_PASSWORD=$(gen_password 12)
ok "Generated CA_PASSWORD       (24 chars)"
ok "Generated SECRET_KEY        (48 chars)"
ok "Generated POSTGRES_PASSWORD (24 chars)"
ok "Generated ADMIN_PASSWORD    (12 chars, human-readable)"

# ─── 6. write .env ────────────────────────────────────────────────────────────
step "[6/7] Writing .env"

if [[ -f .env ]]; then
  warn ".env already exists at $PROJECT_DIR/.env"
  if confirm "Overwrite it? (existing passwords will be lost)" "N"; then
    cp .env ".env.backup.$(date +%s)"
    ok "Backed up old .env"
  else
    die "Aborted by user. Existing .env preserved."
  fi
fi

cat > .env <<EOF
# ──────────────────────────────────────────────────────────────
# Step-CA UI — environment configuration
# Generated by install.sh on $(date '+%Y-%m-%d %H:%M:%S %Z')
# ──────────────────────────────────────────────────────────────

# Host IP — used for SSL cert SAN and Step-CA DNS names
HOST_IP=${HOST_IP}

# Step-CA provisioner identifier (usually an email)
PROVISIONER=${PROVISIONER}

# Step-CA provisioner password (auto-generated)
CA_PASSWORD=${CA_PASSWORD}

# Session/CSRF signing key for the UI (auto-generated, keep secret)
SECRET_KEY=${SECRET_KEY}

# PostgreSQL password (auto-generated)
POSTGRES_PASSWORD=${POSTGRES_PASSWORD}

# One-shot: admin password used by entrypoint to seed the first user.
# Safe to remove from .env after first successful boot.
STEPUI_ADMIN_PASSWORD=${ADMIN_PASSWORD}
EOF
chmod 600 .env
ok "Wrote .env (chmod 600)"

CRED_FILE="$PROJECT_DIR/credentials.txt"
cat > "$CRED_FILE" <<EOF
═══════════════════════════════════════════════════════════════════
  Step-CA UI — admin credentials
  Generated on: $(date '+%Y-%m-%d %H:%M:%S %Z')
═══════════════════════════════════════════════════════════════════

  URL:      https://${HOST_IP}
  Login:    admin
  Password: ${ADMIN_PASSWORD}

═══════════════════════════════════════════════════════════════════
  ⚠  IMPORTANT: change this password right after first login!
     Profile → Change Password

  This file has chmod 600 (owner-read only).
  Delete it once you've stored the password somewhere safe:
     rm credentials.txt
═══════════════════════════════════════════════════════════════════
EOF
chmod 600 "$CRED_FILE"
ok "Saved admin credentials to credentials.txt (chmod 600)"

# ─── 7. build & launch ────────────────────────────────────────────────────────
step "[7/7] Building and starting containers"
echo

$SUDO $COMPOSE_CMD pull --quiet 2>/dev/null || true
$SUDO $COMPOSE_CMD up -d --build

echo
step "Waiting for services to be healthy"

wait_for_healthy() {
  local service="$1" timeout="${2:-120}" elapsed=0
  while (( elapsed < timeout )); do
    local status
    status=$($SUDO $COMPOSE_CMD ps --format '{{.Service}}|{{.Status}}' 2>/dev/null \
            | awk -F'|' -v svc="$service" '$1==svc {print $2; exit}')
    if [[ "$status" == *"healthy"* ]]; then
      ok "$service is healthy"
      return 0
    elif [[ "$status" == *"unhealthy"* ]]; then
      err "$service is unhealthy"
      return 1
    fi
    sleep 2
    ((elapsed+=2))
    printf "."
  done
  echo
  warn "$service did not report healthy within ${timeout}s (it may still be starting)"
  return 1
}

ALL_HEALTHY=true
for svc in postgres step-ca step-ui; do
  printf "  %s " "$svc"
  wait_for_healthy "$svc" 120 || ALL_HEALTHY=false
done

# ─── final report ─────────────────────────────────────────────────────────────
echo
echo
if $ALL_HEALTHY; then
  printf "${C_GREEN}${C_BOLD}╔══════════════════════════════════════════════════════════════╗${C_RESET}\n"
  printf "${C_GREEN}${C_BOLD}║                                                              ║${C_RESET}\n"
  printf "${C_GREEN}${C_BOLD}║              ✓ Step-CA UI is up and running                  ║${C_RESET}\n"
  printf "${C_GREEN}${C_BOLD}║                                                              ║${C_RESET}\n"
  printf "${C_GREEN}${C_BOLD}╚══════════════════════════════════════════════════════════════╝${C_RESET}\n"
else
  printf "${C_YELLOW}${C_BOLD}╔══════════════════════════════════════════════════════════════╗${C_RESET}\n"
  printf "${C_YELLOW}${C_BOLD}║          ⚠  Containers started but not yet healthy           ║${C_RESET}\n"
  printf "${C_YELLOW}${C_BOLD}║          Check logs: docker compose logs -f                  ║${C_RESET}\n"
  printf "${C_YELLOW}${C_BOLD}╚══════════════════════════════════════════════════════════════╝${C_RESET}\n"
fi

cat <<EOF

  ${C_BOLD}URL:${C_RESET}      https://${HOST_IP}
  ${C_BOLD}Login:${C_RESET}    admin
  ${C_BOLD}Password:${C_RESET} ${ADMIN_PASSWORD}

  ${C_DIM}(Also saved to credentials.txt — delete it after copying the password)${C_RESET}

  ${C_YELLOW}⚠ Change the admin password right after first login!${C_RESET}
  ${C_YELLOW}⚠ The SSL certificate is self-signed${C_RESET} — your browser will warn
     you on first visit. Accept the warning, or import certs/root_ca.crt
     into your trusted store.

  ${C_BOLD}Useful commands:${C_RESET}
    ${C_DIM}# View logs${C_RESET}
    docker compose logs -f step-ui

    ${C_DIM}# Stop / start${C_RESET}
    docker compose down
    docker compose up -d

    ${C_DIM}# Update to latest version${C_RESET}
    git pull && docker compose up -d --build

EOF
