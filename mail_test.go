package main

import (
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// mailFix: App mit zwei Domains (de/en), Salon "haar" (Deutsch, Sonderzeichen im
// Namen, Inhaber-Konto auf Englisch) und Salon "cuts" (Englisch, blasse Akzentfarbe).
type mailFix struct {
	a          *app
	haar, cuts *salon
}

func newMailFix(t *testing.T) mailFix {
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
	a := &app{db: db, tpl: tpl, cfg: config{BaseDomain: "kammzeit.test", SiteURL: "https://kammzeit.test",
		PublicURL: "https://{slug}.kammzeit.test", Domains: parseDomains("kammzeit.test=de,kammzeit-en.test=en", "")}}
	for _, q := range []string{
		`INSERT INTO salons (id, slug, name, address, phone, accent, cancel_notice_min, imprint_url, lang)
			VALUES (1, 'haar', 'Haar & Bart <Studio>', 'Hauptstr. 1' || char(10) || '55469 Simmern', '+49 6761 12345', '#3b82f6', 1440, 'salon-haar.example/impressum', 'de')`,
		`INSERT INTO salons (id, slug, name, address, phone, accent, lang) VALUES (2, 'cuts', 'Cuts O''Brien', '1 Main Street', '', '#ffeb3b', 'en')`,
		`INSERT INTO users (salon_id, email, lang) VALUES (1, 'owner@haar.example', 'en'), (2, 'owner@cuts.example', 'en')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	f := mailFix{a: a}
	if f.haar, err = a.loadSalon(1); err != nil {
		t.Fatal(err)
	}
	if f.cuts, err = a.loadSalon(2); err != nil {
		t.Fatal(err)
	}
	return f
}

func mailBooking(s *salon) *booking {
	start := time.Date(2026, 11, 3, 10, 30, 0, 0, s.Loc)
	return &booking{ID: 42, Token: "tok42", Start: start, End: start.Add(45 * time.Minute), Status: "booked",
		Service: "Cut and blow-dry", Staff: "Anna", Customer: "Kim & Co", Email: "kim@example.com", Phone: "0170 1234"}
}

var mailLeftovers = []string{"{{", "}}", "<no value>", "<nil>", "ZgotmplZ", "%!", "&amp;", "&#", "&lt;"}

// TestMailsRender: jede Mail in de und en, Textteil mit den wichtigen Links,
// keine Template-Reste, HTML ohne Skript und nur mit eigenen Bildern.
func TestMailsRender(t *testing.T) {
	f := newMailFix(t)
	now := time.Date(2026, 11, 1, 9, 0, 0, 0, f.haar.Loc)
	dump := os.Getenv("KAMMZEIT_MAIL_DUMP") // Rauchtest: HTML-Teile als Dateien
	for _, lang := range []string{"de", "en"} {
		s := f.haar
		if lang == "en" {
			s = f.cuts
		}
		b := mailBooking(s)
		cust := func(extra map[string]any) func() map[string]any {
			return func() map[string]any {
				d := f.a.customerMailData(s, b, now)
				for k, v := range extra {
					d[k] = v
				}
				return d
			}
		}
		salonD := func() map[string]any { _, d := f.a.salonMailData(s, b); return d }
		site := f.a.siteURLLang(lang)
		book := f.a.bookingURL(s, "/")
		ics, cancel := f.a.bookingURL(s, "/termin/tok42.ics"), f.a.bookingURL(s, "/storno/tok42")
		cases := []struct {
			name  string
			data  func() map[string]any
			links []string
		}{
			{"login", func() map[string]any { return map[string]any{"Link": site + "/login/abc?weiter=%2Fdashboard"} }, []string{site + "/login/abc?weiter=%2Fdashboard"}},
			{"booking-confirm", cust(nil), []string{ics, cancel}},
			{"booking-confirm", cust(map[string]any{"BySalon": true}), []string{ics, cancel}},
			{"reminder", cust(nil), []string{ics, cancel}},
			{"moved", cust(map[string]any{"Old": b.Start.Add(-24 * time.Hour)}), []string{ics, cancel}},
			{"cancelled-by-salon", cust(nil), []string{book}},
			{"cancel-customer", cust(nil), []string{book}},
			{"booking-salon", salonD, []string{"/dashboard/termine/42"}},
			{"cancel-salon", salonD, []string{"/dashboard?tag=2026-11-03"}},
			{"marketing-confirm", func() map[string]any { return map[string]any{"ConfirmURL": site + "/news/bestaetigen/xyz"} }, []string{site + "/news/bestaetigen/xyz"}},
			{"queue-called", func() map[string]any {
				return map[string]any{"Name": "Kim", "Number": 7, "SalonName": s.Name, "StatusURL": book + "warteschlange/abc"}
			}, []string{book + "warteschlange/abc"}},
		}
		for _, c := range cases {
			subj, text, page, err := f.a.renderMail(lang, c.name, c.data())
			if err != nil {
				t.Fatalf("%s/%s: %v", c.name, lang, err)
			}
			for _, part := range []string{subj, text} {
				for _, bad := range mailLeftovers {
					if strings.Contains(part, bad) {
						t.Errorf("%s/%s: %q im Betreff/Text:\n%s", c.name, lang, bad, part)
					}
				}
			}
			for _, l := range c.links {
				if !strings.Contains(text, l) {
					t.Errorf("%s/%s: Link %s fehlt im Text:\n%s", c.name, lang, l, text)
				}
				if !strings.Contains(page, template.HTMLEscapeString(l)) {
					t.Errorf("%s/%s: Link %s fehlt im HTML", c.name, lang, l)
				}
			}
			if subj == "" || strings.Count(text, "\n") < 4 {
				t.Errorf("%s/%s: Betreff %q oder Text zu kurz", c.name, lang, subj)
			}
			for _, bad := range []string{"<script", "<no value>", "<nil>", "ZgotmplZ", "%!", "{{"} {
				if strings.Contains(strings.ToLower(page), strings.ToLower(bad)) {
					t.Errorf("%s/%s: %q im HTML", c.name, lang, bad)
				}
			}
			if !strings.Contains(page, `<html lang="`+lang+`">`) || !strings.HasSuffix(strings.TrimSpace(page), "</html>") {
				t.Errorf("%s/%s: HTML-Gerüst unvollständig", c.name, lang)
			}
			// Kein Tracking: Bilder nur aus /static/img/mail/ der eigenen Domain, Links direkt.
			for _, m := range regexp.MustCompile(`<img src="([^"]*)"`).FindAllStringSubmatch(page, -1) {
				if !strings.HasPrefix(m[1], site+"/static/img/mail/") {
					t.Errorf("%s/%s: fremdes Bild %s", c.name, lang, m[1])
				}
			}
			for _, m := range regexp.MustCompile(`href="([^"]*)"`).FindAllStringSubmatch(page, -1) {
				if !strings.HasPrefix(m[1], "https://") && !strings.HasPrefix(m[1], "tel:") {
					t.Errorf("%s/%s: unerwarteter Link %s", c.name, lang, m[1])
				}
			}
			// Sprache: kein Deutsch in englischen Mails und umgekehrt.
			words := []string{"Hallo", "Termin", "Leistung", " Uhr", "Bitte"}
			if lang == "de" {
				words = []string{"Hello", "appointment", "Please", "booking"}
			}
			for _, w := range words {
				if strings.Contains(text, w) || strings.Contains(page, w) {
					t.Errorf("%s/%s: %q passt nicht zur Sprache", c.name, lang, w)
				}
			}
			if dump != "" {
				base := filepath.Join(dump, c.name+"-"+lang)
				if err := os.WriteFile(base+".html", []byte(page), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(base+".txt", []byte("Betreff: "+subj+"\n\n"+text), 0o644); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}

func TestMailEscapingAndSalon(t *testing.T) {
	f := newMailFix(t)
	b := mailBooking(f.haar)
	now := time.Date(2026, 11, 1, 9, 0, 0, 0, f.haar.Loc)
	subj, text, page, err := f.a.renderMail("de", "booking-confirm", f.a.customerMailData(f.haar, b, now))
	if err != nil {
		t.Fatal(err)
	}
	if subj != "Termin bestätigt: Haar & Bart <Studio>" || !strings.Contains(text, "Hallo Kim & Co,") {
		t.Errorf("Klartext falsch maskiert: %q\n%s", subj, text)
	}
	if strings.Contains(page, "<Studio>") || !strings.Contains(page, "Haar &amp; Bart &lt;Studio&gt;") {
		t.Error("Salon-Name im HTML nicht maskiert")
	}
	for _, want := range []string{
		"Online absagen geht bis Montag, 2.11.2026, 10:30 Uhr", // Absagefrist 24 h
		"Hauptstr. 1", "55469 Simmern", `href="tel:&#43;49676112345"`,
		`bgcolor="#3b82f6"`,                           // Button in der Akzentfarbe
		`href="https://salon-haar.example/impressum"`, // Impressum des Salons
	} {
		if !strings.Contains(page, want) && !strings.Contains(text, want) {
			t.Errorf("fehlt: %s", want)
		}
	}
	if !strings.Contains(page, "Online-Termine mit") {
		t.Error("Gratis-Salon ohne Hinweis im Fuß")
	}
	f.haar.Plan = "pro"
	if _, _, page, _ := f.a.renderMail("de", "booking-confirm", f.a.customerMailData(f.haar, b, now)); strings.Contains(page, "Online-Termine mit") {
		t.Error("Pro-Salon mit Kammzeit-Hinweis")
	}

	// Absagefrist vorbei: kein Absage-Button, stattdessen anrufen.
	late := b.Start.Add(-time.Hour)
	_, text, page, _ = f.a.renderMail("de", "reminder", f.a.customerMailData(f.haar, b, late))
	if strings.Contains(page, "/storno/") || strings.Contains(text, "/storno/") || !strings.Contains(text, "ruf bitte im Salon an: +49 6761 12345") {
		t.Errorf("Absage nach Frist angeboten:\n%s", text)
	}

	// Mails an den Salon in der Sprache des Kontos (hier Englisch), Link auf kammzeit.com-Domain.
	lang, d := f.a.salonMailData(f.haar, b)
	if lang != "en" || d["Dash"] != "https://kammzeit-en.test/dashboard/termine/42" {
		t.Errorf("Salon-Mail: %s %v", lang, d["Dash"])
	}
	// Englischer Salon: Buchungslinks auf der .com-Domain, Termin englisch formatiert.
	_, text, _, _ = f.a.renderMail("en", "booking-confirm", f.a.customerMailData(f.cuts, mailBooking(f.cuts), now))
	if !strings.Contains(text, "https://cuts.kammzeit-en.test/storno/tok42") || !strings.Contains(text, "Tuesday, 3 Nov 2026, 10:30") {
		t.Errorf("englische Kunden-Mail:\n%s", text)
	}
}

func TestSalonButton(t *testing.T) {
	for accent, want := range map[string]string{
		"#3b82f6": "#3b82f6", // Standardblau: hebt sich ab, Schwarz darauf lesbar
		"#1e3a8a": "#1e3a8a",
		"#ffeb3b": mailNight, // Gelb auf Weiß zu blass
		"#ffffff": mailNight,
		"kaputt":  mailNight,
	} {
		if got := salonButton(accent); got.BG != want || contrast(got.BG, got.FG) < 4.5 {
			t.Errorf("salonButton(%s) = %+v, want %s", accent, got, want)
		}
	}
}

func TestQueueMailMinimalData(t *testing.T) {
	f := newMailFix(t)
	_, _, page, err := f.a.renderMail("de", "queue-called", map[string]any{"Name": "Kim", "Number": "A12", "SalonName": "ölmühle", "StatusURL": "https://oel.kammzeit.test/w/1"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(page, ">Ö</td>") || !strings.Contains(page, "A12") || !strings.Contains(page, `bgcolor="#1E6BFF"`) || !strings.Contains(page, "color:#ffffff;\">Ö") {
		t.Errorf("Salon-Kopf der Warteschlangen-Mail fehlt:\n%s", page)
	}
}
