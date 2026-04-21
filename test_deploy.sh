#!/usr/bin/env bash
# ──────────────────────────────────────────────────────────────────────────────
# Step-CA UI — deployment test
#
# Simulates a brand-new user installing from scratch:
#   1. Clones the repo into a temp directory
#   2. Runs install.sh
#   3. Probes the live endpoints
#   4. Reports ✓/✗ for each check, prints relevant logs on failure
#
# Run on a clean VM (preferably one that has NEVER had the project before):
#   curl -fsSL https://raw.githubusercontent.com/UncleFi1/step-ca-ui/main/test_deploy.sh | bash
# Or:
#   wget -O- https://raw.githubusercontent.com/UncleFi1/step-ca-ui/main/test_deploy.sh | bash
# ──────────────────────────────────────────────────────────────────────────────

set -uo pipefail

REPO_URL="${REPO_URL:-https://github.com/UncleFi1/step-ca-ui.git}"
BRANCH="${BRANCH:-main}"
TARGET_DIR="${TARGET_DIR:-/opt/step-ca-ui-test}"

# colors
if [[ -t 1 ]]; then
  R=$'\033[31m'; G=$'\033[32m'; Y=$'\033[33m'; B=$'\033[34m'; D=$'\033[2m'; X=$'\033[0m'
else
  R=''; G=''; Y=''; B=''; D=''; X=''
fi

PASS=0; FAIL=0
check() {
  local name="$1" cmd="$2" expected="${3:-0}"
  printf "  %-60s " "$name"
  local out rc
  out=$(eval "$cmd" 2>&1) ; rc=$?
  if [[ "$rc" -eq "$expected" ]]; then
    printf "${G}✓${X}\n"
    ((PASS++))
  else
    printf "${R}✗${X}\n"
    printf "${D}    rc=$rc, output: ${out:0:200}${X}\n"
    ((FAIL++))
  fi
}

cat <<EOF

${B}╔══════════════════════════════════════════════════════════════╗
║          Step-CA UI — automated deployment test              ║
╚══════════════════════════════════════════════════════════════╝${X}

  Repo:    $REPO_URL
  Branch:  $BRANCH
  Target:  $TARGET_DIR

EOF

# ── 1. clean target ───────────────────────────────────────────────────────────
echo "${B}▸ [1/5] Preparing target directory${X}"
if [[ -d "$TARGET_DIR" ]]; then
  echo "  ${Y}⚠${X} $TARGET_DIR exists, cleaning up containers and removing"
  if [[ -f "$TARGET_DIR/docker-compose.yml" ]]; then
    (cd "$TARGET_DIR" && docker compose down -v 2>/dev/null) || true
  fi
  rm -rf "$TARGET_DIR"
fi
mkdir -p "$(dirname "$TARGET_DIR")"
echo "  ${G}✓${X} Target prepared"

# ── 2. clone ──────────────────────────────────────────────────────────────────
echo
echo "${B}▸ [2/5] Cloning repo${X}"
if ! git clone --depth 1 --branch "$BRANCH" "$REPO_URL" "$TARGET_DIR" 2>&1 | tail -3; then
  echo "  ${R}✗${X} Clone failed"
  exit 1
fi
echo "  ${G}✓${X} Cloned $(cd "$TARGET_DIR" && git rev-parse --short HEAD)"

# ── 3. run installer ──────────────────────────────────────────────────────────
echo
echo "${B}▸ [3/5] Running install.sh (this will take 2–4 minutes)${X}"
cd "$TARGET_DIR"
chmod +x install.sh

# Auto-answer prompts: detected IP, default provisioner
# Pipe "y\n\n" provides defaults for all interactive questions
if ! echo -e "\n\n" | ./install.sh 2>&1 | tail -40; then
  echo "  ${R}✗${X} install.sh exited with non-zero code"
  exit 1
fi

