package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"image/png"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func pwaTestApp(t *testing.T) (*app, http.Handler) {
	t.Helper()
	tpl, err := template.New("").Funcs(templateFuncs).ParseFS(webFS, "web/*.html")
	if err != nil {
		t.Fatal(err)
	}
	a := &app{tpl: tpl, cfg: config{BaseDomain: "kammzeit.test", SiteURL: "https://kammzeit.test",
		Domains: parseDomains("kammzeit.test=de,kammzeit-en.test=en", "")}}
	mux := http.NewServeMux()
	a.routesPWA(mux)
	return a, mux
}

func pwaGet(h http.Handler, host, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", path, nil)
	req.Host = host
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// staticFile liest eine /static/...-Adresse aus dem eingebetteten Dateisystem.
func staticFile(t *testing.T, url string) []byte {
	t.Helper()
	rest, ok := strings.CutPrefix(url, "/static/")
	if !ok {
		t.Fatalf("%s liegt nicht unter /static/", url)
	}
	b, err := webFS.ReadFile("web/static/" + rest)
	if err != nil {
		t.Fatalf("%s fehlt: %v", url, err)
	}
	return b
}

// checkPNG prüft, dass die Datei ein PNG in der angegebenen Größe ("192x192") ist.
func checkPNG(t *testing.T, url, sizes string) {
	t.Helper()
	cfg, err := png.DecodeConfig(bytes.NewReader(staticFile(t, url)))
	if err != nil {
		t.Fatalf("%s: %v", url, err)
	}
	if got := fmt.Sprintf("%dx%d", cfg.Width, cfg.Height); got != sizes {
		t.Errorf("%s ist %s, Manifest sagt %s", url, got, sizes)
	}
}

func TestPWAManifest(t *testing.T) {
	_, h := pwaTestApp(t)
	for host, want := range map[string][2]string{
		"kammzeit.test":    {"de", "Neuer Termin"},
		"kammzeit-en.test": {"en", "New appointment"},
	} {
		rec := pwaGet(h, host, "/manifest.webmanifest")
		if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/manifest+json" {
			t.Fatalf("%s: %d %q", host, rec.Code, rec.Header().Get("Content-Type"))
		}
		var m struct {
			Name, Scope, Display, Lang, ID string
			ShortName                      string `json:"short_name"`
			StartURL                       string `json:"start_url"`
			ThemeColor                     string `json:"theme_color"`
			BackgroundColor                string `json:"background_color"`
			Icons                          []struct{ Src, Sizes, Type, Purpose string }
			Shortcuts                      []struct {
				Name, URL string
				Icons     []struct{ Src, Sizes string }
			}
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
			t.Fatalf("%s: kein gültiges JSON: %v", host, err)
		}
		if m.Name != "Kammzeit" || m.ShortName != "Kammzeit" || m.StartURL != "/dashboard" || m.Scope != "/" ||
			m.Display != "standalone" || m.ThemeColor != "#1E6BFF" || m.BackgroundColor != "#E3F3FF" || m.ID == "" {
			t.Errorf("%s: Pflichtfelder falsch: %+v", host, m)
		}
		if m.Lang != want[0] {
			t.Errorf("%s: lang = %q, want %q", host, m.Lang, want[0])
		}
		have := map[string]bool{}
		for _, ic := range m.Icons {
			checkPNG(t, ic.Src, ic.Sizes)
			have[ic.Sizes+" "+ic.Purpose] = ic.Type == "image/png"
		}
		for _, k := range []string{"192x192 any", "512x512 any", "512x512 maskable"} {
			if !have[k] {
				t.Errorf("%s: Icon %s fehlt", host, k)
			}
		}
		urls := []string{}
		for _, s := range m.Shortcuts {
			urls = append(urls, s.URL)
			for _, ic := range s.Icons {
				checkPNG(t, ic.Src, ic.Sizes)
			}
		}
		if strings.Join(urls, " ") != "/dashboard/termine/neu /dashboard/kunden /dashboard/warteschlange" {
			t.Errorf("%s: Shortcuts %v", host, urls)
		}
		if len(m.Shortcuts) == 0 || m.Shortcuts[0].Name != want[1] {
			t.Errorf("%s: erster Shortcut nicht übersetzt: %+v", host, m.Shortcuts)
		}
	}
	// Nur Hauptdomains: Buchungsseiten der Salons sind keine App.
	for _, host := range []string{"demo.kammzeit.test", "demo.kammzeit-en.test", "evil.example"} {
		for _, p := range []string{"/manifest.webmanifest", "/sw.js", "/offline"} {
			if rec := pwaGet(h, host, p); rec.Code != 404 {
				t.Errorf("%s%s = %d, want 404", host, p, rec.Code)
			}
		}
	}
}

