package main

// Registrierung neuer Salons: /registrieren (kammzeit.de), /signup (kammzeit.com).

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"net/mail"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

func (a *app) routesSignup(mux *http.ServeMux) {
	// Dieselben Handler unter beiden Pfaden. Die Seite selbst leitet auf den Pfad
	// der Domain-Sprache um (signupPage). ponytail: Live-Prüfung und Absenden
	// nehmen beide Pfade an, damit ein schon offenes Formular weiter funktioniert.
	for _, p := range []string{"/registrieren", "/signup"} {
		mux.HandleFunc("GET "+p, a.signupPage)
		mux.HandleFunc("GET "+p+"/adresse", a.signupSlug)
		mux.HandleFunc("POST "+p, a.signup)
	}
	a.routesMarketing(mux) // marketing.go: /newsletter/...
}

// signupPath = Pfad der Registrierung in einer Sprache.
func signupPath(lang string) string {
	if lang == "en" {
		return "/signup"
	}
	return "/registrieren"
}

// Adressen, die kein Salon bekommen darf (Technik, eigene Seiten, Verwechslung).
var reservedSlugs = []string{"www", "mail", "smtp", "imap", "pop", "mx", "ns1", "ns2", "admin", "api", "app",
	"dashboard", "login", "logout", "static", "assets", "cdn", "demo", "kammzeit", "hilfe", "help", "support",
	"status", "blog", "docs", "test", "dev", "staging", "billing", "stripe", "abo", "konto", "registrieren", "signup",
	"impressum", "datenschutz", "agb", "avv", "autoconfig", "autodiscover", "_dmarc", "mta-sts", "webmail"}

// ponytail: im Speicher wie alle Limiter, zählt nur gültige Anmeldungen (die Mails auslösen).
// signupMailLimit begrenzt die Login-Mails je Empfänger, wie loginMailLimit in auth.go.
var (
	signupLimit     = newLimiter(5, time.Hour)
	signupMailLimit = newLimiter(3, 15*time.Minute)
)

// slugProblem prüft nur die Form einer Wunsch-Adresse. "" = in Ordnung.
// Die Texte sind zugleich Statuswerte (deutsch) und Übersetzungsschlüssel.
func slugProblem(slug string) string {
	switch {
	case len(slug) < 3 || len(slug) > 40:
		return "ungültig: 3 bis 40 Zeichen"
	case !slugRe.MatchString(slug):
		return "ungültig: nur a-z, 0-9 und - (nicht am Anfang oder Ende)"
	case slices.Contains(reservedSlugs, slug):
		return "vergeben"
	}
	return ""
}

// slugStatus ist die Antwort der Live-Prüfung: "frei", "vergeben" oder "ungültig: ...".
func slugStatus(q querier, slug string) (string, error) {
	if p := slugProblem(slug); p != "" {
		return p, nil
	}
	err := q.QueryRow(`SELECT 1 FROM salons WHERE slug = ?`, slug).Scan(new(int))
	if errors.Is(err, sql.ErrNoRows) {
		return "frei", nil
	}
	if err != nil {
		return "", err
	}
	return "vergeben", nil
}

var slugChars = strings.NewReplacer("ä", "ae", "ö", "oe", "ü", "ue", "ß", "ss", "é", "e", "è", "e", "ê", "e",
	"á", "a", "à", "a", "â", "a", "ó", "o", "ò", "o", "ô", "o", "í", "i", "ì", "i", "î", "i", "ú", "u", "ù", "u",
	"û", "u", "ç", "c", "ñ", "n")

// suggestSlug macht aus dem Salonnamen eine Adresse: "Salon Müller" -> "salon-mueller".
func suggestSlug(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range slugChars.Replace(strings.ToLower(name)) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case b.Len() > 0 && !dash:
			b.WriteByte('-')
			dash = true
		}
	}
	s := b.String()
	if len(s) > 40 {
		s = s[:40]
	}
	return strings.TrimRight(s, "-")
}

// signupOpen: Registrierung nur auf der Hauptdomain; im Einzelbetrieb nur zur
// Ersteinrichtung, solange es noch keinen Salon gibt. Antwortet sonst selbst.
func (a *app) signupOpen(w http.ResponseWriter, r *http.Request) bool {
	if !a.onRootHost(r) {
		http.Redirect(w, r, a.siteURLFor(r)+signupPath(a.langFor(r)), http.StatusFound)
		return false
	}
	if a.cfg.SingleTenant {
		exists, err := signupHasSalon(a.db)
		if err != nil {
			a.authServerError(w, r, err)
			return false
		}
		if exists {
			a.authFail(w, r, http.StatusNotFound, "Diese Seite gibt es nicht.")
			return false
		}
	}
	return true
}

