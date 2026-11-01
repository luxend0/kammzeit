package main

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// kundenTestApp: App mit temporärer DB und echten Templates/Routen.
func kundenTestApp(t *testing.T) (*app, http.Handler) {
	t.Helper()
	db, err := openDB(t.TempDir() + "/k.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	tpl, err := template.New("").Funcs(templateFuncs).ParseFS(webFS, "web/*.html")
	if err != nil {
		t.Fatal(err)
	}
	a := &app{cfg: config{BaseDomain: "kammzeit.test", SiteURL: "http://kammzeit.test", PublicURL: "http://{slug}.kammzeit.test"}, db: db, tpl: tpl}
	return a, a.routes()
}

// kundenTestSalon legt Salon, Konto und Session an (wie printSession) und gibt
// Cookie-Wert und Salon-ID zurück.
func kundenTestSalon(t *testing.T, a *app, slug string) (string, int64) {
	t.Helper()
	res, err := a.db.Exec(`INSERT INTO salons (slug, name) VALUES (?, ?)`, slug, "Salon "+slug)
	if err != nil {
		t.Fatal(err)
	}
	salonID, _ := res.LastInsertId()
	res, err = a.db.Exec(`INSERT INTO users (salon_id, email) VALUES (?, ?)`, salonID, slug+"@example.com")
	if err != nil {
		t.Fatal(err)
	}
	userID, _ := res.LastInsertId()
	token := randomToken()
	if _, err := a.db.Exec(`INSERT INTO sessions (token_hash, user_id, expires_at) VALUES (?, ?, ?)`,
		hashToken(token), userID, time.Now().Add(time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	return token, salonID
}

func kundenTestKunde(t *testing.T, a *app, salonID int64, name string, email any, phone, notes string) int64 {
	t.Helper()
	res, err := a.db.Exec(`INSERT INTO customers (salon_id, name, email, phone, notes) VALUES (?, ?, ?, ?, ?)`, salonID, name, email, phone, notes)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

// kundenTestTermin legt einen Termin an (mit eigenem Mitarbeiter und eigener Leistung).
func kundenTestTermin(t *testing.T, a *app, salonID, customerID int64, start time.Time, status string) {
	t.Helper()
	res, err := a.db.Exec(`INSERT INTO staff (salon_id, name) VALUES (?, 'Anna')`, salonID)
	if err != nil {
		t.Fatal(err)
	}
	staffID, _ := res.LastInsertId()
	res, err = a.db.Exec(`INSERT INTO services (salon_id, name, duration_min, price_cents) VALUES (?, 'Färben', 60, 7900)`, salonID)
	if err != nil {
		t.Fatal(err)
	}
	serviceID, _ := res.LastInsertId()
	if _, err := a.db.Exec(`INSERT INTO appointments (salon_id, staff_id, service_id, customer_id, starts_at, ends_at, status, cancel_token)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, salonID, staffID, serviceID, customerID, start.Unix(), start.Add(time.Hour).Unix(), status, randomToken()); err != nil {
		t.Fatal(err)
	}
}

// kundenReq schickt eine Anfrage an die Hauptdomain. form != nil -> POST. hdr = Name/Wert-Paare.
func kundenReq(h http.Handler, cookie, path string, form url.Values, hdr ...string) *httptest.ResponseRecorder {
	method, body := http.MethodGet, io.Reader(nil)
	if form != nil {
		method, body = http.MethodPost, strings.NewReader(form.Encode())
	}
	r := httptest.NewRequest(method, "http://kammzeit.test"+path, body)
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
	if form != nil {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Origin", "http://kammzeit.test")
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		r.Header.Set(hdr[i], hdr[i+1])
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func kundenCount(t *testing.T, a *app, where string, args ...any) int {
	t.Helper()
	var n int
	if err := a.db.QueryRow(`SELECT COUNT(*) FROM `+where, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Fremde Kunden lassen sich weder lesen noch ändern noch löschen.
func TestKundenMandantentrennung(t *testing.T) {
	a, h := kundenTestApp(t)
	mine, _ := kundenTestSalon(t, a, "mueller")
	theirs, otherSalon := kundenTestSalon(t, a, "schmidt")
	foreign := kundenTestKunde(t, a, otherSalon, "Bea Fremd", "bea@example.com", "0170 999", "geheime Rezeptur")
	kundenTestTermin(t, a, otherSalon, foreign, time.Now().Add(48*time.Hour), "booked")
	p := fmt.Sprintf("/dashboard/kunden/%d", foreign)

	for _, c := range []struct {
		name string
		path string
		form url.Values
	}{
		{"lesen", p, nil},
		{"ändern", p, url.Values{"name": {"Übernommen"}}},
		{"Löschseite", p + "/loeschen", nil},
		{"löschen", p + "/loeschen", url.Values{}},
		{"ungültige ID", "/dashboard/kunden/abc", nil},
	} {
		w := kundenReq(h, mine, c.path, c.form)
		if w.Code != http.StatusNotFound {
			t.Errorf("%s: Status %d, erwartet 404", c.name, w.Code)
		}
		if strings.Contains(w.Body.String(), "Bea Fremd") || strings.Contains(w.Body.String(), "geheime") {
			t.Errorf("%s: fremde Daten in der Antwort", c.name)
		}
	}
	if n := kundenCount(t, a, `customers WHERE id = ? AND name = 'Bea Fremd'`, foreign); n != 1 {
		t.Fatalf("fremder Kunde verändert oder gelöscht")
	}
	for _, path := range []string{"/dashboard/kunden", "/dashboard/kunden.csv"} {
		if body := kundenReq(h, mine, path, nil).Body.String(); strings.Contains(body, "Bea") {
			t.Errorf("%s zeigt fremden Kunden", path)
		}
	}

	// Gegenprobe: der eigene Salon darf lesen und löschen, Termine gehen mit.
	if w := kundenReq(h, theirs, p, nil); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "geheime Rezeptur") {
		t.Fatalf("eigener Kunde: Status %d", w.Code)
	}
	if w := kundenReq(h, theirs, p+"/loeschen", url.Values{}); w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/dashboard/kunden?geloescht" {
		t.Fatalf("löschen: Status %d, Location %q", w.Code, w.Header().Get("Location"))
	}
	if n := kundenCount(t, a, `customers WHERE id = ?`, foreign) + kundenCount(t, a, `appointments WHERE customer_id = ?`, foreign); n != 0 {
		t.Fatalf("nach dem Löschen noch %d Zeilen übrig", n)
	}
}

// Doppelte E-Mail im selben Salon: Hinweis am Formular statt 500.
func TestKundenDoppelteEmail(t *testing.T) {
	a, h := kundenTestApp(t)
	mine, salonID := kundenTestSalon(t, a, "mueller")
	other, _ := kundenTestSalon(t, a, "schmidt")
	create := func(cookie, name, email string) *httptest.ResponseRecorder {
		return kundenReq(h, cookie, "/dashboard/kunden/neu", url.Values{"name": {name}, "email": {email}, "phone": {""}, "notes": {""}})
	}

	if w := create(mine, "Anna", "anna@example.com"); w.Code != http.StatusSeeOther {
		t.Fatalf("anlegen: Status %d", w.Code)
	}
	w := create(mine, "Anna Zwei", " ANNA@example.com ")
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "gehört schon zu einem anderen Kunden") {
		t.Fatalf("doppelt anlegen: Status %d, Body ohne Hinweis", w.Code)
	}
	if !strings.Contains(w.Body.String(), `value="Anna Zwei"`) {
		t.Error("Eingaben gehen nach dem Fehler verloren")
	}

	// Ohne E-Mail (NULL) dürfen es beliebig viele sein.
	for _, name := range []string{"Berta", "Carl"} {
		if w := create(mine, name, ""); w.Code != http.StatusSeeOther {
			t.Fatalf("%s ohne E-Mail: Status %d", name, w.Code)
		}
	}
	var berta int64
	if err := a.db.QueryRow(`SELECT id FROM customers WHERE salon_id = ? AND name = 'Berta' AND email IS NULL`, salonID).Scan(&berta); err != nil {
		t.Fatal(err)
	}
	w = kundenReq(h, mine, fmt.Sprintf("/dashboard/kunden/%d", berta), url.Values{"name": {"Berta"}, "email": {"anna@example.com"}})
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "gehört schon zu einem anderen Kunden") {
		t.Fatalf("doppelt bearbeiten: Status %d", w.Code)
	}
	if n := kundenCount(t, a, `customers WHERE id = ? AND email IS NULL`, berta); n != 1 {
		t.Fatal("Berta wurde trotz Konflikt geändert")
	}
	if n := kundenCount(t, a, `customers WHERE salon_id = ?`, salonID); n != 3 {
		t.Fatalf("%d Kunden, erwartet 3", n)
	}

	// Dieselbe Adresse in einem anderen Salon ist erlaubt.
	if w := create(other, "Anna", "anna@example.com"); w.Code != http.StatusSeeOther {
		t.Fatalf("anderer Salon: Status %d", w.Code)
	}
	// Pflichtfeld und Längen.
	if w := create(mine, " ", ""); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "Bitte einen Namen") {
		t.Fatalf("ohne Namen: Status %d", w.Code)
	}
	if w := create(mine, strings.Repeat("ä", 101), ""); w.Code != http.StatusBadRequest {
		t.Fatalf("zu langer Name: Status %d", w.Code)
	}
	if w := create(mine, "Dora", "keine-adresse"); w.Code != http.StatusBadRequest {
		t.Fatalf("ungültige E-Mail: Status %d", w.Code)
	}
}

func TestKundenCSVZelle(t *testing.T) {
	for in, want := range map[string]string{
		"Anna":           "Anna",
		"":               "",
		"=1+1":           "'=1+1",
		"+49 170 123":    "'+49 170 123",
		"-2+3":           "'-2+3",
		"@SUMME(A1)":     "'@SUMME(A1)",
		"\t=1":           "'\t=1",
		"Rezeptur 7/1+3": "Rezeptur 7/1+3",
	} {
		if got := kundenCSVZelle(in); got != want {
			t.Errorf("kundenCSVZelle(%q) = %q, erwartet %q", in, got, want)
		}
	}
}

// Export: BOM, Semikolon, keine Zelle beginnt mit einem Formelzeichen, Besuche stimmen.
func TestKundenCSV(t *testing.T) {
	a, h := kundenTestApp(t)
	mine, salonID := kundenTestSalon(t, a, "mueller")
	_, otherSalon := kundenTestSalon(t, a, "schmidt")
	evil := kundenTestKunde(t, a, salonID, `=HYPERLINK("http://x.example","klick")`, "evil@example.com", "+49 170 1", "@SUMME(A1)\nzweite Zeile")
	kundenTestKunde(t, a, salonID, "Anna; Müller", nil, "", "-2+3")
	kundenTestKunde(t, a, otherSalon, "Fremd", nil, "", "")
	kundenTestTermin(t, a, salonID, evil, time.Now().Add(-48*time.Hour), "booked")
	kundenTestTermin(t, a, salonID, evil, time.Now().Add(-24*time.Hour), "cancelled")
	kundenTestTermin(t, a, salonID, evil, time.Now().Add(24*time.Hour), "booked")

	w := kundenReq(h, mine, "/dashboard/kunden.csv", nil)
	if w.Code != http.StatusOK || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/csv") {
		t.Fatalf("Status %d, Content-Type %q", w.Code, w.Header().Get("Content-Type"))
	}
	body := w.Body.Bytes()
	if !bytes.HasPrefix(body, []byte("\xEF\xBB\xBF")) {
		t.Fatal("BOM fehlt")
	}
	cr := csv.NewReader(bytes.NewReader(body[3:]))
	cr.Comma = ';'
	recs, err := cr.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(recs[0], "|") != "Name|E-Mail|Telefon|Notizen|Angelegt|Besuche" {
		t.Fatalf("Kopfzeile %v", recs[0])
	}
	if len(recs) != 3 {
		t.Fatalf("%d Zeilen, erwartet Kopf + 2 eigene Kunden: %v", len(recs), recs)
	}
	for _, rec := range recs {
		for _, cell := range rec {
			if cell != "" && strings.ContainsRune("=+-@", rune(cell[0])) {
				t.Errorf("Zelle %q beginnt mit Formelzeichen", cell)
			}
		}
	}
	// Sortiert nach Name: "=HYPERLINK..." vor "Anna; Müller".
	want := []string{`'=HYPERLINK("http://x.example","klick")`, "evil@example.com", "'+49 170 1", "'@SUMME(A1)\nzweite Zeile"}
	if got := recs[1][:4]; strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("Zeile %q, erwartet %q", got, want)
	}
	if recs[1][5] != "1" {
		t.Errorf("Besuche %q, erwartet 1 (nur vergangene gebuchte)", recs[1][5])
	}
	if recs[2][0] != "Anna; Müller" || recs[2][1] != "" || recs[2][3] != "'-2+3" {
		t.Errorf("Zeile %q", recs[2])
	}
}

// Suche: findet nur eigene Kunden, nach Name, E-Mail und Telefon; htmx bekommt nur die Tabelle.
func TestKundenSuche(t *testing.T) {
	a, h := kundenTestApp(t)
	mine, salonID := kundenTestSalon(t, a, "mueller")
	_, otherSalon := kundenTestSalon(t, a, "schmidt")
	kundenTestKunde(t, a, salonID, "Anna Alt", "anna@alt.example", "0176 111", "")
	kundenTestKunde(t, a, salonID, "Bernd Berg", nil, "", "")
	kundenTestKunde(t, a, otherSalon, "Anna Fremd", "anna@fremd.example", "0176 112", "")

	for _, q := range []string{"anna", "ALT.EXAMPLE", "0176 1", "nna A"} {
		body := kundenReq(h, mine, "/dashboard/kunden?q="+url.QueryEscape(q), nil).Body.String()
		if !strings.Contains(body, "Anna Alt") || strings.Contains(body, "Fremd") || strings.Contains(body, "Bernd") {
			t.Errorf("Suche %q: falsches Ergebnis", q)
		}
	}
	// LIKE-Platzhalter werden wörtlich gesucht.
	if body := kundenReq(h, mine, "/dashboard/kunden?q=%25", nil).Body.String(); strings.Contains(body, "Anna Alt") || !strings.Contains(body, "Keine Kunden gefunden") {
		t.Error("Suche nach % findet alles")
	}
	w := kundenReq(h, mine, "/dashboard/kunden?q=bernd", nil, "HX-Request", "true", "HX-Target", "kunden-tabelle")
	if body := w.Body.String(); strings.Contains(body, "<!doctype") || !strings.Contains(body, "Bernd Berg") || strings.Contains(body, "Anna") {
		t.Errorf("htmx-Fragment falsch: %s", body)
	}

	// Seiten zu je 50, sortiert nach Name.
	for i := range 50 {
		kundenTestKunde(t, a, salonID, fmt.Sprintf("Zora %02d", i), nil, "", "")
	}
	page1 := kundenReq(h, mine, "/dashboard/kunden", nil).Body.String()
	page2 := kundenReq(h, mine, "/dashboard/kunden?seite=2", nil).Body.String()
	if !strings.Contains(page1, "Anna Alt") || strings.Contains(page1, "Zora 48") || !strings.Contains(page1, "seite=2") {
		t.Error("Seite 1 falsch")
	}
	if !strings.Contains(page2, "Zora 48") || !strings.Contains(page2, "Zora 49") || strings.Contains(page2, "Zora 47") || strings.Contains(page2, "seite=3") {
		t.Error("Seite 2 falsch")
	}
}

func TestKundenLeer(t *testing.T) {
	a, h := kundenTestApp(t)
	mine, _ := kundenTestSalon(t, a, "mueller")
	if body := kundenReq(h, mine, "/dashboard/kunden", nil).Body.String(); !strings.Contains(body, "sobald sie online buchen") {
		t.Error("Leerzustand fehlt")
	}
}
