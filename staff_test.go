package main

import (
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// staffFix: Salon "test" (Pro) aus newTestSalon mit Inhaberin und einem
// Mitarbeiter-Login für Anna, dazu der Salon "fremd" mit eigenen Daten.
type staffFix struct {
	*testSalon
	owner, staff                   int64  // users.id
	ownerC, staffC                 string // Session-Cookies
	fremd, fremdCust, fremdStaff   int64
	fremdAppt, ownAppt, fremdOwner int64
}

func newStaffFix(t *testing.T) *staffFix {
	t.Helper()
	ts := newTestSalon(t)
	staffInviteLimit = newLimiter(20, time.Hour)
	f := &staffFix{testSalon: ts}
	ins := func(q string, args ...any) int64 {
		t.Helper()
		res, err := ts.a.db.Exec(q, args...)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	f.owner = ins(`INSERT INTO users (salon_id, email, last_login_at) VALUES (?, 'chefin@example.com', unixepoch())`, ts.salon)
	f.staff = ins(`INSERT INTO users (salon_id, email, role, staff_id, last_login_at) VALUES (?, 'anna@example.com', 'staff', ?, unixepoch())`, ts.salon, ts.anna)
	f.ownerC, f.staffC = staffSession(t, ts.a, f.owner), staffSession(t, ts.a, f.staff)
	if err := ts.a.db.QueryRow(`SELECT id FROM salons WHERE slug = 'fremd'`).Scan(&f.fremd); err != nil {
		t.Fatal(err)
	}
	f.fremdOwner = ins(`INSERT INTO users (salon_id, email) VALUES (?, 'fremd@example.com')`, f.fremd)
	f.fremdStaff = ins(`INSERT INTO staff (salon_id, name) VALUES (?, 'Fremdling')`, f.fremd)
	svc := ins(`INSERT INTO services (salon_id, name, duration_min) VALUES (?, 'Fremdschnitt', 30)`, f.fremd)
	f.fremdCust = ins(`INSERT INTO customers (salon_id, name, email) VALUES (?, 'Geheimkundin', 'geheim@example.com')`, f.fremd)
	start := time.Now().Add(48 * time.Hour).Truncate(time.Hour)
	f.fremdAppt = ins(`INSERT INTO appointments (salon_id, staff_id, service_id, customer_id, starts_at, ends_at, cancel_token)
		VALUES (?, ?, ?, ?, ?, ?, 'fremdtok')`, f.fremd, f.fremdStaff, svc, f.fremdCust, start.Unix(), start.Add(30*time.Minute).Unix())
	f.ownAppt = ins(`INSERT INTO appointments (salon_id, staff_id, service_id, customer_id, starts_at, ends_at, cancel_token)
		VALUES (?, ?, ?, ?, ?, ?, 'owntok')`, ts.salon, ts.ben, ts.schnitt, ts.cid, start.Unix(), start.Add(30*time.Minute).Unix())
	return f
}

func staffSession(t *testing.T, a *app, userID int64) string {
	t.Helper()
	tok := randomToken()
	if _, err := a.db.Exec(`INSERT INTO sessions (token_hash, user_id, expires_at) VALUES (?, ?, ?)`,
		hashToken(tok), userID, time.Now().Add(time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	return tok
}

// req schickt eine Dashboard-Anfrage an host (Standard kammzeit.test) mit Session-Cookie; POST mit gleicher Herkunft.
func (f *staffFix) req(cookie, method, path string, form url.Values) *httptest.ResponseRecorder {
	return f.reqHost("kammzeit.test", cookie, method, path, form)
}

func (f *staffFix) reqHost(host, cookie, method, path string, form url.Values) *httptest.ResponseRecorder {
	body := ""
	if form != nil {
		body = form.Encode()
	}
	r := httptest.NewRequest(method, "http://"+host+path, strings.NewReader(body))
	if method == http.MethodPost {
		r.Header.Set("Origin", "http://"+host)
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if cookie != "" {
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
	}
	w := httptest.NewRecorder()
	f.h.ServeHTTP(w, r)
	return w
}

// TestStaffOwnerRoutes: ein Mitarbeiter-Login kommt auf keine Inhaber-Route
// (403, nichts geändert), auf Kalender, Kunden und Meine Zeiten schon.
func TestStaffOwnerRoutes(t *testing.T) {
	f := newStaffFix(t)
	id := func(n int64) string { return fmt.Sprint(n) }
	ownerGET := []string{"/dashboard/leistungen", "/dashboard/leistungen/neu", "/dashboard/leistungen/" + id(f.schnitt),
		"/dashboard/team", "/dashboard/team/neu", "/dashboard/team/" + id(f.anna), "/dashboard/team/" + id(f.ben),
		"/dashboard/einstellungen", "/dashboard/einstellungen/export", "/dashboard/einstellungen/loeschen",
		"/dashboard/abo", "/dashboard/kunden.csv", "/dashboard/kunden/" + id(f.cid) + "/loeschen"}
	ownerPOST := []string{"/dashboard/leistungen/neu", "/dashboard/leistungen/" + id(f.schnitt),
		"/dashboard/leistungen/" + id(f.schnitt) + "/verschieben", "/dashboard/leistungen/" + id(f.schnitt) + "/loeschen",
		"/dashboard/team/neu", "/dashboard/team/" + id(f.ben), "/dashboard/team/" + id(f.ben) + "/loeschen",
		"/dashboard/team/" + id(f.ben) + "/zugang", "/dashboard/team/" + id(f.anna) + "/zugang/entfernen",
		"/dashboard/einstellungen", "/dashboard/einstellungen/loeschen", "/dashboard/einstellungen/news",
		"/dashboard/abo/checkout", "/dashboard/abo/portal", "/dashboard/kunden/" + id(f.cid) + "/loeschen"}
	form := url.Values{"name": {"Übernahme"}, "email": {"x@example.com"}, "bestaetigen": {"test"}, "richtung": {"hoch"}}
	for _, p := range ownerGET {
		w := f.req(f.staffC, "GET", p, nil)
		if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "Nur für Inhaber") {
			t.Errorf("staff GET %s: %d", p, w.Code)
		}
		if w := f.req(f.ownerC, "GET", p, nil); w.Code != http.StatusOK {
			t.Errorf("owner GET %s: %d", p, w.Code)
		}
	}
	for _, p := range ownerPOST {
		if w := f.req(f.staffC, "POST", p, form); w.Code != http.StatusForbidden {
			t.Errorf("staff POST %s: %d", p, w.Code)
		}
	}
	// Nichts davon ist passiert.
	for q, want := range map[string]int{
		`SELECT COUNT(*) FROM customers WHERE salon_id = ` + id(f.salon):                     1,
		`SELECT COUNT(*) FROM services WHERE salon_id = ` + id(f.salon) + ` AND active = 1`:  2,
		`SELECT COUNT(*) FROM staff WHERE salon_id = ` + id(f.salon) + ` AND name = 'Ben'`:   1,
		`SELECT COUNT(*) FROM users WHERE salon_id = ` + id(f.salon):                         2,
		`SELECT COUNT(*) FROM salons WHERE id = ` + id(f.salon) + ` AND name = 'Salon Test'`: 1,
	} {
		if n := f.count(t, q); n != want {
			t.Errorf("%s = %d, want %d", q, n, want)
		}
	}

	// Erlaubt: Kalender, Kunden, Meine Zeiten.
	for _, p := range []string{"/dashboard", "/dashboard?ansicht=woche", "/dashboard/kunden", "/dashboard/kunden/" + id(f.cid),
		"/dashboard/kunden/neu", "/dashboard/termine/neu", "/dashboard/termine/" + id(f.ownAppt), "/dashboard/meine-zeiten"} {
		if w := f.req(f.staffC, "GET", p, nil); w.Code != http.StatusOK {
			t.Errorf("staff GET %s: %d", p, w.Code)
		}
	}
	// Notiz und Absage an Bens Termin (Tresen: für alle Mitarbeiter), Kundenkontakt bearbeiten.
	if w := f.req(f.staffC, "POST", "/dashboard/termine/"+id(f.ownAppt)+"/notiz", url.Values{"notiz": {"Kommt 5 Min später"}}); w.Code != http.StatusSeeOther {
		t.Errorf("Notiz: %d %s", w.Code, w.Body)
	}
	if w := f.req(f.staffC, "POST", "/dashboard/termine/"+id(f.ownAppt)+"/absagen", url.Values{}); w.Code != http.StatusSeeOther {
		t.Errorf("Absagen: %d %s", w.Code, w.Body)
	}
	if n := f.count(t, `SELECT COUNT(*) FROM appointments WHERE id = ? AND status = 'cancelled' AND note = 'Kommt 5 Min später'`, f.ownAppt); n != 1 {
		t.Error("Notiz/Absage nicht gespeichert")
	}
	if w := f.req(f.staffC, "POST", "/dashboard/kunden/"+id(f.cid), url.Values{"name": {"Kim"}, "email": {"kim@example.com"}, "phone": {"0170 1"}, "notes": {"mag es kurz"}}); w.Code != http.StatusSeeOther {
		t.Errorf("Kunde bearbeiten: %d %s", w.Code, w.Body)
	}
	if n := f.count(t, `SELECT COUNT(*) FROM customers WHERE id = ? AND notes = 'mag es kurz' AND phone = '0170 1'`, f.cid); n != 1 {
		t.Error("Kundennotiz nicht gespeichert")
	}
}

// TestStaffNav: Menü und Seiten ohne Inhaber-Sachen, Rolle sichtbar; englisch über die .com-Domain.
func TestStaffNav(t *testing.T) {
	f := newStaffFix(t)
	staff := f.req(f.staffC, "GET", "/dashboard", nil).Body.String()
	owner := f.req(f.ownerC, "GET", "/dashboard", nil).Body.String()
	for _, h := range []string{`href="/dashboard/leistungen"`, `href="/dashboard/team"`, `href="/dashboard/einstellungen"`, `href="/dashboard/abo"`} {
		if strings.Contains(staff, h) {
			t.Errorf("Mitarbeiter sieht %s", h)
		}
		if !strings.Contains(owner, h) {
			t.Errorf("Inhaber sieht %s nicht", h)
		}
	}
	if !strings.Contains(staff, `href="/dashboard/meine-zeiten"`) || !strings.Contains(staff, "Anna · </span>Mitarbeiter") {
		t.Errorf("Meine Zeiten oder Rolle fehlt:\n%s", staff)
	}
	if strings.Contains(owner, "meine-zeiten") || strings.Contains(owner, "· </span>Mitarbeiter") {
		t.Error("Inhaber sieht Mitarbeiter-Menü")
	}
	if b := f.req(f.staffC, "GET", "/dashboard/kunden", nil).Body.String(); strings.Contains(b, "kunden.csv") {
		t.Error("CSV-Export für Mitarbeiter sichtbar")
	}
	if b := f.req(f.staffC, "GET", fmt.Sprintf("/dashboard/kunden/%d", f.cid), nil).Body.String(); strings.Contains(b, "/loeschen") {
		t.Error("Kunde löschen für Mitarbeiter sichtbar")
	}

	// Englisch
	f.a.cfg.Domains = parseDomains("kammzeit.test=de,kammzeit-en.test=en", "")
	f.h = f.a.routes()
	en := f.reqHost("kammzeit-en.test", f.staffC, "GET", "/dashboard/meine-zeiten", nil).Body.String()
	for _, want := range []string{"My hours", "Staff member", "When you work.", "Save"} {
		if !strings.Contains(en, want) {
			t.Errorf("englisch: %q fehlt", want)
		}
	}
	w := f.reqHost("kammzeit-en.test", f.staffC, "GET", "/dashboard/abo", nil)
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "Owners only") || !strings.Contains(w.Body.String(), "Only the salon owner can do this") {
		t.Errorf("englische 403-Seite: %d", w.Code)
	}
	for path, wants := range map[string][]string{
		fmt.Sprintf("/dashboard/team/%d", f.ben):                  {"Dashboard login", "Login email", "Send invitation", "Nobody needs a password."},
		fmt.Sprintf("/dashboard/team/%d?zugang=entfernt", f.anna): {"Access removed.", "Login active", "Remove access", "Last login: "},
		fmt.Sprintf("/dashboard/team/%d?zugang=vergeben", f.ben):  {"already has a Kammzeit account"},
		"/dashboard/team": {"Login active", "no login"},
	} {
		b := f.reqHost("kammzeit-en.test", f.ownerC, "GET", path, nil).Body.String()
		for _, want := range wants {
			if !strings.Contains(b, want) {
				t.Errorf("englisch %s: %q fehlt", path, want)
			}
		}
		for _, de := range []string{"Zugang", "eingeladen", "Login fürs", "Zuletzt"} {
			if strings.Contains(b, de) {
				t.Errorf("englisch %s: deutsch %q", path, de)
			}
		}
	}
	if b := f.reqHost("kammzeit-en.test", "", "GET", "/login?pausiert", nil).Body.String(); !strings.Contains(b, "Access paused") || !strings.Contains(b, "Nothing has been deleted.") {
		t.Error("englische Pause-Seite")
	}
}

// TestStaffTenantIsolation: ein Mitarbeiter von Salon A sieht und ändert nichts von Salon B.
func TestStaffTenantIsolation(t *testing.T) {
	f := newStaffFix(t)
	for _, p := range []string{fmt.Sprintf("/dashboard/kunden/%d", f.fremdCust), fmt.Sprintf("/dashboard/termine/%d", f.fremdAppt)} {
		if w := f.req(f.staffC, "GET", p, nil); w.Code != http.StatusNotFound {
			t.Errorf("GET %s: %d", p, w.Code)
		}
	}
	if w := f.req(f.staffC, "POST", fmt.Sprintf("/dashboard/termine/%d/absagen", f.fremdAppt), url.Values{}); w.Code != http.StatusNotFound {
		t.Errorf("fremden Termin absagen: %d", w.Code)
	}
	if w := f.req(f.staffC, "POST", fmt.Sprintf("/dashboard/kunden/%d", f.fremdCust), url.Values{"name": {"x"}, "notes": {"gekapert"}}); w.Code != http.StatusNotFound {
		t.Errorf("fremden Kunden bearbeiten: %d", w.Code)
	}
	if f.count(t, `SELECT COUNT(*) FROM appointments WHERE id = ? AND status = 'booked'`, f.fremdAppt) != 1 ||
		f.count(t, `SELECT COUNT(*) FROM customers WHERE id = ? AND notes = ''`, f.fremdCust) != 1 {
		t.Fatal("fremde Daten geändert")
	}
	for _, p := range []string{"/dashboard/kunden", "/dashboard/kunden?q=Geheim", "/dashboard?ansicht=woche"} {
		b := f.req(f.staffC, "GET", p, nil).Body.String()
		if strings.Contains(b, "Geheimkundin") || strings.Contains(b, "Fremdling") {
			t.Errorf("%s zeigt Daten von Salon B", p)
		}
	}
	// Inhaber A kann Mitarbeiter von B weder einladen noch entfernen.
	if w := f.req(f.ownerC, "POST", fmt.Sprintf("/dashboard/team/%d/zugang", f.fremdStaff), url.Values{"email": {"neu@example.com"}}); w.Code != http.StatusNotFound {
		t.Errorf("fremden Mitarbeiter einladen: %d", w.Code)
	}
	f.exec(t, `INSERT INTO users (salon_id, email, role, staff_id) VALUES (?, 'fremdstaff@example.com', 'staff', ?)`, f.fremd, f.fremdStaff)
	if w := f.req(f.ownerC, "POST", fmt.Sprintf("/dashboard/team/%d/zugang/entfernen", f.fremdStaff), nil); w.Code != http.StatusNotFound {
		t.Errorf("fremden Zugang entfernen: %d", w.Code)
	}
	if f.count(t, `SELECT COUNT(*) FROM users WHERE email IN ('neu@example.com', 'fremdstaff@example.com')`) != 1 {
		t.Error("fremde Logins angelegt oder gelöscht")
	}
	// Zeigt staff_id (manipuliert) auf einen Mitarbeiter von B, gibt es keinen Zugang.
	f.exec(t, `UPDATE users SET staff_id = ? WHERE id = ?`, f.fremdStaff, f.staff)
	if w := f.req(f.staffC, "GET", "/dashboard", nil); w.Code != http.StatusFound || w.Header().Get("Location") != "/login?pausiert" {
		t.Errorf("staff_id aus Salon B: %d %s", w.Code, w.Header().Get("Location"))
	}
}

// TestStaffProGate: ohne Pro kein Einladen, bestehende Logins pausiert (nicht gelöscht).
func TestStaffProGate(t *testing.T) {
	f := newStaffFix(t)
	f.exec(t, `UPDATE salons SET plan = 'free' WHERE id = ?`, f.salon)

	w := f.req(f.staffC, "GET", "/dashboard/kunden", nil)
	if w.Code != http.StatusFound || w.Header().Get("Location") != "/login?pausiert" {
		t.Fatalf("pausierter Login: %d %s", w.Code, w.Header().Get("Location"))
	}
	if f.count(t, `SELECT COUNT(*) FROM sessions WHERE user_id = ?`, f.staff) != 0 || f.count(t, `SELECT COUNT(*) FROM users WHERE id = ?`, f.staff) != 1 {
		t.Fatal("Session nicht beendet oder Login gelöscht")
	}
	if b := f.req("", "GET", "/login?pausiert", nil).Body.String(); !strings.Contains(b, "Zugang pausiert") {
		t.Error("Login-Seite erklärt die Pause nicht")
	}
	// htmx-Anfrage: Weiterleitung per Header.
	c := staffSession(t, f.a, f.staff)
	r := httptest.NewRequest("GET", "http://kammzeit.test/dashboard/kunden?q=k", nil)
	r.Header.Set("HX-Request", "true")
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: c})
	hw := httptest.NewRecorder()
	f.h.ServeHTTP(hw, r)
	if hw.Code != http.StatusUnauthorized || hw.Header().Get("HX-Redirect") != "/login?pausiert" {
		t.Errorf("htmx: %d %q", hw.Code, hw.Header().Get("HX-Redirect"))
	}
	// Login-Link: Bestätigung und Einlösen sagen "pausiert", keine Session.
	tok, err := f.a.newLoginToken(f.staff)
	if err != nil {
		t.Fatal(err)
	}
	if w := f.req("", "GET", "/login/"+tok, nil); w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "pausiert") {
		t.Errorf("Login-Bestätigung: %d", w.Code)
	}
	if w := f.req("", "POST", "/login/"+tok, url.Values{}); w.Code != http.StatusForbidden || len(w.Result().Cookies()) != 0 {
		t.Errorf("Login einlösen: %d %v", w.Code, w.Result().Cookies())
	}
	// Der Inhaber kommt weiter rein, kann aber nicht einladen.
	if w := f.req(f.ownerC, "GET", "/dashboard", nil); w.Code != http.StatusOK {
		t.Errorf("Inhaber ohne Pro: %d", w.Code)
	}
	if w := f.req(f.ownerC, "POST", fmt.Sprintf("/dashboard/team/%d/zugang", f.ben), url.Values{"email": {"ben@example.com"}}); w.Code != http.StatusForbidden {
		t.Errorf("Einladen ohne Pro: %d", w.Code)
	}
	if f.count(t, `SELECT COUNT(*) FROM users WHERE email = 'ben@example.com'`) != 0 {
		t.Error("Login ohne Pro angelegt")
	}
	team := f.req(f.ownerC, "GET", fmt.Sprintf("/dashboard/team/%d", f.anna), nil).Body.String()
	if !strings.Contains(team, "Login pausiert") || !strings.Contains(team, "anna@example.com") || strings.Contains(team, `name="email"`) {
		t.Error("Team-Seite zeigt den pausierten Zugang nicht richtig")
	}
	if b := f.req(f.ownerC, "GET", fmt.Sprintf("/dashboard/team/%d", f.ben), nil).Body.String(); !strings.Contains(b, "Pro ansehen") || strings.Contains(b, `name="email"`) {
		t.Error("ohne Pro: Einladen statt Pro-Hinweis")
	}

	// Mit Pro geht es wieder; ein inaktiver Mitarbeiter bleibt draußen.
	f.exec(t, `UPDATE salons SET plan = 'pro' WHERE id = ?`, f.salon)
	c = staffSession(t, f.a, f.staff)
	if w := f.req(c, "GET", "/dashboard", nil); w.Code != http.StatusOK {
		t.Errorf("wieder Pro: %d", w.Code)
	}
	f.exec(t, `UPDATE staff SET active = 0 WHERE id = ?`, f.anna)
	if w := f.req(c, "GET", "/dashboard", nil); w.Code != http.StatusFound || w.Header().Get("Location") != "/login?pausiert" {
		t.Errorf("inaktiver Mitarbeiter: %d", w.Code)
	}
}