// signupDomain = Hauptdomain der Anfrage. Der Salon bekommt deren Sprache, und
// seine Buchungsseite läuft unter <slug>.<Domain seiner Sprache>.
func (a *app) signupDomain(r *http.Request) string {
	if _, _, d := a.tenant(r.Host); d.Host != "" {
		return d.Host
	}
	return a.cfg.BaseDomain
}

func signupHasSalon(q querier) (bool, error) {
	var n int
	err := q.QueryRow(`SELECT EXISTS (SELECT 1 FROM salons)`).Scan(&n)
	return n == 1, err
}

// signupPage zeigt das Formular. Falscher Pfad für die Domain (/signup auf
// kammzeit.de, /registrieren auf kammzeit.com): dauerhaft auf den richtigen.
func (a *app) signupPage(w http.ResponseWriter, r *http.Request) {
	if !a.signupOpen(w, r) {
		return
	}
	if p := signupPath(a.langFor(r)); r.URL.Path != p {
		if r.URL.RawQuery != "" {
			p += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, p, http.StatusMovedPermanently)
		return
	}
	a.signupForm(w, r, http.StatusOK, nil, "")
}

// signupForm zeigt das Formular, nach einem Fehler mit den Eingaben von eben.
// errs = Fehler am Feld (name, slug, vorname, email, agb), msg = Hinweis ohne
// Feldbezug (beides schon übersetzt). Das Häkchen für Kammzeit-News ist nie
// vorausgewählt; nur nach einem Fehler steht es so, wie es abgeschickt wurde.
func (a *app) signupForm(w http.ResponseWriter, r *http.Request, status int, errs map[string]string, msg string) {
	title := "Salon registrieren"
	if a.cfg.SingleTenant {
		title = "Ersteinrichtung"
	}
	lang := a.langFor(r)
	path := signupPath(lang)
	d := a.authPage(r, title, path)
	d["SignupPath"] = path
	if !a.cfg.SingleTenant { // steht in der Sitemap, darf in die Suche
		d["Index"], d["Canonical"] = true, a.siteURLFor(r)+path
		d["Description"] = tr(lang, "Salon bei Kammzeit registrieren: eigene Buchungsseite, gratis für einen Mitarbeiter, ohne Zahlungsdaten.")
		alt := []alternate{{lang, a.siteURLLang(lang) + path}}
		if other, otherLang := a.otherSite(lang); other != "" {
			alt = append(alt, alternate{otherLang, other + signupPath(otherLang)})
		}
		d["Alternates"] = alt
	}
	d["Heading"], d["Error"], d["Errors"], d["Domain"] = tr(lang, title), msg, errs, a.signupDomain(r)
	d["Name"], d["Slug"], d["SlugAuto"] = r.PostFormValue("name"), r.PostFormValue("slug"), r.PostFormValue("slug_auto")
	d["Vorname"], d["Email"], d["AGB"] = r.PostFormValue("vorname"), r.PostFormValue("email"), r.PostFormValue("agb") == "1"
	d["Marketing"], d["MarketingText"] = r.PostFormValue("marketing") == "1", tr(lang, marketingConsentDE)
	a.render(w, status, "signup", d)
}

// signupSlug beantwortet die htmx-Prüfung der Wunsch-Adresse. Kommt die Anfrage
// aus dem Namensfeld (Parameter name), wird die Adresse neu vorgeschlagen,
// solange sie leer ist oder noch dem letzten Vorschlag (slug_auto) entspricht.
func (a *app) signupSlug(w http.ResponseWriter, r *http.Request) {
	if !a.signupOpen(w, r) {
		return
	}
	q := r.URL.Query()
	slug := strings.ToLower(strings.TrimSpace(q.Get("slug")))
	lang := a.langFor(r)
	d := map[string]any{"Domain": a.signupDomain(r), "Lang": lang, "SignupPath": signupPath(lang)}
	tpl := "signup-status"
	if q.Has("name") {
		auto := q.Get("slug_auto")
		if slug == "" || slug == auto {
			slug = suggestSlug(q.Get("name"))
			auto = slug
		}
		d["Slug"], d["SlugAuto"] = slug, auto
		tpl = "signup-slug"
	}
	if slug != "" {
		st, err := slugStatus(a.db, slug)
		if err != nil {
			a.authServerError(w, r, err)
			return
		}
		d["Status"] = st
	}
	a.render(w, http.StatusOK, tpl, d)
}