func TestPWAWorker(t *testing.T) {
	_, h := pwaTestApp(t)
	rec := pwaGet(h, "kammzeit-en.test", "/sw.js")
	hd := rec.Header()
	if rec.Code != 200 || hd.Get("Service-Worker-Allowed") != "/" || hd.Get("Cache-Control") != "no-cache" ||
		!strings.HasPrefix(hd.Get("Content-Type"), "text/javascript") {
		t.Fatalf("sw.js: %d %v", rec.Code, hd)
	}
	js := rec.Body.String()
	if strings.Contains(js, "__VERSION__") || !strings.Contains(js, "kammzeit-"+pwaVersion()) || len(pwaVersion()) != 12 {
		t.Errorf("Version nicht eingesetzt (%q)", pwaVersion())
	}
	// Alles, was vorab in den Cache soll, muss es geben (sonst schlägt die Installation fehl).
	pre := regexp.MustCompile(`(?s)const PRECACHE = \[(.*?)\];`).FindStringSubmatch(js)
	if pre == nil {
		t.Fatal("PRECACHE nicht gefunden")
	}
	for _, m := range regexp.MustCompile(`'([^']+)'`).FindAllStringSubmatch(pre[1], -1) {
		if !strings.HasPrefix(m[1], "/static/") {
			continue // OFFLINE steht als Konstante drin, unten geprüft
		}
		staticFile(t, m[1])
	}
	for _, must := range []string{"req.method !== 'GET'", "HX-Request", "skipWaiting", "caches.delete", "const OFFLINE = '/offline'"} {
		if !strings.Contains(js, must) {
			t.Errorf("sw.js ohne %q", must)
		}
	}
}

func TestPWAOfflineAndPartials(t *testing.T) {
	a, h := pwaTestApp(t)
	for host, want := range map[string]string{
		"kammzeit.test":    `<html lang="de">`,
		"kammzeit-en.test": `<html lang="en">`,
	} {
		rec := pwaGet(h, host, "/offline")
		body := rec.Body.String()
		if rec.Code != 200 || !strings.Contains(body, want) {
			t.Fatalf("%s/offline: %d", host, rec.Code)
		}
		title := map[string]string{"kammzeit.test": "Du bist offline", "kammzeit-en.test": "You are offline"}[host]
		if !strings.Contains(body, "<h1>"+title+"</h1>") {
			t.Errorf("%s/offline ohne %q", host, title)
		}
	}
	// Partials fürs Dashboard: alle verlinkten Dateien existieren, Hinweis ist übersetzt.
	var b strings.Builder
	for _, name := range []string{"pwa-head", "pwa-install-hint"} {
		if err := a.tpl.ExecuteTemplate(&b, name, map[string]any{"Lang": "en"}); err != nil {
			t.Fatal(err)
		}
	}
	out := b.String()
	for _, m := range regexp.MustCompile(`(?:href|src)="(/static/[^"]+)"`).FindAllStringSubmatch(out, -1) {
		staticFile(t, m[1])
	}
	checkPNG(t, "/static/app/apple-touch-icon.png", "180x180")
	for _, must := range []string{`rel="manifest"`, `name="theme-color"`, `rel="apple-touch-icon"`, "apple-mobile-web-app-capable",
		"apple-mobile-web-app-title", "/static/app/register.js", `id="pwa-hint"`, "Install app", "Add to Home Screen"} {
		if !strings.Contains(out, must) {
			t.Errorf("Partials ohne %q", must)
		}
	}
}

// Jeder Text in web/pwa.html braucht eine englische Fassung.
func TestPWATranslations(t *testing.T) {
	src, err := webFS.ReadFile("web/pwa.html")
	if err != nil {
		t.Fatal(err)
	}
	keys := regexp.MustCompile(`\{\{t \.Lang "([^"]+)"\}\}`).FindAllStringSubmatch(string(src), -1)
	if len(keys) < 5 {
		t.Fatalf("nur %d Texte gefunden", len(keys))
	}
	for _, k := range append(keys, []string{"", "Termine, Kunden und Team deines Salons"}, []string{"", "Warteschlange"}) {
		if tr("en", k[1]) == k[1] {
			t.Errorf("keine englische Fassung für %q", k[1])
		}
	}
}
