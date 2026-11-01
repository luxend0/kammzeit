package main

import (
	"database/sql"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// marketingApp: App mit deutscher und englischer Domain. Fehlt das
// Mail-Template der Bestätigung (baut der Mail-Bereich), kommt ein minimales
// dazu, das nur den Link enthält.
func marketingApp(t *testing.T) *app {
	t.Helper()
	a := newSignupApp(t, false)
	a.cfg.Domains = parseDomains("kammzeit.test=de,kammzeit-en.test=en", "")
	if a.tpl.Lookup("mail-marketing-confirm-text") == nil {
		template := `{{define "mail-marketing-confirm-subject"}}Kammzeit-News bestätigen{{end}}` +
			`{{define "mail-marketing-confirm-text"}}{{.ConfirmURL}}{{end}}`
		if _, err := a.tpl.New("marketing-test").Parse(template); err != nil {
			t.Fatal(err)
		}
	}
	marketingDOILimit = newLimiter(3, 24*time.Hour)
	return a
}

var confirmLinkRe = regexp.MustCompile(`https?://[^/\s"<]+/newsletter/bestaetigen/[a-z]+\.[0-9a-f]+`)

// confirmLink wartet auf die Bestätigungsmail im Log (ohne SMTP_HOST) und
// liefert den Link daraus.
func confirmLink(t *testing.T, logs *syncBuf) string {
	t.Helper()
	for i := 0; i < 200; i++ {
		if m := confirmLinkRe.FindString(logs.String()); m != "" {
			return m
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("keine Bestätigungsmail im Log:\n%s", logs.String())
	return ""
}

func marketingReq(a *app, method, host, path, origin string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://"+host+path, strings.NewReader(""))
	r.Host = host
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	a.routes().ServeHTTP(w, r)
	return w
}

func userIDByMail(t *testing.T, a *app, email string) int64 {
	t.Helper()
	var id int64
	if err := a.db.QueryRow(`SELECT id FROM users WHERE email = ?`, email).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// Double-Opt-in: das Häkchen allein schaltet nichts ein, erst die Bestätigung
// aus der Mail. Der Nachweis (Zeitpunkt, Weg, Text) landet am Konto.
func TestMarketingDOI(t *testing.T) {
	a := marketingApp(t)
	h := a.routes()
	var logs syncBuf
	log.SetOutput(&logs)
	defer log.SetOutput(os.Stderr)

	form := url.Values{"name": {"Salon News"}, "vorname": {"Nina"}, "slug": {"salon-news"}, "email": {"news@example.com"},
		"agb": {"1"}, "marketing": {"1"}}
	rec := postSignup(h, form, "198.51.100.40")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Dazu kommt eine zweite Mail") {
		t.Fatalf("Registrierung: %d %s", rec.Code, rec.Body)
	}
	id := userIDByMail(t, a, "news@example.com")
	link := confirmLink(t, &logs)
	if got := a.marketingState(id); got != "ausstehend" {
		t.Fatalf("nach der Registrierung: %q, want ausstehend", got)
	}
	var text, source string
	var at sql.NullInt64
	if err := a.db.QueryRow(`SELECT marketing_text, marketing_source, marketing_at FROM users WHERE id = ?`, id).Scan(&text, &source, &at); err != nil {
		t.Fatal(err)
	}
	if text != marketingConsentDE || source != "" || at.Valid {
		t.Fatalf("vor der Bestätigung: text %q, source %q, at %v", text, source, at)
	}
	if !strings.HasPrefix(link, "https://kammzeit.test/newsletter/bestaetigen/registrierung.") {
		t.Fatalf("Link %q", link)
	}
	path := strings.TrimPrefix(link, "https://kammzeit.test")

	// Ansehen (Mail-Scanner, Link-Vorschau) bestätigt nichts.
	for i := 0; i < 2; i++ {
		w := marketingReq(a, "GET", "kammzeit.test", path, "")
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Ja, Kammzeit-News bestätigen") ||
			!strings.Contains(w.Body.String(), `action="`+path+`"`) {
			t.Fatalf("GET %d: %d %s", i, w.Code, w.Body)
		}
	}
	if got := a.marketingState(id); got != "ausstehend" {
		t.Fatalf("nach GET: %q", got)
	}
	// Fremde Herkunft bestätigt nicht.
	if w := marketingReq(a, "POST", "kammzeit.test", path, "https://evil.example"); w.Code != http.StatusForbidden {
		t.Fatalf("POST fremde Herkunft: %d", w.Code)
	}
	w := marketingReq(a, "POST", "kammzeit.test", path, "http://kammzeit.test")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Danke, bestätigt") {
		t.Fatalf("Bestätigen: %d %s", w.Code, w.Body)
	}
	var pending string
	if err := a.db.QueryRow(`SELECT marketing_source, marketing_at, marketing_pending_hash FROM users WHERE id = ?`, id).Scan(&source, &at, &pending); err != nil {
		t.Fatal(err)
	}
	if got := a.marketingState(id); got != "an" || source != "registrierung" || !at.Valid || pending != "" {
		t.Fatalf("nach Bestätigung: state %q, source %q, at %v, pending %q", got, source, at, pending)
	}
	// Ein zweites Mal geht nicht, auch die Ansicht sagt das.
	if w := marketingReq(a, "POST", "kammzeit.test", path, "http://kammzeit.test"); w.Code != http.StatusGone {
		t.Fatalf("zweites Bestätigen: %d", w.Code)
	}
	if w := marketingReq(a, "GET", "kammzeit.test", path, ""); w.Code != http.StatusGone {
		t.Fatalf("GET nach Bestätigung: %d", w.Code)
	}
	// Schon bestätigt: kein neuer Link, keine neue Mail.
	if err := a.startMarketingDOI(id, "einstellungen"); err != nil || a.marketingState(id) != "an" {
		t.Fatalf("erneut starten: %v, %q", err, a.marketingState(id))
	}

	// Ohne Häkchen: nichts vorgemerkt, keine zweite Mail.
	form = url.Values{"name": {"Salon Ohne"}, "vorname": {"Olaf"}, "slug": {"salon-ohne"}, "email": {"ohne@example.com"}, "agb": {"1"}}
	rec = postSignup(h, form, "198.51.100.41")
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "zweite Mail") {
		t.Fatalf("ohne Häkchen: %d", rec.Code)
	}
	time.Sleep(50 * time.Millisecond)
	if got := a.marketingState(userIDByMail(t, a, "ohne@example.com")); got != "aus" {
		t.Fatalf("ohne Häkchen: %q", got)
	}

	// Bestehendes Konto mit Häkchen: ändert nichts am Konto (keine Anmeldung).
	form = url.Values{"name": {"Salon Ohne Zwei"}, "vorname": {"Olaf"}, "slug": {"salon-ohne-zwei"}, "email": {"ohne@example.com"},
		"agb": {"1"}, "marketing": {"1"}}
	if rec := postSignup(h, form, "198.51.100.42"); rec.Code != http.StatusOK {
		t.Fatalf("bestehendes Konto: %d", rec.Code)
	}
	time.Sleep(50 * time.Millisecond)
	if got := a.marketingState(userIDByMail(t, a, "ohne@example.com")); got != "aus" {
		t.Fatalf("bestehendes Konto über die Registrierung vorgemerkt: %q", got)
	}
	if n := strings.Count(logs.String(), "/newsletter/bestaetigen/"); n != 1 {
		t.Fatalf("%d Bestätigungsmails, want 1:\n%s", n, logs.String())
	}
}

// Auf kammzeit.com: Einwilligungstext englisch gespeichert, Link auf die .com-Domain.
func TestMarketingDOIEnglish(t *testing.T) {
	a := marketingApp(t)
	var logs syncBuf
	log.SetOutput(&logs)
	defer log.SetOutput(os.Stderr)
	form := url.Values{"name": {"Salon London"}, "vorname": {"Liz"}, "slug": {"salon-london"}, "email": {"liz@example.com"},
		"agb": {"1"}, "marketing": {"1"}}
	req := httptest.NewRequest("POST", "/registrieren", strings.NewReader(form.Encode()))
	req.Host = "kammzeit-en.test"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "https://kammzeit-en.test")
	req.RemoteAddr = "198.51.100.43:1234"
	if rec := signupDo(a.routes(), req); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "a second email is on its way") {
		t.Fatalf("Registrierung en: %d %s", rec.Code, rec.Body)
	}
	link := confirmLink(t, &logs)
	if !strings.HasPrefix(link, "https://kammzeit-en.test/newsletter/bestaetigen/") {
		t.Fatalf("Link %q", link)
	}
	var text string
	if err := a.db.QueryRow(`SELECT marketing_text FROM users WHERE email = 'liz@example.com'`).Scan(&text); err != nil {
		t.Fatal(err)
	}
	if text != tr("en", marketingConsentDE) || text == marketingConsentDE {
		t.Fatalf("Einwilligungstext %q", text)
	}
	w := marketingReq(a, "GET", "kammzeit-en.test", strings.TrimPrefix(link, "https://kammzeit-en.test"), "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Yes, confirm Kammzeit news") || !strings.Contains(w.Body.String(), `<html lang="en">`) {
		t.Fatalf("Bestätigungsseite en: %d %s", w.Code, w.Body)
	}
}

