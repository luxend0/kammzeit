package main

import (
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Die Landingpage gibt es nur auf der Hauptdomain, mit ihren SEO-Angaben,
// ohne Skripte (CSP) und ohne Em-Dash; ihre Dateien müssen ausgeliefert werden.
func TestLanding(t *testing.T) {
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
	srv := httptest.NewServer((&app{cfg: config{BaseDomain: "kammzeit.test"}, db: db, tpl: tpl}).routes())
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

	code, body := get("kammzeit.test", "/")
	if code != http.StatusOK {
		t.Fatalf("Landing: Status %d", code)
	}
	for _, want := range []string{
		`<html lang="de">`,
		`<meta name="description"`,
		`<link rel="canonical" href="https://kammzeit.de/">`,
		`<meta property="og:url" content="https://kammzeit.de/">`,
		`<meta property="og:title" content="Kammzeit`,
		`href="/registrieren"`,
		`href="/login"`,
		`href="https://demo.kammzeit.de"`,
		`14,90&nbsp;€`,
		`149&nbsp;€`,
		`§&nbsp;19&nbsp;UStG`,
		`Warteschlange für Laufkundschaft`,
		`Logins fürs Team`,
		`Gibt es Kammzeit als App?`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Landing enthält nicht %q", want)
		}
	}
	for _, bad := range []string{"<script", "\u2014", "{{"} {
		if strings.Contains(body, bad) {
			t.Errorf("Landing enthält %q", bad)
		}
	}

	if _, body := get("mueller.kammzeit.test", "/"); strings.Contains(body, `rel="canonical"`) || !strings.Contains(body, "Salon Müller") {
		t.Error("Salon-Subdomain zeigt die Landingpage statt der Buchungsseite")
	}

	for _, path := range []string{"/static/site.css", "/static/img/kammzeit.svg"} {
		if code, _ := get("kammzeit.test", path); code != http.StatusOK {
			t.Errorf("%s: Status %d", path, code)
		}
	}
}
