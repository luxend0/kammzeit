package main

// Kammzeit als installierbare Web-App (PWA) für Salons: Manifest, Service Worker
// und Offline-Seite. Nur auf den Hauptdomains (dort läuft das Dashboard), Texte in
// der Sprache der Domain. Einbinden ins Dashboard: Partials "pwa-head" und
// "pwa-install-hint" aus web/pwa.html.
//
// Icons in web/static/app/ (aus img/kammzeit.svg und app/icon-maskable.svg, im Ordner web/static/app):
//   magick -background none -density 1536 ../img/kammzeit.svg -resize 512x512 -depth 8 -strip icon-512.png   (ebenso 192)
//   magick -background none -density 1536 icon-maskable.svg -resize 512x512 -strip icon-maskable-512.png
//   magick -background '#1E6BFF' -density 1536 icon-maskable.svg -resize 180x180 -alpha remove -alpha off -depth 8 -strip apple-touch-icon.png
//   magick -background none -density 384 ../img/icons/kalender.svg -resize 72x72 -gravity center -extent 96x96 -depth 8 -strip shortcut-termin.png
//   (kartei.svg -> shortcut-kunden.png, uhr.svg -> shortcut-warteschlange.png)

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
)

const (
	pwaTheme      = "#1E6BFF" // Blau kräftig
	pwaBackground = "#E3F3FF" // Himmel hell (Startbildschirm beim Öffnen)
)

func (a *app) routesPWA(mux *http.ServeMux) {
	mux.HandleFunc("GET /manifest.webmanifest", a.pwaManifest)
	mux.HandleFunc("GET /sw.js", a.pwaWorker)
	mux.HandleFunc("GET /offline", a.pwaOffline)
}

func (a *app) pwaManifest(w http.ResponseWriter, r *http.Request) {
	if !a.onRootHost(r) {
		http.NotFound(w, r)
		return
	}
	lang := a.langFor(r)
	icon := func(src, sizes, purpose string) map[string]string {
		return map[string]string{"src": "/static/app/" + src, "sizes": sizes, "type": "image/png", "purpose": purpose}
	}
	shortcut := func(name, url, png string) map[string]any {
		return map[string]any{"name": tr(lang, name), "url": url, "icons": []map[string]string{icon(png, "96x96", "any")}}
	}
	m := map[string]any{
		"id":               "/dashboard",
		"name":             "Kammzeit",
		"short_name":       "Kammzeit",
		"description":      tr(lang, "Termine, Kunden und Team deines Salons"),
		"lang":             lang,
		"dir":              "ltr",
		"start_url":        "/dashboard",
		"scope":            "/",
		"display":          "standalone",
		"theme_color":      pwaTheme,
		"background_color": pwaBackground,
		"icons": []map[string]string{
			icon("icon-192.png", "192x192", "any"),
			icon("icon-512.png", "512x512", "any"),
			icon("icon-maskable-512.png", "512x512", "maskable"),
		},
		"shortcuts": []map[string]any{
			shortcut("Neuer Termin", "/dashboard/termine/neu", "shortcut-termin.png"),
			shortcut("Kunden", "/dashboard/kunden", "shortcut-kunden.png"),
			shortcut("Warteschlange", "/dashboard/warteschlange", "shortcut-warteschlange.png"),
		},
	}
	w.Header().Set("Content-Type", "application/manifest+json")
	w.Header().Set("Cache-Control", "no-cache")
	json.NewEncoder(w).Encode(m)
}

// pwaWorker liefert web/static/app/sw.js mit eingesetzter Version im Root-Scope.
func (a *app) pwaWorker(w http.ResponseWriter, r *http.Request) {
	if !a.onRootHost(r) {
		http.NotFound(w, r)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/javascript; charset=utf-8")
	h.Set("Service-Worker-Allowed", "/")
	h.Set("Cache-Control", "no-cache")
	io.WriteString(w, pwaWorkerJS())
}

var pwaWorkerJS = sync.OnceValue(func() string {
	src, err := webFS.ReadFile("web/static/app/sw.js")
	if err != nil {
		panic(err) // eingebettet, fehlt nur bei kaputtem Build
	}
	return strings.ReplaceAll(string(src), "__VERSION__", pwaVersion())
})

// pwaVersion = Hash über alle eingebetteten Web-Dateien (Templates, CSS, Bilder,
// sw.js) plus Git-Stand des Builds, falls bekannt (Texte aus Go-Code). Ändert sich
// etwas, bekommt der Service Worker einen neuen Cache und räumt den alten weg.
var pwaVersion = sync.OnceValue(func() string {
	h := sha256.New()
	fs.WalkDir(webFS, "web", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := webFS.ReadFile(p)
		io.WriteString(h, p)
		h.Write(b)
		return err
	})
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			if s.Key == "vcs.revision" || s.Key == "vcs.modified" {
				io.WriteString(h, s.Value)
			}
		}
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
})

// pwaOffline = Seite, die der Service Worker ohne Netz statt einer Dashboard-Seite zeigt.
func (a *app) pwaOffline(w http.ResponseWriter, r *http.Request) {
	if !a.onRootHost(r) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	a.render(w, http.StatusOK, "pwa-offline", map[string]any{"Lang": a.langFor(r)})
}
