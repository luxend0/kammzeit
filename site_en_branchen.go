package main

// Englische Branchenseiten (/for/...), About, Contact für kammzeit.com, jeweils
// mit der deutschen Entsprechung für hreflang. Inhalte in web/site_en_branchen.html,
// Gestaltung wie die deutschen Seiten in web/static/site-branchen.css.

import "net/http"

func (a *app) routesSiteENBranchen(mux *http.ServeMux) {
	a.addPageEN(mux, "/for/hairdressers", "/fuer/friseure", "enbr-hairdressers", "Online booking for hairdressers - Kammzeit",
		"Online booking for hair salons: colour with its real duration, breaks for each staff member, colour formulas on the customer card. Free for one person.")
	a.addPageEN(mux, "/for/barbershops", "/fuer/barbershops", "enbr-barbershops", "Online booking for barbershops - Kammzeit",
		"Online booking for barbershops: start times every 15 minutes, a QR code queue for walk-ins, email reminders against no-shows. Free for one barber.")
	a.addPageEN(mux, "/for/beauty-salons", "/fuer/kosmetik", "enbr-beauty-salons", "Online booking for beauty salons - Kammzeit",
		"Online booking for beauty salons: 60- or 90-minute treatments, minimum notice and cancellation deadline on your terms, skin type notes on the customer card.")
	a.addPageEN(mux, "/for/nail-salons", "/fuer/nagelstudios", "enbr-nail-salons", "Online booking for nail salons - Kammzeit",
		"Online booking for nail salons: new sets and infills with their real duration, customers book their next appointment themselves, shape and colour on file.")
	a.addPageEN(mux, "/about", "/ueber", "enbr-about", "Who is behind Kammzeit - Kammzeit",
		"Kammzeit is a small, independent project by Ben Oliver Mallow, who also runs the hosting company InstantNode: own servers in the Netherlands, open source code.")
	a.addPageEN(mux, "/contact", "/kontakt", "enbr-contact", "Contact - Kammzeit",
		"How to reach Kammzeit: email kontakt@kammzeit.de, replies usually within 1-2 working days. Plus the help centre, frequently asked questions and postal address.")
}
