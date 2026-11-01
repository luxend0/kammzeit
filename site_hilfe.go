package main

// Hilfe-Center, Open Source, Sicherheit, Fragen. Registrierung per a.addPage (site.go),
// Inhalte in web/site_hilfe.html (Templates "hilfe-..."), Gestaltung in web/static/site-hilfe.css.

import "net/http"

func (a *app) routesSiteHilfe(mux *http.ServeMux) {
	a.addPage(mux, "/hilfe", "hilfe-index", "Hilfe-Center - Kammzeit",
		"Anleitungen für Kammzeit mit den echten Menüpunkten: Salon einrichten, Buchungsseite gestalten, Termine, Kundenkartei, Warteschlange, Mitarbeiter-Logins, App, Abo und Daten.")

	// Anleitungen, Reihenfolge wie auf /hilfe
	a.addPage(mux, "/hilfe/erste-schritte", "hilfe-erste-schritte", "Erste Schritte: Salon anlegen und einrichten - Kammzeit",
		"Salon bei Kammzeit registrieren, per Login-Link anmelden und die Buchungsseite in etwa zehn Minuten einrichten, mit allen Menüpunkten und Buttons.")
	a.addPage(mux, "/hilfe/leistungen", "hilfe-leistungen", "Leistungen anlegen: Dauer, Preis, Reihenfolge - Kammzeit",
		"So legst du in Kammzeit Leistungen mit Dauer, Preis und Beschreibung an, sortierst sie für die Buchungsseite und nimmst sie vorübergehend aus dem Angebot.")
	a.addPage(mux, "/hilfe/arbeitszeiten", "hilfe-arbeitszeiten", "Team, Arbeitszeiten und Pausen - Kammzeit",
		"Mitarbeiter anlegen, Leistungen zuordnen und Arbeitszeiten mit Mittagspause eintragen. Dazu, wie du Urlaub löst, solange es keine Abwesenheiten gibt.")
	a.addPage(mux, "/hilfe/buchungsregeln", "hilfe-buchungsregeln", "Buchungsregeln: Vorlauf, Absagefrist, Zeitraum - Kammzeit",
		"Wie kurzfristig Kunden online buchen und absagen dürfen und wie weit im Voraus: die Buchungsregeln in den Einstellungen von Kammzeit Feld für Feld erklärt.")
	a.addPage(mux, "/hilfe/buchungsseite-gestalten", "hilfe-buchungsseite-gestalten", "Buchungsseite gestalten: Logo, Fotos, Design - Kammzeit",
		"Logo, Social-Links und Öffnungszeiten auf deiner Buchungsseite, mit Pro dazu Titelbild, Schrift, dunkles Design, Begrüßungstext und Fotogalerie. So richtest du es ein.")
	a.addPage(mux, "/hilfe/link-teilen", "hilfe-link-teilen", "Buchungslink teilen: Instagram, Google, WhatsApp - Kammzeit",
		"Deine Buchungsseite bekannt machen: Link in der Instagram-Bio, im Google-Unternehmensprofil, im WhatsApp-Status und als Button auf deiner eigenen Website.")
	a.addPage(mux, "/hilfe/termine-eintragen", "hilfe-termine-eintragen", "Telefontermine im Kalender eintragen - Kammzeit",
		"Termine, die am Telefon oder im Salon ausgemacht werden, trägst du in Kammzeit selbst ein. So ist die Zeit auch online belegt, nichts wird doppelt gebucht, und Verschieben geht mit zwei Klicks.")
	a.addPage(mux, "/hilfe/kundenkartei", "hilfe-kundenkartei", "Kundenkartei: Notizen, Historie, CSV-Export - Kammzeit",
		"Kundenkartei in Kammzeit nutzen: Kunden suchen und anlegen, Farbrezepturen notieren, Terminhistorie ansehen, Kunden als CSV exportieren oder löschen.")
	a.addPage(mux, "/hilfe/absagen-und-no-shows", "hilfe-absagen-und-no-shows", "Absagen, Erinnerungen und No-Shows - Kammzeit",
		"Wie Kunden online absagen, wie du selbst einen Termin absagst, wann die Erinnerung 24 Stunden vorher rausgeht und wie du nicht erschienene Kunden markierst.")
	a.addPage(mux, "/hilfe/warteschlange", "hilfe-warteschlange", "Warteschlange für Kunden ohne Termin - Kammzeit",
		"Laufkundschaft stellt sich per QR-Code an und sieht live die Wartezeit: Warteschlange einschalten, QR-Code drucken, Anzeige für den Laden, aufrufen und abhaken.")
	a.addPage(mux, "/hilfe/mitarbeiter-logins", "hilfe-mitarbeiter-logins", "Mitarbeiter einladen: eigene Logins fürs Team - Kammzeit",
		"Mit Pro bekommt dein Team eigene Logins: wer was sieht, wie du einlädst, wie Mitarbeiter ihre Zeiten pflegen und wie du einen Zugang wieder entfernst.")
	a.addPage(mux, "/hilfe/app-installieren", "hilfe-app-installieren", "Kammzeit als App installieren: iPhone, Android, PC - Kammzeit",
		"Das Dashboard auf Handy, Tablet oder Computer wie eine App installieren, ohne App Store. Mit Kurzbefehlen, Offline-Hinweis und ohne Kundendaten auf dem Gerät.")
	a.addPage(mux, "/hilfe/sprache", "hilfe-sprache", "Sprache, Buchungsseite auf Englisch und kammzeit.com - Kammzeit",
		"Buchungsseite auf Deutsch oder Englisch, ein Konto für kammzeit.de und kammzeit.com und in welcher Sprache Kammzeit Mails an dich und deine Kunden schickt.")
	a.addPage(mux, "/hilfe/pro-und-kuendigen", "hilfe-pro-und-kuendigen", "Pro buchen, Abo verwalten und kündigen - Kammzeit",
		"Vom Gratis-Plan zu Pro wechseln, Zahlungsart und Rechnungen im Kundenportal verwalten und das Abo kündigen: so funktioniert das Abo bei Kammzeit.")
	a.addPage(mux, "/hilfe/daten-export-und-loeschen", "hilfe-daten-export-und-loeschen", "Daten exportieren, Kammzeit-News und Konto löschen - Kammzeit",
		"Alle Daten deines Salons als JSON-Datei herunterladen, Kammzeit-News bestellen oder abbestellen und das Konto endgültig löschen. Und was danach mit den Daten passiert.")

	a.addPage(mux, "/open-source", "hilfe-open-source", "Open Source: Kammzeit unter AGPL-3.0 - Kammzeit",
		"Der Code von Kammzeit ist offen (AGPL-3.0). Was die Lizenz erlaubt und verlangt, wie du Kammzeit in fünf Schritten selbst hostest und wie du mitmachst.")
	a.addPage(mux, "/sicherheit", "hilfe-sicherheit", "Daten und Sicherheit - Kammzeit",
		"Wo die Daten deines Salons liegen, wer Zugriff hat, wie Rollen, Login, Cookies, App, Bilder, Sicherungen und Löschung funktionieren und wie du Lücken meldest.")
	a.addPage(mux, "/fragen", "hilfe-fragen", "Häufige Fragen - Kammzeit",
		"Antworten auf häufige Fragen zu Kammzeit: Preise und Kündigung, Team und Warteschlange, App und Sprachen, Buchung für Kunden, Datenschutz und Selbsthosten.")
}
