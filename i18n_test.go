package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestI18nDomainsAndRedirect(t *testing.T) {
	a := &app{cfg: config{BaseDomain: "kammzeit.de", SiteURL: "https://kammzeit.de",
		Domains: parseDomains("kammzeit.de=de,kammzeit.com=en", "x")}}
	for host, want := range map[string][3]string{
		"kammzeit.de":           {"", "root", "de"},
		"kammzeit.com":          {"", "root", "en"},
		"lindner.kammzeit.com":  {"lindner", "", "en"},
		"lindner.kammzeit.de":   {"lindner", "", "de"},
		"evil.com":              {"", "", ""},
		"kammzeit.com.evil.org": {"", "", ""},
	} {
		slug, root, d := a.tenant(host)
		r := ""
		if root {
			r = "root"
		}
		if slug != want[0] || r != want[1] || d.Lang != want[2] {
			t.Errorf("tenant(%q) = %q %q %q, want %v", host, slug, r, d.Lang, want)
		}
	}
	if got := a.siteURLLang("en"); got != "https://kammzeit.com" {
		t.Errorf("siteURLLang(en) = %q", got)
	}
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	h := a.langRedirect(ok)
	cases := []struct {
		host, path, lang, ref string
		want                  int
	}{
		{"kammzeit.com", "/", "de-DE,de;q=0.9,en;q=0.8", "", 302},
		{"kammzeit.com", "/", "en-GB,en;q=0.9,de;q=0.5", "", 200},
		{"kammzeit.com", "/?lang=en", "de-DE", "", 200},             // Sprachumschalter
		{"kammzeit.com", "/", "de-DE", "https://kammzeit.de/", 200}, // kommt von der eigenen Seite
		{"kammzeit.com", "/pricing", "de-DE", "", 200},              // nur Startseite
		{"kammzeit.de", "/", "en-US", "", 200},                      // .de leitet nie um
		{"lindner.kammzeit.com", "/", "de-DE", "", 200},             // Buchungsseiten nie
	}
	for _, c := range cases {
		req := httptest.NewRequest("GET", c.path, nil)
		req.Host = c.host
		req.Header.Set("Accept-Language", c.lang)
		if c.ref != "" {
			req.Header.Set("Referer", c.ref)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Errorf("%s%s (%s, ref %q) = %d, want %d", c.host, c.path, c.lang, c.ref, rec.Code, c.want)
		}
	}
}

func TestTrFallback(t *testing.T) {
	addEN(map[string]string{"Speichern": "Save"})
	if tr("en", "Speichern") != "Save" || tr("de", "Speichern") != "Speichern" || tr("en", "Unbekannt") != "Unbekannt" {
		t.Fatal("tr kaputt")
	}
	if trf("en", "%d Termine", 3) != "3 Termine" {
		t.Fatal("trf ohne Übersetzung muss das deutsche Format nutzen")
	}
}
