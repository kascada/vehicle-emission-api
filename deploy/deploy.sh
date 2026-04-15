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

echo "Done. Running on port $PORT"
