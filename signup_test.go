package main

import (
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestSlugStatus(t *testing.T) {
	a := newSignupApp(t, false)
	if _, err := a.db.Exec(`INSERT INTO salons (slug, name) VALUES ('mueller', 'Salon Müller')`); err != nil {
		t.Fatal(err)
	}
	for slug, want := range map[string]string{
		"salon-mueller":         "frei",
		"abc":                   "frei",
		strings.Repeat("a", 40): "frei",
		"ab":                    "ungültig: 3 bis 40 Zeichen",
		strings.Repeat("a", 41): "ungültig: 3 bis 40 Zeichen",
		"a_b":                   "ungültig: nur a-z, 0-9 und - (nicht am Anfang oder Ende)",
		"Salon":                 "ungültig: nur a-z, 0-9 und - (nicht am Anfang oder Ende)",
		"-abc":                  "ungültig: nur a-z, 0-9 und - (nicht am Anfang oder Ende)",
		"abc-":                  "ungültig: nur a-z, 0-9 und - (nicht am Anfang oder Ende)",
		"sa lon":                "ungültig: nur a-z, 0-9 und - (nicht am Anfang oder Ende)",
		"www":                   "vergeben",
		"admin":                 "vergeben",
		"registrieren":          "vergeben",
		"autodiscover":          "vergeben",
		"mueller":               "vergeben",
	} {
		if got, err := slugStatus(a.db, slug); err != nil || got != want {
			t.Errorf("slugStatus(%q) = %q, %v; want %q", slug, got, err, want)
		}
	}
}

func TestSuggestSlug(t *testing.T) {
	for name, want := range map[string]string{
		"Salon Müller":         "salon-mueller",
		"Haar & Bart  Studio!": "haar-bart-studio",
		"ÄRGER GROẞ":           "aerger-gross",
		"  -Café Élan- ":       "cafe-elan",
		"Haarstudio Schöne Spitzen am Marktplatz 12": "haarstudio-schoene-spitzen-am-marktplatz",
		"Friseur am Marktplatz und Bahnhof Simmern-": "friseur-am-marktplatz-und-bahnhof-simmer",
		strings.Repeat("a", 39) + " bbb":             strings.Repeat("a", 39),
		"!!!":                                        "",
	} {
		if got := suggestSlug(name); got != want {
			t.Errorf("suggestSlug(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestSignup(t *testing.T) {
	a := newSignupApp(t, false)
	h := a.routes()
	ok := func(name, slug, email string) url.Values {
		return url.Values{"name": {name}, "slug": {slug}, "email": {email}, "agb": {"1"}}
	}

	// Neuer Salon: alle Zeilen in einer Transaktion.
	rec := postSignup(h, ok("Salon Müller", "salon-mueller", "Inhaber@Example.com"), "198.51.100.1")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Fast geschafft") {
		t.Fatalf("Registrierung: %d %s", rec.Code, rec.Body)
	}
	firstBody := rec.Body.String()
	var salonID, staffID int64
	var plan, email string
	if err := a.db.QueryRow(`SELECT id, plan, email FROM salons WHERE slug = 'salon-mueller' AND name = 'Salon Müller'`).Scan(&salonID, &plan, &email); err != nil {
		t.Fatal(err)
	}
	if plan != "free" || email != "inhaber@example.com" {
		t.Errorf("Salon: plan %q, email %q", plan, email)
	}
	if err := a.db.QueryRow(`SELECT id FROM staff WHERE salon_id = ? AND name = 'Team'`, salonID).Scan(&staffID); err != nil {
		t.Fatal(err)
	}
	wantCounts := map[string]int{
		`SELECT COUNT(*) FROM salons`: 1,
		`SELECT COUNT(*) FROM users WHERE email = 'inhaber@example.com' AND salon_id = ` + fmt.Sprint(salonID): 1,
		`SELECT COUNT(*) FROM staff`: 1,
		`SELECT COUNT(*) FROM working_hours WHERE staff_id = ` + fmt.Sprint(staffID) + ` AND start_min = 540
			AND ((weekday BETWEEN 2 AND 5 AND end_min = 1080) OR (weekday = 6 AND end_min = 840))`: 5,
		`SELECT COUNT(*) FROM working_hours`: 5,
		`SELECT COUNT(*) FROM services WHERE salon_id = ` + fmt.Sprint(salonID) + ` AND (
			(name = 'Haarschnitt' AND duration_min = 30 AND price_cents = 2500) OR
			(name = 'Waschen, Schneiden, Föhnen' AND duration_min = 60 AND price_cents = 4500))`: 2,
		`SELECT COUNT(*) FROM staff_services WHERE staff_id = ` + fmt.Sprint(staffID): 2,
	}
	signupCounts(t, a, wantCounts)

	// Doppelte Adresse: Fehler am Formular, Eingaben bleiben stehen, nichts angelegt.
	rec = postSignup(h, ok("Anderer Salon", "salon-mueller", "andere@example.com"), "198.51.100.2")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "ist leider vergeben") ||
		!strings.Contains(rec.Body.String(), `value="andere@example.com"`) || !strings.Contains(rec.Body.String(), `value="Anderer Salon"`) {
		t.Fatalf("doppelte Adresse: %d %s", rec.Code, rec.Body)
	}

	// Doppelte E-Mail: dieselbe Antwort wie bei einer neuen Registrierung, nichts doppelt.
	rec = postSignup(h, ok("Zweiter Salon", "zweiter-salon", "inhaber@example.com"), "198.51.100.3")
	if rec.Code != http.StatusOK || rec.Body.String() != firstBody {
		t.Fatalf("doppelte E-Mail verrät sich: %d %s", rec.Code, rec.Body)
	}
	// Dasselbe Formular noch einmal (Doppelklick, Neuladen): ebenfalls Erfolg.
	rec = postSignup(h, ok("Salon Müller", "salon-mueller", "inhaber@example.com"), "198.51.100.4")
	if rec.Code != http.StatusOK || rec.Body.String() != firstBody {
		t.Fatalf("erneutes Absenden: %d %s", rec.Code, rec.Body)
	}

	// Honeypot gefüllt: Erfolgsseite, aber nichts angelegt.
	bot := ok("Bot Salon", "bot-salon", "bot@example.com")
	bot.Set("homepage", "http://spam.example")
	rec = postSignup(h, bot, "198.51.100.5")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Fast geschafft") {
		t.Fatalf("Honeypot: %d %s", rec.Code, rec.Body)
	}
	signupCounts(t, a, wantCounts)
	signupCounts(t, a, map[string]int{`SELECT COUNT(*) FROM users`: 1})

	// Ungültige Eingaben.
	noAGB := ok("Salon Neu", "salon-neu", "neu@example.com")
	noAGB.Del("agb")
	for name, form := range map[string]url.Values{
		"ohne Häkchen":     noAGB,
		"Name zu kurz":     ok("X", "salon-neu", "neu@example.com"),
		"E-Mail ungültig":  ok("Salon Neu", "salon-neu", "keine-mail"),
		"Adresse gesperrt": ok("Salon Neu", "admin", "neu@example.com"),
		"Adresse ungültig": ok("Salon Neu", "salon_neu", "neu@example.com"),
		"Adresse zu kurz":  ok("Salon Neu", "ab", "neu@example.com"),
	} {
		if rec := postSignup(h, form, "198.51.100.6"); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", name, rec.Code)
		}
	}
	signupCounts(t, a, wantCounts)

	// Rate-Limit: 5 gültige Anmeldungen pro Stunde und IP.
	for i := range 6 {
		rec := postSignup(h, ok("Salon Müller", "salon-mueller", "inhaber@example.com"), "198.51.100.7")
		want := http.StatusOK
		if i == 5 {
			want = http.StatusTooManyRequests
		}
		if rec.Code != want {
			t.Fatalf("Versuch %d: %d, want %d", i+1, rec.Code, want)
		}
	}

	// Live-Prüfung und Vorschlag aus dem Namen.
	for query, want := range map[string]string{
		"slug=salon-mueller": ">vergeben<",
		"slug=demo":          ">vergeben<",
		"slug=Neuer-Salon":   ">frei<",
		"slug=a_b":           ">ungültig: nur a-z",
		"name=Studio+Sch%C3%B6n&slug=&slug_auto=":                  `value="studio-schoen"`,
		"name=Studio+X&slug=studio-schoen&slug_auto=studio-schoen": `value="studio-x"`,
		"name=Studio+X&slug=eigene&slug_auto=studio-schoen":        `value="eigene"`,
	} {
		rec := signupDo(h, httptest.NewRequest("GET", "/registrieren/adresse?"+query, nil))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), want) {
			t.Errorf("adresse?%s: %d %s, want %s", query, rec.Code, rec.Body, want)
		}
	}

	// Formular nur auf der Hauptdomain.
	req := httptest.NewRequest("GET", "/registrieren", nil)
	req.Host = "salon-mueller.kammzeit.test"
	if rec := signupDo(h, req); rec.Code != http.StatusFound || rec.Header().Get("Location") != "https://kammzeit.test/registrieren" {
		t.Errorf("Subdomain: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	if rec := signupDo(h, httptest.NewRequest("GET", "/registrieren", nil)); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `name="homepage"`) {
		t.Errorf("Formular: %d", rec.Code)
	}
}