// Abgelaufen (älter als 7 Tage): weder Ansicht noch Bestätigung, Stand "aus".
// Ein Widerruf macht einen offenen Link ebenfalls ungültig.
func TestMarketingExpiredAndWithdrawn(t *testing.T) {
	a := marketingApp(t)
	var logs syncBuf
	log.SetOutput(&logs)
	defer log.SetOutput(os.Stderr)
	res, err := a.db.Exec(`INSERT INTO salons (slug, name) VALUES ('alt', 'Salon Alt')`)
	if err != nil {
		t.Fatal(err)
	}
	salonID, _ := res.LastInsertId()
	res, err = a.db.Exec(`INSERT INTO users (salon_id, email) VALUES (?, 'alt@example.com')`, salonID)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()

	if err := a.startMarketingDOI(id, "einstellungen"); err != nil {
		t.Fatal(err)
	}
	path := strings.TrimPrefix(confirmLink(t, &logs), "https://kammzeit.test")
	if a.marketingState(id) != "ausstehend" {
		t.Fatal("nicht ausstehend")
	}
	if _, err := a.db.Exec(`UPDATE users SET marketing_pending_at = ? WHERE id = ?`, time.Now().Add(-8*24*time.Hour).Unix(), id); err != nil {
		t.Fatal(err)
	}
	if got := a.marketingState(id); got != "aus" {
		t.Fatalf("abgelaufen: %q", got)
	}
	if w := marketingReq(a, "GET", "kammzeit.test", path, ""); w.Code != http.StatusGone || !strings.Contains(w.Body.String(), "Ein Link gilt 7 Tage") {
		t.Fatalf("GET abgelaufen: %d %s", w.Code, w.Body)
	}
	if w := marketingReq(a, "POST", "kammzeit.test", path, "http://kammzeit.test"); w.Code != http.StatusGone {
		t.Fatalf("POST abgelaufen: %d", w.Code)
	}
	if a.marketingState(id) != "aus" {
		t.Fatal("abgelaufener Link hat eingeschaltet")
	}

	// Neuer Link, dann Widerruf vor der Bestätigung.
	logs2 := &syncBuf{}
	log.SetOutput(logs2)
	if err := a.startMarketingDOI(id, "einstellungen"); err != nil {
		t.Fatal(err)
	}
	path = strings.TrimPrefix(confirmLink(t, logs2), "https://kammzeit.test")
	if err := a.withdrawMarketing(id, "einstellungen"); err != nil {
		t.Fatal(err)
	}
	if w := marketingReq(a, "POST", "kammzeit.test", path, "http://kammzeit.test"); w.Code != http.StatusGone || a.marketingState(id) != "aus" {
		t.Fatalf("Bestätigen nach Widerruf: %d, %q", w.Code, a.marketingState(id))
	}

	// Höchstens 3 Bestätigungsmails pro Konto und Tag (2 sind schon raus).
	if err := a.startMarketingDOI(id, "einstellungen"); err != nil {
		t.Fatal(err)
	}
	if err := a.startMarketingDOI(id, "einstellungen"); err != errMarketingTooMany {
		t.Fatalf("vierte Mail am Tag: %v", err)
	}
	if err := a.startMarketingDOI(id, "Böse Quelle"); err == nil {
		t.Fatal("ungültige Quelle angenommen")
	}
}

