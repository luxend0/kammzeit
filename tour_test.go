package main

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

var tourOwnerSteps = []string{"leistungen", "team", "einstellungen", "abo"}
var tourAllSteps = []string{"willkommen", "kalender", "leistungen", "team", "einstellungen", "buchungsseite", "kunden", "warteschlange", "abo", "fertig"}

func tourRender(t *testing.T, tpl *template.Template, lang string, u any) string {
	t.Helper()
	var b strings.Builder
	if err := tpl.ExecuteTemplate(&b, "tour", map[string]any{"Lang": lang, "User": u}); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestTourPartial(t *testing.T) {
	tpl, err := template.New("").Funcs(templateFuncs).ParseFS(webFS, "web/*.html")
	if err != nil {
		t.Fatal(err)
	}
	owner := &user{ID: 7}
	// ponytail: anonyme Struktur statt user, solange user noch kein Feld Role hat
	staff := &struct {
		ID   int64
		Role string
	}{8, "staff"}

	de := tourRender(t, tpl, "de", owner)
	for _, s := range tourAllSteps {
		if !strings.Contains(de, `data-tour-step="`+s+`"`) {
			t.Errorf("Inhaber-Tour ohne Schritt %s", s)
		}
	}
	for _, want := range []string{`data-key="kz-tour-7"`, `data-next="Weiter"`, `href="/hilfe"`, `href="/dashboard#kal-steps-titel"`,
		"/static/tour.js", "/static/tour.css", "Willkommen bei Kammzeit"} {
		if !strings.Contains(de, want) {
			t.Errorf("deutsche Tour ohne %q", want)
		}
	}

	// Englisch: jeder Text aus tour.html (und der Knopf in dash.html) ist übersetzt und
	// kein deutscher Text bleibt stehen. Team heißt in beiden Sprachen gleich.
	en := tourRender(t, tpl, "en", owner)
	src, _ := webFS.ReadFile("web/tour.html")
	keys := regexp.MustCompile(`\{\{t \$l "([^"]+)"\}\}`).FindAllStringSubmatch(string(src), -1)
	if len(keys) < 20 {
		t.Fatalf("nur %d Texte in tour.html gefunden", len(keys))
	}
	keys = append(keys, []string{"", "Tour starten"})
	for _, k := range keys {
		if k[1] == "Team" {
			continue
		}
		if tr("en", k[1]) == k[1] {
			t.Errorf("nicht übersetzt: %q", k[1])
		}
		if strings.Contains(en, template.HTMLEscapeString(k[1])) {
			t.Errorf("deutscher Text in der englischen Tour: %q", k[1])
		}
	}
	for _, want := range []string{`data-next="Next"`, `href="/help"`, "Step %d of %d", "Welcome to Kammzeit", "Your booking page"} {
		if !strings.Contains(en, want) {
			t.Errorf("englische Tour ohne %q", want)
		}
	}

	// Mitarbeiter-Login: ohne Leistungen, Team, Einstellungen, Abo und ohne Erste Schritte.
	for _, lang := range []string{"de", "en"} {
		st := tourRender(t, tpl, lang, staff)
		for _, s := range tourOwnerSteps {
			if strings.Contains(st, `data-tour-step="`+s+`"`) {
				t.Errorf("%s: Mitarbeiter-Tour mit Inhaber-Schritt %s", lang, s)
			}
		}
		for _, s := range []string{"willkommen", "kalender", "buchungsseite", "kunden", "warteschlange", "fertig"} {
			if !strings.Contains(st, `data-tour-step="`+s+`"`) {
				t.Errorf("%s: Mitarbeiter-Tour ohne Schritt %s", lang, s)
			}
		}
		if strings.Contains(st, "kal-steps") || !strings.Contains(st, `data-key="kz-tour-8"`) {
			t.Errorf("%s: Mitarbeiter-Tour falsch:\n%s", lang, st)
		}
	}

	if out := tourRender(t, tpl, "de", (*user)(nil)); strings.TrimSpace(out) != "" {
		t.Errorf("ohne Nutzer keine Tour, bekommen:\n%s", out)
	}
}

// tourStaffer hat eine Methode IsStaff, falls die Rollen so eingebaut werden.
type tourStaffer bool

func (s tourStaffer) IsStaff() bool { return bool(s) }

func TestTourStaff(t *testing.T) {
	for _, c := range []struct {
		u    any
		want bool
	}{
		{nil, false},
		{(*user)(nil), false},
		{&user{ID: 1}, false},
		{struct{ Role string }{"owner"}, false},
		{&struct{ Role string }{"staff"}, true},
		{map[string]any{"Role": "staff"}, false},
		{tourStaffer(true), true},
		{tourStaffer(false), false},
	} {
		if got := tourStaff(c.u); got != c.want {
			t.Errorf("tourStaff(%#v) = %v, erwartet %v", c.u, got, c.want)
		}
	}
}

// Die Tour hängt an jeder Dashboard-Seite, der Knopf steht in der Kopfzeile.
func TestTourInDashboard(t *testing.T) {
	a, h := kundenTestApp(t)
	token, salonID := kundenTestSalon(t, a, "tour")
	var uid string
	a.db.QueryRow(`SELECT id FROM users WHERE salon_id = ?`, salonID).Scan(&uid)
	for _, path := range []string{"/dashboard", "/dashboard/kunden", "/dashboard/einstellungen"} {
		req := httptest.NewRequest(http.MethodGet, "http://kammzeit.test"+path, nil)
		req.Header.Set("Cookie", sessionCookie+"="+token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		body := rec.Body.String()
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: Status %d", path, rec.Code)
		}
		for _, want := range []string{`data-tour-start`, `id="kz-tour"`, `data-key="kz-tour-` + uid + `"`, `<script src="/static/tour.js" defer>`} {
			if !strings.Contains(body, want) {
				t.Errorf("%s ohne %q", path, want)
			}
		}
	}
}
