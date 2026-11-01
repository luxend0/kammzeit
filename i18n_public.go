package main

// Englische Texte und Formatierung für Login, Registrierung, Kammzeit-News und
// die öffentlichen Buchungsseiten (auth.go, signup.go, marketing.go, handlers.go,
// web/auth.html, web/signup.html, web/pages.html).

import (
	"fmt"
	"html"
	"html/template"
	"strconv"
	"time"
)

// dayFor formatiert ein Datum ohne Uhrzeit: "3.11.2026" / "3 Nov 2026".
func dayFor(lang string, t time.Time) string {
	if lang == "en" {
		return fmt.Sprintf("%d %s %d", t.Day(), monthsEN[t.Month()], t.Year())
	}
	return t.Format("2.1.2006")
}

// noticeFor: Frist in Minuten als "24 Stunden" / "24 hours" (siehe noticeText).
func noticeFor(lang string, m int) string {
	if lang != "en" {
		return noticeText(m)
	}
	unit := func(n int, one, many string) string {
		if n == 1 {
			return "1 " + one
		}
		return strconv.Itoa(n) + " " + many
	}
	switch {
	case m%1440 == 0:
		return unit(m/1440, "day", "days")
	case m%60 == 0:
		return unit(m/60, "hour", "hours")
	}
	return unit(m, "minute", "minutes")
}

