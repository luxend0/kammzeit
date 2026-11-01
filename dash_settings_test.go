package main

import (
	"database/sql"
	"encoding/json"
	"html/template"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestParseSettings(t *testing.T) {
	base := url.Values{
		"name": {"Salon Müller"}, "email": {"Info@Mueller.de"}, "phone": {"06761 1234"},
		"address": {"Hauptstr. 1\r\n55469 Simmern"}, "website": {"http://mueller.de"},
		"imprint_url": {"https://mueller.de/impressum"}, "about": {"Seit 1990."}, "accent": {"#AABBCC"},
		"timezone": {"Europe/Vienna"}, "min_notice": {"1440"}, "cancel_notice": {"2880"}, "horizon": {"180"},
	}
	s, errs := parseSettings(base)
	if len(errs) != 0 {
		t.Fatalf("gültiges Formular: %v", errs)
	}
	if s.Accent != "#aabbcc" || s.Address != "Hauptstr. 1\n55469 Simmern" || s.TZ != "Europe/Vienna" ||
		s.MinNoticeMin != 1440 || s.CancelNoticeMin != 2880 || s.HorizonDays != 180 || s.Email != "Info@Mueller.de" {
		t.Fatalf("falsch übernommen: %+v", s)
	}

	cases := []struct {
		field, value string
		bad          bool
	}{
		{"name", "A", true},
		{"name", strings.Repeat("ä", 80), false},
		{"name", strings.Repeat("ä", 81), true},
		{"name", "Salon\nMüller", true},
		{"name", "Salon\tMüller", true},
		{"email", "", false},
		{"email", "keine-mail", true},
		{"phone", strings.Repeat("1", 41), true},
		{"address", strings.Repeat("x", 300), false},
		{"address", strings.Repeat("x", 301), true},
		{"about", strings.Repeat("ü", 500), false},
		{"about", strings.Repeat("x", 501), true},
		{"website", "", false},
		{"website", "https://mueller.de/", false},
		{"website", "javascript:alert(1)", true},
		{"website", "javascript://x.de/%0aalert(1)", true},
		{"website", "ftp://mueller.de", true},
		{"website", "mueller.de", false}, // bekommt https:// vorangestellt
		{"website", "https://bank.de@betrug.de", true},
		{"website", "javascript:alert(1)", true},
		{"website", "https://" + strings.Repeat("a", 190) + ".de", true},
		{"imprint_url", "", false},
		{"imprint_url", "http://mueller.de/impressum", true},
		{"imprint_url", "mueller.de/impressum", false},
		{"imprint_url", "data:text/html,hallo", true},
		{"accent", "red", true},
		{"accent", "#12345", true},
		{"accent", "#1234567", true},
		{"accent", `#3b82f6"`, true},
		{"timezone", "Europe/Zurich", false},
		{"timezone", "America/New_York", true},
		{"min_notice", "0", false},
		{"min_notice", "45", true},
		{"min_notice", "abc", true},
		{"min_notice", "", true},
		{"cancel_notice", "0", false},
		{"cancel_notice", "30", true},
		{"horizon", "14", false},
		{"horizon", "365", true},
	}
	// Ohne Schema: https:// davor, auch beim Impressum.
	f := maps.Clone(base)
	f.Set("website", "salon-demo.de")
	f.Set("imprint_url", "salon-demo.de/impressum")
	if s, errs := parseSettings(f); len(errs) != 0 || s.Website != "https://salon-demo.de" || s.ImprintURL != "https://salon-demo.de/impressum" {
		t.Errorf("ohne Schema: %q %q %v", s.Website, s.ImprintURL, errs)
	}

	for _, c := range cases {
		f := maps.Clone(base)
		f.Set(c.field, c.value)
		_, errs := parseSettings(f)
		if _, got := errs[c.field]; got != c.bad || len(errs) > 1 {
			t.Errorf("%s=%q: Fehler %v, erwartet Fehler=%v", c.field, c.value, errs, c.bad)
		}
	}
}

// --- Ende-zu-Ende über HTTP: zwei Salons, Speichern, Export, Löschen ---

const einstStart = 1798790400 // 2027-01-01 09:00 Berlin

func einstApp(t *testing.T) *app {
	db, err := openDB(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	tpl, err := template.New("").Funcs(templateFuncs).ParseFS(webFS, "web/*.html")
	if err != nil {
		t.Fatal(err)
	}
	cfg := config{BaseDomain: "kammzeit.test", SiteURL: "https://kammzeit.test", PublicURL: "https://{slug}.kammzeit.test"}
	return &app{cfg: cfg, db: db, tpl: tpl}
}

// einstSeed legt einen Salon mit je einer Zeile in jeder Tabelle an und gibt
// Salon-ID und Session-Cookie-Wert zurück.
func einstSeed(t *testing.T, db *sql.DB, slug string) (int64, string) {
	exec := func(q string, args ...any) int64 {
		res, err := db.Exec(q, args...)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	sid := exec(`INSERT INTO salons (slug, name, stripe_customer_id) VALUES (?, ?, ?)`, slug, "Salon "+slug, "cus_"+slug)
	staff := exec(`INSERT INTO staff (salon_id, name) VALUES (?, ?)`, sid, "Mitarbeiter "+slug)
	svc := exec(`INSERT INTO services (salon_id, name, duration_min) VALUES (?, 'Schnitt', 30)`, sid)
	exec(`INSERT INTO staff_services (staff_id, service_id) VALUES (?, ?)`, staff, svc)
	exec(`INSERT INTO working_hours (staff_id, weekday, start_min, end_min) VALUES (?, 1, 540, 1080)`, staff)
	cust := exec(`INSERT INTO customers (salon_id, name, email) VALUES (?, ?, ?)`, sid, "Kunde "+slug, slug+"-kunde@example.com")
	exec(`INSERT INTO appointments (salon_id, staff_id, service_id, customer_id, starts_at, ends_at, cancel_token)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, sid, staff, svc, cust, einstStart, einstStart+1800, "storno-"+slug)
	uid := exec(`INSERT INTO users (salon_id, email) VALUES (?, ?)`, sid, slug+"@example.com")
	exec(`INSERT INTO login_tokens (token_hash, user_id, expires_at) VALUES (?, ?, ?)`, hashToken("login-"+slug), uid, einstStart)
	token := "session-" + slug
	exec(`INSERT INTO sessions (token_hash, user_id, expires_at) VALUES (?, ?, ?)`, hashToken(token), uid, time.Now().Add(time.Hour).Unix())
	return sid, token
}

func einstDo(a *app, method, path, cookie string, form url.Values) *httptest.ResponseRecorder {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	r := httptest.NewRequest(method, "https://kammzeit.test"+path, body)
	if form != nil {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	r.Header.Set("Origin", "https://kammzeit.test")
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
	w := httptest.NewRecorder()
	a.routes().ServeHTTP(w, r)
	return w
}

var einstTables = []string{"salons", "staff", "services", "staff_services", "working_hours", "customers",
	"appointments", "users", "login_tokens", "sessions"}

// einstTotals zählt alle Zeilen je Tabelle (über alle Salons).
func einstTotals(t *testing.T, db *sql.DB) map[string]int {
	out := map[string]int{}
	for _, tbl := range einstTables {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + tbl).Scan(&n); err != nil {
			t.Fatal(err)
		}
		out[tbl] = n
	}
	return out
}

func einstExport(t *testing.T, a *app, cookie string) (string, map[string]any) {
	t.Helper()
	w := einstDo(a, "GET", "/dashboard/einstellungen/export", cookie, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("Export: Status %d", w.Code)
	}
	var data map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
		t.Fatalf("Export ist kein JSON: %v", err)
	}
	return w.Body.String(), data
}

func TestSettingsSaveExportDelete(t *testing.T) {
	a := einstApp(t)
	aID, aCookie := einstSeed(t, a.db, "alpha")
	bID, bCookie := einstSeed(t, a.db, "beta")

	valid := url.Values{
		"name": {"Alpha Neu"}, "email": {""}, "phone": {"0123"}, "address": {"Weg 1"}, "website": {"alpha.de"},
		"imprint_url": {""}, "about": {""}, "accent": {"#112233"}, "timezone": {"Europe/Vienna"},
		"min_notice": {"30"}, "cancel_notice": {"1440"}, "horizon": {"90"},
	}

	// Ungültig: 422, Eingaben bleiben im Formular, nichts gespeichert.
	bad := maps.Clone(valid)
	bad.Set("website", "javascript:alert(1)")
	bad.Set("name", "Mein <Salon>")
	w := einstDo(a, "POST", "/dashboard/einstellungen", aCookie, bad)
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "gültige Web-Adresse") ||
		!strings.Contains(w.Body.String(), `aria-invalid="true" aria-describedby="einst-err-website"`) ||
		!strings.Contains(w.Body.String(), `<span class="field-error" id="einst-err-website">`) ||
		!strings.Contains(w.Body.String(), `value="Mein &lt;Salon&gt;"`) {
		t.Fatalf("ungültiges Formular: Status %d\n%s", w.Code, w.Body.String())
	}
	var name string
	a.db.QueryRow(`SELECT name FROM salons WHERE id = ?`, aID).Scan(&name)
	if name != "Salon alpha" {
		t.Fatalf("trotz Fehler gespeichert: %q", name)
	}

	// Gültig: speichert nur den eigenen Salon.
	w = einstDo(a, "POST", "/dashboard/einstellungen", aCookie, valid)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/dashboard/einstellungen?gespeichert=einstellungen" || einstGespeichert["einstellungen"] == "" {
		t.Fatalf("Speichern: Status %d, Location %q", w.Code, w.Header().Get("Location"))
	}
	sa, _ := a.loadSalon(aID)
	sb, _ := a.loadSalon(bID)
	if sa.Name != "Alpha Neu" || sa.Accent != "#112233" || sa.TZ != "Europe/Vienna" || sa.HorizonDays != 90 || sa.Website != "https://alpha.de" {
		t.Fatalf("nicht gespeichert: %+v", sa)
	}
	if sb.Name != "Salon beta" || sb.TZ != "Europe/Berlin" {
		t.Fatalf("fremder Salon verändert: %+v", sb)
	}
	// Meldung: "Gespeichert." aus dem Layout, nach Umstellung auf .SavedMsg der feste Text.
	w = einstDo(a, "GET", "/dashboard/einstellungen?gespeichert=einstellungen", aCookie, nil)
	body := w.Body.String()
	if w.Code != 200 || !(strings.Contains(body, "Gespeichert.") || strings.Contains(body, einstGespeichert["einstellungen"])) ||
		!strings.Contains(body, "https://alpha.kammzeit.test/") || !strings.Contains(body, `id="teilen"`) ||
		!strings.Contains(body, `id="einst-copy"`) || !strings.Contains(body, "--on-accent: #fff") {
		t.Fatalf("Seite: Status %d\n%s", w.Code, body)
	}

	// Export: nur eigene Daten, ohne Storno-Token und Stripe-IDs, Zeiten in Salon-Zeitzone.
	// New York statt Wien, damit sich der Offset von Server- und UTC-Zeit unterscheidet.
	a.db.Exec(`UPDATE salons SET timezone = 'America/New_York' WHERE id = ?`, aID)
	raw, data := einstExport(t, a, aCookie)
	for _, leak := range []string{"beta", "cancel_token", "storno-", "stripe", "cus_"} {
		if strings.Contains(raw, leak) {
			t.Errorf("Export enthält %q:\n%s", leak, raw)
		}
	}
	if !strings.Contains(raw, "alpha-kunde@example.com") {
		t.Errorf("Export ohne eigene Kunden:\n%s", raw)
	}
	for _, key := range []string{"staff", "services", "staff_services", "working_hours", "customers", "appointments"} {
		if list, _ := data[key].([]any); len(list) != 1 {
			t.Errorf("Export %s: %d Einträge, erwartet 1", key, len(list))
		}
	}
	appt := data["appointments"].([]any)[0].(map[string]any)
	if want := "2027-01-01T03:00:00-05:00"; appt["starts_at"] != want {
		t.Errorf("starts_at = %v, erwartet %s", appt["starts_at"], want)
	}

	// Löschen mit Abo: abgelehnt.
	a.db.Exec(`UPDATE salons SET stripe_subscription_id = 'sub_1' WHERE id = ?`, aID)
	w = einstDo(a, "POST", "/dashboard/einstellungen/loeschen", aCookie, url.Values{"slug": {"alpha"}})
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "/dashboard/abo") {
		t.Fatalf("Löschen mit Abo: Status %d", w.Code)
	}
	a.db.Exec(`UPDATE salons SET stripe_subscription_id = '' WHERE id = ?`, aID)

	// Falscher Slug oder fremder Slug: abgelehnt.
	for _, slug := range []string{"", "alph", "beta"} {
		if w := einstDo(a, "POST", "/dashboard/einstellungen/loeschen", aCookie, url.Values{"slug": {slug}}); w.Code != http.StatusUnprocessableEntity {
			t.Fatalf("Löschen mit Slug %q: Status %d", slug, w.Code)
		}
	}
	for tbl, n := range einstTotals(t, a.db) {
		if n != 2 {
			t.Fatalf("vor dem Löschen %s = %d, erwartet 2", tbl, n)
		}
	}

	// Ohne Abo, richtiger Slug: alles von alpha weg, beta unberührt.
	w = einstDo(a, "POST", "/dashboard/einstellungen/loeschen", aCookie, url.Values{"slug": {" Alpha "}})
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/?geloescht" ||
		!strings.Contains(w.Header().Get("Set-Cookie"), sessionCookie+"=;") {
		t.Fatalf("Löschen: Status %d, Location %q, Cookie %q", w.Code, w.Header().Get("Location"), w.Header().Get("Set-Cookie"))
	}
	for tbl, n := range einstTotals(t, a.db) {
		if n != 1 {
			t.Errorf("nach dem Löschen %s = %d, erwartet 1 (nur beta)", tbl, n)
		}
	}
	if w := einstDo(a, "GET", "/dashboard/einstellungen", aCookie, nil); w.Code != http.StatusFound {
		t.Errorf("alte Session gilt noch: Status %d", w.Code)
	}
	_, data = einstExport(t, a, bCookie)
	if data["salon"].(map[string]any)["slug"] != "beta" {
		t.Errorf("beta-Export kaputt: %v", data["salon"])
	}
	for _, key := range []string{"staff", "services", "staff_services", "working_hours", "customers", "appointments"} {
		if list, _ := data[key].([]any); len(list) != 1 {
			t.Errorf("beta %s: %d Einträge, erwartet 1", key, len(list))
		}
	}
}
