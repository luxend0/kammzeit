package main

// Englische Seiten Features und Pricing für kammzeit.com (Templates in
// web/site_en_main.html, Startseite in web/landing_en.html) und die Helfer, mit
// denen Kopf und Fuß in web/site.html zweisprachig werden. Layout-Texte: i18n_site.go.

import (
	"fmt"
	"html/template"
	"net/http"
)

func (a *app) routesSiteENMain(mux *http.ServeMux) {
	a.addPageEN(mux, "/features", "/funktionen", "en-features", "Features - Kammzeit",
		"Booking page without an app, email reminders and cancellation links, calendar, customer records and staff with breaks. EU servers, data export, open source.")
	a.addPageEN(mux, "/pricing", "/preise", "en-pricing", "Pricing - Kammzeit",
		"Free for one staff member. Pro is €14.90 per month or €149 per year with unlimited staff. No VAT charged under § 19 UStG, cancel monthly or yearly.")
	if a.tpl != nil {
		// ponytail: die Platzhalter aus init() an diese App binden, weil Hreflang und
		// Sprachumschalter die konfigurierten Domains brauchen.
		a.tpl.Funcs(template.FuncMap{"hreflang": a.hreflang, "langSwitch": a.langSwitch})
	}
}

// enPaths = englische Pfade der Seiten, die Kopf und Fuß verlinken (Vertrag kammzeit.com).
var enPaths = map[string]string{
	"/funktionen": "/features", "/preise": "/pricing",
	"/fuer/friseure": "/for/hairdressers", "/fuer/barbershops": "/for/barbershops",
	"/fuer/kosmetik": "/for/beauty-salons", "/fuer/nagelstudios": "/for/nail-salons",
	"/hilfe": "/help", "/fragen": "/faq", "/ratgeber": "/guides", "/kontakt": "/contact", "/ueber": "/about",
	"/open-source": "/open-source", "/sicherheit": "/security",
	"/impressum": "/imprint", "/datenschutz": "/privacy", "/agb": "/terms", "/avv": "/dpa",
}

func init() {
	// {{sitePath .Lang "/preise"}} = Pfad der Seite in der Sprache der Anfrage.
	templateFuncs["sitePath"] = func(lang any, de string) string {
		if p, ok := enPaths[de]; ok && fmt.Sprint(lang) == "en" {
			return p
		}
		return de
	}
	// Platzhalter, damit die Templates parsen; routesSiteENMain bindet die echten.
	templateFuncs["hreflang"] = func(lang any, path string) []alternate { return nil }
	templateFuncs["langSwitch"] = func(lang, path any) string { return "" }
}

func langOf(lang any) string {
	if fmt.Sprint(lang) == "en" {
		return "en"
	}
	return "de"
}

// hreflang = a.alternates für Seiten ohne .Alternates in den Daten (Startseiten).
func (a *app) hreflang(lang any, path string) []alternate { return a.alternates(langOf(lang), path) }

// langSwitch = Ziel des Sprachumschalters: dieselbe Seite in der anderen Sprache,
// sonst deren Startseite; "" ohne zweite Domain. Die englische Startseite bekommt
// ?lang=en, sonst schickt langRedirect deutschsprachige Browser gleich zurück
// (Referrer-Policy same-origin: domainübergreifend kommt kein Referer mit).
func (a *app) langSwitch(lang, path any) string {
	l := langOf(lang)
	other, otherLang := a.otherSite(l)
	if other == "" || a.cfg.SingleTenant {
		return ""
	}
	p, _ := path.(string)
	if p == "" {
		p = "/"
	}
	url := other + "/"
	if alts := a.alternates(l, p); len(alts) > 1 {
		url = alts[1].URL
	}
	if otherLang == "en" && url == other+"/" {
		url += "?lang=en"
	}
	return url
}
