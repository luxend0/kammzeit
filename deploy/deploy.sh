#!/bin/sh
# Baut Kammzeit und spielt es auf den Server (SSH-Alias "kammzeit", überschreibbar per HOST=).
set -eu
HOST=${HOST:-kammzeit}
cd "$(dirname "$0")/.."
GOMAXPROCS=6 CGO_ENABLED=0 nice -n 19 go build -trimpath -ldflags "-s -w" -o /tmp/kammzeit-deploy .
# ponytail: per ssh+cat statt scp, scp (SFTP) blieb bei großen Dateien hängen
ssh "$HOST" 'cat > /usr/local/bin/kammzeit.new && chmod 755 /usr/local/bin/kammzeit.new' < /tmp/kammzeit-deploy
ssh "$HOST" 'cat > /etc/caddy/Caddyfile' < deploy/Caddyfile
ssh "$HOST" 'cat > /etc/systemd/system/kammzeit.service' < deploy/kammzeit.service
ssh "$HOST" 'mv /usr/local/bin/kammzeit.new /usr/local/bin/kammzeit && systemctl daemon-reload && systemctl restart kammzeit && systemctl reload caddy && sleep 1 && systemctl is-active kammzeit caddy'
rm -f /tmp/kammzeit-deploy
