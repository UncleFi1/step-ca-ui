#!/usr/bin/env bash
# ──────────────────────────────────────────────────────────────────────────────
# Step-CA UI — installer (quiet mode)
# https://github.com/UncleFi1/step-ca-ui
#
# Only prints a short progress line for each step. Full output goes to
# /var/log/step-ca-ui-install.log (or /tmp/step-ca-ui-install-<ts>.log if
# /var/log isn't writable).
#
# Run with --verbose to see the original detailed output instead.
# ──────────────────────────────────────────────────────────────────────────────
set -euo pipefail

# ── log setup ────────────────────────────────────────────────────────────────
VERBOSE=0
for a in "$@"; do
  [[ "$a" == "--verbose" || "$a" == "-v" ]] && VERBOSE=1
done

LOG_FILE=/var/log/step-ca-ui-install.log
if ! touch "$LOG_FILE" 2>/dev/null; then
  LOG_FILE="/tmp/step-ca-ui-install-$(date +%s).log"
  touch "$LOG_FILE"
fi
: > "$LOG_FILE"
chmod 600 "$LOG_FILE" 2>/dev/null || true

if [[ -t 1 ]]; then
  C_RESET=$'\033[0m'; C_BOLD=$'\033[1m'; C_DIM=$'\033[2m'
  C_RED=$'\033[31m'; C_GREEN=$'\033[32m'; C_YELLOW=$'\033[33m'
  C_BLUE=$'\033[34m'; C_CYAN=$'\033[36m'
else
  C_RESET=''; C_BOLD=''; C_DIM=''; C_RED=''; C_GREEN=''; C_YELLOW=''; C_BLUE=''; C_CYAN=''
fi

# ── runtime helpers ──────────────────────────────────────────────────────────
# run — execute a command, hide output (unless verbose)
run() {
  if [[ "$VERBOSE" == "1" ]]; then
    "$@" 2>&1 | tee -a "$LOG_FILE"
    return "${PIPESTATUS[0]}"
  else
    "$@" >>"$LOG_FILE" 2>&1
  fi
}

# Echo to log only
_log() { printf '%s\n' "$*" >>"$LOG_FILE"; }

# UI helpers
say_step() { printf "${C_BOLD}${C_BLUE}▸${C_RESET} %s" "$*"; _log "▸ $*"; }
say_ok()   { printf " ${C_GREEN}✓${C_RESET}\n"; _log "  OK"; }
say_fail() { printf " ${C_RED}✗${C_RESET}\n"; _log "  FAIL"; }
say_warn() { printf "\n  ${C_YELLOW}⚠${C_RESET} %s\n" "$*"; _log "WARN: $*"; }
say_info() { printf "  ${C_DIM}%s${C_RESET}\n" "$*"; _log "INFO: $*"; }
die() {
  printf "\n${C_RED}${C_BOLD}✗ Installation failed${C_RESET}\n" >&2
  printf "  Error: %s\n" "$*" >&2
  printf "  Full log: ${C_BOLD}%s${C_RESET}\n" "$LOG_FILE" >&2
  exit 1
}

# Trap unexpected errors
trap 'die "unexpected error at line $LINENO (see log)"' ERR

ask() {
  local prompt="$1" default="${2:-}" reply
  if [[ -n "$default" ]]; then
    read -r -p "$(printf "${C_CYAN}?${C_RESET} %s ${C_DIM}[%s]${C_RESET}: " "$prompt" "$default")" reply
    echo "${reply:-$default}"
  else
    read -r -p "$(printf "${C_CYAN}?${C_RESET} %s: " "$prompt")" reply
    echo "$reply"
  fi
}
confirm() {
  local prompt="$1" default="${2:-N}" reply
  local hint="[y/N]"; [[ "${default^^}" == "Y" ]] && hint="[Y/n]"
  read -r -p "$(printf "${C_CYAN}?${C_RESET} %s ${C_DIM}%s${C_RESET}: " "$prompt" "$hint")" reply
  reply="${reply:-$default}"
  [[ "${reply^^}" =~ ^Y(ES)?$ ]]
}

# ─── banner ──────────────────────────────────────────────────────────────────
cat <<'BANNER'

   ┌─────────────────────────────────────────────────┐
   │                                                 │
   │     STEP-CA UI — installer                      │
   │     Self-hosted PKI management for your LAN     │
   │                                                 │
   └─────────────────────────────────────────────────┘

BANNER
_log "=== install.sh started at $(date -Is) ==="
_log "Log file: $LOG_FILE"
_log "Verbose:  $VERBOSE"

# ── 1. environment check ─────────────────────────────────────────────────────
say_step "Checking environment             "
{
  if [[ $EUID -ne 0 ]]; then
    if command -v sudo >/dev/null 2>&1; then SUDO="sudo"; else die "run as root or install sudo"; fi
  else
    SUDO=""
  fi

  OS_ID=""
  if [[ -f /etc/os-release ]]; then . /etc/os-release; OS_ID="${ID:-unknown}"; fi
  case "$OS_ID" in
    ubuntu|debian)  PKG_MANAGER="apt" ;;
    centos|rhel|rocky|almalinux|fedora)
      PKG_MANAGER="dnf"; command -v dnf >/dev/null 2>&1 || PKG_MANAGER="yum" ;;
    *) PKG_MANAGER="" ;;
  esac
  _log "OS: $OS_ID, pkg: ${PKG_MANAGER:-(unknown)}"
  _log "Arch: $(uname -m)"
} >>"$LOG_FILE" 2>&1
say_ok