// TestStaffInvite: Einladung legt den richtigen Login an, Mail global eindeutig, erneut schicken.
func TestStaffInvite(t *testing.T) {
	f := newStaffFix(t)
	var logs syncBuf
	log.SetOutput(&logs)
	defer log.SetOutput(os.Stderr)
	cleo := int64(f.count(t, `INSERT INTO staff (salon_id, name) VALUES (?, 'Cleo') RETURNING id`, f.salon))
	inactive := int64(f.count(t, `INSERT INTO staff (salon_id, name, active) VALUES (?, 'Dora', 0) RETURNING id`, f.salon))
	invite := func(staff int64, email string) string {
		t.Helper()
		w := f.req(f.ownerC, "POST", fmt.Sprintf("/dashboard/team/%d/zugang", staff), url.Values{"email": {email}})
		if w.Code != http.StatusSeeOther {
			t.Fatalf("Einladen %s: %d %s", email, w.Code, w.Body)
		}
		loc, _ := url.Parse(w.Header().Get("Location"))
		return loc.Query().Get("zugang")
	}

	if code := invite(f.ben, " Ben@Example.com "); code != "eingeladen" {
		t.Fatalf("Code %q", code)
	}
	var salonID, staffID int64
	var role, lang, email string
	var terms sql.NullInt64
	err := f.a.db.QueryRow(`SELECT salon_id, role, staff_id, lang, email, terms_accepted_at FROM users WHERE email = 'ben@example.com'`).
		Scan(&salonID, &role, &staffID, &lang, &email, &terms)
	if err != nil || salonID != f.salon || role != "staff" || staffID != f.ben || lang != "de" || terms.Valid {
		t.Fatalf("Login: %v salon=%d role=%s staff=%d lang=%s", err, salonID, role, staffID, lang)
	}
	if n := f.count(t, `SELECT COUNT(*) FROM login_tokens t JOIN users u ON u.id = t.user_id WHERE u.email = 'ben@example.com'`); n != 1 {
		t.Errorf("Login-Links: %d", n)
	}
	if out := logs.String(); !strings.Contains(out, "an ben@example.com") || !strings.Contains(out, "/login/") || !strings.Contains(out, "Salon Test") {
		t.Errorf("Einladungsmail fehlt:\n%s", out)
	}

	// Noch einmal: kein zweiter Login, neuer Link.
	if code := invite(f.ben, "anders@example.com"); code != "erneut" {
		t.Errorf("erneut: %q", code)
	}
	if f.count(t, `SELECT COUNT(*) FROM users WHERE staff_id = ?`, f.ben) != 1 || f.count(t, `SELECT COUNT(*) FROM users WHERE email = 'anders@example.com'`) != 0 {
		t.Error("zweiter Login angelegt")
	}

	// Vergebene Adressen (auch andere Groß-/Kleinschreibung, auch aus anderen Salons), ungültige, inaktive.
	for _, c := range []struct {
		staff       int64
		email, code string
	}{
		{cleo, "chefin@example.com", "vergeben"},
		{cleo, "ANNA@example.com", "vergeben"},
		{cleo, "fremd@example.com", "vergeben"},
		{cleo, "keine-adresse", "mail"},
		{cleo, "", "mail"},
		{cleo, strings.Repeat("a", 200) + "@example.com", "mail"},
		{inactive, "dora@example.com", "inaktiv"},
	} {
		if code := invite(c.staff, c.email); code != c.code {
			t.Errorf("%q: %q, want %q", c.email, code, c.code)
		}
	}
	if f.count(t, `SELECT COUNT(*) FROM users WHERE staff_id IN (?, ?)`, cleo, inactive) != 0 {
		t.Error("Login trotz Fehler angelegt")
	}
	page := f.req(f.ownerC, "GET", fmt.Sprintf("/dashboard/team/%d?zugang=vergeben", cleo), nil).Body.String()
	if !strings.Contains(page, "schon ein Kammzeit-Konto") || !strings.Contains(page, `name="email"`) {
		t.Error("Meldung zur vergebenen Adresse fehlt")
	}

	// Team-Liste zeigt den Status, Team-Seite die Einladung.
	list := f.req(f.ownerC, "GET", "/dashboard/team", nil).Body.String()
	for _, want := range []string{"Login aktiv", "eingeladen", "kein Login"} {
		if !strings.Contains(list, want) {
			t.Errorf("Team-Liste: %q fehlt", want)
		}
	}
	ben := f.req(f.ownerC, "GET", fmt.Sprintf("/dashboard/team/%d?zugang=eingeladen", f.ben), nil).Body.String()
	if !strings.Contains(ben, "Einladung verschickt") || !strings.Contains(ben, "ben@example.com") || !strings.Contains(ben, "Einladung erneut schicken") {
		t.Error("Team-Seite zeigt die Einladung nicht")
	}

	// Englische Buchungsseite: Login und Mail auf Englisch.
	f.exec(t, `UPDATE salons SET lang = 'en' WHERE id = ?`, f.salon)
	invite(cleo, "cleo@example.com")
	if f.count(t, `SELECT COUNT(*) FROM users WHERE email = 'cleo@example.com' AND lang = 'en'`) != 1 || !strings.Contains(logs.String(), "Your Kammzeit access for Salon Test") {
		t.Error("englische Einladung")
	}

	// Rate-Limit pro Salon.
	staffInviteLimit = newLimiter(0, time.Hour)
	if code := invite(f.ben, ""); code != "zu-viele" {
		t.Errorf("Limit: %q", code)
	}
}

