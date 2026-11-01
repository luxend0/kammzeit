package main

import (
	"database/sql"
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestLeistParsePrice(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		ok   bool
	}{
		{"25", 2500, true},
		{"25,5", 2550, true},
		{"25,50", 2550, true},
		{"25.50", 2550, true},
		{"0", 0, true},
		{"0,05", 5, true},
		{" 12 € ", 1200, true},
		{"99999,99", 9999999, true},
		{"", 0, false},
		{"-5", 0, false},
		{"+5", 0, false},
		{"25,555", 0, false},
		{"1.000,00", 0, false},
		{"25.", 0, false},
		{",50", 0, false},
		{"25,5,0", 0, false},
		{"100000", 0, false},
		{"abc", 0, false},
		{"2 5", 0, false},
		{"1e3", 0, false},
	}
	for _, c := range cases {
		got, err := leistParsePrice(c.in)
		if (err == nil) != c.ok || got != c.want {
			t.Errorf("leistParsePrice(%q) = %d, %v; want %d, ok=%v", c.in, got, err, c.want, c.ok)
		}
	}
}

func TestTeamParseHours(t *testing.T) {
	mon := func(f0, t0, f1, t1 string) []teamDay {
		return []teamDay{{WD: 1, Label: "Montag", From: [2]string{f0, f1}, To: [2]string{t0, t1}}}
	}
	both, first, second := [2]bool{true, true}, [2]bool{true, false}, [2]bool{false, true}
	cases := []struct {
		name   string
		days   []teamDay
		blocks int     // erwartete Blöcke, wenn gültig
		err    string  // erwarteter Anfang der Fehlermeldung am Tag, "" = gültig
		bad    [2]bool // welche Blöcke markiert sind
	}{
		{"zwei Blöcke mit Pause", mon("09:00", "12:00", "13:00", "18:00"), 2, "", [2]bool{}},
		{"Blöcke berühren sich", mon("09:00", "12:00", "12:00", "18:00"), 2, "", [2]bool{}},
		{"nur zweiter Block", mon("", "", "13:00", "18:00"), 1, "", [2]bool{}},
		{"leer = frei", mon("", "", "", ""), 0, "", [2]bool{}},
		{"Überlappung", mon("09:00", "13:00", "12:00", "18:00"), 0, "Montag: Die beiden Blöcke überschneiden sich", both},
		{"Überlappung, später Block zuerst", mon("12:00", "18:00", "09:00", "12:30"), 0, "Montag: Die beiden Blöcke überschneiden", both},
		{"Block im Block", mon("09:00", "18:00", "12:00", "13:00"), 0, "Montag: Die beiden Blöcke überschneiden", both},
		{"Ende vor Beginn", mon("18:00", "09:00", "", ""), 0, "Montag Block 1: Das Ende muss nach dem Beginn liegen", first},
		{"Ende gleich Beginn", mon("09:00", "09:00", "", ""), 0, "Montag Block 1: Das Ende", first},
		{"Block 2 Ende vor Beginn", mon("09:00", "12:00", "18:00", "13:00"), 0, "Montag Block 2: Das Ende", second},
		{"beide Blöcke falsch", mon("09:03", "12:00", "18:00", "13:00"), 0, "Montag Block 1: Bitte Beginn und Ende angeben, im 5-Minuten-Raster (z. B. 09:00). Montag Block 2: Das Ende", both},
		{"nicht im 5-Minuten-Raster", mon("09:03", "12:00", "", ""), 0, "Montag Block 1: Bitte Beginn und Ende angeben, im 5-Minuten-Raster", first},
		{"nur Beginn", mon("09:00", "", "", ""), 0, "Montag Block 1: Bitte Beginn", first},
		{"Unsinn", mon("neun", "zwölf", "", ""), 0, "Montag Block 1: Bitte Beginn", first},
		{"Stunde 24", mon("", "", "09:00", "24:00"), 0, "Montag Block 2: Bitte Beginn", second},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := teamParseHours("de", c.days)
			d := c.days[0]
			if c.err == "" {
				if !ok || d.Err != "" || len(got) != c.blocks {
					t.Fatalf("got %v, %v, %q; want %d blocks", got, ok, d.Err, c.blocks)
				}
				for _, h := range got {
					if h.Weekday != time.Monday {
						t.Fatalf("weekday %v", h.Weekday)
					}
				}
				return
			}
			if ok || got != nil || !strings.HasPrefix(d.Err, c.err) || d.Bad != c.bad {
				t.Fatalf("ok=%v, Err %q, Bad %v; want Err starting with %q, Bad %v", ok, d.Err, d.Bad, c.err, c.bad)
			}
		})
	}
	// Fehler an mehreren Tagen landen jeweils am eigenen Tag.
	days := []teamDay{
		{WD: 1, Label: "Montag", From: [2]string{"18:00"}, To: [2]string{"09:00"}},
		{WD: 2, Label: "Dienstag", From: [2]string{"09:00"}, To: [2]string{"18:00"}},
		{WD: 3, Label: "Mittwoch", From: [2]string{"", "09:00"}, To: [2]string{"", ""}},
	}
	if _, ok := teamParseHours("de", days); ok || days[0].Err == "" || days[1].Err != "" || !strings.HasPrefix(days[2].Err, "Mittwoch Block 2") {
		t.Fatalf("per-day errors: %q / %q / %q", days[0].Err, days[1].Err, days[2].Err)
	}
}

