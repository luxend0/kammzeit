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

// Funktionen und Preise: nur auf der Hauptdomain (Salon-Subdomain 404), ohne Em-Dash
// und ohne Skripte, mit passender Beschreibung und den Pflichtangaben zum Preis.
// Interne Links und Dateien der Seiten (inkl. Startseite) müssen existieren; Seiten
// anderer Bereiche und Illustrationen, die noch geliefert werden, sind ausgenommen.
func TestSiteMain(t *testing.T) {
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
	a := &app{cfg: config{BaseDomain: "kammzeit.test", SiteURL: "https://kammzeit.de"}, db: db, tpl: tpl}
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

	want := map[string][]string{
		"/funktionen": {"<title>Funktionen - Kammzeit</title>", "Geplant", "Anzahlung", "Google Kalender", "Eigene Domain", `aria-current="page">Funktionen`},
		"/preise":     {"<title>Preise - Kammzeit</title>", "14,90&nbsp;€", "149&nbsp;€", "§&nbsp;19&nbsp;UStG", "kündbar", `<table class="compare">`},
	}
	descRe := regexp.MustCompile(`<meta name="description" content="([^"]*)">`)
	linkRe := regexp.MustCompile(`(?:href|src)="(/[^"#]*)`)
	// ponytail: Seiten anderer Bereiche existieren erst nach dem Zusammenführen.
	elsewhere := regexp.MustCompile(`^/(fuer/|hilfe|open-source|sicherheit|fragen|kontakt|ueber|ratgeber)|^/static/img/ill/`)

	pages := map[string]string{}
	for path, needles := range want {
		code, body := get("kammzeit.test", path)
		if code != http.StatusOK {
			t.Fatalf("%s: Status %d", path, code)
		}
		pages[path] = body
		for _, n := range needles {
			if !strings.Contains(body, n) {
				t.Errorf("%s enthält nicht %q", path, n)
			}
		}
		m := descRe.FindStringSubmatch(body)
		if m == nil {
			t.Errorf("%s: keine Beschreibung", path)
		} else if n := utf8.RuneCountInString(m[1]); n < 120 || n > 160 {
			t.Errorf("%s: Beschreibung hat %d Zeichen, erlaubt 120-160", path, n)
		}
		if code, _ := get("mueller.kammzeit.test", path); code != http.StatusNotFound {
			t.Errorf("%s auf Salon-Subdomain: Status %d, erwartet 404", path, code)
		}
	}
	_, pages["/"] = get("kammzeit.test", "/")

	checked := map[string]bool{}
	for path, body := range pages {
		for _, bad := range []string{"\u2014", "<script", "{{"} {
			if strings.Contains(body, bad) {
				t.Errorf("%s enthält %q", path, bad)
			}
		}
		for _, m := range linkRe.FindAllStringSubmatch(body, -1) {
			link := m[1]
			if checked[link] || elsewhere.MatchString(link) {
				continue
			}
			checked[link] = true
			if code, _ := get("kammzeit.test", link); code == http.StatusNotFound {
				t.Errorf("%s verlinkt %s, das gibt 404", path, link)
			}
		}
	}
}
