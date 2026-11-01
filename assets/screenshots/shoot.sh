#!/usr/bin/env bash
# Erzeugt die Screenshots für die Website neu: web/static/img/shots/*.webp
# Braucht: go, python3, node >= 22, /usr/bin/brave, magick (ImageMagick 7).
#
# Ablauf: eigene Binary mit verstellter Uhr (fakeclock.py, nur hier), frische
# Demo-DB (seed.py), Server mit timeout, Brave headless per DevTools (shoot.mjs),
# PNG -> WebP. "Heute" ist immer der kommende Dienstag um 11:20 Uhr.
#
# Aufruf aus dem Repo: assets/screenshots/shoot.sh   (PNG_ONLY=1: nur PNGs, Ordner wird ausgegeben)
set -euo pipefail
cd "$(dirname "$0")/../.."
here=assets/screenshots
out=web/static/img/shots
work=$(mktemp -d)
port=${PORT:-18791}
cdp=${CDP_PORT:-19231}
server_pid= brave_pid=
cleanup() {
	[ -n "$brave_pid" ] && kill "$brave_pid" 2>/dev/null || true
	[ -n "$server_pid" ] && kill "$server_pid" 2>/dev/null || true
	wait 2>/dev/null || true
	[ -n "${PNG_ONLY:-}" ] || rm -rf "$work"
}
trap cleanup EXIT

day=$(TZ=Europe/Berlin date -d tuesday +%F) # heute, falls Dienstag, sonst der nächste
now=$(TZ=Europe/Berlin date -d "$day 11:20" +%s)
echo "Gezeigter Tag: $day, 11:20 Uhr"

python3 "$here/fakeclock.py" "$(go env GOROOT)" "$work/clock"
GOMAXPROCS=2 nice -n 19 go build -overlay "$work/clock/overlay.json" -o "$work/kz-shots" .
python3 "$here/seed.py" "$work/shots.db" "$day"

export KZ_FAKE_START=$now DB_PATH=$work/shots.db BASE_DOMAIN=kammzeit.test PORT=$port \
	PUBLIC_URL='https://{slug}.kammzeit.de' SITE_URL=https://kammzeit.de
unset SMTP_HOST LISTEN KAMMZEIT_SINGLE_TENANT
cookie=$("$work/kz-shots" session lindner@example.com)
timeout 600 nice -n 19 "$work/kz-shots" >"$work/server.log" 2>&1 &
server_pid=$!

LANGUAGE=de LANG=de_DE.UTF-8 timeout 600 nice -n 19 /usr/bin/brave --headless=new --disable-gpu --hide-scrollbars --no-first-run \
	--no-default-browser-check --disable-extensions --lang=de-DE --user-data-dir="$work/profile" \
	--remote-debugging-port="$cdp" \
	--host-resolver-rules="MAP kammzeit.test 127.0.0.1:$port, MAP lindner.kammzeit.test 127.0.0.1:$port" \
	about:blank >"$work/brave.log" 2>&1 &
brave_pid=$!

for _ in $(seq 50); do
	curl -sf "http://127.0.0.1:$cdp/json/version" >/dev/null && curl -s -o /dev/null "http://127.0.0.1:$port/" && break
	sleep 0.2
done

mkdir -p "$work/png"
CDP_PORT=$cdp PORT=$port COOKIE=$cookie OUT=$work/png DAY=$day node "$here/shoot.mjs"

if grep -q 'Template' "$work/server.log"; then
	cat "$work/server.log" >&2
	exit 1
fi
if [ -n "${PNG_ONLY:-}" ]; then
	echo "PNGs in $work/png"
	exit 0
fi
mkdir -p "$out"
for f in "$work"/png/*.png; do
	magick "$f" -strip -quality 85 -define webp:method=6 "$out/$(basename "${f%.png}").webp"
done
ls -l "$out"
