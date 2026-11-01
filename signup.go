package main

// Registrierung neuer Salons: /registrieren.

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
	mux.HandleFunc("GET /registrieren", a.signupPage)
	mux.HandleFunc("GET /registrieren/adresse", a.signupSlug)
	mux.HandleFunc("POST /registrieren", a.signup)
}

// Adressen, die kein Salon bekommen darf (Technik, eigene Seiten, Verwechslung).
var reservedSlugs = []string{"www", "mail", "smtp", "imap", "pop", "mx", "ns1", "ns2", "admin", "api", "app",
	"dashboard", "login", "logout", "static", "assets", "cdn", "demo", "kammzeit", "hilfe", "help", "support",
	"status", "blog", "docs", "test", "dev", "staging", "billing", "stripe", "abo", "konto", "registrieren",
	"impressum", "datenschutz", "agb", "avv", "autoconfig", "autodiscover", "_dmarc", "mta-sts", "webmail"}

// ponytail: im Speicher wie alle Limiter, zählt nur gültige Anmeldungen (die Mails auslösen).
// signupMailLimit begrenzt die Login-Mails je Empfänger, wie loginMailLimit in auth.go.
var (
	signupLimit     = newLimiter(5, time.Hour)
	signupMailLimit = newLimiter(3, 15*time.Minute)
)

// slugProblem prüft nur die Form einer Wunsch-Adresse. "" = in Ordnung.
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
		http.Redirect(w, r, a.siteURL()+"/registrieren", http.StatusFound)
		return false
	}
	if a.cfg.SingleTenant {
		exists, err := signupHasSalon(a.db)
		if err != nil {
			a.serverError(w, nil, err)
			return false
		}
		if exists {
			a.fail(w, nil, http.StatusNotFound, "Diese Seite gibt es nicht.")
			return false
		}
	}
	return true
}

func signupHasSalon(q querier) (bool, error) {
	var n int
	err := q.QueryRow(`SELECT EXISTS (SELECT 1 FROM salons)`).Scan(&n)
	return n == 1, err
}

func (a *app) signupPage(w http.ResponseWriter, r *http.Request) {
	if a.signupOpen(w, r) {
		a.signupForm(w, r, http.StatusOK, nil, "")
	}
}

// signupForm zeigt das Formular, nach einem Fehler mit den Eingaben von eben.
// errs = Fehler am Feld (name, slug, vorname, email, agb), msg = Hinweis ohne Feldbezug.
func (a *app) signupForm(w http.ResponseWriter, r *http.Request, status int, errs map[string]string, msg string) {
	title := "Salon registrieren"
	if a.cfg.SingleTenant {
		title = "Ersteinrichtung"
	}
	d := a.authPage(title, "/registrieren")
	if !a.cfg.SingleTenant { // steht in der Sitemap, darf in die Suche
		d["Index"], d["Canonical"] = true, a.siteURL()+"/registrieren"
		d["Description"] = "Salon bei Kammzeit registrieren: eigene Buchungsseite, gratis für einen Mitarbeiter, ohne Zahlungsdaten."
	}
	d["Heading"], d["Error"], d["Errors"], d["Domain"] = title, msg, errs, a.cfg.BaseDomain
	d["Name"], d["Slug"], d["SlugAuto"] = r.PostFormValue("name"), r.PostFormValue("slug"), r.PostFormValue("slug_auto")
	d["Vorname"], d["Email"], d["AGB"] = r.PostFormValue("vorname"), r.PostFormValue("email"), r.PostFormValue("agb") == "1"
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
	d := map[string]any{"Domain": a.cfg.BaseDomain}
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
			a.serverError(w, nil, err)
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
		http.Error(w, "Ungültige Herkunft der Anfrage.", http.StatusForbidden)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := r.ParseForm(); err != nil {
		a.fail(w, nil, http.StatusBadRequest, "Das Formular war unvollständig.")
		return
	}
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
	taken := "Die Adresse " + slug + "." + a.cfg.BaseDomain + " ist leider vergeben. Bitte wähle eine andere."
	errs := map[string]string{}
	if n < 2 || n > 80 || strings.ContainsFunc(name, unicode.IsControl) {
		errs["name"] = "Bitte den Namen des Salons angeben (2 bis 80 Zeichen)."
	}
	switch {
	case sp == "vergeben":
		errs["slug"] = taken
	case sp != "":
		errs["slug"] = "Die Wunsch-Adresse ist " + sp + "."
	}
	if fn < 1 || fn > 40 || strings.ContainsFunc(first, unicode.IsControl) {
		errs["vorname"] = "Bitte deinen Vornamen angeben (höchstens 40 Zeichen)."
	}
	if mailErr != nil || len(addr.Address) > 200 {
		errs["email"] = "Bitte eine gültige E-Mail-Adresse angeben."
	}
	if !a.cfg.SingleTenant && r.PostFormValue("agb") != "1" {
		errs["agb"] = "Bitte bestätigen, dass du als Unternehmer handelst, und die AGB und den Vertrag zur Auftragsverarbeitung akzeptieren."
	}
	if len(errs) > 0 {
		a.signupForm(w, r, http.StatusBadRequest, errs, "")
		return
	}
	email := strings.ToLower(addr.Address)
	if r.PostFormValue("homepage") != "" { // Honeypot: Bots bekommen dieselbe Antwort, angelegt wird nichts
		a.signupDone(w, email, false)
		return
	}
	if !signupLimit.allow(clientIP(r)) {
		a.signupForm(w, r, http.StatusTooManyRequests, nil, "Zu viele Anmeldungen von deinem Anschluss. Bitte versuche es in einer Stunde noch einmal.")
		return
	}
	if !signupMailLimit.allow(email) { // vor jeder Abfrage: verrät nicht, ob es das Konto gibt
		a.signupForm(w, r, http.StatusTooManyRequests, nil, "Zu viele Anmeldungen mit dieser E-Mail-Adresse. Bitte versuche es in 15 Minuten noch einmal.")
		return
	}

	userID, isTaken, err := a.signupCreate(name, first, slug, email)
	if errors.Is(err, errSignupClosed) {
		a.fail(w, nil, http.StatusNotFound, "Diese Seite gibt es nicht.")
		return
	}
	if err != nil {
		a.serverError(w, nil, err)
		return
	}
	if isTaken {
		a.signupForm(w, r, http.StatusConflict, map[string]string{"slug": taken}, "")
		return
	}
	// Neues und schon bestehendes Konto bekommen dieselbe Seite und denselben
	// Link, damit das Formular nicht verrät, welche Adressen registriert sind.
	sendErr := a.sendLoginLink(userID, email)
	if sendErr != nil {
		log.Printf("Registrierung: Login-Link an %s: %v", email, sendErr)
	}
	a.signupDone(w, email, sendErr != nil)
}

