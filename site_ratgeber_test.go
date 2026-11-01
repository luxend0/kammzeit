package main

import (
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

// Ratgeber: alle Seiten nur auf der Hauptdomain, ohne Em-Dash. Pro Artikel passen
// Wortzahl (900-1500), angegebene Lesezeit (Artikelkopf und Übersicht),
// Inhaltsverzeichnis-Anker, Beschreibungslänge und JSON-LD.
func TestRatgeber(t *testing.T) {
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

	pages := []string{"/ratgeber"}
	for _, r := range ratgeberArtikel {
		pages = append(pages, "/ratgeber/"+r.Slug)
	}
	bodies := map[string]string{}
	for _, p := range pages {
		code, body := get("kammzeit.test", p)
		if code != http.StatusOK {
			t.Fatalf("%s: Status %d", p, code)
		}
		for _, bad := range []string{"\u2014", "{{", "ZgotmplZ"} {
			if strings.Contains(body, bad) {
				t.Errorf("%s enthält %q", p, bad)
			}
		}
		if !strings.Contains(body, `<link rel="canonical" href="https://kammzeit.de`+p+`">`) {
			t.Errorf("%s: canonical fehlt", p)
		}
		if code, _ := get("mueller.kammzeit.test", p); code != http.StatusNotFound {
			t.Errorf("%s auf Salon-Subdomain: Status %d, erwartet 404", p, code)
		}
		bodies[p] = body
	}

	index := bodies["/ratgeber"]
	tags := regexp.MustCompile(`<[^>]+>`)
	hrefs := regexp.MustCompile(`href="#([^"]+)"`)
	ldJSON := regexp.MustCompile(`(?s)<script type="application/ld\+json">(.*?)</script>`)
	for _, r := range ratgeberArtikel {
		p := "/ratgeber/" + r.Slug
		body := bodies[p]
		if n := utf8.RuneCountInString(r.Description); n < 120 || n > 160 {
			t.Errorf("%s: Beschreibung hat %d Zeichen, erwartet 120-160", p, n)
		}
		if !strings.Contains(index, `href="`+p+`"`) {
			t.Errorf("Übersicht verlinkt %s nicht", p)
		}

		_, rest, ok := strings.Cut(body, `<article class="prose rg-body">`)
		text, _, ok2 := strings.Cut(rest, `</article>`)
		if !ok || !ok2 {
			t.Fatalf("%s: Artikeltext nicht gefunden", p)
		}
		words := len(strings.Fields(html.UnescapeString(tags.ReplaceAllString(text, " "))))
		if words < 900 || words > 1500 {
			t.Errorf("%s: %d Wörter, erwartet 900-1500", p, words)
		}
		min := (words + 199) / 200
		if !strings.Contains(body, fmt.Sprintf(`<span class="rg-read">%d Min. Lesezeit</span>`, min)) {
			t.Errorf("%s: Lesezeit im Kopf passt nicht zu %d Wörtern (%d Min.)", p, words, min)
		}
		if !strings.Contains(index, fmt.Sprintf(`data-slug="%s">%d Min.</span>`, r.Slug, min)) {
			t.Errorf("Übersicht: Lesezeit von %s passt nicht zu %d Wörtern (%d Min.)", p, words, min)
		}

		_, toc, _ := strings.Cut(body, `<nav class="rg-toc"`)
		toc, _, _ = strings.Cut(toc, `</nav>`)
		links := hrefs.FindAllStringSubmatch(toc, -1)
		if len(links) < 5 {
			t.Errorf("%s: Inhaltsverzeichnis fehlt oder ist zu kurz", p)
		}
		for _, m := range links {
			if !strings.Contains(text, `id="`+m[1]+`"`) {
				t.Errorf("%s: Inhaltsverzeichnis zeigt auf fehlenden Anker #%s", p, m[1])
			}
		}
		if !strings.Contains(body, `href="/hilfe/`) {
			t.Errorf("%s: kein Link ins Hilfe-Center", p)
		}

		m := ldJSON.FindStringSubmatch(body)
		var ld struct {
			Graph []map[string]any `json:"@graph"`
		}
		if m == nil || json.Unmarshal([]byte(m[1]), &ld) != nil || len(ld.Graph) != 2 || ld.Graph[0]["mainEntityOfPage"] != "https://kammzeit.de"+p {
			t.Errorf("%s: JSON-LD fehlt oder ist ungültig", p)
		}
	}
}
