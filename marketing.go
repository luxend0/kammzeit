package main

// Marketing-Einwilligung der Salon-Inhaber (Kammzeit-News per Mail).
// Freiwillig, Double-Opt-in, Nachweis in users.marketing_* (Migration 2),
// Widerruf jederzeit (Einstellungen oder Abmeldelink in jeder Marketing-Mail).
// Registrierung und Einstellungen rufen startMarketingDOI/withdrawMarketing auf.
//
// Stand eines Kontos:
//   - "ausstehend": marketing_pending_hash gesetzt und jünger als 7 Tage
//   - "an":         marketing_at gesetzt und marketing_withdrawn_at leer
//   - sonst "aus"
//
// Nachweis: marketing_at (wann bestätigt), marketing_source (über welchen Weg:
// registrierung | einstellungen), marketing_text (genau der Text, dem zugestimmt
// wurde, in der Sprache des Kontos), marketing_withdrawn_at (Widerruf).
//
// Abmeldelinks ohne Anmeldung brauchen MARKETING_SECRET (mindestens 16 Zeichen,
// z. B. `openssl rand -hex 32`): der Link trägt ein HMAC über die Konto-ID.
// Ohne Secret führt der Link in die Einstellungen (Abmelden nur angemeldet).

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// marketingConsentDE ist der Einwilligungstext (zugleich Übersetzungsschlüssel).
// Gespeichert wird er in der Sprache des Kontos.
const marketingConsentDE = "Ich möchte gelegentlich Neuigkeiten zu Kammzeit per E-Mail erhalten. Abmelden geht jederzeit."

// marketingMailDE: Bestätigungsmail als Text, falls das HTML-Template fehlt.
const marketingMailDE = "Hallo,\n\ndu möchtest gelegentlich Neuigkeiten zu Kammzeit per E-Mail bekommen. Bitte bestätige das mit diesem Link (7 Tage gültig):\n\n%s\n\n" +
	"Du hast das nicht angefordert? Dann ignoriere diese Mail einfach, wir schicken dir keine News.\n"

// marketingTTL: so lange gilt der Bestätigungslink.
const marketingTTL = 7 * 24 * time.Hour

// marketingDOILimit: höchstens 3 Bestätigungsmails pro Konto und Tag.
// ponytail: im Speicher wie alle Limiter, reicht für eine Instanz.
var marketingDOILimit = newLimiter(3, 24*time.Hour)

// errMarketingTooMany: Tageslimit für Bestätigungsmails erreicht.
var errMarketingTooMany = errors.New("zu viele Bestätigungsmails für Kammzeit-News, morgen wieder")

// Quelle der Einwilligung; steckt im Bestätigungs-Token ("<quelle>.<zufall>"),
// damit sie erst beim Bestätigen in marketing_source landet.
var marketingSourceRe = regexp.MustCompile(`^[a-z]{1,20}$`)

func (a *app) routesMarketing(mux *http.ServeMux) {
	mux.HandleFunc("GET /newsletter/bestaetigen/{token}", a.marketingConfirmPage)
	mux.HandleFunc("POST /newsletter/bestaetigen/{token}", a.marketingConfirm)
	mux.HandleFunc("GET /newsletter/abmelden/{token}", a.marketingUnsubPage)
	mux.HandleFunc("POST /newsletter/abmelden/{token}", a.marketingUnsub)
}

// startMarketingDOI merkt eine Einwilligung als ausstehend vor und schickt die
// Bestätigungsmail (Link 7 Tage gültig). source: "registrierung" | "einstellungen".
// Schon bestätigt: nichts zu tun. Ein neuer Aufruf ersetzt einen offenen Link.
func (a *app) startMarketingDOI(userID int64, source string) error {
	if !marketingSourceRe.MatchString(source) {
		return fmt.Errorf("Kammzeit-News: ungültige Quelle %q", source)
	}
	var email, lang, salonName string
	var on bool
	err := a.db.QueryRow(`SELECT u.email, u.lang, s.name, u.marketing_at IS NOT NULL AND u.marketing_withdrawn_at IS NULL
		FROM users u JOIN salons s ON s.id = u.salon_id WHERE u.id = ?`, userID).Scan(&email, &lang, &salonName, &on)
	if err != nil {
		return err
	}
	if on {
		return nil
	}
	if !marketingDOILimit.allow(strconv.FormatInt(userID, 10)) {
		return errMarketingTooMany
	}
	token := source + "." + randomToken()
	if _, err := a.db.Exec(`UPDATE users SET marketing_pending_hash = ?, marketing_pending_at = ?, marketing_text = ? WHERE id = ?`,
		hashToken(token), time.Now().Unix(), tr(lang, marketingConsentDE), userID); err != nil {
		return err
	}
	link := a.siteURLLang(lang) + "/newsletter/bestaetigen/" + token
	if a.tpl.Lookup("mail-marketing-confirm-text") != nil {
		err = a.mailT(email, lang, "marketing-confirm", map[string]any{
			"ConfirmURL": link,
			"Salon":      salonName,
			"Email":      email,
			"Days":       int(marketingTTL / (24 * time.Hour)),
		})
	} else { // ponytail: Textmail, bis es das Template mail-marketing-confirm-* gibt
		err = a.mail.send(email, tr(lang, "Bitte bestätigen: Kammzeit-News per E-Mail"), trf(lang, marketingMailDE, link))
	}
	if err != nil { // keine Mail, kein "ausstehend" (sonst wartet die Anzeige auf eine Mail, die nie kam)
		a.db.Exec(`UPDATE users SET marketing_pending_hash = '', marketing_pending_at = NULL WHERE id = ? AND marketing_pending_hash = ?`,
			userID, hashToken(token))
	}
	return err
}

