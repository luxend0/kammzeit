package main

import (
	"database/sql"
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
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
		"mta-sts":               "vergeben",
		"webmail":               "vergeben",
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
		return url.Values{"name": {name}, "vorname": {"Anna"}, "slug": {slug}, "email": {email}, "agb": {"1"}}
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
	// Erster Mitarbeiter heißt wie die Inhaberin, nicht "Team".
	if err := a.db.QueryRow(`SELECT id FROM staff WHERE salon_id = ? AND name = 'Anna'`, salonID).Scan(&staffID); err != nil {
		t.Fatal(err)
	}
	wantCounts := map[string]int{
		`SELECT COUNT(*) FROM salons`: 1,
		`SELECT COUNT(*) FROM users WHERE email = 'inhaber@example.com' AND salon_id = ` + fmt.Sprint(salonID): 1,
		`SELECT COUNT(*) FROM staff`: 1,
		`SELECT COUNT(*) FROM working_hours WHERE staff_id = ` + fmt.Sprint(staffID) + ` AND start_min = 540
			AND ((weekday BETWEEN 2 AND 5 AND end_min = 1080) OR (weekday = 6 AND end_min = 840))`: 5,
		`SELECT COUNT(*) FROM working_hours`: 5,
		// Beispiel-Leistungen inaktiv: vor dem ersten Prüfen ist nichts online buchbar.
		`SELECT COUNT(*) FROM services WHERE salon_id = ` + fmt.Sprint(salonID) + ` AND active = 0 AND (
			(name = 'Haarschnitt' AND duration_min = 30 AND price_cents = 2500) OR
			(name = 'Waschen, Schneiden, Föhnen' AND duration_min = 60 AND price_cents = 4500))`: 2,
		`SELECT COUNT(*) FROM staff_services WHERE staff_id = ` + fmt.Sprint(staffID): 2,
	}
	signupCounts(t, a, wantCounts)
	book := httptest.NewRequest("GET", "/", nil)
	book.Host = "salon-mueller.kammzeit.test"
	if body := signupDo(h, book).Body.String(); !strings.Contains(body, "noch keine Termine online buchbar") || strings.Contains(body, "Haarschnitt") {
		t.Errorf("Buchungsseite direkt nach der Registrierung zeigt Beispiel-Leistungen:\n%s", body)
	}

	// Doppelte Adresse: Fehler am Feld, Eingaben bleiben stehen, nichts angelegt.
	rec = postSignup(h, ok("Anderer Salon", "salon-mueller", "andere@example.com"), "198.51.100.2")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "ist leider vergeben") ||
		!strings.Contains(rec.Body.String(), `aria-describedby="signup-status signup-err-slug" aria-invalid="true"`) ||
		!strings.Contains(rec.Body.String(), `value="andere@example.com"`) || !strings.Contains(rec.Body.String(), `value="Anderer Salon"`) ||
		!strings.Contains(rec.Body.String(), `value="Anna"`) {
		t.Fatalf("doppelte Adresse: %d %s", rec.Code, rec.Body)
	}

	// Doppelte E-Mail: dieselbe Antwort wie bei einer neuen Registrierung, nichts doppelt.
	rec = postSignup(h, ok("Zweiter Salon", "zweiter-salon", "inhaber@example.com"), "198.51.100.3")
	if rec.Code != http.StatusOK || rec.Body.String() != firstBody {
		t.Fatalf("doppelte E-Mail verrät sich: %d %s", rec.Code, rec.Body)
	}
	// Dasselbe Formular noch einmal: "vergeben" wie für jeden anderen, sonst
	// verriete die Antwort, welche E-Mail zu welchem Salon gehört.
	rec = postSignup(h, ok("Salon Müller", "salon-mueller", "inhaber@example.com"), "198.51.100.4")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "ist leider vergeben") {
		t.Fatalf("erneutes Absenden: %d", rec.Code)
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
	// Jeder Fehler steht an seinem Feld: aria-invalid, aria-describedby und der Text mit passender id.
	noAGB := ok("Salon Neu", "salon-neu", "neu@example.com")
	noAGB.Del("agb")
	with := func(field, value string) url.Values {
		f := ok("Salon Neu", "salon-neu", "neu@example.com")
		f.Set(field, value)
		return f
	}
	for name, c := range map[string]struct {
		form  url.Values
		field string
	}{
		"ohne Häkchen":     {noAGB, "agb"},
		"Name zu kurz":     {with("name", "X"), "name"},
		"Vorname fehlt":    {with("vorname", " "), "vorname"},
		"Vorname zu lang":  {with("vorname", strings.Repeat("ä", 41)), "vorname"},
		"Vorname Steuerz.": {with("vorname", "An\x00na"), "vorname"},
		"Name Umbruch":     {with("name", "Salon\nNeu"), "name"},
		"E-Mail ungültig":  {with("email", "keine-mail"), "email"},
		"Adresse gesperrt": {with("slug", "admin"), "slug"},
		"Adresse ungültig": {with("slug", "salon_neu"), "slug"},
		"Adresse zu kurz":  {with("slug", "ab"), "slug"},
	} {
		rec := postSignup(h, c.form, "198.51.100.6")
		body := rec.Body.String()
		id := "signup-err-" + c.field
		if rec.Code != http.StatusBadRequest || strings.Count(body, `aria-invalid="true"`) != 1 ||
			!strings.Contains(body, `class="field-error`) || !strings.Contains(body, `id="`+id+`"`) ||
			!regexp.MustCompile(`aria-describedby="[^"]*\b`+id+`"`).MatchString(body) {
			t.Errorf("%s: %d, Fehler nicht am Feld %s:\n%s", name, rec.Code, c.field, body)
		}
	}
	signupCounts(t, a, wantCounts)

	// Rate-Limit: 5 gültige Anmeldungen pro Stunde und IP.
	for i := range 6 {
		rec := postSignup(h, ok("Salon", fmt.Sprintf("salon-ip-%d", i), fmt.Sprintf("ip%d@example.com", i)), "198.51.100.7")
		want := http.StatusOK
		if i == 5 {
			want = http.StatusTooManyRequests
		}
		if rec.Code != want {
			t.Fatalf("Versuch %d: %d, want %d", i+1, rec.Code, want)
		}
	}

	// Login-Mails je Empfänger: 3 in 15 Minuten, egal von welcher IP und ob es das Konto gibt.
	for i := range 4 {
		rec := postSignup(h, ok("Salon", fmt.Sprintf("salon-viel-%d", i), "viel@example.com"), fmt.Sprintf("198.51.100.%d", 20+i))
		want := http.StatusOK
		if i == 3 {
			want = http.StatusTooManyRequests
		}
		if rec.Code != want {
			t.Fatalf("pro Mail, Versuch %d: %d, want %d", i+1, rec.Code, want)
		}
	}

	// Fremde Herkunft (CSRF): abgewiesen, nichts angelegt.
	req := httptest.NewRequest("POST", "/registrieren", strings.NewReader(ok("CSRF", "csrf-salon", "csrf@example.com").Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "https://evil.example")
	if rec := signupDo(h, req); rec.Code != http.StatusForbidden {
		t.Errorf("fremde Herkunft: %d", rec.Code)
	}
	signupCounts(t, a, map[string]int{`SELECT COUNT(*) FROM salons WHERE slug = 'csrf-salon'`: 0})

	// Live-Prüfung und Vorschlag aus dem Namen.
	for query, want := range map[string]string{
		"slug=salon-mueller": `<span class="signup-taken">Adresse ist vergeben</span>`,
		"slug=demo":          `<span class="signup-taken">Adresse ist vergeben</span>`,
		"slug=Neuer-Salon":   `<span class="signup-free">Adresse ist frei</span>`,
		"slug=a_b":           `<span class="signup-taken">Adresse ungültig: nur a-z`,
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
	req = httptest.NewRequest("GET", "/registrieren", nil)
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
	rec := postSignup(h, url.Values{"name": {"Test"}, "vorname": {"Chef"}, "email": {"chef@example.com"}}, "198.51.100.1")
	if rec.Code != http.StatusOK {
		t.Fatalf("Anlegen: %d %s", rec.Code, rec.Body)
	}
	signupCounts(t, a, map[string]int{`SELECT COUNT(*) FROM salons WHERE slug = 'salon'`: 1, // "test" ist gesperrt
		`SELECT COUNT(*) FROM staff WHERE name = 'Chef'`: 1})
	if rec := signupDo(h, httptest.NewRequest("GET", "/registrieren", nil)); rec.Code != http.StatusNotFound {
		t.Errorf("zweiter Aufruf: %d", rec.Code)
	}
	rec = postSignup(h, url.Values{"name": {"Noch einer"}, "vorname": {"Bea"}, "email": {"b@example.com"}}, "198.51.100.2")
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
	signupLimit, signupMailLimit = newLimiter(5, time.Hour), newLimiter(3, 15*time.Minute)
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
	req.Header.Set("Origin", "https://kammzeit.test")
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

// Die Zustimmung zu AGB/AVV muss mit Zeitpunkt und Fassung nachweisbar sein.
func TestSignupStoresTermsAcceptance(t *testing.T) {
	db, err := openDB(t.TempDir() + "/terms.db")
	if err != nil {
		t.Fatal(err)
	}
	var v int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil || v != len(migrations) {
		t.Fatalf("user_version = %d, want %d (%v)", v, len(migrations), err)
	}
	if _, err := db.Exec(`INSERT INTO salons (slug, name) VALUES ('t', 'T')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users (salon_id, email, terms_accepted_at, terms_version) VALUES (1, 'a@b.de', unixepoch(), ?)`, termsVersion); err != nil {
		t.Fatal(err)
	}
	var ver string
	var at sql.NullInt64
	if err := db.QueryRow(`SELECT terms_version, terms_accepted_at FROM users`).Scan(&ver, &at); err != nil || ver != termsVersion || !at.Valid {
		t.Fatalf("terms = %q %v (%v)", ver, at, err)
	}
}

// Registrierung auf kammzeit.com: englisch, Adresse unter .kammzeit.com (hier
// kammzeit-en.test), Salon und Konto bekommen die Sprache der Domain. Das
// Häkchen für Kammzeit-News ist auf beiden Domains nicht vorausgewählt.
func TestSignupEnglish(t *testing.T) {
	a := newSignupApp(t, false)
	a.cfg.Domains = parseDomains("kammzeit.test=de,kammzeit-en.test=en", "")
	h := a.routes()
	newsBox := regexp.MustCompile(`<input type="checkbox" name="marketing" value="1"[^>]*>`)
	for host, want := range map[string][]string{
		"kammzeit.test": {`<html lang="de">`, "Salon anlegen", `<span>.kammzeit.test</span>`, marketingConsentDE, `href="/agb"`},
		"kammzeit-en.test": {`<html lang="en">`, "Create salon", `<span>.kammzeit-en.test</span>`, template.HTMLEscapeString(tr("en", marketingConsentDE)),
			`href="/terms"`, `href="/dpa"`, "at yoursalon.kammzeit-en.test", `hreflang="de" href="https://kammzeit.test/registrieren"`},
	} {
		req := httptest.NewRequest("GET", "/registrieren", nil)
		req.Host = host
		body := signupDo(h, req).Body.String()
		for _, w := range want {
			if !strings.Contains(body, w) {
				t.Errorf("%s: %q fehlt", host, w)
			}
		}
		if box := newsBox.FindString(body); box == "" || strings.Contains(box, "checked") {
			t.Errorf("%s: Häkchen Kammzeit-News fehlt oder ist vorausgewählt: %q", host, box)
		}
	}
	if tr("en", marketingConsentDE) == marketingConsentDE {
		t.Fatal("englischer Einwilligungstext fehlt")
	}

	// Live-Prüfung englisch.
	req := httptest.NewRequest("GET", "/registrieren/adresse?slug=a_b", nil)
	req.Host = "kammzeit-en.test"
	if body := signupDo(h, req).Body.String(); !strings.Contains(body, "Invalid address: only a-z") {
		t.Errorf("Live-Prüfung en: %s", body)
	}

	form := url.Values{"name": {"Salon London"}, "vorname": {"Liz"}, "slug": {"salon-london"}, "email": {"liz@example.com"}, "agb": {"1"}}
	post := func(f url.Values, ip string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/registrieren", strings.NewReader(f.Encode()))
		req.Host = "kammzeit-en.test"
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", "https://kammzeit-en.test")
		req.RemoteAddr = ip + ":1234"
		return signupDo(h, req)
	}
	if rec := post(form, "198.51.100.60"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Almost there") {
		t.Fatalf("Registrierung en: %d %s", rec.Code, rec.Body)
	}
	signupCounts(t, a, map[string]int{
		`SELECT COUNT(*) FROM salons WHERE slug = 'salon-london' AND lang = 'en'`:                          1,
		`SELECT COUNT(*) FROM users WHERE email = 'liz@example.com' AND lang = 'en'`:                       1,
		`SELECT COUNT(*) FROM services WHERE name IN ('Haircut', 'Wash, cut and blow-dry') AND active = 0`: 2,
		// ohne Häkchen keine Einwilligung, auch keine ausstehende
		`SELECT COUNT(*) FROM users WHERE email = 'liz@example.com' AND marketing_pending_hash = '' AND marketing_at IS NULL`: 1,
	})
	// Fehler englisch, mit der .com-Adresse.
	form.Set("email", "other@example.com")
	rec := post(form, "198.51.100.61")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "Sorry, the address salon-london.kammzeit-en.test is already taken.") {
		t.Fatalf("vergeben en: %d %s", rec.Code, rec.Body)
	}
	form.Del("agb")
	form.Set("slug", "salon-london-2")
	if rec := post(form, "198.51.100.62"); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Please confirm that you are acting as a business") {
		t.Fatalf("AGB en: %d %s", rec.Code, rec.Body)
	}
}
