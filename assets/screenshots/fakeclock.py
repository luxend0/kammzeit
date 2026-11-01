#!/usr/bin/env python3
"""Baut ein Go-Overlay (go build -overlay), das time.Now() um einen festen Betrag
verschiebt: KZ_FAKE_START=<Unix-Sekunden> = "jetzt" beim Prozessstart.
Nur für die Screenshot-Binary, am Repo-Code ändert sich nichts.

Aufruf: fakeclock.py <GOROOT> <ausgabe-ordner>  -> <ausgabe-ordner>/overlay.json
"""
import json
import sys
from pathlib import Path

goroot, out = Path(sys.argv[1]), Path(sys.argv[2])
src = goroot / "src/time/time.go"
code = src.read_text()
anchor = "func Now() Time {\n\tsec, nsec, mono := runtimeNow()\n"
if anchor not in code:
    sys.exit(f"fakeclock: time.Now() in {src} sieht anders aus als erwartet, Overlay anpassen")
code = code.replace(anchor, anchor + "\tsec += fakeOffset(sec)\n", 1)

helper = """package time

import (
	"sync"
	"syscall"
)

var (
	fakeOnce sync.Once
	fakeOff  int64
)

func fakeOffset(sec int64) int64 {
	fakeOnce.Do(func() {
		v, _ := syscall.Getenv("KZ_FAKE_START")
		var n int64
		for _, c := range v {
			if c < '0' || c > '9' {
				return
			}
			n = n*10 + int64(c-'0')
		}
		if n > 0 {
			fakeOff = n - sec
		}
	})
	return fakeOff
}
"""

out.mkdir(parents=True, exist_ok=True)
(out / "time.go").write_text(code)
(out / "fakeclock.go").write_text(helper)
(out / "overlay.json").write_text(json.dumps({"Replace": {
    str(src): str(out / "time.go"),
    str(goroot / "src/time/zz_fakeclock.go"): str(out / "fakeclock.go"),
}}))
