package main

// Seiten Funktionen und Preise. Registrierung per a.addPage (site.go),
// Templates in web/site_main.html, Stil in static/site-main.css.

import "net/http"

func (a *app) routesSiteMain(mux *http.ServeMux) {
	a.addPage(mux, "/funktionen", "main-funktionen", "Funktionen - Kammzeit",
		"Buchungsseite ohne App, Erinnerung und Absage per Mail, Kalender, Kundenkartei und Team mit Pausen. Dazu Server in der EU, Export und Open Source.")
	a.addPage(mux, "/preise", "main-preise", "Preise - Kammzeit",
		"Gratis für einen Mitarbeiter, Pro für 14,90 € im Monat oder 149 € im Jahr mit unbegrenzt Mitarbeitern. Endpreise nach § 19 UStG, monatlich kündbar.")
}
