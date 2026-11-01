package main

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// queueTestApp: App mit temporärer DB, echten Templates und nur den Routen der Warteschlange.
func queueTestApp(t *testing.T) (*app, http.Handler) {
	t.Helper()
	db, err := openDB(t.TempDir() + "/q.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	tpl, err := template.New("").Funcs(templateFuncs).ParseFS(webFS, "web/*.html")
	if err != nil {
		t.Fatal(err)
	}
	a := &app{cfg: config{BaseDomain: "kammzeit.test", SiteURL: "http://kammzeit.test", PublicURL: "http://{slug}.kammzeit.test"}, db: db, tpl: tpl}
	mux := http.NewServeMux()
	a.routesQueue(mux)
	return a, securityHeaders(mux)
}

// queueTestSalon legt Salon (Plan, Sprache), Konto mit Session und einen
// Mitarbeiter an, der rund um die Uhr arbeitet (open = true) oder gar nicht.
func queueTestSalon(t *testing.T, a *app, slug, plan, lang string, open bool) (cookie string, s *salon) {
	t.Helper()
	res, err := a.db.Exec(`INSERT INTO salons (slug, name, plan, lang, queue_enabled) VALUES (?, ?, ?, ?, 1)`, slug, "Salon "+slug, plan, lang)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	res, err = a.db.Exec(`INSERT INTO users (salon_id, email) VALUES (?, ?)`, id, slug+"@example.com")
	if err != nil {
		t.Fatal(err)
	}
	userID, _ := res.LastInsertId()
	cookie = randomToken()
	if _, err := a.db.Exec(`INSERT INTO sessions (token_hash, user_id, expires_at) VALUES (?, ?, ?)`,
		hashToken(cookie), userID, time.Now().Add(time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	res, err = a.db.Exec(`INSERT INTO staff (salon_id, name) VALUES (?, 'Anna')`, id)
	if err != nil {
		t.Fatal(err)
	}
	staffID, _ := res.LastInsertId()
	if open {
		for wd := 0; wd < 7; wd++ {
			if _, err := a.db.Exec(`INSERT INTO working_hours (staff_id, weekday, start_min, end_min) VALUES (?, ?, 0, 1440)`, staffID, wd); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := a.db.Exec(`INSERT INTO services (salon_id, name, duration_min) VALUES (?, 'Haarschnitt', 30)`, id); err != nil {
		t.Fatal(err)
	}
	s, err = a.loadSalon(id)
	if err != nil {
		t.Fatal(err)
	}
	return cookie, s
}

// queueReq: host leer = Hauptdomain (Dashboard), sonst Salon-Subdomain. form != nil -> POST mit passender Origin.
func queueReq(h http.Handler, host, cookie, path string, form url.Values, hdr ...string) *httptest.ResponseRecorder {
	if host == "" {
		host = "kammzeit.test"
	}
	method, body := http.MethodGet, io.Reader(nil)
	if form != nil {
		method, body = http.MethodPost, strings.NewReader(form.Encode())
	}
	r := httptest.NewRequest(method, "http://"+host+path, body)
	r.RemoteAddr = "192.0.2.1:1234"
	if cookie != "" {
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
	}
	if form != nil {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Origin", "http://"+host)
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		if hdr[i] == "RemoteAddr" {
			r.RemoteAddr = hdr[i+1]
			continue
		}
		r.Header.Set(hdr[i], hdr[i+1])
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func queueStatusOf(t *testing.T, a *app, token string) string {
	t.Helper()
	var st string
	if err := a.db.QueryRow(`SELECT status FROM queue_entries WHERE token = ?`, token).Scan(&st); err != nil {
		t.Fatal(err)
	}
	return st
}

// Nummern laufen pro Salon und Tag (Salon-Zeitzone) ab 1; neuer Tag beginnt
// wieder bei 1 und räumt die alten Einträge weg; der Tagesdeckel greift.
func TestQueueNumbers(t *testing.T) {
	a, _ := queueTestApp(t)
	_, s1 := queueTestSalon(t, a, "eins", "pro", "de", true)
	_, s2 := queueTestSalon(t, a, "zwei", "pro", "de", true)
	ctx := context.Background()
	// 23:30 UTC ist in Berlin schon der nächste Tag.
	late := time.Date(2026, 11, 2, 23, 30, 0, 0, time.UTC)
	for want := 1; want <= 3; want++ {
		nr, tok, err := a.queueAdd(ctx, s1, late, "Kim", "", 0, 0, 0)
		if err != nil || nr != want || len(tok) != 32 {
			t.Fatalf("Salon 1, Eintrag %d: nr %d, token %q, err %v", want, nr, tok, err)
		}
	}
	if nr, _, err := a.queueAdd(ctx, s2, late, "Lea", "", 0, 0, 0); err != nil || nr != 1 {
		t.Fatalf("Salon 2 beginnt nicht bei 1: %d %v", nr, err)
	}
	var day string
	if err := a.db.QueryRow(`SELECT day FROM queue_entries WHERE salon_id = ? LIMIT 1`, s1.ID).Scan(&day); err != nil || day != "2026-11-03" {
		t.Fatalf("Tag nicht in Salon-Zeitzone: %q %v", day, err)
	}
	next := late.Add(24 * time.Hour)
	if nr, _, err := a.queueAdd(ctx, s1, next, "Kim", "", 0, 0, 0); err != nil || nr != 1 {
		t.Fatalf("neuer Tag beginnt nicht bei 1: %d %v", nr, err)
	}
	if n := queueCount(t, a, `salon_id = ?`, s1.ID); n != 1 {
		t.Fatalf("alte Einträge nicht gelöscht: %d", n)
	}
	if n := queueCount(t, a, `salon_id = ?`, s2.ID); n != 1 {
		t.Fatalf("Aufräumen hat fremden Salon erwischt: %d", n)
	}
	if _, _, err := a.queueAdd(ctx, s1, next, "Kim", "", 0, 0, 2); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.queueAdd(ctx, s1, next, "Kim", "", 0, 0, 2); !errors.Is(err, errQueueFull) {
		t.Fatalf("Tagesdeckel greift nicht: %v", err)
	}
}

func queueCount(t *testing.T, a *app, where string, args ...any) int {
	t.Helper()
	var n int
	if err := a.db.QueryRow(`SELECT COUNT(*) FROM queue_entries WHERE `+where, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestQueueEstimate(t *testing.T) {
	for _, c := range []struct {
		durations []int
		staff     int
		want      int
	}{
		{nil, 2, 0},
		{[]int{30}, 1, 30},
		{[]int{0}, 1, 20},      // ohne Leistung 20 Min.
		{[]int{30, 0}, 2, 25},  // 50 / 2
		{[]int{30, 45}, 2, 40}, // 37,5 -> 38 -> auf 5 Min. aufgerundet
		{[]int{10}, 0, 10},     // keine Mitarbeiter heute: wie einer
		{[]int{7}, 1, 10},
	} {
		if got := queueWait(c.durations, c.staff); got != c.want {
			t.Errorf("queueWait(%v, %d) = %d, want %d", c.durations, c.staff, got, c.want)
		}
	}
	now := time.Now()
	list := []queueEntry{
		{Number: 1, Status: "serving", Duration: 60, Created: now.Add(-40 * time.Minute)},
		{Number: 2, Status: "called", Duration: 30},
		{Number: 3, Status: "waiting", Duration: 0},
		{Number: 4, Status: "done", Duration: 90},
		{Number: 5, Status: "waiting", Duration: 45},
	}
	newWait := queueEstimate(list, 2, now)
	if list[0].SinceMin != 40 || list[1].WaitMin != 0 || list[1].Ahead != 0 {
		t.Fatalf("bedient/aufgerufen falsch: %+v %+v", list[0], list[1])
	}
	if list[2].Ahead != 1 || list[2].WaitMin != 15 { // 30 / 2
		t.Fatalf("Nummer 3: %+v", list[2])
	}
	if list[4].Ahead != 2 || list[4].WaitMin != 25 { // (30 + 20) / 2
		t.Fatalf("Nummer 5: %+v", list[4])
	}
	if newWait != 50 { // (30 + 20 + 45) / 2 = 47,5 -> 50
		t.Fatalf("Wartezeit für Neue %d", newWait)
	}
}

// Gratis-Plan: Dashboard zeigt den Pro-Hinweis, ändern geht nicht, öffentlich 404.
func TestQueueProGate(t *testing.T) {
	a, h := queueTestApp(t)
	free, fs := queueTestSalon(t, a, "frei", "free", "de", true)
	pro, _ := queueTestSalon(t, a, "profi", "pro", "de", true)
	w := queueReq(h, "", free, "/dashboard/warteschlange", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `href="/dashboard/abo"`) || strings.Contains(w.Body.String(), "queue-liste") {
		t.Fatalf("Gratis-Dashboard ohne Pro-Hinweis: %d", w.Code)
	}
	for _, p := range []string{"/dashboard/warteschlange/qr", "/dashboard/warteschlange/anzeige"} {
		if w := queueReq(h, "", free, p, nil); !strings.Contains(w.Body.String(), `href="/dashboard/abo"`) {
			t.Fatalf("%s ohne Pro-Hinweis", p)
		}
	}
	if w := queueReq(h, "", free, "/dashboard/warteschlange/einstellungen", url.Values{"an": {"0"}}); w.Code != http.StatusForbidden {
		t.Fatalf("Gratis darf schalten: %d", w.Code)
	}
	if w := queueReq(h, "", free, "/dashboard/warteschlange/neu", url.Values{"name": {"Kim"}}); w.Code != http.StatusForbidden || queueCount(t, a, `salon_id = ?`, fs.ID) != 0 {
		t.Fatalf("Gratis darf einreihen: %d", w.Code)
	}
	for _, p := range []string{"/warteschlange", "/warteschlange/" + randomToken()} {
		if w := queueReq(h, "frei.kammzeit.test", "", p, nil); w.Code != http.StatusNotFound {
			t.Fatalf("öffentlich %s im Gratis-Plan: %d", p, w.Code)
		}
	}
	if w := queueReq(h, "frei.kammzeit.test", "", "/warteschlange", url.Values{"name": {"Kim"}}); w.Code != http.StatusNotFound {
		t.Fatalf("Einreihen im Gratis-Plan: %d", w.Code)
	}
	if w := queueReq(h, "", pro, "/dashboard/warteschlange", nil); w.Code != 200 || !strings.Contains(w.Body.String(), `id="queue-liste"`) {
		t.Fatalf("Pro-Dashboard: %d", w.Code)
	}
	if w := queueReq(h, "profi.kammzeit.test", "", "/warteschlange", nil); w.Code != 200 || !strings.Contains(w.Body.String(), `name="name"`) {
		t.Fatalf("Pro öffentlich: %d", w.Code)
	}
	// Hinweis auf der Buchungsseite nur bei Pro, eingeschaltet und geöffnet.
	ps, _ := scanSalon(a.db.QueryRow(salonCols + `WHERE slug = 'profi'`))
	if a.queueBookingLink(fs) || !a.queueBookingLink(ps) {
		t.Fatal("queueBookingLink falsch")
	}
	a.db.Exec(`UPDATE salons SET queue_enabled = 0 WHERE id = ?`, ps.ID)
	if a.queueBookingLink(ps) {
		t.Fatal("queueBookingLink trotz ausgeschaltet")
	}
	// Einzelbetrieb: alles erlaubt, auch ohne Pro.
	a.cfg.SingleTenant = true
	if !a.queueAllowed(fs) {
		t.Fatal("Einzelbetrieb ohne Warteschlange")
	}
}

// Fremde Einträge lassen sich weder ändern noch ansehen; fremde Leistungen nicht verwenden.
func TestQueueMandantentrennung(t *testing.T) {
	a, h := queueTestApp(t)
	cookieA, sa := queueTestSalon(t, a, "anna", "pro", "de", true)
	_, sb := queueTestSalon(t, a, "bert", "pro", "de", true)
	_, tokB, err := a.queueAdd(context.Background(), sb, time.Now(), "Geheim Bert", "", 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	var idB int64
	a.db.QueryRow(`SELECT id FROM queue_entries WHERE token = ?`, tokB).Scan(&idB)
	for _, act := range []string{"aufrufen", "bedienen", "fertig", "nichtda", "entfernen"} {
		queueReq(h, "", cookieA, "/dashboard/warteschlange/"+strconv.FormatInt(idB, 10)+"/"+act, url.Values{})
		if st := queueStatusOf(t, a, tokB); st != "waiting" {
			t.Fatalf("%s hat fremden Eintrag geändert: %s", act, st)
		}
	}
	if w := queueReq(h, "", cookieA, "/dashboard/warteschlange", nil); strings.Contains(w.Body.String(), "Geheim Bert") {
		t.Fatal("fremder Eintrag im Dashboard")
	}
	if w := queueReq(h, "", cookieA, "/dashboard/warteschlange/anzeige/stand", nil); strings.Contains(w.Body.String(), "<li>1</li>") {
		t.Fatal("fremde Nummer in der Anzeige")
	}
	if w := queueReq(h, "anna.kammzeit.test", "", "/warteschlange/"+tokB, nil); w.Code != http.StatusNotFound {
		t.Fatalf("fremder Status über eigene Subdomain: %d", w.Code)
	}
	queueReq(h, "anna.kammzeit.test", "", "/warteschlange/"+tokB+"/gehen", url.Values{})
	if st := queueStatusOf(t, a, tokB); st != "waiting" {
		t.Fatalf("fremden Platz verlassen: %s", st)
	}
	var serviceB, staffB int64
	a.db.QueryRow(`SELECT id FROM services WHERE salon_id = ?`, sb.ID).Scan(&serviceB)
	a.db.QueryRow(`SELECT id FROM staff WHERE salon_id = ?`, sb.ID).Scan(&staffB)
	for _, f := range []url.Values{
		{"name": {"Kim"}, "service": {strconv.FormatInt(serviceB, 10)}},
		{"name": {"Kim"}, "staff": {strconv.FormatInt(staffB, 10)}},
	} {
		if w := queueReq(h, "", cookieA, "/dashboard/warteschlange/neu", f); w.Code != http.StatusBadRequest {
			t.Fatalf("fremde Leistung/Mitarbeiter angenommen: %d", w.Code)
		}
	}
	if w := queueReq(h, "anna.kammzeit.test", "", "/warteschlange", url.Values{"name": {"Kim"}, "service": {strconv.FormatInt(serviceB, 10)}}); w.Code != http.StatusBadRequest {
		t.Fatalf("fremde Leistung öffentlich angenommen: %d", w.Code)
	}
	if n := queueCount(t, a, `salon_id = ?`, sa.ID); n != 0 {
		t.Fatalf("%d Einträge bei Salon A", n)
	}
}

// Ablauf: öffentlich anstellen, aufrufen (Mail-Merker), bedienen, fertig;
// ungültige Übergänge ändern nichts; "Ich gehe doch nicht".
func TestQueueStatusFlow(t *testing.T) {
	a, h := queueTestApp(t)
	cookie, s := queueTestSalon(t, a, "ablauf", "pro", "de", true)
	host := "ablauf.kammzeit.test"
	w := queueReq(h, host, "", "/warteschlange", url.Values{"name": {" Kim "}, "email": {"Kim@Example.com"}}, "RemoteAddr", "192.0.2.10:1")
	loc := w.Header().Get("Location")
	if w.Code != http.StatusSeeOther || !strings.HasPrefix(loc, "/warteschlange/") {
		t.Fatalf("Anstellen: %d %q %s", w.Code, loc, w.Body.String())
	}
	tok := strings.TrimPrefix(loc, "/warteschlange/")
	var ck string
	for _, c := range w.Result().Cookies() {
		if c.Name == queueCookie {
			ck = c.Value
		}
	}
	if ck != tok {
		t.Fatal("kein Cookie mit dem Platz")
	}
	var name, email string
	var number int
	a.db.QueryRow(`SELECT name, email, number FROM queue_entries WHERE token = ?`, tok).Scan(&name, &email, &number)
	if name != "Kim" || email != "kim@example.com" || number != 1 {
		t.Fatalf("gespeichert: %q %q %d", name, email, number)
	}
	// Mit Cookie führt /warteschlange direkt zum Platz.
	r := httptest.NewRequest("GET", "http://"+host+"/warteschlange", nil)
	r.AddCookie(&http.Cookie{Name: queueCookie, Value: tok})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != loc {
		t.Fatalf("Cookie-Weiterleitung: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	w = queueReq(h, host, "", loc, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `class="queue-number">1<`) || !strings.Contains(w.Body.String(), `hx-trigger="every 15s"`) {
		t.Fatalf("Statusseite: %d", w.Code)
	}
	var id int64
	a.db.QueryRow(`SELECT id FROM queue_entries WHERE token = ?`, tok).Scan(&id)
	act := func(name string) *httptest.ResponseRecorder {
		return queueReq(h, "", cookie, "/dashboard/warteschlange/"+strconv.FormatInt(id, 10)+"/"+name, url.Values{})
	}
	if w := act("fertig"); w.Code != http.StatusSeeOther {
		t.Fatalf("Aktion ohne htmx: %d", w.Code)
	}
	if st := queueStatusOf(t, a, tok); st != "done" {
		t.Fatalf("fertig: %s", st)
	}
	if act("aufrufen"); queueStatusOf(t, a, tok) != "done" {
		t.Fatal("fertig -> aufgerufen erlaubt")
	}
	a.db.Exec(`UPDATE queue_entries SET status = 'waiting', done_at = NULL WHERE id = ?`, id)
	w = queueReq(h, "", cookie, "/dashboard/warteschlange/"+strconv.FormatInt(id, 10)+"/aufrufen", url.Values{}, "HX-Request", "true")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `id="queue-liste"`) || strings.Contains(w.Body.String(), "<html") {
		t.Fatalf("htmx-Antwort ist nicht die Liste: %d", w.Code)
	}
	var notified, called *int64
	a.db.QueryRow(`SELECT notified_at, called_at FROM queue_entries WHERE id = ?`, id).Scan(&notified, &called)
	if queueStatusOf(t, a, tok) != "called" || notified == nil || called == nil {
		t.Fatalf("aufrufen: notified %v called %v", notified, called)
	}
	if w := queueReq(h, host, "", loc+"/stand", nil); !strings.Contains(w.Body.String(), "Du bist dran!") || strings.Contains(w.Body.String(), "<html") {
		t.Fatalf("Fragment nach Aufruf: %s", w.Body.String())
	}
	act("bedienen")
	if queueStatusOf(t, a, tok) != "serving" {
		t.Fatal("bedienen")
	}
	act("nichtda") // aus "serving" nicht erlaubt
	if queueStatusOf(t, a, tok) != "serving" {
		t.Fatal("bedient -> nicht da erlaubt")
	}
	if act("unsinn").Code != http.StatusNotFound {
		t.Fatal("unbekannte Aktion")
	}
	act("fertig")
	if queueStatusOf(t, a, tok) != "done" {
		t.Fatal("fertig")
	}
	if w := queueReq(h, host, "", loc, nil); strings.Contains(w.Body.String(), "hx-trigger") || strings.Contains(w.Body.String(), "/gehen") {
		t.Fatal("abgeschlossener Platz pollt noch oder bietet Gehen an")
	}

	// Zweiter Kunde geht selbst; Gehen nur mit eigener Herkunft.
	_, tok2, _ := a.queueAdd(context.Background(), s, time.Now(), "Lea", "", 0, 0, 0)
	if w := queueReq(h, host, "", "/warteschlange/"+tok2+"/gehen", url.Values{}, "Origin", "https://evil.test"); w.Code != http.StatusForbidden {
		t.Fatalf("fremde Herkunft: %d", w.Code)
	}
	w = queueReq(h, host, "", "/warteschlange/"+tok2+"/gehen", url.Values{}, "HX-Request", "true")
	if queueStatusOf(t, a, tok2) != "left" || !strings.Contains(w.Body.String(), "nicht mehr in der Warteschlange") {
		t.Fatalf("gehen: %s", w.Body.String())
	}
	// Einstellungen: aus und Hinweis.
	queueReq(h, "", cookie, "/dashboard/warteschlange/einstellungen", url.Values{"an": {"0"}})
	queueReq(h, "", cookie, "/dashboard/warteschlange/einstellungen", url.Values{"hinweis": {"Bitte bleib in der Nähe."}})
	var on bool
	var note string
	a.db.QueryRow(`SELECT queue_enabled, queue_note FROM salons WHERE id = ?`, s.ID).Scan(&on, &note)
	if on || note != "Bitte bleib in der Nähe." {
		t.Fatalf("Einstellungen: %v %q", on, note)
	}
	if w := queueReq(h, "", cookie, "/dashboard/warteschlange/einstellungen", url.Values{"hinweis": {strings.Repeat("x", 301)}}); w.Code != http.StatusBadRequest {
		t.Fatalf("zu langer Hinweis: %d", w.Code)
	}
	w = queueReq(h, host, "", "/warteschlange", nil)
	if !strings.Contains(w.Body.String(), "gerade geschlossen") || strings.Contains(w.Body.String(), `name="name"`) {
		t.Fatal("ausgeschaltet zeigt trotzdem das Formular")
	}
	if w := queueReq(h, host, "", "/warteschlange", url.Values{"name": {"Max"}}, "RemoteAddr", "192.0.2.11:1"); w.Code != http.StatusConflict || queueCount(t, a, `name = 'Max'`) != 0 {
		t.Fatalf("ausgeschaltet angenommen: %d", w.Code)
	}
	// Am Tresen einreihen geht trotzdem.
	if w := queueReq(h, "", cookie, "/dashboard/warteschlange/neu", url.Values{"name": {"Tresen"}}); w.Code != http.StatusSeeOther ||
		w.Header().Get("Location") != "/dashboard/warteschlange?gespeichert=neu&nr=3" {
		t.Fatalf("am Tresen: %d %q", w.Code, w.Header().Get("Location"))
	}
}

// Ohne Arbeitszeiten heute ist geschlossen.
func TestQueueClosed(t *testing.T) {
	a, h := queueTestApp(t)
	queueTestSalon(t, a, "zu", "pro", "de", false)
	w := queueReq(h, "zu.kammzeit.test", "", "/warteschlange", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Gerade ist geschlossen") {
		t.Fatalf("geschlossen: %d", w.Code)
	}
	if w := queueReq(h, "zu.kammzeit.test", "", "/warteschlange", url.Values{"name": {"Kim"}}, "RemoteAddr", "192.0.2.20:1"); w.Code != http.StatusConflict {
		t.Fatalf("geschlossen angenommen: %d", w.Code)
	}
}

func TestQueueHoneypotAndLimits(t *testing.T) {
	a, h := queueTestApp(t)
	_, s := queueTestSalon(t, a, "spam", "pro", "de", true)
	host := "spam.kammzeit.test"
	w := queueReq(h, host, "", "/warteschlange", url.Values{"name": {"Bot"}, "firma": {"ACME"}}, "RemoteAddr", "198.51.100.1:1")
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/warteschlange" || queueCount(t, a, `salon_id = ?`, s.ID) != 0 {
		t.Fatalf("Honeypot: %d %q", w.Code, w.Header().Get("Location"))
	}
	if w := queueReq(h, host, "", "/warteschlange", url.Values{"name": {"Kim"}}, "Origin", "https://evil.test", "RemoteAddr", "198.51.100.2:1"); w.Code != http.StatusForbidden {
		t.Fatalf("fremde Herkunft: %d", w.Code)
	}
	for _, f := range []url.Values{
		{"name": {""}},
		{"name": {strings.Repeat("a", 61)}},
		{"name": {"Kim\r\nBcc: x@y.z"}},
		{"name": {"Kim"}, "email": {"keine-mail"}},
		{"name": {"Kim"}, "service": {"abc"}},
	} {
		if w := queueReq(h, host, "", "/warteschlange", f, "RemoteAddr", "198.51.100.3:1"); w.Code != http.StatusBadRequest {
			t.Fatalf("%v angenommen: %d", f, w.Code)
		}
	}
	// 20 Versuche pro Anschluss und Salon und Stunde.
	ip := "198.51.100.4:1"
	for i := 0; i < 20; i++ {
		if w := queueReq(h, host, "", "/warteschlange", url.Values{"name": {"Kim"}}, "RemoteAddr", ip); w.Code != http.StatusSeeOther {
			t.Fatalf("Versuch %d: %d", i+1, w.Code)
		}
	}
	if w := queueReq(h, host, "", "/warteschlange", url.Values{"name": {"Kim"}}, "RemoteAddr", ip); w.Code != http.StatusTooManyRequests {
		t.Fatalf("Rate-Limit greift nicht: %d", w.Code)
	}
	// Tagesdeckel 50 öffentliche Einträge.
	for i := 20; i < queueMaxPerDay; i++ {
		if _, _, err := a.queueAdd(context.Background(), s, time.Now(), "Kim", "", 0, 0, 0); err != nil {
			t.Fatal(err)
		}
	}
	if w := queueReq(h, host, "", "/warteschlange", url.Values{"name": {"Kim"}}, "RemoteAddr", "198.51.100.5:1"); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "voll") {
		t.Fatalf("Tagesdeckel: %d", w.Code)
	}
}

// Englische Salons bekommen englische Seiten; jeder Schlüssel hat eine Übersetzung.
func TestQueueEnglish(t *testing.T) {
	a, h := queueTestApp(t)
	a.cfg.Domains = parseDomains("kammzeit.test=de,kammzeit-en.test=en", "")
	cookie, s := queueTestSalon(t, a, "lindner", "pro", "en", true)
	w := queueReq(h, "lindner.kammzeit-en.test", "", "/warteschlange", nil)
	body := w.Body.String()
	for _, want := range []string{"Join the queue", "First name", "Nobody is waiting right now", "Not sure yet", "30 min"} {
		if !strings.Contains(body, want) {
			t.Errorf("Einreihseite ohne %q", want)
		}
	}
	for _, de := range []string{"Vorname", "Anstellen", "Weiß ich noch nicht"} {
		if strings.Contains(body, de) {
			t.Errorf("Einreihseite enthält deutsch %q", de)
		}
	}
	_, tok, _ := a.queueAdd(context.Background(), s, time.Now(), "Sam", "", 0, 0, 0)
	_, tok2, _ := a.queueAdd(context.Background(), s, time.Now(), "Kai", "", 0, 0, 0)
	body = queueReq(h, "lindner.kammzeit-en.test", "", "/warteschlange/"+tok2, nil).Body.String()
	for _, want := range []string{"Kai, your number:", "1 person ahead of you.", "Estimated wait: about 20 min.", "I&#39;m not coming after all"} {
		if !strings.Contains(body, want) {
			t.Errorf("Statusseite ohne %q", want)
		}
	}
	_ = tok
	body = queueReq(h, "kammzeit-en.test", cookie, "/dashboard/warteschlange", nil).Body.String()
	for _, want := range []string{"Turn queue off", "Add a person", "Print QR code", "waiting for 0 min"} {
		if !strings.Contains(body, want) {
			t.Errorf("englisches Dashboard ohne %q", want)
		}
	}

	// Alle Schlüssel aus queue.html und queue.go sind übersetzt.
	html, err := webFS.ReadFile("web/queue.html")
	if err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile("queue.go")
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, m := range regexp.MustCompile(`\{\{tf? \$?\.Lang "([^"]+)"`).FindAllStringSubmatch(string(html), -1) {
		keys = append(keys, m[1])
	}
	for _, m := range regexp.MustCompile(`(?:\btrf?\([a-zA-Z.]+, |msg = )("(?:[^"\\]|\\.)*")`).FindAllStringSubmatch(string(src), -1) {
		k, err := strconv.Unquote(m[1])
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, k)
	}
	if len(keys) < 100 {
		t.Fatalf("nur %d Schlüssel gefunden, Regex kaputt?", len(keys))
	}
	for _, k := range keys {
		if tr("en", k) == k && k != "Name" && k != "optional" {
			t.Errorf("keine Übersetzung für %q", k)
		}
	}
}

// QR-Code: Formatbits nach Norm, und echte Lesbarkeit (zbarimg, falls installiert).
func TestQueueQR(t *testing.T) {
	if got := qrFormatBits(0); got != 0x5412 { // Stufe M, Maske 0 = 101010000010010
		t.Fatalf("Formatbits M/0 = %015b", got)
	}
	if got := qrFormatBits(5); got != 0b100000011001110 { // Tabelle C.1: M, Maske 5
		t.Fatalf("Formatbits M/5 = %015b", got)
	}
	if _, err := qrEncode(make([]byte, 214)); err == nil {
		t.Fatal("zu lang ohne Fehler")
	}
	if qrSVG(strings.Repeat("x", 300), "x") != "" {
		t.Fatal("SVG für zu langen Text")
	}
	svg := string(qrSVG("https://demo.kammzeit.de/warteschlange", `QR "x"`))
	if !strings.HasPrefix(svg, "<svg") || !strings.Contains(svg, `aria-label="QR &#34;x&#34;"`) {
		t.Fatalf("SVG: %.120s", svg)
	}
	zbar, err := exec.LookPath("zbarimg")
	if err != nil {
		t.Skip("zbarimg fehlt, Lesbarkeit nicht geprüft")
	}
	dir := t.TempDir()
	for _, text := range []string{
		"x",
		"https://demo.kammzeit.de/warteschlange",
		"https://salon-mit-einem-ziemlich-langen-namen-und-noch-mehr.kammzeit.com/warteschlange",
		"https://" + strings.Repeat("a", 63) + ".kammzeit.com/warteschlange?" + strings.Repeat("b", 40),
		strings.Repeat("Z", 213),
	} {
		q, err := qrEncode([]byte(text))
		if err != nil {
			t.Fatal(err)
		}
		const scale = 6
		n := (q.size + 8) * scale
		img := image.NewGray(image.Rect(0, 0, n, n))
		for y := 0; y < n; y++ {
			for x := 0; x < n; x++ {
				mx, my := x/scale-4, y/scale-4
				c := color.Gray{255}
				if mx >= 0 && my >= 0 && mx < q.size && my < q.size && q.dark[my][mx] {
					c = color.Gray{0}
				}
				img.SetGray(x, y, c)
			}
		}
		path := dir + "/qr.png"
		f, _ := os.Create(path)
		png.Encode(f, img)
		f.Close()
		out, err := exec.Command(zbar, "--raw", "-q", path).Output()
		if err != nil || strings.TrimRight(string(out), "\n") != text {
			t.Fatalf("Version %d (%d Byte) nicht lesbar: %q %v", (q.size-17)/4, len(text), out, err)
		}
	}
}

// Einträge vergangener Tage verschwinden auch ohne Dashboard-Besuch (Hintergrund-Lauf).
func TestPurgeOldQueue(t *testing.T) {
	db, err := openDB(t.TempDir() + "/purge.db")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO salons (slug, name) VALUES ('p', 'P')`); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	for i, day := range []string{"2026-11-01", "2026-11-02", "2026-11-03"} {
		if _, err := db.Exec(`INSERT INTO queue_entries (salon_id, number, day, name, token) VALUES (1, 1, ?, 'X', ?)`, day, fmt.Sprint("t", i)); err != nil {
			t.Fatal(err)
		}
	}
	a := &app{db: db}
	a.purgeOldQueue(now)
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM queue_entries`).Scan(&n)
	if n != 1 {
		t.Fatalf("nach dem Aufräumen %d Einträge, want 1 (nur heute)", n)
	}
}