// Einzelbetrieb: Registrierung nur zur Ersteinrichtung.
func TestSignupSingleTenant(t *testing.T) {
	a := newSignupApp(t, true)
	h := a.routes()
	if rec := signupDo(h, httptest.NewRequest("GET", "/registrieren", nil)); rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), `name="agb"`) {
		t.Fatalf("Ersteinrichtung: %d", rec.Code)
	}
	rec := postSignup(h, url.Values{"name": {"Test"}, "email": {"chef@example.com"}}, "198.51.100.1")
	if rec.Code != http.StatusOK {
		t.Fatalf("Anlegen: %d %s", rec.Code, rec.Body)
	}
	signupCounts(t, a, map[string]int{`SELECT COUNT(*) FROM salons WHERE slug = 'salon'`: 1}) // "test" ist gesperrt
	if rec := signupDo(h, httptest.NewRequest("GET", "/registrieren", nil)); rec.Code != http.StatusNotFound {
		t.Errorf("zweiter Aufruf: %d", rec.Code)
	}
	rec = postSignup(h, url.Values{"name": {"Noch einer"}, "email": {"b@example.com"}}, "198.51.100.2")
	if rec.Code != http.StatusNotFound {
		t.Errorf("zweiter Salon: %d", rec.Code)
	}
	signupCounts(t, a, map[string]int{`SELECT COUNT(*) FROM salons`: 1})
}

func newSignupApp(t *testing.T, single bool) *app {
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
	signupLimit = newLimiter(5, time.Hour)
	return &app{cfg: config{BaseDomain: "kammzeit.test", SiteURL: "https://kammzeit.test", SingleTenant: single}, db: db, tpl: tpl}
}

func signupDo(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	if req.Host == "example.com" { // Standard von httptest
		req.Host = "kammzeit.test"
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func postSignup(h http.Handler, form url.Values, ip string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/registrieren", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = ip + ":1234"
	return signupDo(h, req)
}

func signupCounts(t *testing.T, a *app, want map[string]int) {
	t.Helper()
	for q, n := range want {
		var got int
		if err := a.db.QueryRow(q).Scan(&got); err != nil || got != n {
			t.Errorf("%s = %d (%v), want %d", q, got, err, n)
		}
	}
}
