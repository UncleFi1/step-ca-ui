#!/bin/sh
set -e

echo "======================================="
echo "  Step-CA UI (Go) — starting up"
echo "======================================="

# Ждём PostgreSQL
echo "[*] Waiting for PostgreSQL..."
until nc -z postgres 5432 2>/dev/null; do
  sleep 1
done
echo "[*] PostgreSQL is ready!"

# Ждём Step-CA
echo "[*] Waiting for Step-CA at ${CA_URL}..."
until curl -sk "${CA_URL}/health" >/dev/null 2>&1; do
  sleep 2
done
echo "[*] Step-CA is ready!"

# SSL сертификат для UI
if [ ! -f /opt/step-ui/ssl/server.crt ]; then
  echo "[*] Generating self-signed SSL certificate..."
  openssl req -x509 -nodes -days 3650 -newkey rsa:2048     -keyout /opt/step-ui/ssl/server.key     -out /opt/step-ui/ssl/server.crt     -subj "/CN=${HOST_IP:-localhost}"     -addext "subjectAltName=IP:${HOST_IP:-127.0.0.1},DNS:localhost" 2>/dev/null
fi

echo "[*] Starting Step-CA UI on port ${PORT:-8443}"
exec /opt/step-ui/step-ui
