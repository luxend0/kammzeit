package main

// Öffentliche Seiten von kammzeit.de (Marketing, Hilfe, Ratgeber) plus
// sitemap.xml und robots.txt. Jeder Bereich registriert seine Seiten in seiner
// eigenen Datei über a.addPage, die Sitemap entsteht daraus automatisch.

import (
	"encoding/xml"
	"fmt"
	"net/http"
)

type sitePage struct {
	Path, Template, Title, Description string
	Lang                               string // de | en
	Alt                                string // gleiche Seite in der anderen Sprache (Pfad), für hreflang
}

// sitePages = alle registrierten Seiten in Registrierungsreihenfolge (für die Sitemap).
var sitePages []sitePage

func (a *app) routesSite(mux *http.ServeMux) {
	sitePages = nil
	a.pages = map[string]map[string]http.HandlerFunc{}
	a.routesSiteMain(mux)     // site_main.go: /funktionen, /preise
	a.routesSiteBranchen(mux) // site_branchen.go: /fuer/..., /ueber, /kontakt
	a.routesSiteHilfe(mux)    // site_hilfe.go: /hilfe..., /open-source, /sicherheit, /fragen
	a.routesSiteRatgeber(mux) // site_ratgeber.go: /ratgeber...
	a.routesSiteEN(mux)       // site_en_*.go: englische Seiten für kammzeit.com
	mux.HandleFunc("GET /sitemap.xml", a.sitemap)
	mux.HandleFunc("GET /robots.txt", a.robots)
}

// addPage registriert eine deutsche Seite (nur Hauptdomain auf Deutsch).
// Template-Daten: Title, Description, Canonical, Path, Lang, Alternates (+ a.page).
func (a *app) addPage(mux *http.ServeMux, path, tpl, title, description string) {
	a.addPageLang(mux, "de", path, "", tpl, title, description)
}

// addPageEN registriert eine englische Seite für kammzeit.com; dePath = die
// entsprechende deutsche Seite ("" wenn es keine gibt) für hreflang.
func (a *app) addPageEN(mux *http.ServeMux, path, dePath, tpl, title, description string) {
	a.addPageLang(mux, "en", path, dePath, tpl, title, description)
}

func (a *app) addPageLang(mux *http.ServeMux, lang, path, alt, tpl, title, description string) {
	sitePages = append(sitePages, sitePage{path, tpl, title, description, lang, alt})
	if alt != "" { // Rückverweis für die deutsche Seite
		for i := range sitePages {
			if sitePages[i].Path == alt && sitePages[i].Lang != lang {
				sitePages[i].Alt = path
			}
		}
	}
	h := func(w http.ResponseWriter, r *http.Request) {
		_, root, d := a.tenant(r.Host)
		if a.cfg.SingleTenant || !root || d.Lang != lang {
			http.NotFound(w, r)
			return
		}
		data := a.page(nil, title)
		data["Lang"] = lang
		data["Description"] = description
		data["Canonical"] = a.siteURLFor(r) + path
		data["Path"] = path
		data["Alternates"] = a.alternates(lang, path)
		a.render(w, http.StatusOK, tpl, data)
	}
	if existing, ok := a.pages[path]; ok {
		existing[lang] = h // gleicher Pfad in beiden Sprachen: ein Muster, je Domain ein Handler
		return
	}
	a.pages[path] = map[string]http.HandlerFunc{lang: h}
	mux.HandleFunc("GET "+path, func(w http.ResponseWriter, r *http.Request) {
		_, _, d := a.tenant(r.Host)
		if f, ok := a.pages[path][d.Lang]; ok {
			f(w, r)
			return
		}
		http.NotFound(w, r)
	})
}

type alternate struct{ Lang, URL string }

// alternates = hreflang-Links: diese Seite + ihre Entsprechung in der anderen Sprache.
func (a *app) alternates(lang, path string) []alternate {
	other, otherLang := a.otherSite(lang)
	out := []alternate{{lang, a.siteURLLang(lang) + path}}
	if other == "" {
		return out
	}
	for _, p := range sitePages {
		if p.Path == path && p.Lang == lang && p.Alt != "" {
			out = append(out, alternate{otherLang, other + p.Alt})
		}
	}
	if path == "/" {
		out = append(out, alternate{otherLang, other + "/"})
	}
	return out
}

func (a *app) sitemap(w http.ResponseWriter, r *http.Request) {
	_, root, d := a.tenant(r.Host)
	if a.cfg.SingleTenant || !root {
		http.NotFound(w, r)
		return
	}
	type u struct {
		Loc string `xml:"loc"`
	}
	base := a.siteURLFor(r)
	urls := []u{{base + "/"}}
	for _, p := range sitePages {
		if p.Lang == d.Lang {
			urls = append(urls, u{base + p.Path})
		}
	}
	extra := []string{"/registrieren", "/impressum", "/datenschutz", "/agb", "/avv"}
	if d.Lang == "en" {
		extra = []string{signupPath("en")}
	}
	for _, p := range extra {
		urls = append(urls, u{base + p})
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	fmt.Fprint(w, xml.Header)
	xml.NewEncoder(w).Encode(struct {
		XMLName xml.Name `xml:"urlset"`
		NS      string   `xml:"xmlns,attr"`
		URLs    []u      `xml:"url"`
	}{NS: "http://www.sitemaps.org/schemas/sitemap/0.9", URLs: urls})
}

func (a *app) robots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, "User-agent: *\nDisallow: /dashboard\nDisallow: /login\nDisallow: /storno/\nDisallow: /termin/\n")
	if !a.cfg.SingleTenant && a.onRootHost(r) {
		fmt.Fprintf(w, "\nSitemap: %s/sitemap.xml\n", a.siteURLFor(r))
	}
}