// Abmelden per Link aus der Mail: nur mit MARKETING_SECRET, HMAC über die
// Konto-ID, One-Click-POST ohne Herkunft (RFC 8058). Ohne Secret führt der Link
// in die Einstellungen.
func TestMarketingUnsubscribe(t *testing.T) {
	a := marketingApp(t)
	log.SetOutput(&syncBuf{})
	defer log.SetOutput(os.Stderr)
	res, err := a.db.Exec(`INSERT INTO salons (slug, name) VALUES ('ab', 'Salon Ab')`)
	if err != nil {
		t.Fatal(err)
	}
	salonID, _ := res.LastInsertId()
	res, err = a.db.Exec(`INSERT INTO users (salon_id, email, lang, marketing_at, marketing_source) VALUES (?, 'ab@example.com', 'en', unixepoch(), 'registrierung')`, salonID)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	other, _ := a.db.Exec(`INSERT INTO users (salon_id, email, marketing_at) VALUES (?, 'zwei@example.com', unixepoch())`, salonID)
	otherID, _ := other.LastInsertId()

	t.Setenv("MARKETING_SECRET", "")
	if got := a.marketingUnsubURL(id, "en"); got != "https://kammzeit-en.test/dashboard/einstellungen" {
		t.Fatalf("ohne Secret: %q", got)
	}
	t.Setenv("MARKETING_SECRET", "zu-kurz")
	if got := a.marketingUnsubURL(id, "de"); !strings.HasSuffix(got, "/dashboard/einstellungen") {
		t.Fatalf("kurzes Secret: %q", got)
	}

	t.Setenv("MARKETING_SECRET", "0123456789abcdef0123456789abcdef")
	link := a.marketingUnsubURL(id, "en")
	if !strings.HasPrefix(link, "https://kammzeit-en.test/newsletter/abmelden/") {
		t.Fatalf("Abmeldelink %q", link)
	}
	path := strings.TrimPrefix(link, "https://kammzeit-en.test")
	w := marketingReq(a, "GET", "kammzeit-en.test", path, "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Unsubscribe from Kammzeit news") || a.marketingState(id) != "an" {
		t.Fatalf("GET Abmelden: %d, %q %s", w.Code, a.marketingState(id), w.Body)
	}
	// Gefälscht: andere ID mit derselben Signatur, kaputte Signatur.
	_, mac, _ := strings.Cut(strings.TrimPrefix(path, "/newsletter/abmelden/"), ".")
	for _, bad := range []string{"/newsletter/abmelden/" + itoa(otherID) + "." + mac, path[:len(path)-1] + "0", "/newsletter/abmelden/" + itoa(id)} {
		if w := marketingReq(a, "POST", "kammzeit-en.test", bad, ""); w.Code != http.StatusNotFound {
			t.Errorf("POST %s: %d", bad, w.Code)
		}
	}
	if a.marketingState(otherID) != "an" {
		t.Fatal("fremdes Konto abgemeldet")
	}
	// One-Click: POST ohne Origin, wie ihn Mailprogramme schicken.
	w = marketingReq(a, "POST", "kammzeit-en.test", path, "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Unsubscribed") {
		t.Fatalf("POST Abmelden: %d %s", w.Code, w.Body)
	}
	var withdrawn sql.NullInt64
	if err := a.db.QueryRow(`SELECT marketing_withdrawn_at FROM users WHERE id = ?`, id).Scan(&withdrawn); err != nil {
		t.Fatal(err)
	}
	if a.marketingState(id) != "aus" || !withdrawn.Valid {
		t.Fatalf("nach Abmelden: %q, withdrawn %v", a.marketingState(id), withdrawn)
	}
	// Ohne Secret gilt auch ein alter Link nicht mehr.
	t.Setenv("MARKETING_SECRET", "")
	if w := marketingReq(a, "GET", "kammzeit-en.test", path, ""); w.Code != http.StatusNotFound {
		t.Fatalf("ohne Secret: %d", w.Code)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