var errSignupClosed = errors.New("Registrierung geschlossen")

// signupCreate legt in einer Transaktion Salon, Konto, einen Mitarbeiter (first =
// Vorname der Inhaberin) mit Arbeitszeiten und zwei Beispiel-Leistungen an und
// liefert die Konto-ID. Die Beispiel-Leistungen sind inaktiv: online buchbar
// wird erst, was der Salon selbst geprüft und eingeschaltet hat.
// Gibt es das Konto schon, wird nichts angelegt und dessen ID geliefert.
// taken = die Adresse ist vergeben, immer, auch für das eigene Konto: sonst
// verriete die Antwort, welche E-Mail zu welchem Salon gehört. Wer schon
// registriert ist, meldet sich über /login an.
func (a *app) signupCreate(name, first, slug, email string) (userID int64, taken bool, err error) {
	tx, err := a.db.Begin()
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback()
	if a.cfg.SingleTenant { // erneut in der Transaktion: genau ein Salon
		exists, err := signupHasSalon(tx)
		if err != nil {
			return 0, false, err
		}
		if exists {
			return 0, false, errSignupClosed
		}
	}
	st, err := slugStatus(tx, slug)
	if err != nil {
		return 0, false, err
	}
	if st != "frei" {
		return 0, true, nil
	}
	err = tx.QueryRow(`SELECT id FROM users WHERE email = ?`, email).Scan(&userID)
	if !errors.Is(err, sql.ErrNoRows) {
		return userID, false, err // Konto gibt es schon: nichts anlegen
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
	salonID := exec(`INSERT INTO salons (name, slug, email, plan) VALUES (?, ?, ?, 'free')`, name, slug, email)
	userID = exec(`INSERT INTO users (salon_id, email, terms_accepted_at, terms_version) VALUES (?, ?, unixepoch(), ?)`, salonID, email, termsVersion)
	staffID := exec(`INSERT INTO staff (salon_id, name) VALUES (?, ?)`, salonID, first)
	for wd := time.Tuesday; wd <= time.Saturday; wd++ {
		end := 18 * 60
		if wd == time.Saturday {
			end = 14 * 60
		}
		exec(`INSERT INTO working_hours (staff_id, weekday, start_min, end_min) VALUES (?, ?, ?, ?)`, staffID, int(wd), 9*60, end)
	}
	exec(`INSERT INTO services (salon_id, name, duration_min, price_cents, sort, active) VALUES (?, 'Haarschnitt', 30, 2500, 1, 0)`, salonID)
	exec(`INSERT INTO services (salon_id, name, duration_min, price_cents, sort, active) VALUES (?, 'Waschen, Schneiden, Föhnen', 60, 4500, 2, 0)`, salonID)
	exec(`INSERT INTO staff_services (staff_id, service_id) SELECT ?, id FROM services WHERE salon_id = ?`, staffID, salonID)
	if err != nil {
		return 0, false, err
	}
	return userID, false, tx.Commit()
}

func (a *app) signupDone(w http.ResponseWriter, email string, mailFailed bool) {
	d := a.authPage("Fast geschafft", "/registrieren")
	d["Email"], d["MailFailed"] = email, mailFailed
	a.render(w, http.StatusOK, "signup-done", d)
}
