package main

// Marketing-Einwilligung der Salon-Inhaber (Kammzeit-News per Mail).
// Freiwillig, Double-Opt-in, Nachweis in users.marketing_* (Migration 2),
// Widerruf jederzeit (Einstellungen oder Abmeldelink in jeder Marketing-Mail).
// Die Funktionen füllt der Bereich Registrierung/Login (Signaturen nicht ändern):
// Registrierung und Einstellungen rufen sie auf.

// startMarketingDOI merkt eine Einwilligung als ausstehend vor und schickt die
// Bestätigungsmail (Link 7 Tage gültig). source: "registrierung" | "einstellungen".
func (a *app) startMarketingDOI(userID int64, source string) error { return nil }

// withdrawMarketing widerruft eine Einwilligung (oder eine ausstehende).
func (a *app) withdrawMarketing(userID int64, source string) error { return nil }

// marketingState liefert den Stand für die Anzeige: "aus", "ausstehend" oder "an".
func (a *app) marketingState(userID int64) string { return "aus" }
