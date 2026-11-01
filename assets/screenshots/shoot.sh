#!/usr/bin/env bash
# Erzeugt die Screenshots für die Website neu: web/static/img/shots/*.webp (deutsch,
# kammzeit.de) und web/static/img/shots/en/*.webp (englisch, kammzeit.com).
# Braucht: go, python3, node >= 22, /usr/bin/brave, magick (ImageMagick 7).
#
# Ablauf: eigene Binary mit verstellter Uhr (fakeclock.py, nur hier), frische
# Demo-DB mit einem deutschen und einem englischen Salon (seed.py), ein Server mit
# beiden Hauptdomains (timeout), Brave headless per DevTools (shoot.mjs), je Sprache
# ein eigener Brave mit passender Browsersprache (Datumsfelder), PNG -> WebP.
# "Heute" ist immer der kommende Dienstag um 11:20 Uhr.
#
# Aufruf aus dem Repo: assets/screenshots/shoot.sh [de] [en]   (ohne Angabe beide;
# PNG_ONLY=1: nur PNGs, Ordner wird ausgegeben)
set -euo pipefail
cd "$(dirname "$0")/../.."
here=assets/screenshots
out=web/static/img/shots
work=$(mktemp -d)
port=${PORT:-18791}
cdp=${CDP_PORT:-19231}
langs=${*:-de en}
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

# Englisch läuft über die echte .com-Domain (im Browser auf den Testserver umgeleitet),
# damit Adressen wie lindner-hair.kammzeit.com im Bild stehen.
export KZ_FAKE_START=$now DB_PATH=$work/shots.db BASE_DOMAIN=kammzeit.test PORT=$port \
	BASE_DOMAINS=kammzeit.test=de,kammzeit.com=en PUBLIC_URL='https://{slug}.kammzeit.de' SITE_URL=https://kammzeit.de
unset SMTP_HOST LISTEN KAMMZEIT_SINGLE_TENANT
# Leere DB mit Schema und allen Migrationen: "session" öffnet die DB (legt sie an,
# migriert) und bricht dann ab, weil es das Konto noch nicht gibt.
"$work/kz-shots" session keiner@example.com >/dev/null 2>&1 || true
python3 "$here/seed.py" "$work/shots.db" "$day"
cookie_de=$("$work/kz-shots" session lindner@example.com)
cookie_en=$("$work/kz-shots" session lindner.hair@example.com)
timeout 900 nice -n 19 "$work/kz-shots" >"$work/server.log" 2>&1 &
server_pid=$!

# shoot <sprache> <hauptdomain> <salon-slug> <cookie> <browsersprache> <png-ordner>
shoot() {
	local lang=$1 site=$2 salon=$3.$2 cookie=$4 locale=$5 dir=$6
	LANGUAGE=${locale/-/_} LANG=${locale/-/_}.UTF-8 timeout 600 nice -n 19 /usr/bin/brave --headless=new --disable-gpu \
		--hide-scrollbars --no-first-run --no-default-browser-check --disable-extensions --lang="$locale" \
		--user-data-dir="$work/profile-$lang" --remote-debugging-port="$cdp" \
		--host-resolver-rules="MAP $site 127.0.0.1:$port, MAP $salon 127.0.0.1:$port" \
		about:blank >"$work/brave-$lang.log" 2>&1 &
	brave_pid=$!
	for _ in $(seq 50); do
		curl -sf "http://127.0.0.1:$cdp/json/version" >/dev/null && curl -s -o /dev/null "http://127.0.0.1:$port/" && break
		sleep 0.2
	done
	mkdir -p "$dir"
	CDP_PORT=$cdp PORT=$port COOKIE=$cookie OUT=$dir DAY=$day SHOT_LANG=$lang SITE=$site SALON=$salon \
		node "$here/shoot.mjs"
	kill "$brave_pid" 2>/dev/null || true
	wait "$brave_pid" 2>/dev/null || true
	brave_pid=
}

for lang in $langs; do
	case $lang in
	de) shoot de kammzeit.test lindner "$cookie_de" de-DE "$work/png" ;;
	en) shoot en kammzeit.com lindner-hair "$cookie_en" en-GB "$work/png/en" ;;
	*) echo "unbekannte Sprache $lang" >&2 && exit 2 ;;
	esac
done

if grep -q 'Template' "$work/server.log"; then
	cat "$work/server.log" >&2
	exit 1
fi
if [ -n "${PNG_ONLY:-}" ]; then
	echo "PNGs in $work/png"
	exit 0
fi
for lang in $langs; do
	src=$work/png dst=$out
	[ "$lang" = de ] || src=$work/png/$lang dst=$out/$lang
	mkdir -p "$dst"
	for f in "$src"/*.png; do
		nice -n 19 magick -limit thread 2 "$f" -strip -quality 85 -define webp:method=6 "$dst/$(basename "${f%.png}").webp"
	done
	ls -l "$dst"
done
