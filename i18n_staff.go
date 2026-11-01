package main

// Englische Texte für Mitarbeiter-Logins (staff.go, web/dash_staff.html, Menü in web/dash.html).

func init() {
	addEN(map[string]string{
		// Rechte und Pause
		"Nur für Inhaber": "Owners only",
		"Das darf nur die Inhaberin oder der Inhaber": "Only the salon owner can do this",
		"Abo, Einstellungen, Leistungen und Team verwaltet die Inhaberin oder der Inhaber des Salons. Du kannst Termine eintragen, Kunden ansehen und deine Arbeitszeiten pflegen.": "The salon owner manages the plan, settings, services and team. You can add appointments, view customers and keep your working hours up to date.",
		"Zum Kalender":    "Go to calendar",
		"Zugang pausiert": "Access paused",
		"Dein Mitarbeiter-Zugang ist gerade pausiert: Der Salon hat kein Pro-Abo mehr, oder du bist im Team nicht aktiv. Gelöscht ist nichts. Sprich mit der Inhaberin oder dem Inhaber deines Salons.": "Your staff access is paused right now: the salon no longer has a Pro plan, or you are not active in the team. Nothing has been deleted. Please talk to the owner of your salon.",

		// Menü und Meine Zeiten
		"Meine Zeiten": "My hours",
		"Wann du arbeitest. Online buchbar bist du nur in diesen Zeiten.": "When you work. You can only be booked online during these hours.",
		"Arbeitszeiten gespeichert.":                                      "Working hours saved.",

		// Team-Liste und Karte "Login fürs Dashboard"
		"Login pausiert":            "Login paused",
		"Login aktiv":               "Login active",
		"eingeladen":                "invited",
		"kein Login":                "no login",
		"Login fürs Dashboard":      "Dashboard login",
		"Zuletzt angemeldet: %s.":   "Last login: %s.",
		"Noch nicht angemeldet.":    "Not logged in yet.",
		"Einladung erneut schicken": "Resend invitation",
		"Zugang entfernen":          "Remove access",
		"Zugang einladen":           "Send invitation",
		"Inaktive Mitarbeiter können sich nicht anmelden.":                                                   "Inactive staff members can't log in.",
		"Mitarbeiter-Logins gibt es im Pro-Plan.":                                                            "Staff logins are part of the Pro plan.",
		"Mitarbeiter-Logins gibt es nur im Pro-Plan. Der Zugang bleibt gespeichert und geht mit Pro wieder.": "Staff logins are only available on the Pro plan. The access stays saved and works again with Pro.",
		"Mit eigenem Login sieht dein Mitarbeiter Kalender und Kunden, trägt Termine ein und pflegt seine Arbeitszeiten selbst. Abo, Einstellungen, Leistungen und Team bleiben bei dir.": "With their own login, your staff member sees the calendar and customers, adds appointments and keeps their own working hours up to date. Plan, settings, services and team stay with you.",
		"Wir schicken eine Einladung mit Login-Link. Ein Passwort braucht niemand.":                                                                                                       "We'll send an invitation with a login link. Nobody needs a password.",
		"Mitarbeiter-Logins gibt es im Pro-Plan: Dein Team sieht Kalender und Kunden und pflegt die eigenen Arbeitszeiten.":                                                               "Staff logins come with the Pro plan: your team sees the calendar and customers and keeps their own working hours up to date.",

		// Meldungen nach Einladen/Entfernen
		"Einladung verschickt. Der Login-Link darin gilt 15 Minuten, danach meldet sich dein Mitarbeiter einfach über die Login-Seite an.": "Invitation sent. The login link in it is valid for 15 minutes; after that your staff member simply logs in via the login page.",
		"Einladung noch einmal verschickt.":                             "Invitation sent again.",
		"Zugang entfernt. Alle Anmeldungen dieses Logins sind beendet.": "Access removed. All sessions of this login have been ended.",
		"Diese E-Mail-Adresse hat schon ein Kammzeit-Konto. Jede Adresse kann nur zu einem Login gehören, bitte nimm eine andere.":                "This email address already has a Kammzeit account. Each address can only belong to one login, please use a different one.",
		"Inaktive Mitarbeiter können sich nicht anmelden. Stell den Mitarbeiter zuerst auf aktiv und speichere.":                                  "Inactive staff members can't log in. Set the staff member to active first and save.",
		"Zu viele Einladungen in kurzer Zeit. Bitte versuch es in einer Stunde noch einmal.":                                                      "Too many invitations in a short time. Please try again in an hour.",
		"Die Einladung konnte nicht verschickt werden. Der Zugang ist angelegt, versuch es gleich noch einmal mit \"Einladung erneut schicken\".": "The invitation could not be sent. The access has been created, please try again shortly with \"Resend invitation\".",
	})
}
