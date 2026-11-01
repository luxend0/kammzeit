package main

import (
	"database/sql"
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	cases := []struct {
		name   string
		days   []teamDay
		blocks int    // erwartete Blöcke, wenn gültig
		err    string // erwarteter Teil der Fehlermeldung, "" = gültig
	}{
		{"zwei Blöcke mit Pause", mon("09:00", "12:00", "13:00", "18:00"), 2, ""},
		{"Blöcke berühren sich", mon("09:00", "12:00", "12:00", "18:00"), 2, ""},
		{"nur zweiter Block", mon("", "", "13:00", "18:00"), 1, ""},
		{"leer = frei", mon("", "", "", ""), 0, ""},
		{"Überlappung", mon("09:00", "13:00", "12:00", "18:00"), 0, "überschneiden"},
		{"Überlappung, später Block zuerst", mon("12:00", "18:00", "09:00", "12:30"), 0, "überschneiden"},
		{"Block im Block", mon("09:00", "18:00", "12:00", "13:00"), 0, "überschneiden"},
		{"Ende vor Beginn", mon("18:00", "09:00", "", ""), 0, "Ende"},
		{"Ende gleich Beginn", mon("09:00", "09:00", "", ""), 0, "Ende"},
		{"nicht im 5-Minuten-Raster", mon("09:03", "12:00", "", ""), 0, "Raster"},
		{"nur Beginn", mon("09:00", "", "", ""), 0, "Raster"},
		{"Unsinn", mon("neun", "zwölf", "", ""), 0, "Raster"},
		{"Stunde 24", mon("09:00", "24:00", "", ""), 0, "Raster"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, msg := teamParseHours(c.days)
			if c.err == "" {
				if msg != "" || len(got) != c.blocks {
					t.Fatalf("got %v, %q; want %d blocks", got, msg, c.blocks)
				}
				for _, h := range got {
					if h.Weekday != time.Monday {
						t.Fatalf("weekday %v", h.Weekday)
					}
				}
				return
			}
			if !strings.Contains(msg, c.err) || !strings.HasPrefix(msg, "Montag") {
				t.Fatalf("msg %q, want it to name Montag and contain %q", msg, c.err)
			}
		})
	}
}

// catalogEnv: Dashboard-Routen dieses Bereichs mit temporärer DB und angemeldetem Salon.
type catalogEnv struct {
	db     *sql.DB
	h      http.Handler
	salon  int64
	cookie string
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
	a := &app{db: db, tpl: tpl, cfg: config{BaseDomain: "kammzeit.test", SiteURL: "https://kammzeit.test", PublicURL: "https://{slug}.kammzeit.test"}}
	mux := http.NewServeMux()
	a.routesCatalog(mux)
	e := &catalogEnv{db: db, h: mux}
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
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req := httptest.NewRequest(method, "http://kammzeit.test"+path, body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "http://kammzeit.test")
	req.Header.Set("Cookie", e.cookie)
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
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
