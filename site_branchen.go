package main

// Branchenseiten (/fuer/...), Über uns, Kontakt. Registrierung per a.addPage (site.go).
// Inhalte stehen in web/site_branchen.html, Gestaltung in web/static/site-branchen.css.

import "net/http"

func (a *app) routesSiteBranchen(mux *http.ServeMux) {
	a.addPage(mux, "/fuer/friseure", "branchen-friseure", "Online-Terminbuchung für Friseure - Kammzeit",
		"Online-Terminbuchung für Friseursalons: Färben und Strähnen mit echter Dauer, Pausen je Mitarbeiter, Farbrezeptur in der Kundenkartei. Gratis für eine Person.")
	a.addPage(mux, "/fuer/barbershops", "branchen-barbershops", "Online-Terminbuchung für Barbershops - Kammzeit",
		"Terminbuchung für Barbershops: Termine im 15-Minuten-Raster, Laufkunden schnell im Kalender eintragen, Erinnerung gegen No-Shows. Gratis für einen Barber.")
	a.addPage(mux, "/fuer/kosmetik", "branchen-kosmetik", "Online-Terminbuchung für Kosmetikstudios - Kammzeit",
		"Terminbuchung für Kosmetikstudios: Behandlungen mit 60 oder 90 Minuten, Vorlauf und Absagefrist nach deinen Regeln, Hauttyp-Notizen in der Kundenkartei.")
	a.addPage(mux, "/fuer/nagelstudios", "branchen-nagelstudios", "Online-Terminbuchung für Nagelstudios - Kammzeit",
		"Terminbuchung für Nagelstudios: Neumodellage und Auffüllen mit echter Dauer, Kundinnen buchen den nächsten Termin selbst, Form und Farbe stehen in der Kartei.")
	a.addPage(mux, "/ueber", "branchen-ueber", "Wer hinter Kammzeit steckt - Kammzeit",
		"Kammzeit ist ein kleines, unabhängiges Projekt von Ben Oliver Mallow, dem Betreiber von InstantNode: eigene Server in den Niederlanden, offener Quellcode.")
	a.addPage(mux, "/kontakt", "branchen-kontakt", "Kontakt - Kammzeit",
		"So erreichst du Kammzeit: per E-Mail an kontakt@kammzeit.de, Antwort meist innerhalb von ein bis zwei Werktagen. Dazu Hilfe-Center, Fragen und Postadresse.")
}