func TestTeamCopySource(t *testing.T) {
	week := func(filled ...int) []teamDay {
		var days []teamDay
		for _, wd := range teamOrder {
			d := teamDay{WD: int(wd), Label: weekdaysDE[wd]}
			if slices.Contains(filled, int(wd)) {
				d.From[0], d.To[0] = "09:00", "18:00"
			}
			days = append(days, d)
		}
		return days
	}
	cases := []struct {
		filled []int
		src    int
		label  string
	}{
		{[]int{1, 2, 3, 4, 5}, 1, "Montag auf Dienstag bis Freitag übertragen"},
		{[]int{2, 3, 4, 5, 6}, 2, "Dienstag auf Mittwoch bis Freitag übertragen"}, // montags zu
		{[]int{4, 6}, 4, "Donnerstag auf Freitag übertragen"},
		{[]int{5, 6}, 0, ""}, // nur Fr/Sa: nichts zu übertragen
		{nil, 0, ""},
	}
	for _, c := range cases {
		if src, label := teamCopySource("de", week(c.filled...)); src != c.src || label != c.label {
			t.Errorf("filled %v: %d %q, want %d %q", c.filled, src, label, c.src, c.label)
		}
	}
}

// catalogEnv: Dashboard-Routen dieses Bereichs mit temporärer DB und angemeldetem Salon.
// host = Hauptdomain der Anfragen: kammzeit.test (Deutsch) oder kammzeit-en.test (Englisch).
type catalogEnv struct {
	db     *sql.DB
	h      http.Handler
	salon  int64
	cookie string
	host   string
}

func newCatalogEnv(t *testing.T, plan string) *catalogEnv {
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
	a := &app{db: db, tpl: tpl, cfg: config{BaseDomain: "kammzeit.test", SiteURL: "https://kammzeit.test", PublicURL: "https://{slug}.kammzeit.test",
		Domains: parseDomains("kammzeit.test=de,kammzeit-en.test=en", "")}}
	mux := http.NewServeMux()
	a.routesCatalog(mux)
	e := &catalogEnv{db: db, h: mux, host: "kammzeit.test"}
	e.salon, e.cookie = catalogSalon(t, db, "mueller", plan)
	return e
}

