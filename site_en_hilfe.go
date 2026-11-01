package main

// Englisches Hilfe-Center, Open Source, Sicherheit und FAQ für kammzeit.com.
// Inhalte in web/site_en_hilfe.html (Templates "enhelp-..."), Gestaltung wie die
// deutschen Seiten in web/static/site-hilfe.css. Jede Seite hat ihre deutsche
// Entsprechung (hreflang); /open-source hat in beiden Sprachen denselben Pfad.

import "net/http"

// enHelpGuides: englischer Slug, deutscher Slug, Titel, Beschreibung. Reihenfolge wie auf /help.
var enHelpGuides = []struct{ en, de, title, desc string }{
	{"getting-started", "erste-schritte", "Getting started: create and set up your salon - Kammzeit",
		"Sign up your salon with Kammzeit, log in with a login link and set up your booking page in about ten minutes, with every menu item and button."},
	{"services", "leistungen", "Services: duration, price and order - Kammzeit",
		"How to add services in Kammzeit with duration, price and description, sort them for your booking page and take them offline for a while."},
	{"working-hours", "arbeitszeiten", "Team, working hours and breaks - Kammzeit",
		"Add staff, assign services and enter working hours with a lunch break. Plus how to handle holidays while there is no absence feature yet."},
	{"booking-rules", "buchungsregeln", "Booking rules: minimum notice, cancellation deadline, booking window - Kammzeit",
		"How short-notice customers may book and cancel online and how far ahead: the booking rules in Kammzeit's settings explained one by one."},
	{"customise-booking-page", "buchungsseite-gestalten", "Customise your booking page: logo, photos, design - Kammzeit",
		"Logo, social links and opening hours on your booking page, plus a cover image, font, dark design, welcome message and photo gallery with Pro. How to set it up."},
	{"share-link", "link-teilen", "Share your booking link: Instagram, Google, WhatsApp - Kammzeit",
		"Make your booking page known: a link in your Instagram bio, your Google Business Profile, your WhatsApp status and a button on your own website."},
	{"add-appointments", "termine-eintragen", "Add phone bookings to the calendar - Kammzeit",
		"Appointments made by phone or in the salon go into Kammzeit's calendar too. The time is then taken online, nothing is double-booked, and rescheduling takes two clicks."},
	{"customer-records", "kundenkartei", "Customer records: notes, history, CSV export - Kammzeit",
		"Using Kammzeit's customer records: find and add customers, note colour formulas, see the appointment history, export customers as CSV or delete them."},
	{"cancellations-and-no-shows", "absagen-und-no-shows", "Cancellations, reminders and no-shows - Kammzeit",
		"How customers cancel online, how you cancel or reschedule an appointment, when the reminder goes out 24 hours before and how to mark no-shows."},
	{"queue", "warteschlange", "A queue for walk-in customers - Kammzeit",
		"Walk-ins join with a QR code and see their wait live: turn the queue on, print the QR code, put up a screen in your salon, call customers and tick them off."},
	{"staff-logins", "mitarbeiter-logins", "Invite your staff: logins for your team - Kammzeit",
		"With Pro your team gets their own logins: who sees what, how to invite them, how staff keep their own hours up to date and how to remove an access again."},
	{"install-app", "app-installieren", "Install Kammzeit as an app: iPhone, Android, computer - Kammzeit",
		"Install your dashboard like an app on your phone, tablet or computer, no app store needed. With shortcuts, an offline notice and no customer data on the device."},
	{"language", "sprache", "Language, German booking pages and kammzeit.de - Kammzeit",
		"Your booking page in English or German, one account for kammzeit.com and kammzeit.de, and which language Kammzeit uses for emails to you and your customers."},
	{"pro-and-cancelling", "pro-und-kuendigen", "Upgrade to Pro, manage and cancel your plan - Kammzeit",
		"Switch from Free to Pro, manage your payment method and invoices in the customer portal and cancel your plan: how plans work at Kammzeit."},
	{"export-and-delete", "daten-export-und-loeschen", "Export your data, Kammzeit news and deleting your account - Kammzeit",
		"Download all of your salon's data as a JSON file, subscribe to or unsubscribe from Kammzeit news and delete your account for good. Plus what happens afterwards."},
}

func (a *app) routesSiteENHilfe(mux *http.ServeMux) {
	a.addPageEN(mux, "/help", "/hilfe", "enhelp-index", "Help centre - Kammzeit",
		"Step-by-step guides for Kammzeit with the real menu items: set up and customise your salon, appointments, customer records, queue, staff logins, app, plan and data.")
	for _, g := range enHelpGuides {
		a.addPageEN(mux, "/help/"+g.en, "/hilfe/"+g.de, "enhelp-"+g.en, g.title, g.desc)
	}
	a.addPageEN(mux, "/open-source", "/open-source", "enhelp-open-source", "Open source: Kammzeit under AGPL-3.0 - Kammzeit",
		"Kammzeit's code is open (AGPL-3.0). What the licence allows and requires, how to host Kammzeit yourself in five steps and how to contribute.")
	a.addPageEN(mux, "/security", "/sicherheit", "enhelp-security", "Data and security - Kammzeit",
		"Where your salon's data is stored, who has access, how roles, login, cookies, the app, images, backups and deletion work, and how to report a vulnerability.")
	a.addPageEN(mux, "/faq", "/fragen", "enhelp-faq", "Frequently asked questions - Kammzeit",
		"Answers to common questions about Kammzeit: prices, VAT and cancelling, team and queue, app and languages, booking for customers, GDPR and self-hosting.")
}