// TestStaffRevoke: Entfernen löscht Login und Sessions; Löschen des Mitarbeiters auch.
func TestStaffRevoke(t *testing.T) {
	f := newStaffFix(t)
	other := staffSession(t, f.a, f.staff) // zweites Gerät
	if w := f.req(f.staffC, "GET", "/dashboard", nil); w.Code != http.StatusOK {
		t.Fatalf("vorher: %d", w.Code)
	}
	w := f.req(f.ownerC, "POST", fmt.Sprintf("/dashboard/team/%d/zugang/entfernen", f.anna), nil)
	if w.Code != http.StatusSeeOther || !strings.Contains(w.Header().Get("Location"), "zugang=entfernt") {
		t.Fatalf("Entfernen: %d %s", w.Code, w.Header().Get("Location"))
	}
	if f.count(t, `SELECT COUNT(*) FROM users WHERE id = ?`, f.staff) != 0 || f.count(t, `SELECT COUNT(*) FROM sessions WHERE user_id = ?`, f.staff) != 0 {
		t.Fatal("Login oder Sessions noch da")
	}
	for _, c := range []string{f.staffC, other} {
		if w := f.req(c, "GET", "/dashboard", nil); w.Code != http.StatusFound || !strings.HasPrefix(w.Header().Get("Location"), "/login?weiter=") {
			t.Errorf("nach Entfernen: %d %s", w.Code, w.Header().Get("Location"))
		}
	}
	if w := f.req(f.ownerC, "POST", fmt.Sprintf("/dashboard/team/%d/zugang/entfernen", f.anna), nil); w.Code != http.StatusNotFound {
		t.Errorf("zweimal entfernen: %d", w.Code)
	}
	// Mitarbeiter ohne Termine löschen: Login verschwindet mit.
	f.exec(t, `DELETE FROM appointments WHERE staff_id = ?`, f.ben)
	f.exec(t, `INSERT INTO users (salon_id, email, role, staff_id) VALUES (?, 'ben@example.com', 'staff', ?)`, f.salon, f.ben)
	if w := f.req(f.ownerC, "POST", fmt.Sprintf("/dashboard/team/%d/loeschen", f.ben), nil); w.Code != http.StatusSeeOther {
		t.Fatalf("Mitarbeiter löschen: %d", w.Code)
	}
	if f.count(t, `SELECT COUNT(*) FROM users WHERE email = 'ben@example.com'`) != 0 {
		t.Error("Login des gelöschten Mitarbeiters noch da")
	}
}

