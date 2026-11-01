package main

// Ratgeber: Übersicht /ratgeber und je Artikel /ratgeber/<slug>.
// Die Texte stehen komplett in web/site_ratgeber.html (Template "ratgeber-<slug>").
// Lesezeit und Artikelliste stehen dort von Hand; site_ratgeber_test.go prüft,
// dass sie zur Textlänge passen.

import "net/http"

// ratgeberArtikel in der Reihenfolge der Übersicht.
var ratgeberArtikel = []struct{ Slug, Title, Description string }{
	{"online-terminbuchung-friseur", "Online-Terminbuchung im Friseursalon",
		"Was Online-Terminbuchung im Friseursalon bringt, wann das Telefon besser ist und wie du sie einführst: Datenschutz, Kosten, Absageregeln, Stammkunden."},
	{"no-shows-reduzieren", "No-Shows im Salon reduzieren",
		"No-Shows im Salon reduzieren: Erinnerung am Vortag, Absage per Link, passende Absagefrist, Umgang mit Wiederholungstätern, Anzahlung und Ausfallhonorar."},
	{"buchungslink-instagram-google", "Buchungslink in Instagram, Google und WhatsApp",
		"Buchungslink Schritt für Schritt in Instagram-Bio, Google-Unternehmensprofil, WhatsApp Business, Website und als QR-Code im Salon einrichten und testen."},
	{"preisliste-und-dauer", "Leistungen und Dauer im Salon planen",
		"Leistungen und Dauer im Salon realistisch planen: eigene Zeiten messen, Puffer, Einwirkzeit beim Färben, Pausen und eine Beispiel-Preisliste zum Anpassen."},
	{"dsgvo-kundenkartei", "Kundenkartei und DSGVO im Salon",
		"Kundenkartei und DSGVO im Salon: was du notieren darfst, warum Allergien besonders sensibel sind, Auskunft, Löschung und AVV mit Software-Anbietern."},
}

func (a *app) routesSiteRatgeber(mux *http.ServeMux) {
	a.addPage(mux, "/ratgeber", "ratgeber-index", "Ratgeber für Salons - Kammzeit",
		"Ratgeber für Friseure und Salons: Online-Terminbuchung, No-Shows, Buchungslink, Leistungen und Dauer, Kundenkartei und DSGVO. Konkret und praxisnah.")
	for _, r := range ratgeberArtikel {
		a.addPage(mux, "/ratgeber/"+r.Slug, "ratgeber-"+r.Slug, r.Title+" - Kammzeit", r.Description)
	}
}
