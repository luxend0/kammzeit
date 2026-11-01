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

// Hilfe-Center, Open Source, Sicherheit und Fragen: jede Seite gibt es nur auf
// der Hauptdomain, mit eigenem Titel, ohne Em-Dash und Skripte, und jeder Link
// auf eine Seite dieses Bereichs führt zu einer registrierten Seite.
func TestSiteHilfe(t *testing.T) {
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
	srv := httptest.NewServer((&app{cfg: config{BaseDomain: "kammzeit.test", SiteURL: "https://kammzeit.de"}, db: db, tpl: tpl}).routes())
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

	own := func(path string) bool {
		return path == "/hilfe" || strings.HasPrefix(path, "/hilfe/") || path == "/open-source" || path == "/sicherheit" || path == "/fragen"
	}
	href := regexp.MustCompile(`href="(/[^"#?]*)`)
	var pages int
	links := map[string]string{} // Ziel -> Seite, auf der der Link steht
	for _, p := range sitePages {
		if !strings.HasPrefix(p.Template, "hilfe-") {
			continue
		}
		pages++
		code, body := get("kammzeit.test", p.Path)
		if code != http.StatusOK {
			t.Errorf("%s: Status %d", p.Path, code)
			continue
		}
		for _, want := range []string{
			"<title>" + template.HTMLEscapeString(p.Title) + "</title>",
			`<link rel="canonical" href="https://kammzeit.de` + p.Path + `">`,
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
		for _, m := range href.FindAllStringSubmatch(body, -1) {
			if own(m[1]) {
				links[m[1]] = p.Path
			}
		}
		if code, _ := get("mueller.kammzeit.test", p.Path); code != http.StatusNotFound {
			t.Errorf("%s auf der Salon-Subdomain: Status %d, erwartet 404", p.Path, code)
		}
	}
	if pages != 14 {
		t.Errorf("%d Seiten registriert, erwartet 14", pages)
	}
	for target, from := range links {
		if code, _ := get("kammzeit.test", target); code != http.StatusOK {
			t.Errorf("Link auf %s (von %s): Status %d", target, from, code)
		}
	}
	if code, _ := get("kammzeit.test", "/static/site-hilfe.css"); code != http.StatusOK {
		t.Errorf("site-hilfe.css: Status %d", code)
	}
}