// TestStaffOwnHours: Mitarbeiter ändern nur die eigenen Arbeitszeiten.
func TestStaffOwnHours(t *testing.T) {
	f := newStaffFix(t)
	hours := func(staff int64) int {
		return f.count(t, `SELECT COUNT(*) FROM working_hours WHERE staff_id = ?`, staff)
	}
	// Fremde IDs im Formular werden ignoriert, die ID kommt nur aus dem Login.
	form := url.Values{"von-1-0": {"10:00"}, "bis-1-0": {"16:00"}, "id": {fmt.Sprint(f.ben)}, "staff_id": {fmt.Sprint(f.ben)}}
	w := f.req(f.staffC, "POST", "/dashboard/meine-zeiten", form)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/dashboard/meine-zeiten?gespeichert" {
		t.Fatalf("Speichern: %d %s", w.Code, w.Body)
	}
	if hours(f.anna) != 1 || hours(f.ben) != 7 || f.count(t, `SELECT COUNT(*) FROM working_hours WHERE staff_id = ? AND weekday = 1 AND start_min = 600 AND end_min = 960`, f.anna) != 1 {
		t.Fatalf("Arbeitszeiten: Anna %d, Ben %d", hours(f.anna), hours(f.ben))
	}
	page := f.req(f.staffC, "GET", "/dashboard/meine-zeiten?gespeichert", nil).Body.String()
	if !strings.Contains(page, `value="10:00"`) || !strings.Contains(page, "Arbeitszeiten gespeichert.") {
		t.Error("Meine Zeiten zeigt die gespeicherten Zeiten nicht")
	}
	// Ungültig: 422, nichts geändert. Übertragen: nur Vorschau.
	if w := f.req(f.staffC, "POST", "/dashboard/meine-zeiten", url.Values{"von-2-0": {"18:00"}, "bis-2-0": {"09:00"}}); w.Code != http.StatusUnprocessableEntity {
		t.Errorf("ungültig: %d", w.Code)
	}
	if w := f.req(f.staffC, "POST", "/dashboard/meine-zeiten", url.Values{"von-1-0": {"08:00"}, "bis-1-0": {"12:00"}, "uebertragen": {"1"}}); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Noch nicht gespeichert") {
		t.Errorf("übertragen: %d", w.Code)
	}
	if hours(f.anna) != 1 {
		t.Error("ungültige Zeiten oder Vorschau gespeichert")
	}
	// Bens Formular bleibt zu, der Inhaber pflegt Zeiten unter Team.
	if w := f.req(f.staffC, "POST", fmt.Sprintf("/dashboard/team/%d", f.ben), url.Values{"name": {"Ben"}, "aktiv": {"1"}}); w.Code != http.StatusForbidden || hours(f.ben) != 7 {
		t.Errorf("fremde Zeiten über Team: %d", w.Code)
	}
	if w := f.req(f.ownerC, "GET", "/dashboard/meine-zeiten", nil); w.Code != http.StatusFound || w.Header().Get("Location") != "/dashboard/team" {
		t.Errorf("Inhaber: %d %s", w.Code, w.Header().Get("Location"))
	}
}