# ── 2. docker ────────────────────────────────────────────────────────────────
say_step "Checking Docker                  "
{
  if ! command -v docker >/dev/null 2>&1; then
    _log "Docker not found — installing via get.docker.com"
    if ! command -v curl >/dev/null 2>&1; then
      if [[ "$PKG_MANAGER" == "apt" ]]; then $SUDO apt-get update -qq && $SUDO apt-get install -y -qq curl
      elif [[ -n "$PKG_MANAGER" ]]; then $SUDO "$PKG_MANAGER" install -y curl
      else die "curl is required"; fi
    fi
    curl -fsSL https://get.docker.com | $SUDO sh
    $SUDO systemctl enable --now docker
  fi

  if docker compose version >/dev/null 2>&1; then
    COMPOSE_CMD="docker compose"
  elif command -v docker-compose >/dev/null 2>&1; then
    COMPOSE_CMD="docker-compose"
  else
    die "Docker Compose not found"
  fi

  $SUDO docker info >/dev/null 2>&1 || die "Docker daemon not running"
} >>"$LOG_FILE" 2>&1
say_ok

# ── 3. project files ─────────────────────────────────────────────────────────
say_step "Checking project files           "
PROJECT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$PROJECT_DIR"
{
  [[ -f docker-compose.yml ]] || die "docker-compose.yml not found"
  [[ -f .env.example ]]       || die ".env.example not found"
  [[ -d step-ui-go ]]         || die "step-ui-go/ not found"
} >>"$LOG_FILE" 2>&1
say_ok

# ── 4. config (interactive part stays visible) ───────────────────────────────
printf "\n"
printf "${C_BOLD}${C_BLUE}▸${C_RESET} Configuration\n"

# Private-IP detection (same logic as before)
detect_private_ips() {
  ip -4 -o addr show 2>/dev/null \
    | awk '{
        split($4, a, "/"); ip = a[1]; iface = $2;
        if (iface ~ /^(lo|docker|br-|veth|cni|flannel|cali|tun|tailscale)/) next;
        if (ip ~ /^10\./)                             print ip "|" iface;
        else if (ip ~ /^192\.168\./)                  print ip "|" iface;
        else if (ip ~ /^172\.(1[6-9]|2[0-9]|3[01])\./) print ip "|" iface;
      }'
}
mapfile -t PRIVATE_IPS < <(detect_private_ips)
_log "Detected IPs: ${PRIVATE_IPS[*]}"

if [[ "${#PRIVATE_IPS[@]}" -eq 0 ]]; then
  say_warn "No private IPv4 addresses found on local interfaces."
  HOST_IP=$(ask "Enter the IP address of this server")

elif [[ "${#PRIVATE_IPS[@]}" -eq 1 ]]; then
  single="${PRIVATE_IPS[0]}"; single_ip="${single%%|*}"; single_iface="${single##*|}"
  printf "  Detected private IP: ${C_BOLD}%s${C_RESET} (%s)\n" "$single_ip" "$single_iface"
  HOST_IP=$(ask "Use this IP (or enter another)" "$single_ip")

