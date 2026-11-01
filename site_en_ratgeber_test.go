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

// Englische Guides: nur auf der englischen Hauptdomain, ohne Em-Dash und ohne Links auf
// deutsche Seiten. Pro Artikel passen Wortzahl (900-1500), Lesezeit (Kopf und Übersicht),
// Inhaltsverzeichnis-Anker, Beschreibungslänge, Hilfe-Link, JSON-LD und hreflang zum Ratgeber.
func TestENGuides(t *testing.T) {
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

	help := map[string]string{
		"online-booking-for-salons":              "/help/getting-started",
		"reduce-no-shows":                        "/help/cancellations-and-no-shows",
		"booking-link-instagram-google-whatsapp": "/help/share-link",
		"services-and-durations":                 "/help/services",
		"client-records-gdpr":                    "/help/customer-records",
	}
	guides := map[string]bool{}
	pages := []string{"/guides"}
	for _, g := range enGuides {
		guides["/guides/"+g.Slug] = true
		pages = append(pages, "/guides/"+g.Slug)
	}

	mainRe := regexp.MustCompile(`(?s)<main[^>]*>(.*)</main>`)
	linkRe := regexp.MustCompile(`href="(/[^"]*)"`)
	bodies := map[string]string{}
	for _, p := range pages {
		code, body := get("kammzeit-en.test", p)
		if code != http.StatusOK {
			t.Fatalf("%s: Status %d", p, code)
		}
		for _, bad := range []string{"\u2014", "{{", "ZgotmplZ"} {
			if strings.Contains(body, bad) {
				t.Errorf("%s enthält %q", p, bad)
			}
		}
		if !strings.Contains(body, `<link rel="canonical" href="https://kammzeit-en.test`+p+`">`) {
			t.Errorf("%s: canonical fehlt", p)
		}
		for _, host := range []string{"kammzeit.test", "mueller.kammzeit-en.test"} {
			if code, _ := get(host, p); code != http.StatusNotFound {
				t.Errorf("%s auf %s: Status %d, erwartet 404", p, host, code)
			}
		}
		// Eigener Inhalt (ohne Kopf/Fuß des Layouts): keine deutschen Reste, interne Links nur auf
		// englische Seiten und vorhandene Guides.
		m := mainRe.FindStringSubmatch(body)
		if m == nil {
			t.Fatalf("%s: <main> fehlt", p)
		}
		for _, de := range []string{"Lesezeit", "Ratgeber", " und ", " für ", "Kostenlos starten"} {
			if strings.Contains(m[1], de) {
				t.Errorf("%s: deutscher Text %q im Inhalt", p, de)
			}
		}
		for _, l := range linkRe.FindAllStringSubmatch(m[1], -1) {
			h := l[1]
			if strings.HasPrefix(h, "/ratgeber") || strings.HasPrefix(h, "/hilfe") || h == "/avv" {
				t.Errorf("%s verlinkt deutsche Seite %s", p, h)
			}
			if strings.HasPrefix(h, "/guides/") && !guides[h] {
				t.Errorf("%s verlinkt unbekannten Guide %s", p, h)
			}
		}
		bodies[p] = body
	}

	if n := len(a.alternates("en", "/guides")); n != 2 {
		t.Errorf("/guides: %d hreflang-Einträge, erwartet 2", n)
	}
	index := bodies["/guides"]
	tags := regexp.MustCompile(`<[^>]+>`)
	hrefs := regexp.MustCompile(`href="#([^"]+)"`)
	ldJSON := regexp.MustCompile(`(?s)<script type="application/ld\+json">(.*?)</script>`)
	for _, g := range enGuides {
		p := "/guides/" + g.Slug
		body := bodies[p]
		if n := utf8.RuneCountInString(g.Description); n < 120 || n > 160 {
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
		if !strings.Contains(body, fmt.Sprintf(`<span class="rg-read">%d min read</span>`, min)) {
			t.Errorf("%s: Lesezeit im Kopf passt nicht zu %d Wörtern (%d min)", p, words, min)
		}
		if !strings.Contains(index, fmt.Sprintf(`data-slug="%s">%d min</span>`, g.Slug, min)) {
			t.Errorf("Übersicht: Lesezeit von %s passt nicht zu %d Wörtern (%d min)", p, words, min)
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
		if !strings.Contains(body, `href="`+help[g.Slug]+`"`) {
			t.Errorf("%s: Link auf %s fehlt", p, help[g.Slug])
		}

		m := ldJSON.FindStringSubmatch(body)
		var ld struct {
			Graph []map[string]any `json:"@graph"`
		}
		if m == nil || json.Unmarshal([]byte(m[1]), &ld) != nil || len(ld.Graph) != 2 ||
			ld.Graph[0]["mainEntityOfPage"] != "https://kammzeit-en.test"+p || ld.Graph[0]["inLanguage"] != "en" {
			t.Errorf("%s: JSON-LD fehlt oder ist ungültig", p)
		}

		// hreflang in beide Richtungen: Guide <-> Ratgeber-Artikel
		want := alternate{"de", "https://kammzeit.de/ratgeber/" + g.DE}
		if alt := a.alternates("en", p); len(alt) != 2 || alt[1] != want {
			t.Errorf("%s: hreflang %v, erwartet %v", p, alt, want)
		}
		back := alternate{"en", "https://kammzeit-en.test" + p}
		if alt := a.alternates("de", "/ratgeber/"+g.DE); len(alt) != 2 || alt[1] != back {
			t.Errorf("/ratgeber/%s: hreflang %v, erwartet %v", g.DE, alt, back)
		}
	}
}
