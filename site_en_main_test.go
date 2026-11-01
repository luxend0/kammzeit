package main

import (
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

// Englische Startseite, Features und Pricing nur auf der englischen Hauptdomain,
// mit hreflang und Sprachumschalter in beide Richtungen; ohne Em-Dash, Skripte
// und deutsche Reste. Ohne zweite Domain kein Umschalter und kein hreflang.
func TestSiteENMain(t *testing.T) {
	db, err := openDB(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tpl, err := template.New("").Funcs(templateFuncs).ParseFS(webFS, "web/*.html")
	if err != nil {
		t.Fatal(err)
	}
	a := &app{cfg: config{BaseDomain: "kammzeit.test", SiteURL: "https://kammzeit.test",
		Domains: parseDomains("kammzeit.test=de,kammzeit-en.test=en", "")}, db: db, tpl: tpl}
	srv := httptest.NewServer(a.routes())
	defer srv.Close()
	get := func(host, path string) (int, string) {
		t.Helper()
		req, _ := http.NewRequest("GET", srv.URL+path, nil)
		req.Host = host
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(b)
	}

	const de, en = "https://kammzeit.test", "https://kammzeit-en.test"
	alt := func(lang, url string) string {
		return `<link rel="alternate" hreflang="` + lang + `" href="` + url + `">`
	}
	sw := func(url string) string { return `<a class="site-lang" href="` + url + `" hreflang="` }
	cases := []struct {
		host, path string
		want       []string
	}{
		{"kammzeit-en.test", "/", []string{`<html lang="en">`, alt("en", en+"/"), alt("de", de+"/"), sw(de + "/"), ">Deutsch</a>",
			`content="en_GB"`, `href="/features"`, `href="/pricing"`, `href="/for/hairdressers"`, `href="/privacy"`, "€14.90", "€149", "reverse charge",
			`href="/signup"`, `href="https://demo-en.kammzeit.com"`, "A queue for walk-ins", "Logins for your team", "Is there a Kammzeit app?"}},
		{"kammzeit-en.test", "/features", []string{`<html lang="en">`, "<title>Features - Kammzeit</title>", `<link rel="canonical" href="` + en + `/features">`,
			alt("en", en+"/features"), alt("de", de+"/funktionen"), sw(de + "/funktionen"), `aria-current="page">Features`, "shown in German",
			`href="/signup"`, `href="https://demo-en.kammzeit.com"`, "A queue for walk-ins", "Logins for your team", "Install it as an app", "A tour on your first login", "Kammzeit news"}},
		{"kammzeit-en.test", "/pricing", []string{"<title>Pricing - Kammzeit</title>", alt("de", de+"/preise"), sw(de + "/preise"),
			`aria-current="page">Pricing`, "€14.90", "€149", "§&nbsp;19&nbsp;UStG", "reverse charge procedure may apply", `<table class="compare">`,
			`href="/signup"`, `href="https://demo-en.kammzeit.com"`, "Staff logins (owner and staff member roles)</th><td>No</td>", "staff logins are paused, the queue is off", "Nothing is deleted"}},
		{"kammzeit-en.test", "/roadmap", []string{"<title>Roadmap: what&#39;s coming next - Kammzeit</title>", alt("de", de+"/geplant"), sw(de + "/geplant"),
			`aria-current="page">Roadmap`, "Custom domain", "Google Calendar", "Holidays and time off", "More languages", "Import customers from CSV", "Today:",
			`href="/signup"`, `href="https://demo-en.kammzeit.com"`, `href="/roadmap">Roadmap</a>`}},
		{"kammzeit.test", "/", []string{`<html lang="de">`, alt("de", de+"/"), alt("en", en+"/"), sw(en + "/?lang=en"), ">English</a>", `content="de_DE"`}},
		{"kammzeit.test", "/funktionen", []string{`<html lang="de">`, alt("en", en+"/features"), sw(en + "/features"), `aria-current="page">Funktionen`, `href="/geplant">Geplant</a>`}},
		{"kammzeit.test", "/preise", []string{alt("en", en+"/pricing"), sw(en + "/pricing"), `href="/datenschutz"`}},
	}
	descRe := regexp.MustCompile(`<meta name="description" content="([^"]*)">`)
	linkRe := regexp.MustCompile(`(?:href|src)="(/[^"#?]*)`)
	// ponytail: die englischen Hilfeseiten und den Alias /signup (Docs-Agent) gibt es erst nach dem Zusammenführen.
	elsewhere := regexp.MustCompile(`^/(help|open-source|security|faq|signup)`)
	// Was es inzwischen gibt, darf nicht mehr als fehlend dastehen.
	gone := []string{"separate staff logins", ", an app.", "only you can see", `href="/registrieren"`}
	german := regexp.MustCompile(`\b(und|der|die|das|für|mit|Kostenlos|Anmelden|Funktionen|Preise|Mitarbeiter|Kunden)\b`)
	tags := regexp.MustCompile(`(?s)<[^>]*>`)
	checked := map[string]bool{}
	for _, c := range cases {
		code, body := get(c.host, c.path)
		if code != http.StatusOK {
			t.Fatalf("%s%s: Status %d", c.host, c.path, code)
		}
		for _, n := range c.want {
			if !strings.Contains(body, n) {
				t.Errorf("%s%s enthält nicht %q", c.host, c.path, n)
			}
		}
		for _, bad := range []string{"\u2014", "<script", "{{", "demo.kammzeit"} {
			if strings.Contains(body, bad) && (c.host == "kammzeit-en.test" || bad != "demo.kammzeit") {
				t.Errorf("%s%s enthält %q", c.host, c.path, bad)
			}
		}
		if c.host != "kammzeit-en.test" {
			continue
		}
		inner := body[strings.Index(body, "<main"):strings.Index(body, "</main>")]
		for _, g := range gone {
			if strings.Contains(inner, g) {
				t.Errorf("%s%s enthält noch %q", c.host, c.path, g)
			}
		}
		text := tags.ReplaceAllString(body[strings.Index(body, "<body"):], " ")
		if m := german.FindAllString(text, 5); m != nil {
			t.Errorf("%s: deutscher Text auf der englischen Seite: %q", c.path, m)
		}
		if c.path != "/" {
			if m := descRe.FindStringSubmatch(body); m == nil {
				t.Errorf("%s: keine Beschreibung", c.path)
			} else if n := utf8.RuneCountInString(m[1]); n < 120 || n > 160 {
				t.Errorf("%s: Beschreibung hat %d Zeichen, erlaubt 120-160", c.path, n)
			}
		}
		for _, m := range linkRe.FindAllStringSubmatch(body, -1) {
			if link := m[1]; !checked[link] && !elsewhere.MatchString(link) {
				checked[link] = true
				if code, _ := get(c.host, link); code == http.StatusNotFound {
					t.Errorf("%s verlinkt %s, das gibt 404", c.path, link)
				}
			}
		}
	}
	for _, p := range [][2]string{{"kammzeit.test", "/features"}, {"kammzeit.test", "/pricing"}, {"mueller.kammzeit-en.test", "/features"},
		{"kammzeit-en.test", "/funktionen"}, {"kammzeit-en.test", "/preise"}} {
		if code, _ := get(p[0], p[1]); code != http.StatusNotFound {
			t.Errorf("%s%s: Status %d, erwartet 404", p[0], p[1], code)
		}
	}
	// Login nutzt Kopf und Fuß ohne .Path: muss trotzdem vollständig rendern.
	for _, host := range []string{"kammzeit.test", "kammzeit-en.test"} {
		if _, body := get(host, "/login"); !strings.Contains(body, "</html>") {
			t.Errorf("%s/login bricht ab", host)
		}
	}

	// Jeder Text, den das Layout per t übersetzt, hat einen englischen Eintrag.
	src, _ := webFS.ReadFile("web/site.html")
	for _, m := range regexp.MustCompile(`\{\{t \.Lang "([^"]+)"\}\}`).FindAllStringSubmatch(string(src), -1) {
		if tr("en", m[1]) == m[1] {
			t.Errorf("site.html: keine Übersetzung für %q", m[1])
		}
	}

	// Nur eine Domain (Selbsthosten): kein Umschalter, kein hreflang.
	tpl2, err := template.New("").Funcs(templateFuncs).ParseFS(webFS, "web/*.html")
	if err != nil {
		t.Fatal(err)
	}
	solo := &app{cfg: config{BaseDomain: "kammzeit.test", SiteURL: "https://kammzeit.test"}, db: db, tpl: tpl2}
	srv2 := httptest.NewServer(solo.routes())
	defer srv2.Close()
	req, _ := http.NewRequest("GET", srv2.URL+"/funktionen", nil)
	req.Host = "kammzeit.test"
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || strings.Contains(string(b), "site-lang") || strings.Contains(string(b), "hreflang") {
		t.Errorf("Einzeldomain: Status %d, Umschalter oder hreflang vorhanden", res.StatusCode)
	}
}
