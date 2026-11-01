package main

import (
	"bytes"
	"database/sql"
	"html/template"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

// testSalon: Salon "test" (Pro, also beide buchbar) mit Anna (sort 0) und Ben
// (sort 1), beide rund um die Uhr da. Anna kann nur Schnitt, Ben Schnitt und Farbe.
type testSalon struct {
	a                   *app
	h                   http.Handler
	salon               int64
	anna, ben           int64
	schnitt, farbe, cid int64
}

func newTestSalon(t *testing.T) *testSalon {
	t.Helper()
	db, err := openDB(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	tpl, err := template.New("").Funcs(templateFuncs).ParseFS(webFS, "web/*.html")
	if err != nil {
		t.Fatal(err)
	}
	bookLimit, slotLimit = newLimiter(10, time.Hour), newLimiter(120, time.Minute)
	bookSalonLimit, bookMailLimit = newLimiter(30, time.Hour), newLimiter(3, time.Hour)
	ts := &testSalon{a: &app{cfg: config{BaseDomain: "kammzeit.test", PublicURL: "https://{slug}.kammzeit.test"}, db: db, tpl: tpl}}
	ts.h = ts.a.routes()
	ins := func(q string, args ...any) int64 {
		t.Helper()
		res, err := db.Exec(q, args...)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	ts.salon = ins(`INSERT INTO salons (slug, name, phone, address, min_notice_min) VALUES ('test', 'Salon Test', '06761 123 45', 'Hauptstr. 1' || char(10) || '55469 Simmern', 0)`)
	ts.exec(t, `UPDATE salons SET plan = 'pro' WHERE id = ?`, ts.salon)
	ins(`INSERT INTO salons (slug, name) VALUES ('fremd', 'Fremder Salon')`)
	ts.anna = ins(`INSERT INTO staff (salon_id, name, sort) VALUES (?, 'Anna', 0)`, ts.salon)
	ts.ben = ins(`INSERT INTO staff (salon_id, name, sort) VALUES (?, 'Ben', 1)`, ts.salon)
	ts.schnitt = ins(`INSERT INTO services (salon_id, name, duration_min, price_cents) VALUES (?, 'Schnitt', 30, 2500)`, ts.salon)
	ts.farbe = ins(`INSERT INTO services (salon_id, name, duration_min, price_cents) VALUES (?, 'Farbe', 60, 7900)`, ts.salon)
	for _, p := range [][2]int64{{ts.anna, ts.schnitt}, {ts.ben, ts.schnitt}, {ts.ben, ts.farbe}} {
		ins(`INSERT INTO staff_services (staff_id, service_id) VALUES (?, ?)`, p[0], p[1])
	}
	for _, st := range []int64{ts.anna, ts.ben} {
		for wd := 0; wd < 7; wd++ {
			ins(`INSERT INTO working_hours (staff_id, weekday, start_min, end_min) VALUES (?, ?, 0, 1440)`, st, wd)
		}
	}
	ts.cid = ins(`INSERT INTO customers (salon_id, name, email) VALUES (?, 'Kim', 'kim@example.com')`, ts.salon)
	return ts
}

func (ts *testSalon) exec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := ts.a.db.Exec(q, args...); err != nil {
		t.Fatal(err)
	}
}

func (ts *testSalon) count(t *testing.T, q string, args ...any) int {
	t.Helper()
	var n int
	if err := ts.a.db.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (ts *testSalon) do(method, host, path string, form url.Values) *httptest.ResponseRecorder {
	var body *strings.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	} else {
		body = strings.NewReader("")
	}
	r := httptest.NewRequest(method, "http://"+host+path, body)
	if form != nil {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	w := httptest.NewRecorder()
	ts.h.ServeHTTP(w, r)
	return w
}

// appt legt einen gebuchten Termin an und gibt das Storno-Token zurück.
func (ts *testSalon) appt(t *testing.T, staff, customer int64, start time.Time, dur time.Duration) string {
	t.Helper()
	tok := randomToken()
	ts.exec(t, `INSERT INTO appointments (salon_id, staff_id, service_id, customer_id, starts_at, ends_at, cancel_token) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		ts.salon, staff, ts.schnitt, customer, start.Unix(), start.Add(dur).Unix(), tok)
	return tok
}

func bookForm(service, staff int64, start time.Time) url.Values {
	f := url.Values{"name": {"Eva"}, "email": {"eva@example.com"}, "service": {strconv.FormatInt(service, 10)}, "start": {strconv.FormatInt(start.Unix(), 10)}}
	if staff == 0 {
		f.Set("staff", "egal")
	} else {
		f.Set("staff", strconv.FormatInt(staff, 10))
	}
	return f
}

// Zukunftstermin im 15-Minuten-Raster: übermorgen 10:00 Ortszeit.
func dayAfterTomorrow10(loc *time.Location) time.Time {
	y, m, d := time.Now().In(loc).AddDate(0, 0, 2).Date()
	return time.Date(y, m, d, 10, 0, 0, 0, loc)
}

func TestStaffServicesFilter(t *testing.T) {
	ts := newTestSalon(t)
	w := ts.do("GET", "test.kammzeit.test", "/mitarbeiter?service="+strconv.FormatInt(ts.farbe, 10), nil)
	if body := w.Body.String(); !strings.Contains(body, "Ben") || strings.Contains(body, "Anna") || !strings.Contains(body, "Egal, wer frei ist") {
		t.Fatalf("Farbe: nur Ben + egal erwartet, got %s", body)
	}
	if w.Header().Get("HX-Trigger-After-Swap") != "staff-loaded" {
		t.Error("HX-Trigger-After-Swap fehlt")
	}
	// Auswahl Ben bleibt beim Wechsel auf Schnitt erhalten.
	w = ts.do("GET", "test.kammzeit.test", "/mitarbeiter?service="+strconv.FormatInt(ts.schnitt, 10)+"&staff="+strconv.FormatInt(ts.ben, 10), nil)
	if body := w.Body.String(); !strings.Contains(body, `value="`+strconv.FormatInt(ts.ben, 10)+`" checked`) || !strings.Contains(body, "Anna") {
		t.Fatalf("Schnitt mit Ben gewählt: %s", body)
	}
	// Leistung eines fremden Salons: keine Mitarbeiter.
	w = ts.do("GET", "fremd.kammzeit.test", "/mitarbeiter?service="+strconv.FormatInt(ts.schnitt, 10), nil)
	if strings.Contains(w.Body.String(), "Anna") {
		t.Fatal("fremder Salon sieht Mitarbeiter")
	}

	s, _ := ts.a.loadSalon(ts.salon)
	start := dayAfterTomorrow10(s.Loc)
	// Anna bietet Farbe nicht an: weder Slots noch Buchung.
	q := "/slots?service=" + strconv.FormatInt(ts.farbe, 10) + "&staff=" + strconv.FormatInt(ts.anna, 10) + "&date=" + start.Format(time.DateOnly)
	if strings.Contains(ts.do("GET", "test.kammzeit.test", q, nil).Body.String(), `name="start"`) {
		t.Fatal("Slots für Anna bei Farbe")
	}
	if w := ts.do("POST", "test.kammzeit.test", "/book", bookForm(ts.farbe, ts.anna, start)); w.Code != http.StatusBadRequest {
		t.Fatalf("Buchung Anna/Farbe: %d", w.Code)
	}
	if n := ts.count(t, `SELECT COUNT(*) FROM appointments`); n != 0 {
		t.Fatalf("%d Termine gespeichert", n)
	}
}

func TestBookAnyStaff(t *testing.T) {
	ts := newTestSalon(t)
	s, _ := ts.a.loadSalon(ts.salon)
	start := dayAfterTomorrow10(s.Loc)
	ts.appt(t, ts.anna, ts.cid, start, time.Hour) // Anna ist um 10 belegt

	// Vereinigung: 10:00 ist bei "egal" frei (Ben), bei Anna nicht.
	slotsQ := func(staff string) string {
		return ts.do("GET", "test.kammzeit.test", "/slots?service="+strconv.FormatInt(ts.schnitt, 10)+"&staff="+staff+"&date="+start.Format(time.DateOnly), nil).Body.String()
	}
	at10 := `value="` + strconv.FormatInt(start.Unix(), 10) + `"`
	if !strings.Contains(slotsQ("egal"), at10) || strings.Contains(slotsQ(strconv.FormatInt(ts.anna, 10)), at10) {
		t.Fatal("Slot-Vereinigung falsch")
	}

	w := ts.do("POST", "test.kammzeit.test", "/book", bookForm(ts.schnitt, 0, start))
	if w.Code != http.StatusOK {
		t.Fatalf("Buchung egal: %d %s", w.Code, w.Body)
	}
	var staff int64
	if err := ts.a.db.QueryRow(`SELECT staff_id FROM appointments WHERE starts_at = ? AND customer_id <> ?`, start.Unix(), ts.cid).Scan(&staff); err != nil {
		t.Fatal(err)
	}
	if staff != ts.ben || !strings.Contains(w.Body.String(), "Ben") {
		t.Fatalf("zugeteilt %d, erwartet Ben %d", staff, ts.ben)
	}
	// Jetzt sind beide belegt: sauber ablehnen, nichts speichern.
	if w := ts.do("POST", "test.kammzeit.test", "/book", bookForm(ts.schnitt, 0, start)); w.Code != http.StatusConflict {
		t.Fatalf("Doppelbuchung: %d", w.Code)
	}
	if n := ts.count(t, `SELECT COUNT(*) FROM appointments WHERE starts_at = ?`, start.Unix()); n != 2 {
		t.Fatalf("%d Termine um 10 Uhr", n)
	}
	// Ohne Belegung bekommt der Erste nach sort (Anna) den Termin.
	later := start.Add(3 * time.Hour)
	if w := ts.do("POST", "test.kammzeit.test", "/book", bookForm(ts.schnitt, 0, later)); w.Code != http.StatusOK {
		t.Fatalf("Buchung 13 Uhr: %d", w.Code)
	}
	if err := ts.a.db.QueryRow(`SELECT staff_id FROM appointments WHERE starts_at = ?`, later.Unix()).Scan(&staff); err != nil || staff != ts.anna {
		t.Fatalf("13 Uhr an %d, erwartet Anna", staff)
	}
}

func TestNoticeAndHorizon(t *testing.T) {
	ts := newTestSalon(t)
	ts.exec(t, `UPDATE salons SET min_notice_min = 60, booking_horizon_days = 7 WHERE id = ?`, ts.salon)
	s, _ := ts.a.loadSalon(ts.salon)
	staff := []staffMember{{ID: ts.anna}}
	now := time.Date(2030, 3, 4, 9, 10, 0, 0, s.Loc)
	free, err := slotsFor(ts.a.db, s, staff, 30*time.Minute, now, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(free) == 0 || free[0].Format("15:04") != "10:15" {
		t.Fatalf("erster Slot %v, erwartet 10:15 (9:10 + 60 Min. Vorlauf)", free)
	}
	for _, c := range []struct {
		days int
		want bool
	}{{-1, false}, {7, true}, {8, false}} {
		free, _ := slotsFor(ts.a.db, s, staff, 30*time.Minute, now.AddDate(0, 0, c.days), now)
		if (len(free) > 0) != c.want {
			t.Errorf("Tag +%d: %d Slots", c.days, len(free))
		}
	}

	// Server prüft beim Buchen selbst, nicht nur das Formular.
	ts.exec(t, `UPDATE salons SET min_notice_min = 120, booking_horizon_days = 60 WHERE id = ?`, ts.salon)
	soon := time.Now().Truncate(15 * time.Minute).Add(45 * time.Minute)
	if w := ts.do("POST", "test.kammzeit.test", "/book", bookForm(ts.schnitt, 0, soon)); w.Code != http.StatusConflict {
		t.Fatalf("Buchung innerhalb des Vorlaufs: %d", w.Code)
	}
	y, m, d := time.Now().In(s.Loc).AddDate(0, 0, 61).Date()
	far := time.Date(y, m, d, 10, 0, 0, 0, s.Loc)
	if w := ts.do("POST", "test.kammzeit.test", "/book", bookForm(ts.schnitt, 0, far)); w.Code != http.StatusConflict {
		t.Fatalf("Buchung hinter dem Horizont: %d", w.Code)
	}
	if !strings.Contains(ts.do("GET", "test.kammzeit.test", "/slots?service="+strconv.FormatInt(ts.schnitt, 10)+"&date="+far.Format(time.DateOnly), nil).Body.String(), "Online buchbar bis") {
		t.Fatal("Slots hinter dem Horizont ohne Hinweis")
	}
	if !strings.Contains(ts.do("GET", "test.kammzeit.test", "/", nil).Body.String(), `max="`+far.AddDate(0, 0, -1).Format(time.DateOnly)+`"`) {
		t.Fatal("Datumsfeld ohne max")
	}
	ok := time.Now().Truncate(15 * time.Minute).Add(3 * time.Hour)
	if l := ok.In(s.Loc); l.Hour() == 23 && l.Minute() >= 30 {
		ok = ok.Add(time.Hour) // 30 Minuten passen nicht mehr vor Mitternacht ins Zeitfenster
	}
	if w := ts.do("POST", "test.kammzeit.test", "/book", bookForm(ts.schnitt, 0, ok)); w.Code != http.StatusOK {
		t.Fatalf("Buchung in 3 Stunden: %d %s", w.Code, w.Body)
	}
}

func TestCancelNotice(t *testing.T) {
	ts := newTestSalon(t)
	ts.exec(t, `UPDATE salons SET cancel_notice_min = 180 WHERE id = ?`, ts.salon)
	now := time.Now().Truncate(time.Minute)
	late := ts.appt(t, ts.anna, ts.cid, now.Add(2*time.Hour), 30*time.Minute)
	early := ts.appt(t, ts.ben, ts.cid, now.Add(5*time.Hour), 30*time.Minute)

	body := ts.do("GET", "test.kammzeit.test", "/storno/"+late, nil).Body.String()
	if !strings.Contains(body, "Bitte ruf den Salon an") || !strings.Contains(body, `href="tel:0676112345"`) || strings.Contains(body, "<form") {
		t.Fatalf("zu spät: Telefon-Hinweis fehlt: %s", body)
	}
	if w := ts.do("POST", "test.kammzeit.test", "/storno/"+late, url.Values{}); w.Code != http.StatusConflict {
		t.Fatalf("späte Absage: %d", w.Code)
	}
	if n := ts.count(t, `SELECT COUNT(*) FROM appointments WHERE cancel_token = ? AND status = 'booked'`, late); n != 1 {
		t.Fatal("späte Absage wurde trotzdem storniert")
	}

	body = ts.do("GET", "test.kammzeit.test", "/storno/"+early, nil).Body.String()
	if !strings.Contains(body, "Online-Absage bis") || !strings.Contains(body, "<form") {
		t.Fatalf("rechtzeitig: %s", body)
	}
	// Token eines anderen Salons gilt nicht.
	if w := ts.do("POST", "fremd.kammzeit.test", "/storno/"+early, url.Values{}); w.Code != http.StatusNotFound {
		t.Fatalf("fremder Salon: %d", w.Code)
	}
	if w := ts.do("POST", "test.kammzeit.test", "/storno/"+early, url.Values{}); w.Code != http.StatusOK {
		t.Fatalf("rechtzeitige Absage: %d", w.Code)
	}
	if n := ts.count(t, `SELECT COUNT(*) FROM appointments WHERE cancel_token = ? AND status = 'cancelled'`, early); n != 1 {
		t.Fatal("nicht storniert")
	}
}

func TestHoneypot(t *testing.T) {
	ts := newTestSalon(t)
	f := bookForm(ts.schnitt, 0, dayAfterTomorrow10(time.Local))
	f.Set("firma", "Spam GmbH")
	w := ts.do("POST", "test.kammzeit.test", "/book", f)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Termin gebucht") {
		t.Fatalf("Honeypot soll Erfolg vortäuschen: %d", w.Code)
	}
	if n := ts.count(t, `SELECT (SELECT COUNT(*) FROM appointments) + (SELECT COUNT(*) FROM customers WHERE email = 'eva@example.com')`); n != 0 {
		t.Fatal("Honeypot-Buchung gespeichert")
	}
}

func TestBookRateLimit(t *testing.T) {
	ts := newTestSalon(t)
	f := url.Values{"name": {""}}
	for i := 0; i < 10; i++ {
		ts.do("POST", "test.kammzeit.test", "/book", f)
	}
	if w := ts.do("POST", "test.kammzeit.test", "/book", f); w.Code != http.StatusTooManyRequests {
		t.Fatalf("11. Buchung: %d", w.Code)
	}
}

func TestICS(t *testing.T) {
	berlin, _ := time.LoadLocation("Europe/Berlin")
	start := time.Date(2030, 3, 4, 10, 0, 0, 0, berlin)
	ics := buildICS("abc@kammzeit", "Schnitt, Föhnen; Glätten \\ extra bei Salon Müller", "Salon Müller, Lange Straße 123, 55469 Simmern-Unterzentrum-Hunsrück",
		"Bei Anna\nAbsagen: https://demo.kammzeit.de/storno/0123456789abcdef0123456789abcdef", start, start.Add(time.Hour), start)
	if !strings.HasSuffix(ics, "\r\n") || strings.Count(ics, "\n") != strings.Count(ics, "\r\n") {
		t.Fatal("Zeilenenden müssen CRLF sein")
	}
	folded := false
	for _, l := range strings.Split(strings.TrimSuffix(ics, "\r\n"), "\r\n") {
		if len(l) > 75 || !utf8.ValidString(l) {
			t.Errorf("Zeile zu lang oder UTF-8 zerteilt: %q", l)
		}
		folded = folded || strings.HasPrefix(l, " ")
	}
	if !folded {
		t.Error("lange Zeilen nicht gefaltet")
	}
	un := strings.ReplaceAll(ics, "\r\n ", "")
	for _, want := range []string{
		"BEGIN:VCALENDAR\r\nVERSION:2.0\r\n", "UID:abc@kammzeit\r\n",
		"DTSTART:20300304T090000Z\r\n", "DTEND:20300304T100000Z\r\n",
		`SUMMARY:Schnitt\, Föhnen\; Glätten \\ extra bei Salon Müller` + "\r\n",
		`LOCATION:Salon Müller\, Lange Straße 123\, 55469 Simmern-Unterzentrum-Hunsrück` + "\r\n",
		`DESCRIPTION:Bei Anna\nAbsagen: https://demo.kammzeit.de/storno/`, "END:VEVENT\r\nEND:VCALENDAR\r\n",
	} {
		if !strings.Contains(un, want) {
			t.Errorf("fehlt: %q", want)
		}
	}

	ts := newTestSalon(t)
	tok := ts.appt(t, ts.anna, ts.cid, time.Now().Add(48*time.Hour), 30*time.Minute)
	w := ts.do("GET", "test.kammzeit.test", "/termin/"+tok+".ics", nil)
	if w.Code != http.StatusOK || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/calendar") ||
		!strings.Contains(w.Body.String(), "SUMMARY:Schnitt bei Salon Test") || !strings.Contains(w.Body.String(), `LOCATION:Salon Test\, Hauptstr. 1\, 55469 Simmern`) {
		t.Fatalf("ics: %d %s", w.Code, w.Body)
	}
	for _, c := range []struct{ host, path string }{
		{"fremd.kammzeit.test", "/termin/" + tok + ".ics"}, // anderer Salon
		{"test.kammzeit.test", "/termin/" + tok},           // ohne Endung
		{"test.kammzeit.test", "/termin/falsch.ics"},
	} {
		if w := ts.do("GET", c.host, c.path, nil); w.Code != http.StatusNotFound {
			t.Errorf("%s%s: %d", c.host, c.path, w.Code)
		}
	}
}

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func TestRemindersSkipCustomersWithoutEmail(t *testing.T) {
	ts := newTestSalon(t)
	res, err := ts.a.db.Exec(`INSERT INTO customers (salon_id, name, email) VALUES (?, 'Laufkunde', NULL)`, ts.salon)
	if err != nil {
		t.Fatal(err)
	}
	walkIn, _ := res.LastInsertId()
	soon := time.Now().Add(2 * time.Hour)
	withMail := ts.appt(t, ts.anna, ts.cid, soon, 30*time.Minute)
	noMail := ts.appt(t, ts.ben, walkIn, soon, 30*time.Minute)

	var logs syncBuf
	log.SetOutput(&logs)
	defer log.SetOutput(os.Stderr)
	ts.a.sendReminders(time.Now())

	if strings.Contains(logs.String(), "Erinnerung") && strings.Contains(logs.String(), "NULL") {
		t.Fatalf("Scan-Fehler: %s", logs.String())
	}
	sent := func(tok string) bool {
		var at sql.NullInt64
		if err := ts.a.db.QueryRow(`SELECT reminder_sent_at FROM appointments WHERE cancel_token = ?`, tok).Scan(&at); err != nil {
			t.Fatal(err)
		}
		return at.Valid
	}
	if !sent(withMail) || sent(noMail) {
		t.Fatalf("Erinnerung mit Mail %v, ohne Mail %v", sent(withMail), sent(noMail))
	}
	out := logs.String()
	if !strings.Contains(out, "an kim@example.com") || !strings.Contains(out, "Hauptstr. 1") || !strings.Contains(out, "/termin/"+withMail+".ics") {
		t.Fatalf("Erinnerungsmail unvollständig: %s", out)
	}
}

func TestBookingHeader(t *testing.T) {
	ts := newTestSalon(t)
	ts.exec(t, `UPDATE salons SET website = 'www.salon-test.de', imprint_url = 'javascript:alert(1)', accent = '#ffeb3b', plan = 'pro' WHERE id = ?`, ts.salon)
	body := ts.do("GET", "test.kammzeit.test", "/", nil).Body.String()
	for _, want := range []string{`href="https://www.salon-test.de"`, `>www.salon-test.de</a>`, `href="tel:0676112345"`, `--on-accent: #000`, "Hauptstr. 1"} {
		if !strings.Contains(body, want) {
			t.Errorf("fehlt: %s", want)
		}
	}
	if strings.Contains(body, "javascript:") || strings.Contains(body, "Online-Termine mit") {
		t.Error("unsichere Impressum-URL oder Gratis-Hinweis bei Pro")
	}
}

func TestOnAccent(t *testing.T) {
	for in, want := range map[string]string{
		"#ffffff": "#000", "#000000": "#fff", "#ffeb3b": "#000", "#1e3a8a": "#fff",
		"#3b82f6": "#000", // Standardblau: Schwarz 5,7:1, Weiß nur 3,7:1
		"kaputt":  "#000",
	} {
		if got := onAccent(in); got != want {
			t.Errorf("onAccent(%s) = %s, want %s", in, got, want)
		}
	}
}

// Buchungsseite in der Sprache des Salons (salons.lang), egal über welche
// Domain: Texte, Preise, Daten, Fehler, Storno und Kalendereintrag englisch.
func TestBookingEnglish(t *testing.T) {
	ts := newTestSalon(t)
	ts.a.cfg.Domains = parseDomains("kammzeit.test=de,kammzeit-en.test=en", "")
	log.SetOutput(&syncBuf{})
	defer log.SetOutput(os.Stderr)
	ts.exec(t, `UPDATE salons SET lang = 'en', plan = 'free', cancel_notice_min = 1440, booking_horizon_days = 30 WHERE id = ?`, ts.salon)
	host := "test.kammzeit-en.test"
	contains := func(what, body string, want ...string) {
		t.Helper()
		for _, w := range want {
			if !strings.Contains(body, w) {
				t.Errorf("%s: %q fehlt", what, w)
			}
		}
	}
	body := ts.do("GET", host, "/", nil).Body.String()
	contains("Buchungsseite", body, `<html lang="en">`, "<title>Salon Test - Book appointment</title>", "1. Service", "30 min",
		"€25.00", "4. Your details", "Confirm booking", "You can cancel online up to 1 day before the appointment.",
		`Online booking by <a href="https://kammzeit.com">Kammzeit</a>`)
	if strings.Contains(body, "Leistung") || strings.Contains(body, "Verbindlich") {
		t.Error("deutsche Reste auf der englischen Buchungsseite")
	}
	// Dieselbe Seite über die deutsche Domain bleibt englisch (Sprache gehört dem Salon).
	contains("über .de", ts.do("GET", "test.kammzeit.test", "/", nil).Body.String(), "1. Service")

	s, _ := ts.a.loadSalon(ts.salon)
	start := dayAfterTomorrow10(s.Loc)
	sv := strconv.FormatInt(ts.schnitt, 10)
	far := start.AddDate(0, 0, 60).Format(time.DateOnly)
	_, last := bookingDays(s, time.Now())
	contains("Slots", ts.do("GET", host, "/slots?service="+sv+"&staff=egal&date="+far, nil).Body.String(),
		"Online booking is available until "+dayFor("en", last)+".")
	contains("Mitarbeiter", ts.do("GET", host, "/mitarbeiter?service="+sv, nil).Body.String(), "Anyone available")

	f := bookForm(ts.schnitt, 0, start)
	f.Set("name", "")
	if w := ts.do("POST", host, "/book", f); w.Code != http.StatusBadRequest {
		t.Fatalf("ohne Namen: %d", w.Code)
	} else {
		contains("Fehler", w.Body.String(), "Please enter your name.", "Back to booking", "<h1>Notice</h1>")
	}
	w := ts.do("POST", host, "/book", bookForm(ts.schnitt, 0, start))
	if w.Code != http.StatusOK {
		t.Fatalf("Buchung: %d %s", w.Code, w.Body)
	}
	contains("Bestätigung", w.Body.String(), "Appointment booked", "Thank you, Eva!", dateFor("en", start),
		"The confirmation is on its way to <strong>eva@example.com</strong>.", "Add to my calendar",
		"(online until "+dateFor("en", start.Add(-24*time.Hour))+")")
	var tok string
	if err := ts.a.db.QueryRow(`SELECT cancel_token FROM appointments WHERE starts_at = ?`, start.Unix()).Scan(&tok); err != nil {
		t.Fatal(err)
	}
	contains("Storno", ts.do("GET", host, "/storno/"+tok, nil).Body.String(), "Cancel appointment", "When",
		"You can cancel online until "+dateFor("en", start.Add(-24*time.Hour))+".")
	ics := strings.ReplaceAll(ts.do("GET", host, "/termin/"+tok+".ics", nil).Body.String(), "\r\n ", "")
	contains("Kalendereintrag", ics, "SUMMARY:Schnitt at Salon Test", `DESCRIPTION:With Anna\nPhone: 06761 123 45\nCancel: https://test.kammzeit-en.test/storno/`+tok)
	if w := ts.do("POST", host, "/storno/"+tok, nil); w.Code != http.StatusOK {
		t.Fatalf("Absagen: %d", w.Code)
	} else {
		contains("Abgesagt", w.Body.String(), "Appointment cancelled", "Thanks for letting us know. Salon Test has been notified.")
	}
	contains("unbekannter Salon", ts.do("GET", "gibtsnicht.kammzeit-en.test", "/", nil).Body.String(), "This salon does not exist.")
	contains("unbekannter Salon .de", ts.do("GET", "gibtsnicht.kammzeit.test", "/", nil).Body.String(), "Diesen Salon gibt es nicht.")
	contains("Datum", dayFor("en", time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC))+"|"+noticeFor("en", 90)+"|"+noticeFor("en", 120), "3 Nov 2026|90 minutes|2 hours")
}