// TestStaffInviteMailRenders: Einladung in de und en, Link in Text und HTML, keine Template-Reste.
func TestStaffInviteMailRenders(t *testing.T) {
	f := newMailFix(t)
	for _, lang := range []string{"de", "en"} {
		site := f.a.siteURLLang(lang)
		link := site + "/login/abc123"
		subj, text, page, err := f.a.renderMail(lang, "staff-invite", map[string]any{
			"Link": link, "LoginURL": site + "/login", "Salon": f.haar.Name, "Name": "Anna"})
		if err != nil {
			t.Fatal(err)
		}
		for _, part := range []string{subj, text} {
			for _, bad := range mailLeftovers {
				if strings.Contains(part, bad) {
					t.Errorf("%s: %q in Betreff/Text", lang, bad)
				}
			}
		}
		if !strings.Contains(subj, "Haar & Bart <Studio>") || !strings.Contains(text, link) || !strings.Contains(text, site+"/login ") {
			t.Errorf("%s: Betreff/Text unvollständig:\n%s\n%s", lang, subj, text)
		}
		if !strings.Contains(page, template.HTMLEscapeString(link)) || !strings.Contains(page, "Haar &amp; Bart &lt;Studio&gt;") ||
			!strings.Contains(page, `<html lang="`+lang+`">`) || strings.Contains(page, "<script") {
			t.Errorf("%s: HTML unvollständig", lang)
		}
	}
}
