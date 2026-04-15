#!/bin/bash
SERVER="root@vpn.white.akte.de"
REMOTE_PATH="/opt/vehicle-api"
PORT=8081
DIR="$(cd "$(dirname "$0")" && pwd)"

set -e

echo "Building..."
cd "$DIR/.."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o vehicle-api

echo "Stopping..."
ssh "$SERVER" "pkill vehicle-api 2>/dev/null; sleep 1"

echo "Uploading..."
scp vehicle-api "$SERVER:$REMOTE_PATH"
scp "$DIR/2.vehicle.akte.de.conf" "$SERVER:/etc/apache2/sites-available/"

echo "Starting..."
ssh "$SERVER" "nohup $REMOTE_PATH serve -port $PORT > /var/log/vehicle-api.log 2>&1 &"

echo "Smoke test..."
sleep 2
BASE="https://vehicle.akte.de"
FAIL=0

# Erfolgsfall: bekanntes Fahrzeug
STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/vehicle/47085?email=user@gmail.com")
if [ "$STATUS" = "200" ]; then echo "  OK  200 /vehicle/47085"; else echo "  FAIL expected 200, got $STATUS"; FAIL=1; fi

# Fehlende Email → 401
STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/vehicle/47085")
if [ "$STATUS" = "401" ]; then echo "  OK  401 missing email"; else echo "  FAIL expected 401, got $STATUS"; FAIL=1; fi

# Wegwerf-Email → 403
STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/vehicle/47085?email=user@mailinator.com")
if [ "$STATUS" = "403" ]; then echo "  OK  403 disposable email"; else echo "  FAIL expected 403, got $STATUS"; FAIL=1; fi

# Ungültige ID → 400
STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/vehicle/abc?email=user@gmail.com")
if [ "$STATUS" = "400" ]; then echo "  OK  400 invalid ID"; else echo "  FAIL expected 400, got $STATUS"; FAIL=1; fi

# Root → 404
STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/")
if [ "$STATUS" = "404" ]; then echo "  OK  404 root"; else echo "  FAIL expected 404, got $STATUS"; FAIL=1; fi

# Rate Limit → 429 (61 Requests mit eigener Email)
EMAIL="smoke-ratelimit@gmail.com"
GOT429=0
for i in $(seq 1 61); do
    STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/vehicle/47085?email=$EMAIL")
    if [ "$STATUS" = "429" ]; then GOT429=1; break; fi
done
if [ "$GOT429" = "1" ]; then echo "  OK  429 rate limit"; else echo "  FAIL expected 429 after 61 requests"; FAIL=1; fi

if [ "$FAIL" = "1" ]; then echo "SMOKE TEST FAILED"; exit 1; fi
echo "Done. All smoke tests passed."
