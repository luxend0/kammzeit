#!/bin/sh
# Baut Kammzeit und spielt es auf den Server (SSH-Alias "kammzeit", überschreibbar per HOST=).
set -eu
HOST=${HOST:-kammzeit}
# Jeder Schritt mit Timeout und ohne Verbindungs-Multiplexing: ein hängendes ssh brach sonst still ab.
SSH="timeout 120 ssh -o ConnectTimeout=10 -o ServerAliveInterval=5 -o ServerAliveCountMax=3 -o ControlMaster=no"
cd "$(dirname "$0")/.."
GOMAXPROCS=6 CGO_ENABLED=0 nice -n 19 go build -trimpath -ldflags "-s -w" -o /tmp/kammzeit-deploy .
# ponytail: per ssh+cat statt scp, scp (SFTP) blieb bei großen Dateien hängen
$SSH "$HOST" 'cat > /usr/local/bin/kammzeit.new && chmod 755 /usr/local/bin/kammzeit.new' < /tmp/kammzeit-deploy
$SSH "$HOST" 'cat > /etc/caddy/Caddyfile' < deploy/Caddyfile
$SSH "$HOST" 'cat > /etc/systemd/system/kammzeit.service' < deploy/kammzeit.service
$SSH "$HOST" 'mv /usr/local/bin/kammzeit.new /usr/local/bin/kammzeit && systemctl daemon-reload && systemctl restart kammzeit && systemctl reload caddy && sleep 1 && systemctl is-active kammzeit caddy'
rm -f /tmp/kammzeit-deploy
