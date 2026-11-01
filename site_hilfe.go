package main

// Hilfe-Center, Open Source, Sicherheit, Fragen. Registrierung per a.addPage (site.go),
// Inhalte in web/site_hilfe.html (Templates "hilfe-..."), Gestaltung in web/static/site-hilfe.css.

import "net/http"

func (a *app) routesSiteHilfe(mux *http.ServeMux) {
	a.addPage(mux, "/hilfe", "hilfe-index", "Hilfe-Center - Kammzeit",
		"Anleitungen für Kammzeit mit den echten Menüpunkten: Salon einrichten, Leistungen und Arbeitszeiten, Termine eintragen, Kundenkartei, Abo und Daten.")

	// Anleitungen, Reihenfolge wie auf /hilfe
	a.addPage(mux, "/hilfe/erste-schritte", "hilfe-erste-schritte", "Erste Schritte: Salon anlegen und einrichten - Kammzeit",
		"Salon bei Kammzeit registrieren, per Login-Link anmelden und die Buchungsseite in etwa zehn Minuten einrichten, mit allen Menüpunkten und Buttons.")
	a.addPage(mux, "/hilfe/leistungen", "hilfe-leistungen", "Leistungen anlegen: Dauer, Preis, Reihenfolge - Kammzeit",
		"So legst du in Kammzeit Leistungen mit Dauer, Preis und Beschreibung an, sortierst sie für die Buchungsseite und nimmst sie vorübergehend aus dem Angebot.")
	a.addPage(mux, "/hilfe/arbeitszeiten", "hilfe-arbeitszeiten", "Team, Arbeitszeiten und Pausen - Kammzeit",
		"Mitarbeiter anlegen, Leistungen zuordnen und Arbeitszeiten mit Mittagspause eintragen. Dazu, wie du Urlaub löst, solange es keine Abwesenheiten gibt.")
	a.addPage(mux, "/hilfe/buchungsregeln", "hilfe-buchungsregeln", "Buchungsregeln: Vorlauf, Absagefrist, Zeitraum - Kammzeit",
		"Wie kurzfristig Kunden online buchen und absagen dürfen und wie weit im Voraus: die Buchungsregeln in den Einstellungen von Kammzeit Feld für Feld erklärt.")
	a.addPage(mux, "/hilfe/link-teilen", "hilfe-link-teilen", "Buchungslink teilen: Instagram, Google, WhatsApp - Kammzeit",
		"Deine Buchungsseite bekannt machen: Link in der Instagram-Bio, im Google-Unternehmensprofil, im WhatsApp-Status und als Button auf deiner eigenen Website.")
	a.addPage(mux, "/hilfe/termine-eintragen", "hilfe-termine-eintragen", "Telefontermine im Kalender eintragen - Kammzeit",
		"Termine, die am Telefon oder im Salon ausgemacht werden, trägst du in Kammzeit selbst ein. So ist die Zeit auch online belegt und nichts wird doppelt gebucht.")
	a.addPage(mux, "/hilfe/kundenkartei", "hilfe-kundenkartei", "Kundenkartei: Notizen, Historie, CSV-Export - Kammzeit",
		"Kundenkartei in Kammzeit nutzen: Kunden suchen und anlegen, Farbrezepturen notieren, Terminhistorie ansehen, Kunden als CSV exportieren oder löschen.")
	a.addPage(mux, "/hilfe/absagen-und-no-shows", "hilfe-absagen-und-no-shows", "Absagen, Erinnerungen und No-Shows - Kammzeit",
		"Wie Kunden online absagen, wie du selbst einen Termin absagst, wann die Erinnerung 24 Stunden vorher rausgeht und wie du nicht erschienene Kunden markierst.")
	a.addPage(mux, "/hilfe/pro-und-kuendigen", "hilfe-pro-und-kuendigen", "Pro buchen, Abo verwalten und kündigen - Kammzeit",
		"Vom Gratis-Plan zu Pro wechseln, Zahlungsart und Rechnungen im Kundenportal verwalten und das Abo kündigen: so funktioniert das Abo bei Kammzeit.")
	a.addPage(mux, "/hilfe/daten-export-und-loeschen", "hilfe-daten-export-und-loeschen", "Daten exportieren und Konto löschen - Kammzeit",
		"Alle Daten deines Salons als JSON-Datei herunterladen, Kunden als CSV exportieren und das Konto endgültig löschen. Und was danach mit den Daten passiert.")

	a.addPage(mux, "/open-source", "hilfe-open-source", "Open Source: Kammzeit unter AGPL-3.0 - Kammzeit",
		"Der Code von Kammzeit ist offen (AGPL-3.0). Was die Lizenz erlaubt und verlangt, wie du Kammzeit in fünf Schritten selbst hostest und wie du mitmachst.")
	a.addPage(mux, "/sicherheit", "hilfe-sicherheit", "Daten und Sicherheit - Kammzeit",
		"Wo die Daten deines Salons liegen, wer Zugriff hat, wie Login, Verschlüsselung, Sicherungen und Löschung funktionieren und wie du Sicherheitslücken meldest.")
	a.addPage(mux, "/fragen", "hilfe-fragen", "Häufige Fragen - Kammzeit",
		"Antworten auf häufige Fragen zu Kammzeit: Preise und Kündigung, Buchung für deine Kunden, Datenschutz und Datenstandort, Technik und Selbsthosten.")
}
