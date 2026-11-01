package main

import (
	"slices"
	"testing"
	"time"
)

func TestFreeSlots(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	monday := time.Date(2026, 11, 2, 0, 0, 0, 0, berlin)
	at := func(h, m int) time.Time { return time.Date(2026, 11, 2, h, m, 0, 0, berlin) }
	mon := func(fromH, toH int) WorkingHours {
		return WorkingHours{Weekday: time.Monday, StartMin: fromH * 60, EndMin: toH * 60}
	}
	hhmm := func(ts []time.Time) []string {
		out := []string{}
		for _, t := range ts {
			out = append(out, t.Format("15:04"))
		}
		return out
	}

	cases := []struct {
		name  string
		hours []WorkingHours
		busy  []Busy
		dur   time.Duration
		step  time.Duration
		want  []string
	}{
		{"leerer Block", []WorkingHours{mon(9, 12)}, nil, time.Hour, 30 * time.Minute,
			[]string{"09:00", "09:30", "10:00", "10:30", "11:00"}},
		{"Termin passt genau bis Blockende", []WorkingHours{mon(9, 10)}, nil, time.Hour, 15 * time.Minute,
			[]string{"09:00"}},
		{"Leistung länger als Block", []WorkingHours{mon(9, 10)}, nil, 90 * time.Minute, 15 * time.Minute,
			[]string{}},
		{"Überlappung, Ränder berühren zählt nicht", []WorkingHours{mon(9, 12)},
			[]Busy{{at(10, 0), at(10, 30)}}, time.Hour, 30 * time.Minute,
			[]string{"09:00", "10:30", "11:00"}},
		{"Pause durch zwei Blöcke", []WorkingHours{mon(13, 15), mon(9, 12)}, nil, time.Hour, time.Hour,
			[]string{"09:00", "10:00", "11:00", "13:00", "14:00"}},
		{"ganzer Tag belegt", []WorkingHours{mon(9, 12)}, []Busy{{at(8, 0), at(13, 0)}}, 15 * time.Minute, 15 * time.Minute,
			[]string{}},
		{"anderer Wochentag", []WorkingHours{{Weekday: time.Tuesday, StartMin: 540, EndMin: 720}}, nil, time.Hour, time.Hour,
			[]string{}},
		{"doppelte Blöcke ergeben keine doppelten Slots", []WorkingHours{mon(9, 10), mon(9, 10)}, nil, time.Hour, time.Hour,
			[]string{"09:00"}},
		{"Dauer 0", []WorkingHours{mon(9, 12)}, nil, 0, time.Hour, []string{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := freeSlots(c.hours, c.busy, c.dur, monday, c.step)
			if !slices.Equal(hhmm(got), c.want) {
				t.Fatalf("got %v, want %v", hhmm(got), c.want)
			}
			for _, s := range got {
				if s.Location() != berlin {
					t.Fatalf("slot %v not in salon time zone", s)
				}
			}
		})
	}
}

// Mandantentrennung: nur genau eine Subdomain-Ebene unter BASE_DOMAIN ist ein Salon.
func TestTenantSlug(t *testing.T) {
	cases := []struct {
		host, slug string
		root       bool
	}{
		{"kammzeit.de", "", true},
		{"KAMMZEIT.DE:443", "", true},
		{"salon-mueller.kammzeit.de", "salon-mueller", false},
		{"demo.kammzeit.de:8080", "demo", false},
		{"a.b.kammzeit.de", "", false},
		{"-x.kammzeit.de", "", false},
		{"evilkammzeit.de", "", false},
		{"salon.other.de", "", false},
	}
	for _, c := range cases {
		slug, root := tenantSlug(c.host, "kammzeit.de")
		if slug != c.slug || root != c.root {
			t.Errorf("tenantSlug(%q) = %q, %v; want %q, %v", c.host, slug, root, c.slug, c.root)
		}
	}
}

// Caddy holt nur Zertifikate, wenn tlsAllowed zustimmt: fremde oder
// erfundene Subdomains dürfen keine Let's-Encrypt-Limits verbrauchen.
func TestTLSAllowed(t *testing.T) {
	db, err := openDB(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	// Ein Salon zählt erst, wenn sein Inhaber den Login-Link eingelöst hat:
	// Registrieren mit erfundenen Adressen allein verbraucht kein Zertifikat.
	if _, err := db.Exec(`INSERT INTO salons (id, slug, name) VALUES (1, 'mueller', 'Salon Müller'), (2, 'neu', 'Salon Neu'), (3, 'ohnekonto', 'Ohne Konto');
		INSERT INTO users (salon_id, email, last_login_at) VALUES (1, 'inhaber@mueller.test', unixepoch());
		INSERT INTO users (salon_id, email) VALUES (2, 'inhaber@neu.test')`); err != nil {
		t.Fatal(err)
	}
	if err := seedDemo(db); err != nil {
		t.Fatal(err)
	}
	a := &app{cfg: config{BaseDomain: "kammzeit.de"}, db: db}
	for domain, want := range map[string]bool{
		"kammzeit.de":            true,
		"www.kammzeit.de":        true,
		"mueller.kammzeit.de":    true,
		"MUELLER.kammzeit.de":    true,
		"demo.kammzeit.de":       true, // kammzeit demo setzt last_login_at
		"neu.kammzeit.de":        false,
		"ohnekonto.kammzeit.de":  false,
		"gibtsnicht.kammzeit.de": false,
		"a.mueller.kammzeit.de":  false,
		"evilkammzeit.de":        false,
		"example.com":            false,
		"":                       false,
	} {
		if got := a.tlsAllowed(domain); got != want {
			t.Errorf("tlsAllowed(%q) = %v, want %v", domain, got, want)
		}
	}
	// Schon vorhandener Demo-Salon ohne Login: erneutes `kammzeit demo` holt das nach.
	if _, err := db.Exec(`UPDATE users SET last_login_at = NULL WHERE email = 'demo@example.com'`); err != nil {
		t.Fatal(err)
	}
	if a.tlsAllowed("demo.kammzeit.de") {
		t.Fatal("Demo ohne Login zugelassen")
	}
	if err := seedDemo(db); err != nil {
		t.Fatal(err)
	}
	if !a.tlsAllowed("demo.kammzeit.de") {
		t.Fatal("erneutes seedDemo setzt last_login_at nicht")
	}
}

// Ohne LISTEN lauscht der Server auf allen Schnittstellen; dann gibt es beim
// Start einen Hinweis.
func TestAllInterfaces(t *testing.T) {
	for addr, want := range map[string]bool{
		":8080": true, "0.0.0.0:8080": true, "[::]:8080": true,
		"127.0.0.1:8080": false, "[::1]:8080": false, "192.0.2.10:8080": false, "localhost:8080": false, "kaputt": false,
	} {
		if got := allInterfaces(addr); got != want {
			t.Errorf("allInterfaces(%q) = %v, want %v", addr, got, want)
		}
	}
}