// withdrawMarketing widerruft eine Einwilligung (oder eine ausstehende).
// ponytail: kein eigenes Feld für den Weg des Widerrufs (source), der Zeitpunkt
// in marketing_withdrawn_at ist der Nachweis.
func (a *app) withdrawMarketing(userID int64, source string) error {
	_, err := a.db.Exec(`UPDATE users SET
		marketing_withdrawn_at = CASE WHEN marketing_at IS NOT NULL AND marketing_withdrawn_at IS NULL THEN ? ELSE marketing_withdrawn_at END,
		marketing_pending_hash = '', marketing_pending_at = NULL WHERE id = ?`, time.Now().Unix(), userID)
	return err
}

// marketingState liefert den Stand für die Anzeige: "aus", "ausstehend" oder "an".
func (a *app) marketingState(userID int64) string {
	var on, pending bool
	err := a.db.QueryRow(`SELECT marketing_at IS NOT NULL AND marketing_withdrawn_at IS NULL,
		marketing_pending_hash != '' AND marketing_pending_at > ? FROM users WHERE id = ?`,
		time.Now().Add(-marketingTTL).Unix(), userID).Scan(&on, &pending)
	switch {
	case err != nil:
		return "aus"
	case on:
		return "an"
	case pending:
		return "ausstehend"
	}
	return "aus"
}

// confirmMarketing löst einen Bestätigungs-Token atomar ein und liefert das Konto
// (sql.ErrNoRows: unbekannt, abgelaufen oder schon benutzt).
func (a *app) confirmMarketing(token string) (int64, error) {
	source, _, _ := strings.Cut(token, ".")
	if !marketingSourceRe.MatchString(source) {
		return 0, sql.ErrNoRows
	}
	now := time.Now()
	var id int64
	err := a.db.QueryRow(`UPDATE users SET marketing_at = ?, marketing_source = ?, marketing_withdrawn_at = NULL,
		marketing_pending_hash = '', marketing_pending_at = NULL
		WHERE marketing_pending_hash = ? AND marketing_pending_at > ? RETURNING id`,
		now.Unix(), source, hashToken(token), now.Add(-marketingTTL).Unix()).Scan(&id)
	return id, err
}