// catalogSalon legt Salon, Inhaber und Session an (wie printSession in main.go).
func catalogSalon(t *testing.T, db *sql.DB, slug, plan string) (int64, string) {
	t.Helper()
	salon := catalogInsert(t, db, `INSERT INTO salons (slug, name, plan) VALUES (?, ?, ?)`, slug, slug, plan)
	user := catalogInsert(t, db, `INSERT INTO users (salon_id, email) VALUES (?, ?)`, salon, slug+"@example.com")
	token := randomToken()
	catalogInsert(t, db, `INSERT INTO sessions (token_hash, user_id, expires_at) VALUES (?, ?, ?)`,
		hashToken(token), user, time.Now().Add(time.Hour).Unix())
	return salon, sessionCookie + "=" + token
}

func catalogInsert(t *testing.T, db *sql.DB, q string, args ...any) int64 {
	t.Helper()
	res, err := db.Exec(q, args...)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

func (e *catalogEnv) count(t *testing.T, q string, args ...any) int {
	t.Helper()
	var n int
	if err := e.db.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (e *catalogEnv) do(method, path string, form url.Values) (int, string) {
	rec := e.rec(method, path, form)
	return rec.Code, rec.Body.String()
}

func (e *catalogEnv) rec(method, path string, form url.Values) *httptest.ResponseRecorder {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req := httptest.NewRequest(method, "http://"+e.host+path, body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "http://"+e.host)
	req.Header.Set("Cookie", e.cookie)
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

func TestTeamPlanLimit(t *testing.T) {
	e := newCatalogEnv(t, "free")
	catalogInsert(t, e.db, `INSERT INTO staff (salon_id, name) VALUES (?, 'Anna')`, e.salon)
	carla := strconv.FormatInt(catalogInsert(t, e.db, `INSERT INTO staff (salon_id, name, active) VALUES (?, 'Carla', 0)`, e.salon), 10)
	active := func() int { return e.count(t, `SELECT COUNT(*) FROM staff WHERE salon_id = ? AND active = 1`, e.salon) }

	// Gratis: Hinweis statt Formular, zweiter aktiver Mitarbeiter scheitert.
	code, body := e.do("GET", "/dashboard/team/neu", nil)
	if code != http.StatusOK || !strings.Contains(body, `href="/dashboard/abo"`) || strings.Contains(body, `name="name"`) {
		t.Fatalf("GET neu: %d, want hint with /dashboard/abo and no form\n%s", code, body)
	}
	code, body = e.do("POST", "/dashboard/team/neu", url.Values{"name": {"Ben"}, "aktiv": {"1"}})
	if code != http.StatusForbidden || !strings.Contains(body, "/dashboard/abo") || active() != 1 {
		t.Fatalf("POST neu on free plan: %d, %d active", code, active())
	}
	// Reaktivieren zählt mit.
	code, body = e.do("POST", "/dashboard/team/"+carla, url.Values{"name": {"Carla"}, "aktiv": {"1"}})
	if code != http.StatusForbidden || !strings.Contains(body, "/dashboard/abo") || active() != 1 {
		t.Fatalf("reactivate on free plan: %d, %d active", code, active())
	}
	// Inaktive bearbeiten geht weiter.
	if code, _ := e.do("POST", "/dashboard/team/"+carla, url.Values{"name": {"Carla B."}}); code != http.StatusSeeOther {
		t.Fatalf("edit inactive: %d", code)
	}

	// Pro: beides geht.
	if _, err := e.db.Exec(`UPDATE salons SET plan = 'pro' WHERE id = ?`, e.salon); err != nil {
		t.Fatal(err)
	}
	if code, _ := e.do("POST", "/dashboard/team/neu", url.Values{"name": {"Ben"}, "aktiv": {"1"}}); code != http.StatusSeeOther || active() != 2 {
		t.Fatalf("POST neu on pro: %d, %d active", code, active())
	}
	if code, _ := e.do("POST", "/dashboard/team/"+carla, url.Values{"name": {"Carla"}, "aktiv": {"1"}}); code != http.StatusSeeOther || active() != 3 {
		t.Fatalf("reactivate on pro: %d, %d active", code, active())
	}
}

func TestTeamServicesTenant(t *testing.T) {
	e := newCatalogEnv(t, "pro")
	other, _ := catalogSalon(t, e.db, "fremd", "pro")
	own := catalogInsert(t, e.db, `INSERT INTO services (salon_id, name, duration_min) VALUES (?, 'Schnitt', 30)`, e.salon)
	foreign := catalogInsert(t, e.db, `INSERT INTO services (salon_id, name, duration_min) VALUES (?, 'Fremd', 30)`, other)
	anna := catalogInsert(t, e.db, `INSERT INTO staff (salon_id, name) VALUES (?, 'Anna')`, e.salon)
	stranger := catalogInsert(t, e.db, `INSERT INTO staff (salon_id, name) VALUES (?, 'Fremder')`, other)
	id := func(n int64) string { return strconv.FormatInt(n, 10) }

	// Fremde service_id im Formular wird ignoriert, eigene gespeichert.
	code, body := e.do("POST", "/dashboard/team/"+id(anna), url.Values{
		"name": {"Anna"}, "aktiv": {"1"}, "leistung": {id(own), id(foreign)},
		"von-1-0": {"09:00"}, "bis-1-0": {"12:00"}, "von-1-1": {"13:00"}, "bis-1-1": {"18:00"},
	})
	if code != http.StatusSeeOther {
		t.Fatalf("save: %d\n%s", code, body)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM staff_services WHERE staff_id = ?`, anna); n != 1 {
		t.Fatalf("staff_services rows = %d, want 1", n)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM staff_services WHERE staff_id = ? AND service_id = ?`, anna, foreign); n != 0 {
		t.Fatal("foreign service assigned")
	}
	if n := e.count(t, `SELECT COUNT(*) FROM working_hours WHERE staff_id = ?`, anna); n != 2 {
		t.Fatalf("working_hours rows = %d, want 2", n)
	}

	// Fremde Mitarbeiter und Leistungen: nicht sehen, nicht ändern, nicht löschen.
	for _, c := range []struct{ method, path string }{
		{"GET", "/dashboard/team/" + id(stranger)},
		{"POST", "/dashboard/team/" + id(stranger)},
		{"POST", "/dashboard/team/" + id(stranger) + "/loeschen"},
		{"GET", "/dashboard/leistungen/" + id(foreign)},
		{"POST", "/dashboard/leistungen/" + id(foreign)},
		{"POST", "/dashboard/leistungen/" + id(foreign) + "/loeschen"},
		{"POST", "/dashboard/leistungen/" + id(foreign) + "/verschieben"},
	} {
		form := url.Values{"name": {"gehackt"}, "aktiv": {"1"}, "dauer": {"30"}, "preis": {"1"}}
		if code, _ := e.do(c.method, c.path, form); code != http.StatusNotFound {
			t.Errorf("%s %s: %d, want 404", c.method, c.path, code)
		}
	}
	if n := e.count(t, `SELECT COUNT(*) FROM staff WHERE id = ? AND name = 'Fremder'`, stranger); n != 1 {
		t.Fatal("foreign staff changed or deleted")
	}
	if n := e.count(t, `SELECT COUNT(*) FROM services WHERE id = ? AND name = 'Fremd'`, foreign); n != 1 {
		t.Fatal("foreign service changed or deleted")
	}

	// Neue Leistung geht an alle aktiven Mitarbeiter, aber nur des eigenen Salons.
	code, _ = e.do("POST", "/dashboard/leistungen/neu", url.Values{"name": {"Bart"}, "dauer": {"15"}, "preis": {"9,5"}, "aktiv": {"1"}})
	if code != http.StatusSeeOther {
		t.Fatalf("new service: %d", code)
	}
	var bart, cents int64
	if err := e.db.QueryRow(`SELECT id, price_cents FROM services WHERE salon_id = ? AND name = 'Bart'`, e.salon).Scan(&bart, &cents); err != nil || cents != 950 {
		t.Fatalf("new service: %v, %d cents", err, cents)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM staff_services WHERE service_id = ?`, bart); n != 1 {
		t.Fatalf("new service assigned to %d staff, want 1 (Anna)", n)
	}
}

// Fehler stehen am Feld: aria-invalid, aria-describedby und der Text mit passender id.
func TestCatalogFieldErrors(t *testing.T) {
	e := newCatalogEnv(t, "pro")
	code, body := e.do("POST", "/dashboard/leistungen/neu", url.Values{"name": {""}, "dauer": {"7"}, "preis": {"abc"}, "beschreibung": {"kurz"}})
	if code != http.StatusUnprocessableEntity || !strings.Contains(body, catalogInvalid) {
		t.Fatalf("leistung: %d\n%s", code, body)
	}
	for _, f := range []string{"name", "dauer", "preis"} {
		if !regexp.MustCompile(`name="`+f+`"[^>]*aria-invalid="true"[^>]*aria-describedby="err-`+f+`"`).MatchString(body) ||
			!strings.Contains(body, `<span class="field-error" id="err-`+f+`">`) {
			t.Errorf("leistung: field %s not marked", f)
		}
	}
	if strings.Contains(body, `id="err-beschreibung"`) || strings.Count(body, `aria-invalid="true"`) != 3 {
		t.Errorf("leistung: unexpected markers\n%s", body)
	}
	if !strings.Contains(body, `aria-describedby="leist-beschreibung-hint"`) || !strings.Contains(body, `value="abc"`) {
		t.Error("leistung: hint not linked or input lost")
	}
	if n := e.count(t, `SELECT COUNT(*) FROM services WHERE salon_id = ?`, e.salon); n != 0 {
		t.Fatalf("saved despite errors: %d", n)
	}

	anna := strconv.FormatInt(catalogInsert(t, e.db, `INSERT INTO staff (salon_id, name) VALUES (?, 'Anna')`, e.salon), 10)
	code, body = e.do("POST", "/dashboard/team/"+anna, url.Values{"name": {"Anna"}, "aktiv": {"1"},
		"von-1-0": {"18:00"}, "bis-1-0": {"09:00"}, "von-1-1": {"13:00"}, "bis-1-1": {"18:00"}, "von-2-0": {"09:00"}, "bis-2-0": {"18:00"}})
	if code != http.StatusUnprocessableEntity || !strings.Contains(body, catalogInvalid) {
		t.Fatalf("team: %d\n%s", code, body)
	}
	if !strings.Contains(body, `<span class="field-error" id="err-tag-1">Montag Block 1: Das Ende muss nach dem Beginn liegen.</span>`) {
		t.Errorf("team: Montag error missing\n%s", body)
	}
	for _, f := range []string{"von-1-0", "bis-1-0"} {
		if !regexp.MustCompile(`name="` + f + `"[^>]*aria-invalid="true" aria-describedby="err-tag-1"`).MatchString(body) {
			t.Errorf("team: %s not marked", f)
		}
	}
	if regexp.MustCompile(`name="von-1-1"[^>]*aria-invalid`).MatchString(body) || strings.Contains(body, `err-tag-2`) {
		t.Error("team: valid fields marked")
	}
	if n := e.count(t, `SELECT COUNT(*) FROM working_hours WHERE staff_id = ?`, anna); n != 0 {
		t.Fatalf("hours saved despite errors: %d", n)
	}

	// Gültig gespeichert: Umleitung mit festem Code für die Meldung.
	for path, want := range map[string]string{"/dashboard/leistungen/neu": "leistung-neu", "/dashboard/team/" + anna: "mitarbeiter"} {
		rec := e.rec("POST", path, url.Values{"name": {"Anna"}, "aktiv": {"1"}, "dauer": {"30"}, "preis": {"25"}})
		if loc := rec.Header().Get("Location"); rec.Code != http.StatusSeeOther || !strings.HasSuffix(loc, "?gespeichert="+want) || catalogSaved[want] == "" {
			t.Errorf("POST %s: %d %q, want code %s", path, rec.Code, loc, want)
		}
	}
}

// "Zeiten übertragen" kopiert die Vorlage auf die Werktage danach bis Freitag und speichert nichts.
func TestTeamCopyHours(t *testing.T) {
	e := newCatalogEnv(t, "pro")
	anna := strconv.FormatInt(catalogInsert(t, e.db, `INSERT INTO staff (salon_id, name) VALUES (?, 'Anna')`, e.salon), 10)
	code, body := e.do("POST", "/dashboard/team/"+anna, url.Values{"name": {"Anna B."}, "aktiv": {"1"}, "uebertragen": {"2"},
		"von-2-0": {"09:00"}, "bis-2-0": {"12:00"}, "von-2-1": {"13:00"}, "bis-2-1": {"18:00"},
		"von-3-0": {"10:00"}, "bis-3-0": {"14:00"}, "von-6-0": {"08:00"}, "bis-6-0": {"13:00"}})
	if code != http.StatusOK || !strings.Contains(body, "Zeiten übertragen: Dienstag auf Mittwoch bis Freitag. Noch nicht gespeichert.") {
		t.Fatalf("copy: %d\n%s", code, body)
	}
	for _, want := range []string{
		`name="von-3-0" value="09:00"`, `name="bis-3-1" value="18:00"`, // Mittwoch überschrieben
		`name="von-5-0" value="09:00"`, `name="bis-5-1" value="18:00"`, // Freitag
		`name="von-1-0" value=""`,      // Montag bleibt frei
		`name="von-6-0" value="08:00"`, // Samstag bleibt
		`value="Anna B."`,
		`name="uebertragen" value="2">Dienstag auf Mittwoch bis Freitag übertragen</button>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("copy: missing %s", want)
		}
	}
	// Enter im Formular soll speichern: der erste Absende-Knopf hat keinen Namen.
	form := body[strings.Index(body, `<form method="post" class="form form-wide"`):]
	if first := form[strings.Index(form, `type="submit"`)-40:]; strings.Contains(first[:80], "uebertragen") {
		t.Error("copy button is the default button")
	}
	if n := e.count(t, `SELECT COUNT(*) FROM working_hours WHERE staff_id = ?`, anna); n != 0 {
		t.Fatalf("copy saved hours: %d", n)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM staff WHERE name = 'Anna'`); n != 1 {
		t.Fatal("copy saved the name")
	}
}

// Löschen: Rückfrage per <details>, mit Terminen nur deaktivieren, Meldung über festen Code.
func TestCatalogDelete(t *testing.T) {
	e := newCatalogEnv(t, "pro")
	id := func(n int64) string { return strconv.FormatInt(n, 10) }
	anna := catalogInsert(t, e.db, `INSERT INTO staff (salon_id, name) VALUES (?, 'Anna')`, e.salon)
	ben := catalogInsert(t, e.db, `INSERT INTO staff (salon_id, name) VALUES (?, 'Ben')`, e.salon)
	used := catalogInsert(t, e.db, `INSERT INTO services (salon_id, name, duration_min) VALUES (?, 'Schnitt', 30)`, e.salon)
	unused := catalogInsert(t, e.db, `INSERT INTO services (salon_id, name, duration_min) VALUES (?, 'Bart', 15)`, e.salon)
	cust := catalogInsert(t, e.db, `INSERT INTO customers (salon_id, name) VALUES (?, 'Kunde')`, e.salon)
	catalogInsert(t, e.db, `INSERT INTO appointments (salon_id, staff_id, service_id, customer_id, starts_at, ends_at, cancel_token)
		VALUES (?, ?, ?, ?, 1000, 2800, 'tok')`, e.salon, anna, used, cust)

	cases := []struct {
		page, confirm, action, saved string
		gone                         bool
	}{
		{"/dashboard/leistungen/" + id(used), "Ja, deaktivieren", "/dashboard/leistungen/" + id(used) + "/loeschen", "leistung-deaktiviert", false},
		{"/dashboard/leistungen/" + id(unused), "Ja, löschen", "/dashboard/leistungen/" + id(unused) + "/loeschen", "leistung-geloescht", true},
		{"/dashboard/team/" + id(anna), "Ja, deaktivieren", "/dashboard/team/" + id(anna) + "/loeschen", "mitarbeiter-deaktiviert", false},
		{"/dashboard/team/" + id(ben), "Ja, löschen", "/dashboard/team/" + id(ben) + "/loeschen", "mitarbeiter-geloescht", true},
	}
	for _, c := range cases {
		code, body := e.do("GET", c.page, nil)
		if code != http.StatusOK || !strings.Contains(body, `<details class="catalog-del">`) || !strings.Contains(body, c.confirm+"</button>") {
			t.Errorf("GET %s: %d, want details with %q", c.page, code, c.confirm)
			continue
		}
		rec := e.rec("POST", c.action, url.Values{})
		loc, _ := url.Parse(rec.Header().Get("Location"))
		if rec.Code != http.StatusSeeOther || loc.Query().Get("gespeichert") != c.saved || catalogSaved[c.saved] == "" {
			t.Errorf("POST %s: %d %q, want code %s", c.action, rec.Code, rec.Header().Get("Location"), c.saved)
		}
	}
	if e.count(t, `SELECT COUNT(*) FROM services WHERE id = ? AND active = 0`, used) != 1 || e.count(t, `SELECT COUNT(*) FROM services WHERE id = ?`, unused) != 0 {
		t.Error("services not deactivated/deleted")
	}
	if e.count(t, `SELECT COUNT(*) FROM staff WHERE id = ? AND active = 0`, anna) != 1 || e.count(t, `SELECT COUNT(*) FROM staff WHERE id = ?`, ben) != 0 {
		t.Error("staff not deactivated/deleted")
	}
	// Deaktiviert und mit Terminen: kein Löschen-Knopf mehr, nur der Hinweis.
	if _, body := e.do("GET", "/dashboard/leistungen/"+id(used), nil); strings.Contains(body, "<details") || !strings.Contains(body, "kann deshalb nicht gelöscht werden") {
		t.Error("inactive service with appointments still offers delete")
	}
	// Verschieben: fester Code und Anker auf die Zeile.
	rec := e.rec("POST", "/dashboard/leistungen/"+id(used)+"/verschieben", url.Values{"richtung": {"hoch"}})
	if loc := rec.Header().Get("Location"); loc != "/dashboard/leistungen?gespeichert=reihenfolge#leist-"+id(used) {
		t.Errorf("verschieben: Location %q", loc)
	}
}

func TestTeamHoursSummary(t *testing.T) {
	var hours []WorkingHours
	for wd := time.Tuesday; wd <= time.Friday; wd++ {
		hours = append(hours, WorkingHours{wd, 9 * 60, 18 * 60})
	}
	hours = append(hours, WorkingHours{time.Saturday, 9 * 60, 14 * 60})
	for lang, want := range map[string]string{"de": "Di-Fr 9:00-18:00|Sa 9:00-14:00", "en": "Tue-Fri 9:00-18:00|Sat 9:00-14:00"} {
		if got := strings.Join(teamHoursSummary(lang, hours), "|"); got != want {
			t.Errorf("%s: %q, want %q", lang, got, want)
		}
	}
}

// Englisch auf der .com-Domain: Texte, Wochentage, Preise, Meldungen und Feldfehler.
func TestCatalogEnglish(t *testing.T) {
	e := newCatalogEnv(t, "free")
	e.host = "kammzeit-en.test"
	has := func(name, body string, want ...string) {
		t.Helper()
		for _, s := range want {
			if !strings.Contains(body, s) {
				t.Errorf("%s: missing %q", name, s)
			}
		}
	}
	svc := strconv.FormatInt(catalogInsert(t, e.db, `INSERT INTO services (salon_id, name, duration_min, price_cents) VALUES (?, 'Cut', 30, 2550)`, e.salon), 10)
	anna := strconv.FormatInt(catalogInsert(t, e.db, `INSERT INTO staff (salon_id, name) VALUES (?, 'Anna')`, e.salon), 10)
	catalogInsert(t, e.db, `INSERT INTO working_hours (staff_id, weekday, start_min, end_min) VALUES (?, 2, 540, 1080), (?, 3, 540, 1080)`, anna, anna)

	_, body := e.do("GET", "/dashboard/leistungen?gespeichert=reihenfolge", nil)
	has("services", body, "<h1>Services</h1>", "New service", "€25.50", "30 min", `aria-label="Move Cut up"`,
		"Order changed. Services appear in this order on your booking page too.")
	_, body = e.do("GET", "/dashboard/leistungen/"+svc, nil)
	has("service form", body, "<h1>Edit service</h1>", `value="25.50"`, `placeholder="25.00"`, "Price in euros", "Delete service", "Yes, delete</button>")
	code, body := e.do("POST", "/dashboard/leistungen/neu", url.Values{"name": {""}, "dauer": {"7"}, "preis": {"abc"}})
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("service errors: %d", code)
	}
	has("service errors", body, "Please check the highlighted fields. Nothing has been saved yet.",
		"The duration must be between 5 and 480 minutes, in 5-minute steps.", "Please enter a valid price, e.g. 25 or 25.50.")
	if rec := e.rec("POST", "/dashboard/leistungen/neu", url.Values{"name": {"Colour"}, "dauer": {"60"}, "preis": {"54.90"}, "aktiv": {"1"}}); rec.Code != http.StatusSeeOther ||
		e.count(t, `SELECT COUNT(*) FROM services WHERE name = 'Colour' AND price_cents = 5490`) != 1 {
		t.Fatalf("service with English price: %d", rec.Code)
	}

	_, body = e.do("GET", "/dashboard/team?gespeichert=mitarbeiter", nil)
	has("team", body, "<h1>Team</h1>", "New staff member", "Working hours", "Tue-Wed 9:00-18:00", "Staff member saved.",
		"The Free plan allows 1 active staff member. Pro allows unlimited staff.", "See Pro")
	_, body = e.do("GET", "/dashboard/team/"+anna, nil)
	has("team form", body, "<h1>Edit staff member</h1>", ">Monday</th>", ">Sunday</th>", `aria-label="Tuesday, block 1, start"`,
		"Copy Tuesday to Wednesday-Friday</button>", "Empty means day off.")
	code, body = e.do("POST", "/dashboard/team/"+anna, url.Values{"name": {"Anna"}, "aktiv": {"1"}, "von-1-0": {"18:00"}, "bis-1-0": {"09:00"},
		"von-2-0": {"09:00"}, "bis-2-0": {"13:00"}, "von-2-1": {"12:00"}, "bis-2-1": {"18:00"}})
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("team errors: %d", code)
	}
	has("team errors", body, ">Monday block 1: The end must be after the start.</span>", ">Tuesday: The two blocks overlap.</span>")
	_, body = e.do("POST", "/dashboard/team/"+anna, url.Values{"name": {"Anna"}, "aktiv": {"1"}, "uebertragen": {"2"}, "von-2-0": {"09:00"}, "bis-2-0": {"18:00"}})
	has("copy", body, "Hours copied: Tuesday to Wednesday-Friday. Not saved yet.")
	code, body = e.do("GET", "/dashboard/team/neu", nil)
	has("limit", body, "See Pro", "With Pro you can have as many active staff as you like, for €14.90 per month or €149 per year.")
	if code != http.StatusOK {
		t.Errorf("limit: %d", code)
	}
}
