package main

// Login per Magic-Link: /login, /login/{token}, /logout.

import (
	"cmp"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"
)

const loginTokenTTL = 15 * time.Minute

// Rate-Limits für POST /login (Variablen, damit Tests sie zurücksetzen können).
var (
	loginIPLimit   = newLimiter(5, 15*time.Minute)
	loginMailLimit = newLimiter(3, 15*time.Minute)
)

func (a *app) routesAuth(mux *http.ServeMux) {
	mux.HandleFunc("GET /login", a.loginPage)
	mux.HandleFunc("POST /login", a.loginRequest)
	mux.HandleFunc("GET /login/{token}", a.loginConfirm)
	mux.HandleFunc("POST /login/{token}", a.loginRedeem)
	mux.HandleFunc("POST /logout", a.logout)
}

// loginNext lässt nur Pfade auf der eigenen Seite durch (kein Open Redirect):
// "/..." ja, "//host", "/\host", "https://...", Steuerzeichen (Browser
// entfernen Tab/Zeilenumbruch, aus "/\t/host" wird "//host") nein.
func loginNext(s string) string {
	if len(s) > 1000 || !strings.HasPrefix(s, "/") || strings.HasPrefix(s, "//") || strings.Contains(s, `\`) {
		return ""
	}
	for _, c := range s {
		if c < 0x20 || c == 0x7f {
			return ""
		}
	}
	return s
}

// authRootOnly: Login läuft nur auf der Hauptdomain, Salon-Subdomains leiten um.
func (a *app) authRootOnly(w http.ResponseWriter, r *http.Request) bool {
	if a.onRootHost(r) {
		return true
	}
	http.Redirect(w, r, a.siteURLFor(r)+r.URL.RequestURI(), http.StatusFound)
	return false
}

// authPage liefert die Daten für das Layout von Login und Registrierung
// ("auth-top" in web/auth.html, Website-Look; im Einzelbetrieb ohne Website-Menü).
// Sprache = Domain der Anfrage, title = deutscher Text (wird übersetzt).
func (a *app) authPage(r *http.Request, title, path string) map[string]any {
	lang := a.langFor(r)
	d := a.page(nil, tr(lang, title)+" - Kammzeit")
	d["Lang"], d["Path"], d["Single"] = lang, path, a.cfg.SingleTenant
	return d
}

// authMessage zeigt eine Meldung im Login-Layout (Texte deutsch, werden übersetzt).
// link = Ziel des Knopfs, ill = Kammi-Bild aus static/img/ill.
func (a *app) authMessage(w http.ResponseWriter, r *http.Request, status int, heading, msg, linkText, link, ill string) {
	d := a.authPage(r, heading, "/login")
	lang := d["Lang"].(string)
	d["Heading"], d["Message"], d["LinkText"] = tr(lang, heading), tr(lang, msg), tr(lang, linkText)
	d["Link"], d["Ill"] = link, ill
	a.render(w, status, "auth-message", d)
}

// authFail und authServerError: wie fail/serverError, aber in der Sprache der Domain.
func (a *app) authFail(w http.ResponseWriter, r *http.Request, status int, msg string) {
	a.failLang(w, nil, a.langFor(r), status, msg)
}

func (a *app) authServerError(w http.ResponseWriter, r *http.Request, err error) {
	log.Print(err)
	a.authFail(w, r, http.StatusInternalServerError, "Da ist etwas schiefgelaufen. Bitte später noch einmal versuchen.")
}

// badOrigin antwortet auf POSTs von fremden Seiten (CSRF).
func (a *app) badOrigin(w http.ResponseWriter, r *http.Request) {
	http.Error(w, tr(a.langFor(r), "Ungültige Herkunft der Anfrage."), http.StatusForbidden)
}

func (a *app) loginPage(w http.ResponseWriter, r *http.Request) {
	if !a.authRootOnly(w, r) {
		return
	}
	next := loginNext(r.URL.Query().Get("weiter"))
	u, err := a.currentUser(r)
	if err != nil {
		a.authServerError(w, r, err)
		return
	}
	if u != nil {
		http.Redirect(w, r, cmp.Or(next, "/dashboard"), http.StatusFound)
		return
	}
	if r.URL.Query().Has("pausiert") { // a.dash hat einen pausierten Mitarbeiter-Login abgemeldet
		a.staffPausedPage(w, r, http.StatusOK)
		return
	}
	d := a.authPage(r, "Anmelden", "/login")
	d["Next"] = next
	d["LoggedOut"] = r.URL.Query().Has("abgemeldet")
	a.render(w, http.StatusOK, "auth-login", d)
}

// loginRequest antwortet immer gleich, egal ob es das Konto gibt.
func (a *app) loginRequest(w http.ResponseWriter, r *http.Request) {
	if !a.authRootOnly(w, r) {
		return
	}
	if !sameOrigin(r) {
		a.badOrigin(w, r)
		return
	}
	if !loginIPLimit.allow(clientIP(r)) {
		a.loginTooMany(w, r)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	if err := r.ParseForm(); err != nil {
		a.authFail(w, r, http.StatusBadRequest, "Das Formular war unvollständig.")
		return
	}
	next := loginNext(r.PostFormValue("weiter"))
	d := a.authPage(r, "Anmelden", "/login")
	d["Next"] = next
	raw := strings.TrimSpace(r.PostFormValue("email"))
	addr, err := mail.ParseAddress(raw)
	if err != nil || len(addr.Address) > 200 {
		d["Email"], d["Error"] = raw, tr(d["Lang"].(string), "Bitte eine gültige E-Mail-Adresse angeben.")
		a.render(w, http.StatusBadRequest, "auth-login", d)
		return
	}
	email := strings.ToLower(addr.Address)
	if !loginMailLimit.allow(email) {
		a.loginTooMany(w, r)
		return
	}
	// Konto suchen, Token anlegen und Mail schicken im Hintergrund: die Antwort
	// dauert so gleich lang, egal ob es das Konto gibt (sonst verrät die Zeit,
	// welche Adressen existieren).
	go a.loginMail(email, next, a.langFor(r))
	d["Sent"] = true
	a.render(w, http.StatusOK, "auth-login", d)
}

// loginMail schickt einen Login-Link, wenn es ein Konto mit der Adresse gibt.
// lang = Sprache der Domain, über die der Link angefordert wurde: wird die
// Sprache des Kontos, damit Link und Mail zu dieser Domain passen.
// Läuft im Hintergrund, Fehler landen nur im Log.
func (a *app) loginMail(email, next, lang string) {
	var userID int64
	var stored string
	err := a.db.QueryRow(`SELECT id, email FROM users WHERE email = ?`, email).Scan(&userID, &stored)
	if errors.Is(err, sql.ErrNoRows) {
		return
	}
	if err == nil {
		_, err = a.db.Exec(`UPDATE users SET lang = ? WHERE id = ?`, lang, userID)
	}
	var token string
	if err == nil {
		token, err = a.newLoginToken(userID)
	}
	if err == nil {
		err = a.mailLoginLink(stored, token, next)
	}
	if err != nil {
		log.Printf("Login-Link an %s: %v", email, err)
	}
}

func (a *app) loginTooMany(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Retry-After", "900")
	a.authMessage(w, r, http.StatusTooManyRequests, "Zu viele Versuche",
		"Bitte warte ein paar Minuten und versuch es dann noch einmal.", "Zurück zum Login", "/login", "")
}

func (a *app) loginInvalid(w http.ResponseWriter, r *http.Request) {
	a.authMessage(w, r, http.StatusGone, "Link nicht mehr gültig",
		"Dieser Login-Link ist abgelaufen, wurde schon benutzt oder ist unvollständig. Ein Link gilt 15 Minuten und nur einmal.",
		"Neuen Link anfordern", "/login", "")
}

// loginConfirm zeigt nur die Bestätigung. Eingelöst wird per POST, damit
// Link-Vorschauen und Mail-Scanner den Token nicht verbrauchen.
func (a *app) loginConfirm(w http.ResponseWriter, r *http.Request) {
	if !a.authRootOnly(w, r) {
		return
	}
	token := r.PathValue("token")
	var userID int64
	err := a.db.QueryRow(`SELECT user_id FROM login_tokens WHERE token_hash = ? AND used_at IS NULL AND expires_at > ?`,
		hashToken(token), time.Now().Unix()).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		a.loginInvalid(w, r)
		return
	} else if err != nil {
		a.authServerError(w, r, err)
		return
	}
	if paused, err := a.staffPaused(userID); err != nil {
		a.authServerError(w, r, err)
		return
	} else if paused {
		a.staffPausedPage(w, r, http.StatusForbidden)
		return
	}
	d := a.authPage(r, "Anmelden", "/login")
	d["Token"], d["Next"] = token, loginNext(r.URL.Query().Get("weiter"))
	a.render(w, http.StatusOK, "auth-confirm", d)
}

// loginRedeem löst den Token atomar ein: das UPDATE trifft ihn nur, solange er
// unbenutzt und gültig ist (RETURNING liefert dann genau eine Zeile).
// sameOrigin verhindert Login-CSRF (fremde Seite meldet dich in ihrem Konto an).
func (a *app) loginRedeem(w http.ResponseWriter, r *http.Request) {
	if !a.authRootOnly(w, r) {
		return
	}
	if !sameOrigin(r) {
		a.badOrigin(w, r)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	now := time.Now().Unix()
	var userID int64
	err := a.db.QueryRow(`UPDATE login_tokens SET used_at = ?
		WHERE token_hash = ? AND used_at IS NULL AND expires_at > ? RETURNING user_id`,
		now, hashToken(r.PathValue("token")), now).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		a.loginInvalid(w, r)
		return
	} else if err != nil {
		a.authServerError(w, r, err)
		return
	}
	if paused, err := a.staffPaused(userID); err != nil {
		a.authServerError(w, r, err)
		return
	} else if paused {
		a.staffPausedPage(w, r, http.StatusForbidden)
		return
	}
	if err := a.startSession(w, r, userID); err != nil {
		a.authServerError(w, r, err)
		return
	}
	http.Redirect(w, r, cmp.Or(loginNext(r.PostFormValue("weiter")), "/dashboard"), http.StatusSeeOther)
}

func (a *app) logout(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		a.badOrigin(w, r)
		return
	}
	a.endSession(w, r)
	http.Redirect(w, r, "/login?abgemeldet", http.StatusSeeOther)
}

// newLoginToken legt einen Einmal-Token an (gespeichert nur als Hash) und
// räumt abgelaufene weg.
func (a *app) newLoginToken(userID int64) (string, error) {
	now := time.Now()
	a.db.Exec(`DELETE FROM login_tokens WHERE expires_at < ?`, now.Unix())
	token := randomToken()
	_, err := a.db.Exec(`INSERT INTO login_tokens (token_hash, user_id, expires_at) VALUES (?, ?, ?)`,
		hashToken(token), userID, now.Add(loginTokenTTL).Unix())
	return token, err
}

// mailLoginLink schickt den Link; next (schon geprüft) kommt als ?weiter= mit.
func (a *app) mailLoginLink(email, token, next string) error {
	// Link auf die Domain der Sprache des Kontos (users.lang: de -> kammzeit.de, en -> kammzeit.com).
	var lang string
	a.db.QueryRow(`SELECT lang FROM users WHERE email = ?`, email).Scan(&lang)
	if lang != "en" {
		lang = "de"
	}
	link := a.siteURLLang(lang) + "/login/" + token
	if next != "" {
		link += "?weiter=" + url.QueryEscape(next)
	}
	return a.mailT(email, lang, "login", map[string]any{"Link": link}) // web/mail.html
}

// sendLoginLink mailt einen Einmal-Link (15 Minuten gültig) an das Konto.
// Wird auch von der Registrierung benutzt. Signatur nicht ändern.
func (a *app) sendLoginLink(userID int64, email string) error {
	token, err := a.newLoginToken(userID)
	if err != nil {
		return err
	}
	return a.mailLoginLink(email, token, "")
}
