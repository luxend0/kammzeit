package main

// Englische Rechtstexte für kammzeit.com (web/legal_en.html). Es sind
// Übersetzungen, maßgeblich ist die deutsche Fassung (web/legal.html).
// Die deutschen Seiten laufen über a.static (handlers.go), nicht über a.addPage:
// dePath sorgt deshalb nur auf den englischen Seiten für hreflang und den Link
// auf die deutsche Fassung, die deutschen Seiten bekommen keinen Rückverweis.

import "net/http"

func (a *app) routesSiteENLegal(mux *http.ServeMux) {
	a.addPageEN(mux, "/imprint", "/impressum", "en-imprint", "Imprint - Kammzeit",
		"Legal notice for Kammzeit, the online booking service for salons: provider, postal address in Germany, email, phone and VAT identification number.")
	a.addPageEN(mux, "/privacy", "/datenschutz", "en-privacy", "Privacy policy - Kammzeit",
		"How Kammzeit processes personal data: servers in the EU, one login cookie, no tracking, Stripe for Pro payments, and your rights under the GDPR.")
	a.addPageEN(mux, "/terms", "/agb", "en-terms", "Terms and conditions - Kammzeit",
		"Terms and conditions for Kammzeit, online booking for salons: Free and Pro plans, payment via Stripe, cancellation, liability and your data.")
	a.addPageEN(mux, "/dpa", "/avv", "en-dpa", "Data processing agreement - Kammzeit",
		"Kammzeit's data processing agreement under Art. 28 GDPR: processing on behalf of salons, security measures and sub-processors in the EU.")
}
