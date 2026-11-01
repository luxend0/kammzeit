#!/bin/sh
# Baut Kammzeit und spielt es auf den Server (SSH-Alias "kammzeit", überschreibbar per HOST=).
set -eu
HOST=${HOST:-kammzeit}
cd "$(dirname "$0")/.."
GOMAXPROCS=6 CGO_ENABLED=0 nice -n 19 go build -trimpath -ldflags "-s -w" -o /tmp/kammzeit-deploy .
scp -q /tmp/kammzeit-deploy "$HOST":/usr/local/bin/kammzeit.new
scp -q deploy/Caddyfile "$HOST":/etc/caddy/Caddyfile
scp -q deploy/kammzeit.service "$HOST":/etc/systemd/system/kammzeit.service
ssh "$HOST" 'mv /usr/local/bin/kammzeit.new /usr/local/bin/kammzeit && systemctl daemon-reload && systemctl restart kammzeit && systemctl reload caddy && sleep 1 && systemctl is-active kammzeit caddy'
rm -f /tmp/kammzeit-deploy