func init() {
	// pubHTML übersetzt einen festen Katalogtext mit HTML (Links, <strong>) und
	// setzt Werte maskiert ein. NUR für Text-Literale aus den Templates, nie für
	// Eingaben: der Text selbst wird nicht maskiert.
	templateFuncs["pubHTML"] = func(lang any, de string, args ...any) template.HTML {
		for i, v := range args {
			args[i] = html.EscapeString(fmt.Sprint(v))
		}
		return template.HTML(fmt.Sprintf(tr(fmt.Sprint(lang), de), args...))
	}

	addEN(map[string]string{
		// Login (auth.go, web/auth.html)
		"Zum Inhalt springen":             "Skip to content",
		"Impressum":                       "Imprint",
		"Datenschutz":                     "Privacy",
		"Anmelden":                        "Log in",
		"Ungültige Herkunft der Anfrage.": "Invalid request origin.",
		"Bitte eine gültige E-Mail-Adresse angeben.":                    "Please enter a valid email address.",
		"Zu viele Versuche":                                             "Too many attempts",
		"Bitte warte ein paar Minuten und versuch es dann noch einmal.": "Please wait a few minutes and try again.",
		"Zurück zum Login":                                              "Back to login",
		"Link nicht mehr gültig":                                        "Link no longer valid",
		"Dieser Login-Link ist abgelaufen, wurde schon benutzt oder ist unvollständig. Ein Link gilt 15 Minuten und nur einmal.": "This login link has expired, has already been used or is incomplete. A link is valid for 15 minutes and works only once.",
		"Neuen Link anfordern": "Request a new link",
		"Link ist unterwegs":   "Your link is on its way",
		"Wenn es ein Konto mit dieser Adresse gibt, ist ein Login-Link unterwegs. Er ist 15 Minuten gültig.": "If there is an account with this address, a login link is on its way. It is valid for 15 minutes.",
		"Öffne den Link auf dem Gerät, auf dem du angemeldet sein willst.":                                   "Open the link on the device you want to be logged in on.",
		"Nichts angekommen? Schau im Spam-Ordner nach.":                                                      "Nothing arrived? Check your spam folder.",
		"Wir schicken dir einen Login-Link per E-Mail. Ein Passwort brauchst du nicht.":                      "We'll email you a login link. You don't need a password.",
		"Du bist abgemeldet.":    "You are logged out.",
		"E-Mail":                 "Email",
		"Login-Link schicken":    "Send login link",
		"Noch kein Konto?":       "No account yet?",
		"Kostenlos registrieren": "Sign up for free",
		"Ein Klick noch, dann bist du im Dashboard.": "One more click and you're in your dashboard.",
		"Jetzt anmelden": "Log in now",
		"Da ist etwas schiefgelaufen. Bitte später noch einmal versuchen.": "Something went wrong. Please try again later.",
		"Das Formular war unvollständig.":                                  "The form was incomplete.",
		"Diese Seite gibt es nicht.":                                       "This page does not exist.",

		// Registrierung (signup.go, web/signup.html)
		"Salon registrieren": "Sign up your salon",
		"Ersteinrichtung":    "Initial setup",
		"Salon bei Kammzeit registrieren: eigene Buchungsseite, gratis für einen Mitarbeiter, ohne Zahlungsdaten.":              "Sign up your salon with Kammzeit: your own booking page, free for one staff member, no payment details needed.",
		"Die Adresse %s ist leider vergeben. Bitte wähle eine andere.":                                                          "Sorry, the address %s is already taken. Please choose another one.",
		"Bitte den Namen des Salons angeben (2 bis 80 Zeichen).":                                                                "Please enter the name of your salon (2 to 80 characters).",
		"Die Wunsch-Adresse ist ungültig: 3 bis 40 Zeichen.":                                                                    "The address is invalid: it needs 3 to 40 characters.",
		"Die Wunsch-Adresse ist ungültig: nur a-z, 0-9 und - (nicht am Anfang oder Ende).":                                      "The address is invalid: only a-z, 0-9 and - are allowed (not at the start or end).",
		"Bitte deinen Vornamen angeben (höchstens 40 Zeichen).":                                                                 "Please enter your first name (up to 40 characters).",
		"Bitte bestätigen, dass du als Unternehmer handelst, und die AGB und den Vertrag zur Auftragsverarbeitung akzeptieren.": "Please confirm that you are acting as a business and accept the Terms and the Data Processing Agreement.",
		"Zu viele Anmeldungen von deinem Anschluss. Bitte versuche es in einer Stunde noch einmal.":                             "Too many sign-ups from your connection. Please try again in an hour.",
		"Zu viele Anmeldungen mit dieser E-Mail-Adresse. Bitte versuche es in 15 Minuten noch einmal.":                          "Too many sign-ups with this email address. Please try again in 15 minutes.",
		"Haarschnitt":                "Haircut",
		"Waschen, Schneiden, Föhnen": "Wash, cut and blow-dry",
		"Fast geschafft":             "Almost there",
		"Lege deinen Salon an. Danach kommst du per Link aus einer E-Mail in dein Dashboard.": "Create your salon. A link in an email then takes you to your dashboard.",
		"Gratis starten, ohne Zahlungsdaten.":                                                 "Start for free, no payment details needed.",
		"Bitte die markierten Felder prüfen.":                                                 "Please check the highlighted fields.",
		"Dein Salon":                                                                          "Your salon",
		"Salonname":                                                                           "Salon name",
		"Über dich":                                                                           "About you",
		"Dein Vorname":                                                                        "Your first name",
		"Unter diesem Namen buchen Kunden bei dir. Weitere Mitarbeiter legst du später an.": "Customers book with you under this name. You can add more staff later.",
		"Kein Passwort: Du meldest dich immer über einen Link per E-Mail an.":               "No password: you always log in with a link sent by email.",
		`Ich melde den Salon als Unternehmer an (§ 14 BGB) und akzeptiere die <a href="/agb" target="_blank" rel="noopener">AGB</a> und den <a href="/avv" target="_blank" rel="noopener">Vertrag zur Auftragsverarbeitung</a>.`: `I am signing up the salon as a business (§ 14 BGB, German Civil Code) and accept the <a href="/terms" target="_blank" rel="noopener">Terms</a> and the <a href="/dpa" target="_blank" rel="noopener">Data Processing Agreement</a>.`,
		marketingConsentDE:             "I'd like to receive occasional news about Kammzeit by email. I can unsubscribe at any time.",
		"(freiwillig)":                 "(optional)",
		"Homepage (bitte leer lassen)": "Homepage (please leave empty)",
		"Salon anlegen":                "Create salon",
		"Schon registriert?":           "Already signed up?",
		"Was du bekommst":              "What you get",
		"Deine Buchungsseite":          "Your booking page",
		"unter deinsalon.%s, Kunden buchen rund um die Uhr selbst": "at yoursalon.%s, where customers book around the clock",
		"Erinnerung per Mail":                                              "Email reminders",
		"an deine Kunden, 24 Stunden vor dem Termin":                       "for your customers, 24 hours before the appointment",
		"Kundenkartei":                                                     "Customer records",
		"mit Notizen und Terminhistorie":                                   "with notes and appointment history",
		"Gratis für 1 Mitarbeiter":                                         "Free for 1 staff member",
		", ohne Zeitlimit und ohne Zahlungsdaten":                          ", with no time limit and no payment details",
		"Wunsch-Adresse deiner Buchungsseite":                              "Your booking page address",
		"Adresse ist frei":                                                 "Address is available",
		"Adresse ist vergeben":                                             "Address is taken",
		"Adresse ungültig: 3 bis 40 Zeichen":                               "Invalid address: 3 to 40 characters",
		"Adresse ungültig: nur a-z, 0-9 und - (nicht am Anfang oder Ende)": "Invalid address: only a-z, 0-9 and - (not at the start or end)",
		"Wir haben dir einen Link an <strong>%s</strong> geschickt. Damit kommst du in dein Dashboard.":                             "We've sent a link to <strong>%s</strong>. It takes you to your dashboard.",
		"Der Link ist 15 Minuten gültig. Schau auch im Spam-Ordner nach.":                                                           "The link is valid for 15 minutes. Check your spam folder too.",
		"Du hast Kammzeit-News angehakt: Dazu kommt eine zweite Mail. Erst wenn du darin bestätigst, schicken wir dir Neuigkeiten.": "You ticked Kammzeit news: a second email is on its way. We'll only send you news once you confirm in that email.",
		"Kein Link angekommen?": "No link arrived?",
		"Hier neu anfordern":    "Request a new one here",

		// Kammzeit-News (marketing.go)
		"Dieser Bestätigungslink ist abgelaufen, wurde schon benutzt oder ist unvollständig. Ein Link gilt 7 Tage. In den Einstellungen kannst du einen neuen anfordern.": "This confirmation link has expired, has already been used or is incomplete. A link is valid for 7 days. You can request a new one in Settings.",
		"Bitte bestätigen: Kammzeit-News per E-Mail": "Please confirm: Kammzeit news by email",
		marketingMailDE: "Hello,\n\nyou'd like to receive occasional news about Kammzeit by email. Please confirm with this link (valid for 7 days):\n\n%s\n\n" +
			"Didn't ask for this? Just ignore this email and we won't send you any news.\n",
		"Zu den Einstellungen":     "Go to Settings",
		"Kammzeit-News bestätigen": "Confirm Kammzeit news",
		"Bestätige, dass du gelegentlich Neuigkeiten zu Kammzeit per E-Mail bekommen möchtest. Abbestellen geht jederzeit mit einem Klick.": "Please confirm that you'd like to receive occasional news about Kammzeit by email. You can unsubscribe at any time with one click.",
		"Ja, Kammzeit-News bestätigen": "Yes, confirm Kammzeit news",
		"Danke, bestätigt":             "Thanks, confirmed",
		"Du bekommst ab jetzt gelegentlich Kammzeit-News per E-Mail. In jeder Mail steht ein Link zum Abbestellen, und in den Einstellungen kannst du sie jederzeit abschalten.": "From now on you'll receive occasional Kammzeit news by email. Every email contains an unsubscribe link, and you can turn the news off in Settings at any time.",
		"Zum Dashboard": "Go to dashboard",
		"Link ungültig": "Invalid link",
		"Dieser Abmeldelink ist ungültig. Du kannst Kammzeit-News in den Einstellungen abbestellen.": "This unsubscribe link is invalid. You can unsubscribe from Kammzeit news in Settings.",
		"Kammzeit-News abbestellen": "Unsubscribe from Kammzeit news",
		"Du bekommst dann keine Neuigkeiten zu Kammzeit mehr per E-Mail. Mails zu deinem Konto (Login-Links, Rechnungen, wichtige Hinweise) kommen weiter.": "You will no longer receive news about Kammzeit by email. Emails about your account (login links, invoices, important notices) will still arrive.",
		"Abbestellen": "Unsubscribe",
		"Abbestellt":  "Unsubscribed",
		"Du bekommst keine Kammzeit-News mehr. In den Einstellungen kannst du sie jederzeit wieder einschalten.": "You won't receive any more Kammzeit news. You can turn it back on in Settings at any time.",

		// Buchungsseite, Storno, Bestätigung (handlers.go, web/pages.html)
		"Hinweis":                     "Notice",
		"Zurück zur Buchung":          "Back to booking",
		"Diesen Salon gibt es nicht.": "This salon does not exist.",
		// "Termin buchen" steht in i18n_dash1.go ("Book appointment")
		"Online-Termine mit": "Online booking by",
		"AGB":                "Terms",
		"AVV":                "DPA",
		"1. Leistung":        "1. Service",
		"%d Min.":            "%d min",
		"2. Bei wem?":        "2. With whom?",
		"3. Tag und Uhrzeit": "3. Date and time",
		"Tag":                "Day",
		"Freie Uhrzeiten":    "Available times",
		"Für die Uhrzeitauswahl bitte JavaScript aktivieren oder im Salon anrufen.": "To choose a time, please enable JavaScript or call the salon.",
		"4. Deine Daten":           "4. Your details",
		"Telefon":                  "Phone",
		"Firma, bitte leer lassen": "Company, please leave empty",
		"Verbindlich buchen":       "Confirm booking",
		"Du bekommst eine Bestätigung per E-Mail mit Kalendereintrag und Link zum Absagen.": "You'll receive a confirmation email with a calendar entry and a link to cancel.",
		"Online absagen kannst du bis %s vor dem Termin.":                                   "You can cancel online up to %s before the appointment.",
		"Hinweise zum Datenschutz":                                                          "Privacy information",
		"Für diesen Salon sind noch keine Termine online buchbar.":                          "This salon doesn't offer online booking yet.",
		"Ruf gern an:":       "Feel free to call:",
		"Egal, wer frei ist": "Anyone available",
		"Diese Leistung bietet gerade niemand an.":                                                                                 "Nobody offers this service at the moment.",
		"Zu viele Anfragen. Bitte warte kurz und versuch es dann noch einmal.":                                                     "Too many requests. Please wait a moment and try again.",
		"Bitte Leistung und Tag wählen.":                                                                                           "Please choose a service and a day.",
		"Bitte einen Tag ab heute wählen.":                                                                                         "Please choose a day from today onwards.",
		"Online buchbar bis %s. Für spätere Termine ruf bitte im Salon an.":                                                        "Online booking is available until %s. For later appointments, please call the salon.",
		"An diesem Tag ist nichts mehr frei. Bitte wähl einen anderen Tag.":                                                        "Nothing is available on this day. Please choose another day.",
		"Zu viele Buchungen von deinem Anschluss. Bitte versuch es später noch einmal oder ruf im Salon an.":                       "Too many bookings from your connection. Please try again later or call the salon.",
		"Bitte gib deinen Namen an.":                                                                                               "Please enter your name.",
		"Bitte gib Name und Telefonnummer ohne Zeilenumbrüche oder Tabulatoren an.":                                                "Please enter your name and phone number without line breaks or tabs.",
		"Bitte gib eine gültige E-Mail-Adresse an.":                                                                                "Please enter a valid email address.",
		"Die Telefonnummer ist zu lang.":                                                                                           "The phone number is too long.",
		"Bitte wähl eine Uhrzeit.":                                                                                                 "Please choose a time.",
		"Diese Leistung oder diesen Mitarbeiter gibt es so nicht mehr. Bitte wähl neu.":                                            "This service or staff member is no longer available. Please choose again.",
		"Dieser Termin ist leider nicht mehr frei. Bitte wähl eine andere Uhrzeit.":                                                "Sorry, this time is no longer available. Please choose another time.",
		"Mit dieser E-Mail-Adresse wurde gerade schon mehrmals gebucht. Bitte versuch es später noch einmal oder ruf im Salon an.": "Several bookings have just been made with this email address. Please try again later or call the salon.",
		"Gerade gehen sehr viele Buchungen ein. Bitte versuch es später noch einmal oder ruf im Salon an.":                         "We are receiving a lot of bookings right now. Please try again later or call the salon.",
		"Termin gebucht":                      "Appointment booked",
		"Danke, %s! Wir freuen uns auf dich.": "Thank you, %s! We look forward to seeing you.",
		"Danke! Wir freuen uns auf dich.":     "Thank you! We look forward to seeing you.",
		"Wann":                                "When",
		"Leistung":                            "Service",
		"Bei":                                 "With",
		"Wo":                                  "Where",
		"Die Bestätigung geht an <strong>%s</strong>.": "The confirmation is on its way to <strong>%s</strong>.",
		"In meinen Kalender eintragen":                 "Add to my calendar",
		"Du kannst doch nicht?":                        "Can't make it after all?",
		"Termin absagen":                               "Cancel appointment",
		"(online bis %s)":                              "(online until %s)",
		"Dieser Termin ist bereits abgesagt.":          "This appointment has already been cancelled.",
		"Dieser Termin liegt in der Vergangenheit.":    "This appointment is in the past.",
		"Online-Absage war bis %s möglich.":            "Online cancellation was possible until %s.",
		"Bitte ruf den Salon an:":                      "Please call the salon:",
		"Bitte melde dich direkt beim Salon.":          "Please contact the salon directly.",
		"Online-Absage bis %s möglich.":                "You can cancel online until %s.",
		"Termin abgesagt":                              "Appointment cancelled",
		"Danke für die Absage. %s ist informiert.":     "Thanks for letting us know. %s has been notified.",
		"Neuen Termin buchen":                          "Book a new appointment",
		"Diesen Termin gibt es nicht.":                 "This appointment does not exist.",
		"Diesen Termin gibt es nicht (mehr).":          "This appointment does not exist (any more).",
		// Kalendereintrag (.ics)
		"Bei %s":  "With %s",
		"Absagen": "Cancel",
		// eigener Schlüssel (gleiche deutsche Ausgabe), "%s bei %s" ist im Dashboard "Leistung bei Mitarbeiter"
		"%[1]s bei %[2]s": "%[1]s at %[2]s",
	})
}
