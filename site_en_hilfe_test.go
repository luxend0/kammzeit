package main

import (
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// Englisches Hilfe-Center: jede Seite nur auf kammzeit.com (Testhost
// kammzeit-en.test), mit Titel, Canonical, englischem Inhalt ohne deutsche
// Reste, hreflang-Partner auf der deutschen Seite (und zurück), Links nur auf
// englische Seiten. /open-source liefert je Domain die eigene Sprache.
func TestSiteENHilfe(t *testing.T) {
	db, err := openDB(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO salons (slug, name) VALUES ('mueller', 'Salon Müller')`); err != nil {
		t.Fatal(err)
	}
	tpl, err := template.New("").Funcs(templateFuncs).ParseFS(webFS, "web/*.html")
	if err != nil {
		t.Fatal(err)
	}
	a := &app{cfg: config{BaseDomain: "kammzeit.test", SiteURL: "https://kammzeit.de",
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

	const en = "kammzeit-en.test"
	own := func(p string) bool {
		return p == "/help" || strings.HasPrefix(p, "/help/") || p == "/open-source" || p == "/security" || p == "/faq"
	}
	// Pfade der deutschen Website, auf die eine englische Seite nicht zeigen darf.
	germanPath := regexp.MustCompile(`^/(hilfe|fragen|sicherheit|funktionen|preise|fuer|ueber|kontakt|ratgeber|impressum|datenschutz|agb|avv)(/|$)`)
	germanText := regexp.MustCompile(`[äöüÄÖÜß„]|\b(und|oder|nicht|Kunden?|Termine?|Leistungen?|Mitarbeiter|Einstellungen|Speichern)\b`)
	href := regexp.MustCompile(`href="(/[^"#?]*)`)

	de := map[string]sitePage{}
	for _, p := range sitePages {
		if p.Lang == "de" {
			de[p.Path] = p
		}
	}

	var pages int
	links := map[string]string{} // Ziel -> Seite, auf der der Link steht
	for _, p := range sitePages {
		if !strings.HasPrefix(p.Template, "enhelp-") {
			continue
		}
		pages++
		if p.Lang != "en" {
			t.Errorf("%s: Sprache %q", p.Path, p.Lang)
		}
		// hreflang-Partner: deutsche Hilfe-Seite, die auf diese Seite zurückzeigt
		if d, ok := de[p.Alt]; !ok || !strings.HasPrefix(d.Template, "hilfe-") || d.Alt != p.Path {
			t.Errorf("%s: deutsche Entsprechung %q fehlt oder zeigt nicht zurück (%+v)", p.Path, p.Alt, d)
		}
		code, body := get(en, p.Path)
		if code != http.StatusOK {
			t.Errorf("%s: Status %d", p.Path, code)
			continue
		}
		for _, want := range []string{
			"<title>" + template.HTMLEscapeString(p.Title) + "</title>",
			`<link rel="canonical" href="https://` + en + p.Path + `">`,
			`<link rel="stylesheet" href="/static/site-hilfe.css">`,
			"<h1",
		} {
			if !strings.Contains(body, want) {
				t.Errorf("%s enthält nicht %q", p.Path, want)
			}
		}
		for _, bad := range []string{"\u2014", "<script", "{{", "ZgotmplZ"} {
			if strings.Contains(body, bad) {
				t.Errorf("%s enthält %q", p.Path, bad)
			}
		}
		// Inhalt zwischen <main> und </main>: Kopf und Fuß gehören zum gemeinsamen Layout.
		_, main, _ := strings.Cut(body, "<main")
		main, _, _ = strings.Cut(main, "</main>")
		if m := germanText.FindString(main); m != "" {
			t.Errorf("%s: deutscher Text im Inhalt: %q", p.Path, m)
		}
		for _, m := range href.FindAllStringSubmatch(main, -1) {
			if germanPath.MatchString(m[1]) {
				t.Errorf("%s verlinkt die deutsche Seite %s", p.Path, m[1])
			}
			if own(m[1]) {
				links[m[1]] = p.Path
			}
		}
		if code, _ := get("mueller."+en, p.Path); code != http.StatusNotFound {
			t.Errorf("%s auf der Salon-Subdomain: Status %d, erwartet 404", p.Path, code)
		}
		if p.Path != "/open-source" {
			if code, _ := get("kammzeit.test", p.Path); code != http.StatusNotFound {
				t.Errorf("%s auf der deutschen Domain: Status %d, erwartet 404", p.Path, code)
			}
		}
	}
	if pages != 14 {
		t.Errorf("%d Seiten registriert, erwartet 14", pages)
	}
	for target, from := range links {
		if code, _ := get(en, target); code != http.StatusOK {
			t.Errorf("Link auf %s (von %s): Status %d", target, from, code)
		}
	}

	// Gleicher Pfad, je Domain die eigene Sprache
	if _, body := get(en, "/open-source"); !strings.Contains(body, "The code is open.") {
		t.Error("/open-source auf kammzeit.com ist nicht englisch")
	}
	if _, body := get("kammzeit.test", "/open-source"); !strings.Contains(body, "Der Code ist offen.") {
		t.Error("/open-source auf kammzeit.de ist nicht deutsch")
	}

	// hreflang: englische Anleitung kennt ihre deutsche Fassung
	alt := a.alternates("en", "/help/services")
	if len(alt) != 2 || alt[1] != (alternate{"de", "https://kammzeit.de/hilfe/leistungen"}) {
		t.Errorf("alternates /help/services = %v", alt)
	}

	// Sitemap je Domain
	_, smEN := get(en, "/sitemap.xml")
	_, smDE := get("kammzeit.test", "/sitemap.xml")
	if !strings.Contains(smEN, "https://"+en+"/help/customer-records<") || strings.Contains(smEN, "/hilfe") {
		t.Error("Sitemap kammzeit.com: /help fehlt oder /hilfe enthalten")
	}
	if strings.Contains(smDE, "/help") {
		t.Error("Sitemap kammzeit.de enthält /help")
	}
}
