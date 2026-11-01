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

// Englische Rechtstexte: nur auf kammzeit.com (deutsche Domain und Salon-Subdomains
// 404), Übersetzungshinweis mit Link auf die deutsche Fassung, hreflang-Daten,
// Sitemap, interne Links im Text erreichbar, kein Em-Dash. Der Absatz zur
// Browsersprache steht in beiden Datenschutzerklärungen.
func TestSiteENLegal(t *testing.T) {
	db, err := openDB(t.TempDir() + "/t.db")
	if err != nil {
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

	pages := map[string]struct{ de, title, needle string }{
		"/imprint": {"/impressum", "Imprint - Kammzeit", "DE463618169"},
		"/privacy": {"/datenschutz", "Privacy policy - Kammzeit", "Accept-Language"},
		"/terms":   {"/agb", "Terms and conditions - Kammzeit", "reverse charge"},
		"/dpa":     {"/avv", "Data processing agreement - Kammzeit", "Art. 28(3)(a) GDPR"},
	}
	article := regexp.MustCompile(`(?s)<article class="prose">(.*)</article>`)
	href := regexp.MustCompile(`href="(/[^"#?]*)`)
	for path, p := range pages {
		code, body := get("kammzeit-en.test", path)
		if code != http.StatusOK {
			t.Fatalf("%s auf kammzeit.com: Status %d", path, code)
		}
		for _, want := range []string{
			"<title>" + p.title + "</title>",
			`<link rel="canonical" href="https://kammzeit-en.test` + path + `">`,
			"This is a translation for convenience. The German version is legally binding.",
			`<a href="https://kammzeit.test` + p.de + `" hreflang="de"`,
			p.needle,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("%s enthält nicht %q", path, want)
			}
		}
		for _, bad := range []string{"—", "–", "{{", "ZgotmplZ"} {
			if strings.Contains(body, bad) {
				t.Errorf("%s enthält %q", path, bad)
			}
		}
		m := article.FindStringSubmatch(body)
		if m == nil {
			t.Fatalf("%s: kein Artikel", path)
		}
		for _, l := range href.FindAllStringSubmatch(m[1], -1) {
			if code, _ := get("kammzeit-en.test", l[1]); code != http.StatusOK {
				t.Errorf("%s verlinkt %s: Status %d", path, l[1], code)
			}
		}
		alt := a.alternates("en", path)
		if len(alt) != 2 || alt[1].Lang != "de" || alt[1].URL != "https://kammzeit.test"+p.de {
			t.Errorf("%s: hreflang %v", path, alt)
		}
		for _, host := range []string{"kammzeit.test", "mueller.kammzeit-en.test"} {
			if code, _ := get(host, path); code != http.StatusNotFound {
				t.Errorf("%s auf %s: Status %d, erwartet 404", path, host, code)
			}
		}
	}

	if _, sm := get("kammzeit-en.test", "/sitemap.xml"); !strings.Contains(sm, "<loc>https://kammzeit-en.test/dpa</loc>") {
		t.Error("englische Sitemap ohne /dpa")
	}
	if _, sm := get("kammzeit.test", "/sitemap.xml"); strings.Contains(sm, "/imprint") {
		t.Error("deutsche Sitemap enthält /imprint")
	}
	if _, de := get("kammzeit.test", "/datenschutz"); !strings.Contains(de, "(Accept-Language). Wir speichern sie nicht") {
		t.Error("deutsche Datenschutzerklärung ohne Absatz zur Browsersprache")
	}
}
