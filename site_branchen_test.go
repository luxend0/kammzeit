package main

import (
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Branchenseiten, Über und Kontakt gibt es nur auf der Hauptdomain, mit Titel,
// Canonical und Sitemap-Eintrag, ohne Em-Dash und ohne Skripte (CSP).
func TestSiteBranchen(t *testing.T) {
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
	srv := httptest.NewServer((&app{cfg: config{BaseDomain: "kammzeit.test", SiteURL: "https://kammzeit.test"}, db: db, tpl: tpl}).routes())
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

	_, sitemap := get("kammzeit.test", "/sitemap.xml")
	pages := map[string]string{ // Pfad -> Text, der auf der Seite stehen muss
		"/fuer/friseure":     "Farbrezeptur",
		"/fuer/barbershops":  "Laufkunde",
		"/fuer/kosmetik":     "Gesundheitsdaten",
		"/fuer/nagelstudios": "Auffüllen",
		"/ueber":             "Ben Oliver Mallow",
		"/kontakt":           `href="mailto:kontakt@kammzeit.de"`,
	}
	for path, want := range pages {
		code, body := get("kammzeit.test", path)
		if code != http.StatusOK {
			t.Errorf("%s: Status %d", path, code)
			continue
		}
		for _, w := range []string{want, `<link rel="canonical" href="https://kammzeit.test` + path + `">`, " - Kammzeit</title>", "/static/site-branchen.css"} {
			if !strings.Contains(body, w) {
				t.Errorf("%s enthält nicht %q", path, w)
			}
		}
		for _, bad := range []string{"\u2014", "<script", "{{", "Hunsrück"} {
			if strings.Contains(body, bad) {
				t.Errorf("%s enthält %q", path, bad)
			}
		}
		if !strings.Contains(sitemap, "<loc>https://kammzeit.test"+path+"</loc>") {
			t.Errorf("%s fehlt in sitemap.xml", path)
		}
		if code, _ := get("mueller.kammzeit.test", path); code != http.StatusNotFound {
			t.Errorf("%s auf Salon-Subdomain: Status %d, erwartet 404", path, code)
		}
	}
	if code, _ := get("kammzeit.test", "/static/site-branchen.css"); code != http.StatusOK {
		t.Errorf("site-branchen.css: Status %d", code)
	}
}
