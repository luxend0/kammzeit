package main

import (
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Englische Branchenseiten, About und Contact gibt es nur auf der englischen
// Hauptdomain (404 auf der deutschen und auf Salon-Subdomains), mit Canonical,
// hreflang-Partner, Sitemap-Eintrag, ohne Em-Dash und ohne deutsche Reste im Inhalt.
func TestSiteENBranchen(t *testing.T) {
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

	_, sitemapEN := get("kammzeit-en.test", "/sitemap.xml")
	_, sitemapDE := get("kammzeit.test", "/sitemap.xml")
	pages := []struct{ path, de, want string }{ // englischer Pfad, deutsche Entsprechung, Pflichttext
		{"/for/hairdressers", "/fuer/friseure", "colour"},
		{"/for/barbershops", "/fuer/barbershops", "walk-in"},
		{"/for/beauty-salons", "/fuer/kosmetik", "health data"},
		{"/for/nail-salons", "/fuer/nagelstudios", "infill"},
		{"/about", "/ueber", "Ben Oliver Mallow"},
		{"/contact", "/kontakt", `href="mailto:kontakt@kammzeit.de"`},
	}
	for _, p := range pages {
		code, body := get("kammzeit-en.test", p.path)
		if code != http.StatusOK {
			t.Errorf("%s: Status %d", p.path, code)
			continue
		}
		for _, w := range []string{p.want, `<link rel="canonical" href="https://kammzeit-en.test` + p.path + `">`, " - Kammzeit</title>", "/static/site-branchen.css"} {
			if !strings.Contains(body, w) {
				t.Errorf("%s enthält nicht %q", p.path, w)
			}
		}
		for _, bad := range []string{"\u2014", "<script", "{{", "Hunsrück"} {
			if strings.Contains(body, bad) {
				t.Errorf("%s enthält %q", p.path, bad)
			}
		}
		// Seiteninhalt (ohne das gemeinsame Layout) muss englisch sein.
		_, main, _ := strings.Cut(body, `<main id="inhalt">`)
		main, _, _ = strings.Cut(main, "</main>")
		if !strings.Contains(main, p.want) {
			t.Errorf("%s: Inhalt zwischen <main> und </main> nicht gefunden", p.path)
		}
		for _, de := range []string{"ä", "ö", "ü", " und ", " nicht ", "Termin", "/fuer/", "/preise", "/avv", "/impressum"} {
			if strings.Contains(main, de) {
				t.Errorf("%s: deutscher Rest %q im Inhalt", p.path, de)
			}
		}
		if !strings.Contains(sitemapEN, "<loc>https://kammzeit-en.test"+p.path+"</loc>") {
			t.Errorf("%s fehlt in der englischen sitemap.xml", p.path)
		}
		if strings.Contains(sitemapDE, p.path+"</loc>") {
			t.Errorf("%s steht in der deutschen sitemap.xml", p.path)
		}
		alt := a.alternates("en", p.path)
		if len(alt) != 2 || alt[1].Lang != "de" || alt[1].URL != "https://kammzeit.test"+p.de {
			t.Errorf("%s: hreflang = %v, erwartet de %s", p.path, alt, p.de)
		}
		for _, host := range []string{"kammzeit.test", "mueller.kammzeit-en.test", "mueller.kammzeit.test"} {
			if code, _ := get(host, p.path); code != http.StatusNotFound {
				t.Errorf("%s auf %s: Status %d, erwartet 404", p.path, host, code)
			}
		}
	}
}
