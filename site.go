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
}

// sitePages = alle registrierten Seiten in Registrierungsreihenfolge (für die Sitemap).
var sitePages []sitePage

func (a *app) routesSite(mux *http.ServeMux) {
	sitePages = nil
	a.routesSiteMain(mux)     // site_main.go: /funktionen, /preise
	a.routesSiteBranchen(mux) // site_branchen.go: /fuer/..., /ueber, /kontakt
	a.routesSiteHilfe(mux)    // site_hilfe.go: /hilfe..., /open-source, /sicherheit, /fragen
	a.routesSiteRatgeber(mux) // site_ratgeber.go: /ratgeber...
	mux.HandleFunc("GET /sitemap.xml", a.sitemap)
	mux.HandleFunc("GET /robots.txt", a.robots)
}

// addPage registriert eine statische Seite. Sie existiert nur auf der Hauptdomain
// (nicht auf Salon-Subdomains und nicht im Einzelbetrieb).
// Template-Daten: Title, Description, Canonical, Path (+ alles aus a.page).
func (a *app) addPage(mux *http.ServeMux, path, tpl, title, description string) {
	sitePages = append(sitePages, sitePage{path, tpl, title, description})
	mux.HandleFunc("GET "+path, func(w http.ResponseWriter, r *http.Request) {
		if a.cfg.SingleTenant || !a.onRootHost(r) {
			http.NotFound(w, r)
			return
		}
		d := a.page(nil, title)
		d["Description"] = description
		d["Canonical"] = a.siteURL() + path
		d["Path"] = path
		a.render(w, http.StatusOK, tpl, d)
	})
}

func (a *app) sitemap(w http.ResponseWriter, r *http.Request) {
	if a.cfg.SingleTenant || !a.onRootHost(r) {
		http.NotFound(w, r)
		return
	}
	type u struct {
		Loc string `xml:"loc"`
	}
	urls := []u{{a.siteURL() + "/"}}
	for _, p := range sitePages {
		urls = append(urls, u{a.siteURL() + p.Path})
	}
	for _, p := range []string{"/registrieren", "/impressum", "/datenschutz", "/agb", "/avv"} {
		urls = append(urls, u{a.siteURL() + p})
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
		fmt.Fprintf(w, "\nSitemap: %s/sitemap.xml\n", a.siteURL())
	}
}