# Detect what IP it ended up using
HOST_IP=$(grep '^HOST_IP=' .env | cut -d= -f2)
[[ -n "$HOST_IP" ]] || { echo "${R}Cannot detect HOST_IP from .env${X}"; exit 1; }
echo "  ${G}✓${X} install.sh completed, HOST_IP=$HOST_IP"

# ── 4. probe endpoints ────────────────────────────────────────────────────────
echo
echo "${B}▸ [4/5] Probing live endpoints${X}"

# Wait a bit for app to stabilize
sleep 5

check "containers running"            \
      "docker compose ps --format '{{.Service}} {{.State}}' | grep -c running" \
      0  # any non-error exit; we just check the command runs

check "step-ui container is healthy"  \
      "bash -c 'st=\$(docker compose ps --format \"{{.Service}}|{{.Status}}\" | grep \"^step-ui|\"); \
        echo \"\$st\" | grep -q healthy || { \
          echo \"\$st\" | grep -q \"Up \" && \
          curl -sk -o /dev/null -w \"%{http_code}\" --max-time 5 https://${HOST_IP}/login | grep -qE \"^(200|302|303)$\" ; \
        }'"

check "postgres container is healthy" \
      "docker compose ps --format '{{.Service}}|{{.Status}}' | grep '^postgres|' | grep -q healthy"

check "step-ca container is healthy"  \
      "docker compose ps --format '{{.Service}}|{{.Status}}' | grep '^step-ca|' | grep -q healthy"

check "HTTPS responds (any 2xx/3xx)"  \
      "curl -sk -o /dev/null -w '%{http_code}' --max-time 5 https://${HOST_IP}/login | grep -qE '^(200|302|303)$'"

check "/login serves HTML"            \
      "curl -sk --max-time 5 https://${HOST_IP}/login | grep -qi '<form'"

check "/static/css/base.css → 200"    \
      "curl -sk -o /dev/null -w '%{http_code}' --max-time 5 https://${HOST_IP}/static/css/base.css | grep -q '^200$'"

check "/static/js/datepicker.js → 200" \
      "curl -sk -o /dev/null -w '%{http_code}' --max-time 5 https://${HOST_IP}/static/js/datepicker.js | grep -q '^200$'"

check "credentials.txt was created"   \
      "test -f credentials.txt"

check "credentials.txt has chmod 600" \
      "stat -c '%a' credentials.txt | grep -q '^600$'"

check ".env has chmod 600"            \
      "stat -c '%a' .env | grep -q '^600$'"

check "no errors in step-ui logs (last 50 lines)" \
      "! docker compose logs --tail 50 step-ui 2>&1 | grep -iE 'panic|fatal|err.*template|cannot'"

# ── 5. summary ────────────────────────────────────────────────────────────────
echo
echo "${B}▸ [5/5] Summary${X}"
TOTAL=$((PASS + FAIL))
echo "  Passed: ${G}$PASS${X} / $TOTAL"
echo "  Failed: ${R}$FAIL${X} / $TOTAL"
echo

if (( FAIL == 0 )); then
  cat <<EOF
${G}╔══════════════════════════════════════════════════════════════╗
║                                                              ║
║                ✓ All checks passed                           ║
║                                                              ║
║      Deployment is healthy and ready for users.              ║
║                                                              ║
╚══════════════════════════════════════════════════════════════╝${X}

  Visit:      https://${HOST_IP}
  Credentials: cat $TARGET_DIR/credentials.txt

  ${D}To clean up the test deployment:${X}
    cd $TARGET_DIR && docker compose down -v && rm -rf $TARGET_DIR

EOF
  exit 0
else
  cat <<EOF
${R}╔══════════════════════════════════════════════════════════════╗
║          ✗  Some checks failed                               ║
║          See output above and inspect logs:                  ║
║                                                              ║
║          cd $TARGET_DIR
║          docker compose logs --tail 100                      ║
╚══════════════════════════════════════════════════════════════╝${X}

EOF
  exit 1
fi
