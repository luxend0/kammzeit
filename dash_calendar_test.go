package main

import (
	"database/sql"
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

type kalFix struct {
	salon, staff, service, customer int64
	cookie                          string
	loc                             *time.Location
}

func kalTestApp(t *testing.T) *app {
	t.Helper()
	db, err := openDB(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	tpl := template.Must(template.New("").Funcs(templateFuncs).ParseFS(webFS, "web/*.html"))
	return &app{db: db, tpl: tpl, cfg: config{BaseDomain: "kammzeit.test", SiteURL: "http://kammzeit.test",
		PublicURL: "http://{slug}.kammzeit.test"}}
}

// kalSeed legt einen Salon mit Konto, Session, einem Mitarbeiter, einer
// 60-Minuten-Leistung und einem Kunden (mit Mail) an.
func kalSeed(t *testing.T, a *app, slug, tz string) kalFix {
	t.Helper()
	f := kalFix{cookie: randomToken(), loc: location(tz)}
	ins := func(q string, args ...any) int64 {
		res, err := a.db.Exec(q, args...)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	f.salon = ins(`INSERT INTO salons (slug, name, timezone) VALUES (?, ?, ?)`, slug, "Salon "+slug, tz)
	user := ins(`INSERT INTO users (salon_id, email) VALUES (?, ?)`, f.salon, slug+"@example.com")
	ins(`INSERT INTO sessions (token_hash, user_id, expires_at) VALUES (?, ?, ?)`, hashToken(f.cookie), user, time.Now().Add(time.Hour).Unix())
	f.staff = ins(`INSERT INTO staff (salon_id, name) VALUES (?, ?)`, f.salon, "Anna-"+slug)
	f.service = ins(`INSERT INTO services (salon_id, name, duration_min, price_cents) VALUES (?, 'Schnitt', 60, 3000)`, f.salon)
	f.customer = ins(`INSERT INTO customers (salon_id, name, email) VALUES (?, ?, ?)`, f.salon, "Stamm-"+slug, "stamm@"+slug+".example")
	return f
}

// kalAppt trägt einen Termin direkt ein.
func (f kalFix) appt(t *testing.T, a *app, start time.Time, status string) int64 {
	t.Helper()
	res, err := a.db.Exec(`INSERT INTO appointments (salon_id, staff_id, service_id, customer_id, starts_at, ends_at, status, cancel_token)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, f.salon, f.staff, f.service, f.customer, start.Unix(), start.Add(time.Hour).Unix(), status, randomToken())
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

func kalDo(t *testing.T, a *app, cookie, method, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if form != nil {
		req = httptest.NewRequest(method, "http://kammzeit.test"+path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req = httptest.NewRequest(method, "http://kammzeit.test"+path, nil)
	}
	req.Header.Set("Origin", "http://kammzeit.test")
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
	rec := httptest.NewRecorder()
	a.routes().ServeHTTP(rec, req)
	return rec
}

func kalCount(t *testing.T, a *app, q string, args ...any) int {
	t.Helper()
	var n int
	if err := a.db.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestKalUeberschneidung(t *testing.T) {
	a := kalTestApp(t)
	f := kalSeed(t, a, "eins", "Europe/Berlin")
	day := "2030-03-04"
	f.appt(t, a, time.Date(2030, 3, 4, 10, 0, 0, 0, f.loc), "booked") // 10:00-11:00
	book := func(zeit string) int {
		return kalDo(t, a, f.cookie, "POST", "/dashboard/termine/neu", url.Values{
			"leistung": {fmt.Sprint(f.service)}, "mitarbeiter": {fmt.Sprint(f.staff)}, "tag": {day}, "zeit": {zeit},
			"kunde": {fmt.Sprint(f.customer)}}).Code
	}
	for _, c := range []struct {
		zeit string
		want int
	}{
		{"10:30", http.StatusConflict},   // liegt im Termin
		{"09:30", http.StatusConflict},   // endet im Termin
		{"10:02", http.StatusBadRequest}, // kein 5-Minuten-Raster
		{"11:00", http.StatusSeeOther},   // schließt direkt an
		{"09:00", http.StatusSeeOther},   // endet genau bei Beginn
		{"20:00", http.StatusSeeOther},   // außerhalb der Arbeitszeit ist erlaubt
		{"09:00", http.StatusConflict},   // jetzt belegt
	} {
		if got := book(c.zeit); got != c.want {
			t.Errorf("Termin um %s: Status %d, erwartet %d", c.zeit, got, c.want)
		}
	}
	if n := kalCount(t, a, `SELECT COUNT(*) FROM appointments WHERE salon_id = ? AND source = 'manual'`, f.salon); n != 3 {
		t.Fatalf("%d manuelle Termine, erwartet 3", n)
	}
	// Abgesagte Termine blockieren nicht.
	var id int64
	a.db.QueryRow(`SELECT id FROM appointments WHERE salon_id = ? AND starts_at = ?`, f.salon, time.Date(2030, 3, 4, 10, 0, 0, 0, f.loc).Unix()).Scan(&id)
	if rec := kalDo(t, a, f.cookie, "POST", fmt.Sprintf("/dashboard/termine/%d/absagen", id), url.Values{}); rec.Code != http.StatusSeeOther {
		t.Fatalf("absagen: %d", rec.Code)
	}
	if got := book("10:00"); got != http.StatusSeeOther {
		t.Fatalf("nach Absage: Status %d", got)
	}
}

func TestKalMandantentrennung(t *testing.T) {
	a := kalTestApp(t)
	mine := kalSeed(t, a, "meins", "Europe/Berlin")
	other := kalSeed(t, a, "fremd", "Europe/Berlin")
	start := time.Date(2030, 3, 4, 10, 0, 0, 0, other.loc)
	foreign := other.appt(t, a, start, "booked")
	a.db.Exec(`UPDATE appointments SET note = 'geheim' WHERE id = ?`, foreign)

	// Gegenprobe: der eigene Salon sieht seinen Termin.
	if rec := kalDo(t, a, other.cookie, "GET", fmt.Sprintf("/dashboard/termine/%d", foreign), nil); rec.Code != 200 || !strings.Contains(rec.Body.String(), "Stamm-fremd") {
		t.Fatalf("eigener Termin nicht sichtbar: %d", rec.Code)
	}
	for _, path := range []string{
		fmt.Sprintf("/dashboard/termine/%d", foreign),
		"/dashboard/termine/abc",
	} {
		if rec := kalDo(t, a, mine.cookie, "GET", path, nil); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s: %d, erwartet 404", path, rec.Code)
		}
	}
	for _, action := range []string{"absagen", "nicht-erschienen", "notiz"} {
		path := fmt.Sprintf("/dashboard/termine/%d/%s", foreign, action)
		if rec := kalDo(t, a, mine.cookie, "POST", path, url.Values{"notiz": {"x"}, "informieren": {"1"}}); rec.Code != http.StatusNotFound {
			t.Errorf("POST %s: %d, erwartet 404", path, rec.Code)
		}
	}
	var status, note string
	a.db.QueryRow(`SELECT status, note FROM appointments WHERE id = ?`, foreign).Scan(&status, &note)
	if status != "booked" || note != "geheim" {
		t.Fatalf("fremder Termin verändert: %s, %q", status, note)
	}
	for _, path := range []string{"/dashboard?tag=2030-03-04", "/dashboard?ansicht=woche&tag=2030-03-04", "/dashboard/termine/kundensuche?q=Stamm"} {
		rec := kalDo(t, a, mine.cookie, "GET", path, nil)
		if rec.Code != 200 || strings.Contains(rec.Body.String(), "Stamm-fremd") || strings.Contains(rec.Body.String(), "Anna-fremd") {
			t.Errorf("GET %s: %d oder fremde Daten sichtbar", path, rec.Code)
		}
	}
	// Vorbelegung mit fremdem Kunden wird ignoriert, eigener Kunde ist vorausgewählt.
	rec := kalDo(t, a, mine.cookie, "GET", fmt.Sprintf("/dashboard/termine/neu?kunde=%d", other.customer), nil)
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "Stamm-fremd") {
		t.Errorf("?kunde=fremd: %d oder fremder Name sichtbar", rec.Code)
	}
	rec = kalDo(t, a, mine.cookie, "GET", fmt.Sprintf("/dashboard/termine/neu?kunde=%d", mine.customer), nil)
	if !strings.Contains(rec.Body.String(), fmt.Sprintf(`value="%d" checked`, mine.customer)) {
		t.Errorf("?kunde=eigen nicht vorausgewählt")
	}
	// Fremde IDs beim Anlegen: 404, nichts gespeichert.
	base := url.Values{"leistung": {fmt.Sprint(mine.service)}, "mitarbeiter": {fmt.Sprint(mine.staff)}, "tag": {"2030-03-05"},
		"zeit": {"10:00"}, "kunde": {fmt.Sprint(mine.customer)}}
	for field, id := range map[string]int64{"leistung": other.service, "mitarbeiter": other.staff, "kunde": other.customer} {
		v := url.Values{}
		for k, vs := range base {
			v[k] = vs
		}
		v.Set(field, fmt.Sprint(id))
		if rec := kalDo(t, a, mine.cookie, "POST", "/dashboard/termine/neu", v); rec.Code != http.StatusNotFound {
			t.Errorf("fremde %s: %d, erwartet 404", field, rec.Code)
		}
	}
	if n := kalCount(t, a, `SELECT COUNT(*) FROM appointments WHERE salon_id = ?`, mine.salon); n != 0 {
		t.Fatalf("%d Termine angelegt, erwartet 0", n)
	}
}

func TestKalNeukunde(t *testing.T) {
	a := kalTestApp(t)
	f := kalSeed(t, a, "eins", "Europe/Berlin")
	post := func(v url.Values) {
		t.Helper()
		v.Set("leistung", fmt.Sprint(f.service))
		v.Set("mitarbeiter", fmt.Sprint(f.staff))
		v.Set("tag", "2030-03-04")
		if rec := kalDo(t, a, f.cookie, "POST", "/dashboard/termine/neu", v); rec.Code != http.StatusSeeOther {
			t.Fatalf("Status %d: %s", rec.Code, rec.Body.String())
		}
	}
	// Ohne Mail: neuer Kunde mit email = NULL, keine Erinnerung.
	post(url.Values{"zeit": {"09:00"}, "name": {"Laufkunde"}, "telefon": {"0123"}})
	var email sql.NullString
	var reminded sql.NullInt64
	var source string
	err := a.db.QueryRow(`SELECT c.email, a.reminder_sent_at, a.source FROM appointments a JOIN customers c ON c.id = a.customer_id
		WHERE c.name = 'Laufkunde'`).Scan(&email, &reminded, &source)
	if err != nil {
		t.Fatal(err)
	}
	if email.Valid || !reminded.Valid || source != "manual" {
		t.Fatalf("email %v, reminder %v, source %s", email, reminded, source)
	}
	// Zweiter Laufkunde ohne Mail: eigene Karteikarte (NULL ist nicht eindeutig).
	post(url.Values{"zeit": {"11:00"}, "name": {"Laufkunde"}})
	if n := kalCount(t, a, `SELECT COUNT(*) FROM customers WHERE name = 'Laufkunde' AND email IS NULL`); n != 2 {
		t.Fatalf("%d Laufkunden, erwartet 2", n)
	}
	// Mit bekannter Mail: bestehender Kunde wird verwendet, Name bleibt; Erinnerung offen (Termin > 24 h).
	post(url.Values{"zeit": {"13:00"}, "name": {"Anderer Name"}, "email": {"STAMM@eins.example"}})
	if n := kalCount(t, a, `SELECT COUNT(*) FROM customers WHERE salon_id = ?`, f.salon); n != 3 {
		t.Fatalf("%d Kunden, erwartet 3", n)
	}
	var cust int64
	a.db.QueryRow(`SELECT customer_id, reminder_sent_at FROM appointments WHERE starts_at = ?`,
		time.Date(2030, 3, 4, 13, 0, 0, 0, f.loc).Unix()).Scan(&cust, &reminded)
	if cust != f.customer || reminded.Valid {
		t.Fatalf("Kunde %d (erwartet %d), reminder %v", cust, f.customer, reminded)
	}
	// Neukunde ohne Namen wird abgelehnt.
	if rec := kalDo(t, a, f.cookie, "POST", "/dashboard/termine/neu", url.Values{"leistung": {fmt.Sprint(f.service)},
		"mitarbeiter": {fmt.Sprint(f.staff)}, "tag": {"2030-03-04"}, "zeit": {"15:00"}}); rec.Code != http.StatusBadRequest {
		t.Fatalf("ohne Namen: %d", rec.Code)
	}
}

// Tagesgrenzen gelten in der Zeitzone des Salons, nicht in UTC oder Serverzeit.
func TestKalTagesgrenzen(t *testing.T) {
	a := kalTestApp(t)
	f := kalSeed(t, a, "ny", "America/New_York")
	late := f.appt(t, a, time.Date(2030, 3, 4, 23, 0, 0, 0, f.loc), "booked")  // 04:00 UTC am 5.3.
	early := f.appt(t, a, time.Date(2030, 3, 5, 0, 30, 0, 0, f.loc), "booked") // 05:30 UTC am 5.3.
	// Nur die Blöcke im Raster zählen (die Kennzahl "Nächster Termin" verlinkt auch).
	block := func(id int64) string { return fmt.Sprintf(`href="/dashboard/termine/%d" title=`, id) }
	for _, c := range []struct {
		path      string
		in, notIn int64
	}{
		{"/dashboard?tag=2030-03-04", late, early},
		{"/dashboard?tag=2030-03-05", early, late},
	} {
		body := kalDo(t, a, f.cookie, "GET", c.path, nil).Body.String()
		if !strings.Contains(body, block(c.in)) || strings.Contains(body, block(c.notIn)) {
			t.Errorf("%s: Termin %d fehlt oder Termin %d ist zu sehen", c.path, c.in, c.notIn)
		}
	}
	// Wochenansicht: der späte Termin steht beim Montag, nicht beim Dienstag.
	body := kalDo(t, a, f.cookie, "GET", "/dashboard?ansicht=woche&tag=2030-03-06", nil).Body.String()
	mon, tue := strings.Index(body, "Montag, 4.3.2030"), strings.Index(body, "Dienstag, 5.3.2030")
	link := fmt.Sprintf(`href="/dashboard/termine/%d"`, late)
	if i := strings.Index(body[max(mon, 0):], link) + max(mon, 0); mon < 0 || tue < 0 || i < mon || i > tue {
		t.Errorf("Woche: Termin %d nicht unter Montag (Positionen %d, %d, %d)", late, mon, i, tue)
	}
	if day := kalParseDay("2030-03-05", f.loc, time.Now()); day.Unix() != time.Date(2030, 3, 5, 5, 0, 0, 0, time.UTC).Unix() {
		t.Errorf("kalParseDay: %v", day)
	}
}

func TestKalLanes(t *testing.T) {
	bs := []kalBox{{s: 600, e: 660}, {s: 600, e: 660}, {s: 700, e: 720}}
	kalLanes(bs)
	if bs[0].Width != 50 || bs[1].Left != 50 || bs[2].Width != 100 || bs[2].Left != 0 {
		t.Fatalf("Spuren falsch: %+v", bs)
	}
}