func (a *app) signup(w http.ResponseWriter, r *http.Request) {
	if !a.signupOpen(w, r) {
		return
	}
	if !sameOrigin(r) {
		a.badOrigin(w, r)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := r.ParseForm(); err != nil {
		a.authFail(w, r, http.StatusBadRequest, "Das Formular war unvollständig.")
		return
	}
	lang := a.langFor(r)
	name := strings.TrimSpace(r.PostFormValue("name"))
	first := strings.TrimSpace(r.PostFormValue("vorname"))
	slug := strings.ToLower(strings.TrimSpace(r.PostFormValue("slug")))
	if a.cfg.SingleTenant { // keine Subdomains, die Adresse sieht niemand
		if slug = suggestSlug(name); slugProblem(slug) != "" {
			slug = "salon"
		}
	}
	addr, mailErr := mail.ParseAddress(strings.TrimSpace(r.PostFormValue("email")))
	n, fn, sp := utf8.RuneCountInString(name), utf8.RuneCountInString(first), slugProblem(slug)
	taken := trf(lang, "Die Adresse %s ist leider vergeben. Bitte wähle eine andere.", slug+"."+a.signupDomain(r))
	errs := map[string]string{}
	if n < 2 || n > 80 || strings.ContainsFunc(name, unicode.IsControl) {
		errs["name"] = tr(lang, "Bitte den Namen des Salons angeben (2 bis 80 Zeichen).")
	}
	switch {
	case sp == "vergeben":
		errs["slug"] = taken
	case sp != "":
		errs["slug"] = tr(lang, "Die Wunsch-Adresse ist "+sp+".")
	}
	if fn < 1 || fn > 40 || strings.ContainsFunc(first, unicode.IsControl) {
		errs["vorname"] = tr(lang, "Bitte deinen Vornamen angeben (höchstens 40 Zeichen).")
	}
	if mailErr != nil || len(addr.Address) > 200 {
		errs["email"] = tr(lang, "Bitte eine gültige E-Mail-Adresse angeben.")
	}
	if !a.cfg.SingleTenant && r.PostFormValue("agb") != "1" {
		errs["agb"] = tr(lang, "Bitte bestätigen, dass du als Unternehmer handelst, und die AGB und den Vertrag zur Auftragsverarbeitung akzeptieren.")
	}
	if len(errs) > 0 {
		a.signupForm(w, r, http.StatusBadRequest, errs, "")
		return
	}
	email := strings.ToLower(addr.Address)
	// Kammzeit-News: freiwillig, aktiv erst nach Bestätigung per Mail (Double-Opt-in).
	marketing := !a.cfg.SingleTenant && r.PostFormValue("marketing") == "1"
	if r.PostFormValue("homepage") != "" { // Honeypot: Bots bekommen dieselbe Antwort, angelegt wird nichts
		a.signupDone(w, r, email, false, marketing)
		return
	}
	if !signupLimit.allow(clientIP(r)) {
		a.signupForm(w, r, http.StatusTooManyRequests, nil, tr(lang, "Zu viele Anmeldungen von deinem Anschluss. Bitte versuche es in einer Stunde noch einmal."))
		return
	}
	if !signupMailLimit.allow(email) { // vor jeder Abfrage: verrät nicht, ob es das Konto gibt
		a.signupForm(w, r, http.StatusTooManyRequests, nil, tr(lang, "Zu viele Anmeldungen mit dieser E-Mail-Adresse. Bitte versuche es in 15 Minuten noch einmal."))
		return
	}

	userID, created, isTaken, err := a.signupCreate(name, first, slug, email, lang)
	if errors.Is(err, errSignupClosed) {
		a.authFail(w, r, http.StatusNotFound, "Diese Seite gibt es nicht.")
		return
	}
	if err != nil {
		a.authServerError(w, r, err)
		return
	}
	if isTaken {
		a.signupForm(w, r, http.StatusConflict, map[string]string{"slug": taken}, "")
		return
	}
	// Neues und schon bestehendes Konto bekommen dieselbe Seite und denselben
	// Link, damit das Formular nicht verrät, welche Adressen registriert sind.
	// Bestehendes Konto: wie beim Login geht der Link auf die Domain dieser Anfrage.
	if !created {
		if _, err := a.db.Exec(`UPDATE users SET lang = ? WHERE id = ?`, lang, userID); err != nil {
			log.Printf("Registrierung: Sprache für Konto %d: %v", userID, err)
		}
	}
	sendErr := a.sendLoginLink(userID, email)
	if sendErr != nil {
		log.Printf("Registrierung: Login-Link an %s: %v", email, sendErr)
	}
	// Nur für neue Konten: ein bestehendes ändert niemand ohne Anmeldung. Im
	// Hintergrund, damit die Antwortzeit nicht verrät, ob das Konto neu ist.
	if created && marketing {
		go func() {
			if err := a.startMarketingDOI(userID, "registrierung"); err != nil {
				log.Printf("Registrierung: Kammzeit-News für Konto %d: %v", userID, err)
			}
		}()
	}
	a.signupDone(w, r, email, sendErr != nil, marketing)
}

var errSignupClosed = errors.New("Registrierung geschlossen")

// signupCreate legt in einer Transaktion Salon, Konto, einen Mitarbeiter (first =
// Vorname der Inhaberin) mit Arbeitszeiten und zwei Beispiel-Leistungen an und
// liefert die Konto-ID. Die Beispiel-Leistungen sind inaktiv: online buchbar
// wird erst, was der Salon selbst geprüft und eingeschaltet hat.
// lang = Sprache der Domain: gilt für Buchungsseite, Konto und Beispiel-Leistungen.
// Gibt es das Konto schon, wird nichts angelegt und dessen ID geliefert
// (created = false). taken = die Adresse ist vergeben, immer, auch für das
// eigene Konto: sonst verriete die Antwort, welche E-Mail zu welchem Salon
// gehört. Wer schon registriert ist, meldet sich über /login an.
func (a *app) signupCreate(name, first, slug, email, lang string) (userID int64, created, taken bool, err error) {
	tx, err := a.db.Begin()
	if err != nil {
		return 0, false, false, err
	}
	defer tx.Rollback()
	if a.cfg.SingleTenant { // erneut in der Transaktion: genau ein Salon
		exists, err := signupHasSalon(tx)
		if err != nil {
			return 0, false, false, err
		}
		if exists {
			return 0, false, false, errSignupClosed
		}
	}
	st, err := slugStatus(tx, slug)
	if err != nil {
		return 0, false, false, err
	}
	if st != "frei" {
		return 0, false, true, nil
	}
	err = tx.QueryRow(`SELECT id FROM users WHERE email = ?`, email).Scan(&userID)
	if !errors.Is(err, sql.ErrNoRows) {
		return userID, false, false, err // Konto gibt es schon: nichts anlegen
	}

	err = nil
	exec := func(q string, args ...any) int64 {
		if err != nil {
			return 0
		}
		res, e := tx.Exec(q, args...)
		if e != nil {
			err = e
			return 0
		}
		id, _ := res.LastInsertId()
		return id
	}
	salonID := exec(`INSERT INTO salons (name, slug, email, plan, lang) VALUES (?, ?, ?, 'free', ?)`, name, slug, email, lang)
	userID = exec(`INSERT INTO users (salon_id, email, terms_accepted_at, terms_version, lang) VALUES (?, ?, unixepoch(), ?, ?)`,
		salonID, email, termsVersion, lang)
	staffID := exec(`INSERT INTO staff (salon_id, name) VALUES (?, ?)`, salonID, first)
	for wd := time.Tuesday; wd <= time.Saturday; wd++ {
		end := 18 * 60
		if wd == time.Saturday {
			end = 14 * 60
		}
		exec(`INSERT INTO working_hours (staff_id, weekday, start_min, end_min) VALUES (?, ?, ?, ?)`, staffID, int(wd), 9*60, end)
	}
	exec(`INSERT INTO services (salon_id, name, duration_min, price_cents, sort, active) VALUES (?, ?, 30, 2500, 1, 0)`, salonID, tr(lang, "Haarschnitt"))
	exec(`INSERT INTO services (salon_id, name, duration_min, price_cents, sort, active) VALUES (?, ?, 60, 4500, 2, 0)`, salonID, tr(lang, "Waschen, Schneiden, Föhnen"))
	exec(`INSERT INTO staff_services (staff_id, service_id) SELECT ?, id FROM services WHERE salon_id = ?`, staffID, salonID)
	if err != nil {
		return 0, false, false, err
	}
	return userID, true, false, tx.Commit()
}

// signupDone: dieselbe Seite für neue und bestehende Konten. Der Hinweis auf die
// zweite Mail (Kammzeit-News) hängt nur am Häkchen, nicht am Konto.
func (a *app) signupDone(w http.ResponseWriter, r *http.Request, email string, mailFailed, marketing bool) {
	d := a.authPage(r, "Fast geschafft", signupPath(a.langFor(r)))
	d["Email"], d["MailFailed"], d["Marketing"] = email, mailFailed, marketing
	a.render(w, http.StatusOK, "signup-done", d)
}
