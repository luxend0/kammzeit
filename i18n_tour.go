package main

// Englische Texte der Dashboard-Tour (web/tour.html). Gleiche Schlüssel wie andere
// Bereiche nur mit gleichem Text.
func init() {
	addEN(map[string]string{
		// gemeinsam (auch in anderen Katalogen, gleicher Text)
		"Weiter":              "Next",
		"Zurück":              "Back",
		"Fertig":              "Done",
		"Kalender":            "Calendar",
		"Kunden":              "Customers",
		"Leistungen":          "Services",
		"Team":                "Team",
		"Einstellungen":       "Settings",
		"Abo":                 "Plan",
		"Warteschlange":       "Queue",
		"Deine Buchungsseite": "Your booking page",
		"Erste Schritte":      "Getting started",
		"Hilfe-Center":        "Help centre",

		// Rahmen
		"Tour starten":          "Start tour",
		"Überspringen":          "Skip",
		"Schritt %d von %d":     "Step %d of %d",
		"Tour durchs Dashboard": "Dashboard tour",

		// Schritte
		"Willkommen bei Kammzeit": "Welcome to Kammzeit",
		"In zwei Minuten zeigen wir dir, wo du was findest. Über das Fragezeichen oben startest du die Tour jederzeit neu.":                                                                                                  "In two minutes we'll show you where everything is. You can restart the tour at any time with the question mark at the top.",
		"Hier siehst du alle Termine, online gebucht und selbst eingetragen. Ruft jemand an, trägst du den Termin über „Neuer Termin“ ein.":                                                                                  "This is where you see every appointment, booked online or added by you. When someone phones, add the appointment with “New appointment”.",
		"Prüf Preise und Dauer. Die Beispiel-Leistungen sind anfangs inaktiv: Stell sie auf aktiv, damit sie online buchbar sind.":                                                                                           "Check prices and durations. The sample services start out inactive: set them to active so customers can book them online.",
		"Trag für jeden Mitarbeiter die Arbeitszeiten ein. Eine Mittagspause sind einfach zwei Blöcke, z. B. 9:00-12:00 und 13:00-18:00.":                                                                                    "Enter the working hours for each staff member. A lunch break is simply two blocks, e.g. 9:00-12:00 and 13:00-18:00.",
		"Trag Adresse und den Link zu deinem Impressum ein. Unter „Aussehen“ lädst du dein Logo hoch und wählst Akzentfarbe und Sprache der Buchungsseite. Social-Links und Öffnungszeiten zeigt die Buchungsseite auch an.": "Add your address and the link to your legal notice. Under “Appearance” you upload your logo and choose the accent colour and booking page language. Your booking page also shows social links and opening hours.",
		"Hier öffnest du deine Buchungsseite. Teil den Link in der Instagram-Bio, im Google-Unternehmensprofil und im WhatsApp-Status, dann buchen Kunden rund um die Uhr.":                                                  "This opens your booking page. Share the link in your Instagram bio, your Google Business Profile and your WhatsApp status, and customers can book around the clock.",
		"Die Kundenkartei füllt sich mit jeder Buchung von selbst. Auf der Karteikarte notierst du z. B. Farbrezepturen und siehst alle bisherigen Termine.":                                                                 "Your customer records fill up by themselves with every booking. On the customer card you can note things like colour formulas and see all past appointments.",
		"Für Laufkundschaft: Kunden ohne Termin stellen sich per QR-Code an, und du rufst sie der Reihe nach auf. Gehört zu Pro.":                                                                                            "For walk-ins: customers without an appointment join via QR code and you call them up in turn. Included with Pro.",
		"Gratis ist ein aktiver Mitarbeiter dabei. Mit Pro für 14,90 € im Monat sind es beliebig viele, und der Kammzeit-Hinweis auf der Buchungsseite fällt weg.":                                                           "Free includes one active staff member. With Pro for €14.90 a month you can have as many as you like, and the Kammzeit notice disappears from your booking page.",
		"Das war's schon": "That's it",
		"Was noch fehlt, zeigt dir die Liste „Erste Schritte“ im Kalender. Im Hilfe-Center ist jede Funktion Schritt für Schritt erklärt.": "The “Getting started” list in the calendar shows what's still to do. The Help centre explains every feature step by step.",
		"Im Hilfe-Center ist jede Funktion Schritt für Schritt erklärt. Die Tour startest du jederzeit über das Fragezeichen oben neu.":    "The Help centre explains every feature step by step. You can restart the tour at any time with the question mark at the top.",
	})
}
