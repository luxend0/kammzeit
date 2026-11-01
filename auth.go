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
	http.Redirect(w, r, a.siteURL()+r.URL.RequestURI(), http.StatusFound)
	return false
}

func (a *app) authMessage(w http.ResponseWriter, status int, heading, msg, linkText string) {
	d := a.page(nil, heading)
	d["Heading"], d["Message"], d["LinkText"] = heading, msg, linkText
	a.render(w, status, "auth-message", d)
}

func (a *app) loginPage(w http.ResponseWriter, r *http.Request) {
	if !a.authRootOnly(w, r) {
		return
	}
	next := loginNext(r.URL.Query().Get("weiter"))
	u, err := a.currentUser(r)
	if err != nil {
		a.serverError(w, nil, err)
		return
	}
	if u != nil {
		http.Redirect(w, r, cmp.Or(next, "/dashboard"), http.StatusFound)
		return
	}
	d := a.page(nil, "Anmelden")
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
		http.Error(w, "Ungültige Herkunft der Anfrage.", http.StatusForbidden)
		return
	}
	if !loginIPLimit.allow(clientIP(r)) {
		a.loginTooMany(w)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	if err := r.ParseForm(); err != nil {
		a.fail(w, nil, http.StatusBadRequest, "Das Formular war unvollständig.")
		return
	}
	next := loginNext(r.PostFormValue("weiter"))
	d := a.page(nil, "Anmelden")
	d["Next"] = next
	raw := strings.TrimSpace(r.PostFormValue("email"))
	addr, err := mail.ParseAddress(raw)
	if err != nil || len(addr.Address) > 200 {
		d["Email"], d["Error"] = raw, "Bitte eine gültige E-Mail-Adresse angeben."
		a.render(w, http.StatusBadRequest, "auth-login", d)
		return
	}
	email := strings.ToLower(addr.Address)
	if !loginMailLimit.allow(email) {
		a.loginTooMany(w)
		return
	}
	// Konto suchen, Token anlegen und Mail schicken im Hintergrund: die Antwort
	// dauert so gleich lang, egal ob es das Konto gibt (sonst verrät die Zeit,
	// welche Adressen existieren).
	go a.loginMail(email, next)
	d["Sent"] = true
	a.render(w, http.StatusOK, "auth-login", d)
}

// loginMail schickt einen Login-Link, wenn es ein Konto mit der Adresse gibt.
// Läuft im Hintergrund, Fehler landen nur im Log.
func (a *app) loginMail(email, next string) {
	var userID int64
	var stored string
	err := a.db.QueryRow(`SELECT id, email FROM users WHERE email = ?`, email).Scan(&userID, &stored)
	if errors.Is(err, sql.ErrNoRows) {
		return
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

func (a *app) loginTooMany(w http.ResponseWriter) {
	w.Header().Set("Retry-After", "900")
	a.authMessage(w, http.StatusTooManyRequests, "Zu viele Versuche",
		"Bitte warte ein paar Minuten und versuch es dann noch einmal.", "Zurück zum Login")
}

func (a *app) loginInvalid(w http.ResponseWriter) {
	a.authMessage(w, http.StatusGone, "Link nicht mehr gültig",
		"Dieser Login-Link ist abgelaufen, wurde schon benutzt oder ist unvollständig. Ein Link gilt 15 Minuten und nur einmal.",
		"Neuen Link anfordern")
}

// loginConfirm zeigt nur die Bestätigung. Eingelöst wird per POST, damit
// Link-Vorschauen und Mail-Scanner den Token nicht verbrauchen.
func (a *app) loginConfirm(w http.ResponseWriter, r *http.Request) {
	if !a.authRootOnly(w, r) {
		return
	}
	token := r.PathValue("token")
	var one int
	err := a.db.QueryRow(`SELECT 1 FROM login_tokens WHERE token_hash = ? AND used_at IS NULL AND expires_at > ?`,
		hashToken(token), time.Now().Unix()).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		a.loginInvalid(w)
		return
	} else if err != nil {
		a.serverError(w, nil, err)
		return
	}
	d := a.page(nil, "Anmelden")
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
		http.Error(w, "Ungültige Herkunft der Anfrage.", http.StatusForbidden)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	now := time.Now().Unix()
	var userID int64
	err := a.db.QueryRow(`UPDATE login_tokens SET used_at = ?
		WHERE token_hash = ? AND used_at IS NULL AND expires_at > ? RETURNING user_id`,
		now, hashToken(r.PathValue("token")), now).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		a.loginInvalid(w)
		return
	} else if err != nil {
		a.serverError(w, nil, err)
		return
	}
	if err := a.startSession(w, r, userID); err != nil {
		a.serverError(w, nil, err)
		return
	}
	http.Redirect(w, r, cmp.Or(loginNext(r.PostFormValue("weiter")), "/dashboard"), http.StatusSeeOther)
}

func (a *app) logout(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		http.Error(w, "Ungültige Herkunft der Anfrage.", http.StatusForbidden)
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
	link := a.siteURL() + "/login/" + token
	if next != "" {
		link += "?weiter=" + url.QueryEscape(next)
	}
	body := "Hallo,\n\nhier ist dein Login-Link für Kammzeit:\n\n" + link + "\n\n" +
		"Er ist 15 Minuten gültig und funktioniert nur einmal.\n" +
		"Öffne den Link auf dem Gerät, auf dem du angemeldet sein willst.\n\n" +
		"Du hast keinen Login angefordert? Dann kannst du diese Mail einfach ignorieren.\n"
	return a.mail.send(email, "Dein Login für Kammzeit", body)
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
