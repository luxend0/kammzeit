#!/bin/sh
# Rendert assets/blender/ill_<name>.py sichtbar in Blender und wandelt nach WebP.
# Nur ein Blender gleichzeitig (flock), Monitor muss an sein, PC-Schutz: -t 6, nice, ionice.
# KZ_TEST=1: halbe Aufloesung, Ergebnis nur in scratchpad/kammzeit-assets/ill-test/.
set -eu
HERE=$(cd "$(dirname "$0")" && pwd)
WT=$(cd "$HERE/../.." && pwd)
ASSETS=/tmp/claude-1000/-home-luxend/392722f8-b013-47c4-8bce-99419030ab5b/scratchpad/kammzeit-assets
NAME=$1
SCRIPT=$(echo "$NAME" | tr - _)
mkdir -p "$ASSETS/ill/png" "$ASSETS/ill/log" "$ASSETS/ill-test" "$WT/web/static/img/ill"
LOG="$ASSETS/ill/log/$NAME.log"
START=$(date +%s)
flock /tmp/kammzeit-blender.lock sh -c '
  i=0
  until hyprctl monitors | grep -q "dpmsStatus: 1"; do
    i=$((i+1)); [ $i -gt 360 ] && echo "Monitor aus, Abbruch" && exit 3
    sleep 10
  done
  exec nice -n 19 ionice -c3 timeout 1800 blender --factory-startup -t 6 --python "$1"
' sh "$HERE/ill_$SCRIPT.py" > "$LOG" 2>&1 || { echo "blender fehlgeschlagen, siehe $LOG"; tail -30 "$LOG"; exit 1; }
echo "Blender-Sitzung $(( $(date +%s) - START )) s"
if [ -n "${KZ_TEST:-}" ]; then
  PNG="$ASSETS/ill-test/$NAME.png"
else
  PNG="$ASSETS/ill/png/$NAME.png"
fi
[ -f "$PNG" ] || { echo "kein PNG"; tail -30 "$LOG"; exit 1; }
SZ=$(magick identify -format %wx%h "$PNG")
nice -n 19 magick -limit thread 2 -size "$SZ" xc:'#E3F3FF' "$PNG" -composite -alpha off "${PNG%.png}-auf-himmel.png"
if [ -z "${KZ_TEST:-}" ]; then
  OUT="$WT/web/static/img/ill/$NAME.webp"
  nice -n 19 magick -limit thread 2 "$PNG" -quality 88 -define webp:alpha-quality=100 -define webp:method=6 "$OUT"
  cp "$OUT" "$ASSETS/ill/$NAME.webp"
  ls -la "$OUT"
fi
grep "\[kz\]" "$LOG" | grep -E "Render|Beende|Kamera-Abstand" | tail -4
grep -E "Traceback|Error" "$LOG" | head -5 || true
