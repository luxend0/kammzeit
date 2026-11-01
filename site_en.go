package main

// Englische Seiten für kammzeit.com. Jeder Bereich registriert seine Seiten in
// seiner eigenen Datei (site_en_<bereich>.go) per a.addPageEN.

import "net/http"

func (a *app) routesSiteEN(mux *http.ServeMux) {
	a.routesSiteENMain(mux)     // site_en_main.go: /features, /pricing
	a.routesSiteENBranchen(mux) // site_en_branchen.go: /for/..., /about, /contact
	a.routesSiteENHilfe(mux)    // site_en_hilfe.go: /help..., /open-source, /security, /faq
	a.routesSiteENRatgeber(mux) // site_en_ratgeber.go: /guides...
	a.routesSiteENLegal(mux)    // site_en_legal.go: /imprint, /privacy, /terms, /dpa
}