func (a *app) marketingPending(token string) (bool, error) {
	err := a.db.QueryRow(`SELECT 1 FROM users WHERE marketing_pending_hash = ? AND marketing_pending_at > ?`,
		hashToken(token), time.Now().Add(-marketingTTL).Unix()).Scan(new(int))
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (a *app) marketingInvalid(w http.ResponseWriter, r *http.Request) {
	a.authMessage(w, r, http.StatusGone, "Link nicht mehr gültig",
		"Dieser Bestätigungslink ist abgelaufen, wurde schon benutzt oder ist unvollständig. Ein Link gilt 7 Tage. In den Einstellungen kannst du einen neuen anfordern.",
		"Zu den Einstellungen", "/dashboard/einstellungen", "")
}

// marketingAction zeigt eine Seite mit einem Knopf, der per POST auslöst (wie
// beim Login: Link-Vorschauen und Mail-Scanner lösen nichts aus).
func (a *app) marketingAction(w http.ResponseWriter, r *http.Request, heading, msg, button, ill string) {
	d := a.authPage(r, heading, "/login")
	lang := d["Lang"].(string)
	d["Heading"], d["Message"], d["Button"] = tr(lang, heading), tr(lang, msg), tr(lang, button)
	d["Action"], d["Ill"] = r.URL.Path, ill
	a.render(w, http.StatusOK, "auth-action", d)
}

func (a *app) marketingConfirmPage(w http.ResponseWriter, r *http.Request) {
	if !a.authRootOnly(w, r) {
		return
	}
	ok, err := a.marketingPending(r.PathValue("token"))
	if err != nil {
		a.authServerError(w, r, err)
		return
	}
	if !ok {
		a.marketingInvalid(w, r)
		return
	}
	a.marketingAction(w, r, "Kammzeit-News bestätigen",
		"Bestätige, dass du gelegentlich Neuigkeiten zu Kammzeit per E-Mail bekommen möchtest. Abbestellen geht jederzeit mit einem Klick.",
		"Ja, Kammzeit-News bestätigen", "kammi-handy.webp")
}

func (a *app) marketingConfirm(w http.ResponseWriter, r *http.Request) {
	if !a.authRootOnly(w, r) {
		return
	}
	if !sameOrigin(r) {
		a.badOrigin(w, r)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	_, err := a.confirmMarketing(r.PathValue("token"))
	if errors.Is(err, sql.ErrNoRows) {
		a.marketingInvalid(w, r)
		return
	} else if err != nil {
		a.authServerError(w, r, err)
		return
	}
	a.authMessage(w, r, http.StatusOK, "Danke, bestätigt",
		"Du bekommst ab jetzt gelegentlich Kammzeit-News per E-Mail. In jeder Mail steht ein Link zum Abbestellen, und in den Einstellungen kannst du sie jederzeit abschalten.",
		"Zum Dashboard", "/dashboard", "kammi-winkt.webp")
}

// --- Abmelden ohne Anmeldung ---

// marketingSecret: Schlüssel für Abmeldelinks aus MARKETING_SECRET. Kein Ersatz
// aus der Datenbank: wer die Datenbank kennt, soll keine Links fälschen können.
func marketingSecret() string {
	if s := os.Getenv("MARKETING_SECRET"); len(s) >= 16 {
		return s
	}
	return ""
}

func unsubMAC(secret string, userID int64) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte("kammzeit-news-abmelden:" + strconv.FormatInt(userID, 10)))
	return hex.EncodeToString(m.Sum(nil)[:16])
}

// marketingUnsubURL = Abmeldelink für Marketing-Mails (Fuß der Mail und Header
// List-Unsubscribe mit List-Unsubscribe-Post: List-Unsubscribe=One-Click).
// Ohne MARKETING_SECRET: die Einstellungen (Abmelden nur angemeldet).
func (a *app) marketingUnsubURL(userID int64, lang string) string {
	secret := marketingSecret()
	if secret == "" {
		return a.siteURLLang(lang) + "/dashboard/einstellungen"
	}
	return a.siteURLLang(lang) + "/newsletter/abmelden/" + strconv.FormatInt(userID, 10) + "." + unsubMAC(secret, userID)
}

// unsubUser prüft einen Abmelde-Token ("<id>.<hmac>") und liefert die Konto-ID, 0 = ungültig.
func unsubUser(token string) int64 {
	secret := marketingSecret()
	idStr, mac, ok := strings.Cut(token, ".")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if secret == "" || !ok || err != nil || id <= 0 || !hmac.Equal([]byte(mac), []byte(unsubMAC(secret, id))) {
		return 0
	}
	return id
}

func (a *app) unsubInvalid(w http.ResponseWriter, r *http.Request) {
	a.authMessage(w, r, http.StatusNotFound, "Link ungültig",
		"Dieser Abmeldelink ist ungültig. Du kannst Kammzeit-News in den Einstellungen abbestellen.",
		"Zu den Einstellungen", "/dashboard/einstellungen", "")
}

func (a *app) marketingUnsubPage(w http.ResponseWriter, r *http.Request) {
	if !a.authRootOnly(w, r) {
		return
	}
	if unsubUser(r.PathValue("token")) == 0 {
		a.unsubInvalid(w, r)
		return
	}
	a.marketingAction(w, r, "Kammzeit-News abbestellen",
		"Du bekommst dann keine Neuigkeiten zu Kammzeit mehr per E-Mail. Mails zu deinem Konto (Login-Links, Rechnungen, wichtige Hinweise) kommen weiter.",
		"Abbestellen", "kammi-winkt.webp")
}

// marketingUnsub meldet ab. Ohne Herkunftsprüfung: Mailprogramme schicken den
// One-Click-POST (RFC 8058) von außen; schützen muss hier das HMAC im Token.
func (a *app) marketingUnsub(w http.ResponseWriter, r *http.Request) {
	if !a.authRootOnly(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	id := unsubUser(r.PathValue("token"))
	if id == 0 {
		a.unsubInvalid(w, r)
		return
	}
	if err := a.withdrawMarketing(id, "abmeldelink"); err != nil {
		a.authServerError(w, r, err)
		return
	}
	a.authMessage(w, r, http.StatusOK, "Abbestellt",
		"Du bekommst keine Kammzeit-News mehr. In den Einstellungen kannst du sie jederzeit wieder einschalten.",
		"Zu den Einstellungen", "/dashboard/einstellungen", "kammi-schlaeft.webp")
}