else
  printf "  ${C_BOLD}Multiple private IPs detected:${C_RESET}\n"
  for i in "${!PRIVATE_IPS[@]}"; do
    entry="${PRIVATE_IPS[$i]}"; entry_ip="${entry%%|*}"; entry_iface="${entry##*|}"
    printf "    ${C_BOLD}[%d]${C_RESET}  %-15s  ${C_DIM}(%s)${C_RESET}\n" "$((i+1))" "$entry_ip" "$entry_iface"
  done
  echo
  while true; do
    choice=$(ask "Pick a number (or type a custom IP)" "1")
    if [[ "$choice" =~ ^[0-9]+$ ]] && (( choice >= 1 && choice <= ${#PRIVATE_IPS[@]} )); then
      entry="${PRIVATE_IPS[$((choice-1))]}"; HOST_IP="${entry%%|*}"; break
    elif [[ "$choice" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
      HOST_IP="$choice"; break
    else
      printf "  ${C_YELLOW}⚠${C_RESET} Invalid choice: '%s'.\n" "$choice"
    fi
  done
fi
[[ "$HOST_IP" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "Invalid IP: $HOST_IP"

PROVISIONER_DEFAULT="admin@$(hostname -s 2>/dev/null || echo "localhost").local"
PROVISIONER=$(ask "Provisioner email/identifier" "$PROVISIONER_DEFAULT")
_log "HOST_IP=$HOST_IP PROVISIONER=$PROVISIONER"
echo

# ── 5. generate secrets ──────────────────────────────────────────────────────
say_step "Generating secrets               "
gen_secret()  { openssl rand -base64 48 2>/dev/null | tr -dc 'A-Za-z0-9' | head -c "${1:-32}"; }
gen_password(){ openssl rand -base64 32 2>/dev/null | tr -dc 'A-HJ-NP-Za-km-z2-9' | head -c "${1:-12}"; }

CA_PASSWORD=$(gen_secret 24)
SECRET_KEY=$(gen_secret 48)
POSTGRES_PASSWORD=$(gen_secret 24)
ADMIN_PASSWORD=$(gen_password 12)
_log "Secrets generated (values not logged)"
say_ok

# ── 6. write .env ────────────────────────────────────────────────────────────
say_step "Writing .env + credentials.txt   "
{
  if [[ -f .env ]]; then
    if confirm ".env already exists — overwrite? (existing passwords will be lost)" "N" >&2; then
      cp .env ".env.backup.$(date +%s)"
    else
      die "Aborted by user. Existing .env preserved."
    fi
  fi >>"$LOG_FILE" 2>&1 || true

  cat > .env <<EOF
# Step-CA UI — environment configuration
# Generated by install.sh on $(date '+%Y-%m-%d %H:%M:%S %Z')
HOST_IP=${HOST_IP}
PROVISIONER=${PROVISIONER}
CA_PASSWORD=${CA_PASSWORD}
SECRET_KEY=${SECRET_KEY}
POSTGRES_PASSWORD=${POSTGRES_PASSWORD}
STEPUI_ADMIN_PASSWORD=${ADMIN_PASSWORD}
EOF
  chmod 600 .env

  cat > credentials.txt <<EOF
═══════════════════════════════════════════════════════════════════
  Step-CA UI — admin credentials
  Generated on: $(date '+%Y-%m-%d %H:%M:%S %Z')
═══════════════════════════════════════════════════════════════════

  URL:      https://${HOST_IP}
  Login:    admin
  Password: ${ADMIN_PASSWORD}

═══════════════════════════════════════════════════════════════════
  ⚠ IMPORTANT: change this password right after first login!
     Profile → Change Password

  This file has chmod 600 (owner-read only).
  Delete it once you've stored the password somewhere safe:
     rm credentials.txt
═══════════════════════════════════════════════════════════════════
EOF
  chmod 600 credentials.txt
} >>"$LOG_FILE" 2>&1
say_ok

# ── 7. build & launch ────────────────────────────────────────────────────────
say_step "Building and starting containers "
{
  $SUDO $COMPOSE_CMD pull --quiet 2>/dev/null || true
  $SUDO $COMPOSE_CMD up -d --build
} >>"$LOG_FILE" 2>&1
say_ok

# ── 8. wait for healthy ──────────────────────────────────────────────────────
say_step "Waiting for services to be ready "
wait_for_healthy() {
  local service="$1" timeout="${2:-120}" elapsed=0 status
  while (( elapsed < timeout )); do
    status=$($SUDO $COMPOSE_CMD ps --format '{{.Service}}|{{.Status}}' 2>/dev/null \
             | awk -F'|' -v svc="$service" '$1==svc {print $2; exit}')
    [[ "$status" == *"healthy"* ]]   && return 0
    [[ "$status" == *"unhealthy"* ]] && return 1
    sleep 2; ((elapsed+=2))
  done
  return 1
}

ALL_HEALTHY=true
for svc in postgres step-ca step-ui; do
  wait_for_healthy "$svc" 120 >>"$LOG_FILE" 2>&1 || ALL_HEALTHY=false
done

# HTTP-based fallback: if not reported healthy, but HTTPS is up, treat as ok
if ! $ALL_HEALTHY; then
  CODE=$(curl -sk -o /dev/null -w "%{http_code}" --max-time 5 "https://${HOST_IP}/login" 2>/dev/null || echo 000)
  _log "Fallback HTTPS probe: https://${HOST_IP}/login → $CODE"
  if [[ "$CODE" =~ ^(200|302|303)$ ]]; then
    ALL_HEALTHY=true
  fi
fi

if $ALL_HEALTHY; then
  say_ok
else
  say_fail
fi

# ── final report ─────────────────────────────────────────────────────────────
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
  printf "${C_YELLOW}${C_BOLD}║          docker compose logs -f                              ║${C_RESET}\n"
  printf "${C_YELLOW}${C_BOLD}╚══════════════════════════════════════════════════════════════╝${C_RESET}\n"
fi

cat <<EOF

  ${C_BOLD}URL:${C_RESET}      https://${HOST_IP}
  ${C_BOLD}Login:${C_RESET}    admin
  ${C_BOLD}Password:${C_RESET} ${ADMIN_PASSWORD}

  ${C_DIM}Credentials also saved to:  credentials.txt${C_RESET}
  ${C_DIM}Full install log:           ${LOG_FILE}${C_RESET}

  ${C_YELLOW}⚠${C_RESET} Change the admin password right after first login.
  ${C_YELLOW}⚠${C_RESET} The SSL certificate is self-signed — accept the browser warning
     or import certs/root_ca.crt into your trusted store.

EOF
_log "=== install.sh finished at $(date -Is) ==="
