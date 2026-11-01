package main

// Seiten Funktionen und Preise. Registrierung per a.addPage (site.go),
// Templates in web/site_main.html, Stil in static/site-main.css.

import "net/http"

func (a *app) routesSiteMain(mux *http.ServeMux) {
	a.addPage(mux, "/funktionen", "main-funktionen", "Funktionen - Kammzeit",
		"Buchungsseite ohne App, Erinnerung per Mail, Kalender, Kundenkartei, Dashboard als App. Mit Pro Warteschlange und Mitarbeiter-Logins. Server in der EU.")
	a.addPage(mux, "/preise", "main-preise", "Preise - Kammzeit",
		"Gratis für einen Mitarbeiter. Pro für 14,90 € im Monat oder 149 € im Jahr: unbegrenzt Mitarbeiter, Logins fürs Team, Warteschlange. Endpreise nach § 19 UStG.")
}
