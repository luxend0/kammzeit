package main

import (
	"fmt"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestSocialURL(t *testing.T) {
	cases := []struct {
		net, in, want string
		ok            bool
	}{
		{"instagram", "", "", true},
		{"instagram", "@salon.lindner", "https://www.instagram.com/salon.lindner/", true},
		{"instagram", "salon_lindner", "https://www.instagram.com/salon_lindner/", true},
		{"instagram", "https://www.instagram.com/salon.lindner/?hl=de", "https://www.instagram.com/salon.lindner/?hl=de", true},
		{"instagram", "instagram.com/salon", "https://instagram.com/salon", true},
		{"instagram", "http://instagram.com/salon", "https://instagram.com/salon", true}, // wird https
		{"instagram", "HTTPS://WWW.INSTAGRAM.COM/Salon", "https://www.instagram.com/Salon", true},
		{"instagram", "https://instagram.com", "", false},           // ohne Profil
		{"instagram", "https://instagram.com.evil.de/x", "", false}, // fremde Domain
		{"instagram", "https://evil.de/instagram.com/x", "", false},
		{"instagram", "https://instagram.com@evil.de/x", "", false},
		{"instagram", "https://user@instagram.com/x", "", false},
		{"instagram", "https://instagram.com:8443/x", "", false},
		{"instagram", "javascript:alert(1)", "", false},
		{"instagram", "javascript://instagram.com/%0aalert(1)", "", false},
		{"instagram", "data:text/html,x", "", false},
		{"instagram", "salon lindner", "", false},
		{"instagram", "@" + strings.Repeat("a", 31), "", false},
		{"tiktok", "@salon", "https://www.tiktok.com/@salon", true},
		{"tiktok", "https://www.tiktok.com/@salon", "https://www.tiktok.com/@salon", true},
		{"tiktok", "https://vm.tiktok.com/ZMabc/", "https://vm.tiktok.com/ZMabc/", true},
		{"tiktok", "https://tiktok.evil.com/@salon", "", false},
		{"facebook", "SalonLindner", "https://www.facebook.com/SalonLindner", true},
		{"facebook", "https://m.facebook.com/profile.php?id=123", "https://m.facebook.com/profile.php?id=123", true},
		{"facebook", "https://fb.me/salon", "https://fb.me/salon", true},
		{"facebook", "https://facebook.de/salon", "", false},
		{"google", "https://maps.app.goo.gl/AbC123", "https://maps.app.goo.gl/AbC123", true},
		{"google", "g.page/salon-lindner", "https://g.page/salon-lindner", true},
		{"google", "https://www.google.de/maps/place/Salon+Lindner/@50.0,7.5,17z", "https://www.google.de/maps/place/Salon+Lindner/@50.0,7.5,17z", true},
		{"google", "https://www.google.co.uk/maps/place/x", "https://www.google.co.uk/maps/place/x", true},
		{"google", "https://search.google.com/local/writereview?placeid=ChIJ", "https://search.google.com/local/writereview?placeid=ChIJ", true},
		{"google", "https://maps.google.com/?cid=123", "https://maps.google.com/?cid=123", true},
		{"google", "salonlindner", "", false}, // kein Name bei Google
		{"google", "https://google.evil.com/maps", "", false},
		{"google", "https://goo.gl/abc", "", false},
		{"google", "https://www.google.de/url?q=https://evil.de", "https://www.google.de/url?q=https://evil.de", true}, // ponytail: Google-Domain genügt
	}
	for _, c := range cases {
		got, ok := socialURL(c.net, c.in)
		if ok != c.ok || ok && got != c.want {
			t.Errorf("socialURL(%s, %q) = %q, %v; erwartet %q, %v", c.net, c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestWhatsappNumber(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"", "", true},
		{"+49 170 1234567", "491701234567", true},
		{"0049 (170) 123-45-67", "491701234567", true},
		{"+43/664/1234567", "436641234567", true},
		{"https://wa.me/491701234567", "491701234567", true},
		{"wa.me/491701234567?text=Hallo", "491701234567", true},
		{"0170 1234567", "", false}, // ohne Ländervorwahl
		{"+49 170 abc", "", false},
		{"+0170 1234567", "", false},
		{"+49 1", "", false},
		{"+49 1701234567890123", "", false},
		{"49+1701234567", "", false},
		{"wa.me/", "", false},
	}
	for _, c := range cases {
		got, ok := whatsappNumber(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("whatsappNumber(%q) = %q, %v; erwartet %q, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestParseCustom(t *testing.T) {
	f := url.Values{"instagram": {"@salon"}, "whatsapp": {"0170 123"}, "facebook": {"https://evil.de/x"},
		"theme_font": {"comic"}, "theme_radius": {"round"}, "welcome": {strings.Repeat("ä", 301)}, "show_hours": {"1"}}
	var s salon
	errs := map[string]string{}
	parseCustom(f, &s, errs)
	for _, k := range []string{"whatsapp", "facebook", "theme_font", "welcome"} {
		if errs[k] == "" {
			t.Errorf("%s: kein Fehler", k)
		}
	}
	if errs["instagram"] != "" || errs["theme_radius"] != "" || s.Instagram != "https://www.instagram.com/salon/" || s.ThemeRadius != "round" ||
		!s.ShowHours || s.ThemeMode != "light" || s.WhatsApp != "0170 123" || s.Facebook != "https://evil.de/x" {
		t.Errorf("falsch übernommen: %+v %v", s, errs)
	}
	errs = map[string]string{}
	parseCustom(url.Values{}, &s, errs) // leeres Formular: Standardwerte, keine Fehler
	if len(errs) != 0 || s.ThemeFont != "schlicht" || s.ThemeRadius != "square" || s.ThemeMode != "light" || s.ShowHours {
		t.Errorf("leer: %+v %v", s, errs)
	}
}

func TestOpeningHours(t *testing.T) {
	wh := func(wd time.Weekday, from, to int) WorkingHours { return WorkingHours{wd, from, to} }
	var hours []WorkingHours
	for wd := time.Tuesday; wd <= time.Friday; wd++ {
		hours = append(hours, wh(wd, 540, 780), wh(wd, 720, 1080)) // zwei Mitarbeiter, überlappend: 9-18
	}
	hours = append(hours, wh(time.Saturday, 540, 720), wh(time.Saturday, 780, 840), wh(time.Saturday, 720, 780)) // lückenlos 9-14
	hours = append(hours, wh(time.Thursday, 1080, 1260))                                                         // Donnerstag länger
	got := openingHours(hours, "de", time.Wednesday)
	want := []string{
		"Montag|geschlossen|true|false",
		"Dienstag - Mittwoch|09:00 - 18:00|false|true",
		"Donnerstag|09:00 - 21:00|false|false",
		"Freitag|09:00 - 18:00|false|false",
		"Samstag|09:00 - 14:00|false|false",
		"Sonntag|geschlossen|true|false",
	}
	row := func(r hoursRow) string {
		return fmt.Sprintf("%s|%s|%v|%v", r.Days, strings.Join(r.Times, ", "), r.Closed, r.Today)
	}
	if len(got) != len(want) {
		t.Fatalf("Zeilen: %+v", got)
	}
	for i := range want {
		if row(got[i]) != want[i] {
			t.Errorf("Zeile %d: %s, erwartet %s", i, row(got[i]), want[i])
		}
	}
	en := openingHours([]WorkingHours{wh(time.Monday, 540, 720), wh(time.Monday, 780, 1440)}, "en", time.Sunday)
	if len(en) != 2 || row(en[0]) != "Monday|09:00 - 12:00, 13:00 - 24:00|false|false" || len(en[0].Times) != 2 ||
		row(en[1]) != "Tuesday - Sunday|closed|true|true" {
		t.Errorf("englisch: %+v", en)
	}
	if openingHours(nil, "de", time.Monday) != nil {
		t.Error("ohne Arbeitszeiten keine Box")
	}
}

// Buchungsseite mit allem: Gratis zeigt Logo, Social-Links und Öffnungszeiten,
// aber kein Titelbild, keine Galerie und kein Design; Pro zeigt alles. DE und EN.
func TestBookingPageCustom(t *testing.T) {
	a := einstApp(t)
	a.cfg.Domains = parseDomains("kammzeit.test=de,kammzeit-en.test=en", "")
	id, _ := einstSeed(t, a.db, "alpha")
	einstSeed(t, a.db, "beta")
	a.db.Exec(`UPDATE salons SET instagram = 'https://www.instagram.com/alpha/', whatsapp = '491701234567', google = 'https://g.page/alpha',
		theme_font = 'modern', theme_radius = 'round', theme_mode = 'dark', welcome_text = 'Willkommen <b>bei uns</b>' WHERE id = ?`, id)
	for _, kind := range []string{"logo", "cover", "gallery"} {
		a.db.Exec(`INSERT INTO salon_media (salon_id, kind, mime, data, width, height, hash, alt) VALUES (?, ?, 'image/jpeg', x'00', 40, 30, ?, ?)`,
			id, kind, strings.Repeat(kind[:1], 32), "Foto "+kind)
	}
	get := func(host string) string {
		r := httptest.NewRequest("GET", "https://"+host+"/", nil)
		w := httptest.NewRecorder()
		a.routes().ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("%s: %d", host, w.Code)
		}
		return w.Body.String()
	}
	has := func(what, body string, want []string, not []string) {
		t.Helper()
		for _, s := range want {
			if !strings.Contains(body, s) {
				t.Errorf("%s: %q fehlt", what, s)
			}
		}
		for _, s := range not {
			if strings.Contains(body, s) {
				t.Errorf("%s: %q sollte fehlen", what, s)
			}
		}
	}
	logo, cover, foto := "/media/"+strings.Repeat("l", 32), "/media/"+strings.Repeat("c", 32), "/media/"+strings.Repeat("g", 32)

	body := get("alpha.kammzeit.test")
	has("Gratis", body, []string{`class="salon-logo" src="` + logo + `"`, `href="https://www.instagram.com/alpha/"`, `href="https://wa.me/491701234567"`,
		`href="https://g.page/alpha"`, `social.svg#s-instagram`, "Öffnungszeiten", "Montag", "09:00 - 18:00", "Dienstag - Sonntag", "geschlossen", `rel="noopener"`},
		[]string{cover, foto, "Willkommen", `class="salon `, "Einblicke"})

	a.db.Exec(`UPDATE salons SET plan = 'pro', show_hours = 0 WHERE id = ?`, id)
	body = get("alpha.kammzeit.test")
	has("Pro", body, []string{`class="salon font-modern round dark"`, `class="cover" src="` + cover + `"`, "Willkommen &lt;b&gt;bei uns&lt;/b&gt;",
		"Einblicke", `src="` + foto + `" width="40" height="30" alt="Foto gallery" loading="lazy"`, logo},
		[]string{"Öffnungszeiten", "<b>bei uns"})

	a.db.Exec(`UPDATE salons SET lang = 'en', show_hours = 1 WHERE id = ?`, id)
	body = get("alpha.kammzeit-en.test")
	has("englisch", body, []string{"Opening hours", "Monday", "Tuesday - Sunday", "closed", "A look inside", `aria-label="Social media"`},
		[]string{"Öffnungszeiten", "Montag", "geschlossen", "Einblicke"})

	// Fremde Subdomain zeigt nichts davon.
	has("fremder Salon", get("beta.kammzeit.test"), []string{`<body class="salon"`}, []string{logo, "instagram.com/alpha", "/media/"})
}

// Kunden-Mails tragen das Logo (absolute Adresse auf der Domain der Salon-Sprache), sonst die Initiale.
func TestMailLogo(t *testing.T) {
	f := newMailFix(t)
	now := time.Date(2026, 11, 1, 9, 0, 0, 0, f.haar.Loc)
	_, _, page, err := f.a.renderMail("de", "booking-confirm", f.a.customerMailData(f.haar, mailBooking(f.haar), now))
	if err != nil || strings.Contains(page, "/media/") || !strings.Contains(page, ">H</td>") {
		t.Fatalf("ohne Logo: %v", err)
	}
	hash := strings.Repeat("a", 32)
	f.a.db.Exec(`INSERT INTO salon_media (salon_id, kind, mime, data, width, height, hash) VALUES (2, 'logo', 'image/png', x'00', 512, 128, ?)`, hash)
	s, _ := f.a.loadSalon(2) // Salon "cuts", Sprache en
	_, _, page, err = f.a.renderMail("en", "booking-confirm", f.a.customerMailData(s, mailBooking(s), now))
	if err != nil || !strings.Contains(page, `<img src="https://kammzeit-en.test/media/`+hash+`" width="160" height="40"`) || strings.Contains(page, ">C</td>") {
		t.Fatalf("mit Logo: %v\n%s", err, page)
	}
}
