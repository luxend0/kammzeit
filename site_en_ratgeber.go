package main

// Englische Guides für kammzeit.com: Übersicht /guides und je Artikel /guides/<slug>,
// Gegenstück zum Ratgeber (site_ratgeber.go, hreflang über DE). Die Texte stehen komplett in
// web/site_en_ratgeber.html (Template "enguide-<slug>"); sie sind für Salons in der
// ganzen EU geschrieben, keine Übersetzung. site_en_ratgeber_test.go prüft Lesezeiten,
// Anker, Links und hreflang.

import "net/http"

// enGuides in der Reihenfolge der Übersicht; DE = Slug des deutschen Ratgeber-Artikels.
var enGuides = []struct{ Slug, DE, Title, Description string }{
	{"online-booking-for-salons", "online-terminbuchung-friseur", "Online booking for salons",
		"What online booking changes in a salon, when the phone is still better and how to introduce it: GDPR, costs, cancellation rules and regular clients."},
	{"reduce-no-shows", "no-shows-reduzieren", "Reducing no-shows in your salon",
		"Reduce no-shows in your salon: reminders the day before, cancelling by link, a sensible cancellation deadline, repeat no-shows, deposits and fees."},
	{"booking-link-instagram-google-whatsapp", "buchungslink-instagram-google", "Your booking link on Instagram, Google and WhatsApp",
		"Add your booking link to your Instagram bio, Google Business Profile, WhatsApp Business, website and an in-salon QR code, step by step, then test it."},
	{"services-and-durations", "preisliste-und-dauer", "Planning services and durations",
		"Plan salon services and durations realistically: measure your own times, add buffers, allow processing time for colour, protect breaks, see an example."},
	{"client-records-gdpr", "dsgvo-kundenkartei", "Client records and the GDPR",
		"Client records and the GDPR in your salon: what you may note, why allergies are special category data (Art. 9), access, erasure and processor agreements."},
}

func (a *app) routesSiteENRatgeber(mux *http.ServeMux) {
	a.addPageEN(mux, "/guides", "/ratgeber", "enguide-index", "Guides for salons - Kammzeit",
		"Guides for hairdressers and salons across Europe: online booking, no-shows, booking links, services and durations, client records and the GDPR.")
	for _, g := range enGuides {
		a.addPageEN(mux, "/guides/"+g.Slug, "/ratgeber/"+g.DE, "enguide-"+g.Slug, g.Title+" - Kammzeit", g.Description)
	}
}
