package main

// Anpassung der Buchungsseite: Social-Links, Öffnungszeiten-Box, Design
// (Schrift, Ecken, Hell/Dunkel, Begrüßungstext), Logo, Titelbild und Galerie.
// Logo, Social-Links und Öffnungszeiten gibt es für alle; Titelbild, Design und
// Galerie nur mit Pro (im Einzelbetrieb immer). Ohne Pro bleiben gespeicherte
// Werte erhalten, die Buchungsseite zeigt dann nur das Standard-Design.

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	fontChoices   = []choice[string]{{"schlicht", "Schlicht"}, {"modern", "Modern"}, {"klassisch", "Klassisch"}}
	radiusChoices = []choice[string]{{"square", "Eckig"}, {"round", "Rund"}}
	modeChoices   = []choice[string]{{"light", "Hell"}, {"dark", "Dunkel"}}
)

// proFeatures: Titelbild, Design und Galerie.
func (a *app) proFeatures(s *salon) bool { return a.cfg.SingleTenant || s.Pro() }

// themeClass = CSS-Klassen für <body> der Buchungsseite (style.css).
func (a *app) themeClass(s *salon) string {
	if s == nil || !a.proFeatures(s) {
		return ""
	}
	return themeClasses(s)
}

// themeClasses = Design-Klassen ohne Pro-Prüfung (auch für die Vorschau).
func themeClasses(s *salon) string {
	var c []string
	if s.ThemeFont != "schlicht" && allowed(fontChoices, s.ThemeFont) {
		c = append(c, "font-"+s.ThemeFont)
	}
	if s.ThemeRadius == "round" {
		c = append(c, "round")
	}
	if s.ThemeMode == "dark" {
		c = append(c, "dark")
	}
	return strings.Join(c, " ")
}

// --- Social-Links ---

var socialHandle = regexp.MustCompile(`^@?[A-Za-z0-9._]{1,30}$`)
var googleHost = regexp.MustCompile(`^(maps\.)?google\.[a-z]{2,3}(\.[a-z]{2})?$`)

// socialHostOK: erlaubte Domains je Netzwerk (ohne "www.").
func socialHostOK(network, host string) bool {
	switch network {
	case "instagram":
		return host == "instagram.com"
	case "tiktok":
		return host == "tiktok.com" || host == "vm.tiktok.com"
	case "facebook":
		return slices.Contains([]string{"facebook.com", "m.facebook.com", "fb.com", "fb.me"}, host)
	case "google":
		return googleHost.MatchString(host) || slices.Contains([]string{"g.page", "maps.app.goo.gl", "search.google.com", "g.co"}, host)
	}
	return false
}

// socialURL prüft einen Social-Link: leer, ein Name ("@salon", nicht bei
// Google) oder eine Adresse auf einer Domain des Netzwerks. Gespeichert wird
// die vollständige https-Adresse (http wird zu https, alles andere abgelehnt).
func socialURL(network, in string) (string, bool) {
	in = strings.TrimSpace(in)
	if in == "" {
		return "", true
	}
	if len(in) > 500 || strings.ContainsFunc(in, unicode.IsSpace) {
		return "", false
	}
	if network != "google" && !strings.Contains(in, "/") && socialHandle.MatchString(in) {
		h := strings.TrimPrefix(in, "@")
		switch network {
		case "instagram":
			return "https://www.instagram.com/" + h + "/", true
		case "tiktok":
			return "https://www.tiktok.com/@" + h, true
		case "facebook":
			return "https://www.facebook.com/" + h, true
		}
	}
	raw := in
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" && u.Scheme != "http" || u.User != nil || u.Port() != "" || u.Opaque != "" {
		return "", false
	}
	if !socialHostOK(network, strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")) || strings.Trim(u.Path, "/") == "" && u.RawQuery == "" {
		return "", false
	}
	u.Scheme, u.Host, u.Fragment = "https", strings.ToLower(u.Host), ""
	return u.String(), true
}

// whatsappNumber: Telefonnummer mit Ländervorwahl ("+49 170 1234567",
// "0049 ...", "wa.me/49170...") als reine Ziffernfolge (E.164 ohne +).
func whatsappNumber(in string) (string, bool) {
	in = strings.TrimSpace(in)
	if in == "" {
		return "", true
	}
	if len(in) > 60 {
		return "", false
	}
	if _, rest, ok := strings.Cut(in, "wa.me/"); ok {
		num, _, _ := strings.Cut(rest, "?")
		in = "+" + strings.TrimSuffix(num, "/")
	}
	var b strings.Builder
	for i, r := range in {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '+' && i == 0, r == ' ', r == '-', r == '/', r == '(', r == ')', r == '.':
		default:
			return "", false
		}
	}
	n := b.String()
	switch {
	case strings.HasPrefix(in, "+"):
	case strings.HasPrefix(n, "00"):
		n = n[2:]
	default:
		return "", false // ohne Ländervorwahl
	}
	if len(n) < 8 || len(n) > 15 || n[0] == '0' {
		return "", false
	}
	return n, true
}

type socialLink struct{ Net, Label, URL string }

// socialLinks: Buttons für die Buchungsseite, in fester Reihenfolge.
func socialLinks(s *salon) []socialLink {
	var out []socialLink
	for _, l := range []socialLink{
		{"instagram", "Instagram", s.Instagram}, {"tiktok", "TikTok", s.TikTok}, {"facebook", "Facebook", s.Facebook},
		{"google", "Google", s.Google},
	} {
		if l.URL != "" {
			out = append(out, l)
		}
	}
	if s.WhatsApp != "" {
		out = append(out, socialLink{"whatsapp", "WhatsApp", "https://wa.me/" + s.WhatsApp})
	}
	return out
}

// parseCustom liest die Felder der Anpassung aus dem Einstellungsformular
// (Teil von parseSettings). Fehlende Design-Felder = Standard.
func parseCustom(f url.Values, s *salon, errs map[string]string) {
	var ok bool
	for _, n := range []struct {
		key string
		dst *string
		msg string
	}{
		{"instagram", &s.Instagram, "Bitte deinen Instagram-Namen (@salon) oder einen Link auf instagram.com angeben."},
		{"tiktok", &s.TikTok, "Bitte deinen TikTok-Namen (@salon) oder einen Link auf tiktok.com angeben."},
		{"facebook", &s.Facebook, "Bitte den Namen deiner Facebook-Seite oder einen Link auf facebook.com angeben."},
		{"google", &s.Google, "Bitte einen Link zu Google Maps angeben, zum Beispiel maps.app.goo.gl/... oder g.page/..."},
	} {
		raw := f.Get(n.key)
		if *n.dst, ok = socialURL(n.key, raw); !ok {
			*n.dst = strings.TrimSpace(raw)
			errs[n.key] = n.msg
		}
	}
	raw := f.Get("whatsapp")
	if s.WhatsApp, ok = whatsappNumber(raw); !ok {
		s.WhatsApp = strings.TrimSpace(raw)
		errs["whatsapp"] = "Bitte die WhatsApp-Nummer mit Ländervorwahl angeben, zum Beispiel +49 170 1234567."
	}
	s.ShowHours = f.Get("show_hours") == "1"
	pick := func(key string, list []choice[string], dst *string) {
		if *dst = f.Get(key); *dst == "" {
			*dst = list[0].Value
		} else if !allowed(list, *dst) {
			errs[key] = "Bitte einen Wert aus der Liste wählen."
		}
	}
	pick("theme_font", fontChoices, &s.ThemeFont)
	pick("theme_radius", radiusChoices, &s.ThemeRadius)
	pick("theme_mode", modeChoices, &s.ThemeMode)
	s.Welcome = strings.ReplaceAll(strings.TrimSpace(f.Get("welcome")), "\r\n", "\n")
	if utf8.RuneCountInString(s.Welcome) > 300 || strings.ContainsFunc(s.Welcome, func(r rune) bool { return r != '\n' && unicode.IsControl(r) }) {
		errs["welcome"] = "Der Begrüßungstext darf höchstens 300 Zeichen haben."
	}
}

// --- Öffnungszeiten ---

type hoursRow struct {
	Days          string
	Times         []string // je Block "09:00 - 12:00"; geschlossen: ["geschlossen"]
	Closed, Today bool     // Today: heutiger Wochentag gehört zu dieser Zeile
}

// openingHours fasst Arbeitszeiten (Salon-Zeitzone, wie gespeichert) zu
// Öffnungszeiten zusammen: je Wochentag die Vereinigung aller Blöcke, gleiche
// aufeinanderfolgende Tage in einer Zeile ("Montag - Freitag"), Montag zuerst.
// Ohne jede Arbeitszeit: nil (keine Box). today = heutiger Wochentag im Salon.
func openingHours(hours []WorkingHours, lang string, today time.Weekday) []hoursRow {
	var days [7][][2]int
	for _, h := range hours {
		if h.Weekday >= 0 && h.Weekday <= 6 && h.EndMin > h.StartMin {
			days[h.Weekday] = append(days[h.Weekday], [2]int{h.StartMin, h.EndMin})
		}
	}
	var text [7]string
	open := false
	for wd, iv := range days {
		slices.SortFunc(iv, func(a, b [2]int) int { return a[0] - b[0] })
		var merged [][2]int
		for _, x := range iv {
			if n := len(merged); n > 0 && x[0] <= merged[n-1][1] {
				merged[n-1][1] = max(merged[n-1][1], x[1])
			} else {
				merged = append(merged, x)
			}
		}
		var parts []string
		for _, x := range merged {
			parts = append(parts, fmt.Sprintf("%02d:%02d - %02d:%02d", x[0]/60, x[0]%60, x[1]/60, x[1]%60))
		}
		text[wd] = strings.Join(parts, ", ") // Vergleichsschlüssel für gleiche Tage
		open = open || len(parts) > 0
	}
	if !open {
		return nil
	}
	name := func(wd int) string {
		if lang == "en" {
			return weekdaysEN[wd]
		}
		return weekdaysDE[wd]
	}
	order := []int{1, 2, 3, 4, 5, 6, 0}
	var rows []hoursRow
	for i := 0; i < len(order); {
		j := i
		for j+1 < len(order) && text[order[j+1]] == text[order[i]] {
			j++
		}
		r := hoursRow{Days: name(order[i]), Times: strings.Split(text[order[i]], ", ")}
		if j > i {
			r.Days += " - " + name(order[j])
		}
		if text[order[i]] == "" {
			r.Closed, r.Times = true, []string{tr(lang, "geschlossen")}
		}
		r.Today = slices.Contains(order[i:j+1], int(today))
		rows = append(rows, r)
		i = j + 1
	}
	return rows
}

// salonHours: Arbeitszeiten aller aktiven Mitarbeiter des Salons.
func (a *app) salonHours(salonID int64) ([]WorkingHours, error) {
	rows, err := a.db.Query(`SELECT wh.weekday, wh.start_min, wh.end_min FROM working_hours wh
		JOIN staff st ON st.id = wh.staff_id WHERE st.salon_id = ? AND st.active = 1`, salonID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WorkingHours
	for rows.Next() {
		var h WorkingHours
		if err := rows.Scan(&h.Weekday, &h.StartMin, &h.EndMin); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// --- Galerie ---

type galleryItem struct {
	ID            int64
	Hash, Alt     string
	Width, Height int
	Pos           int  // 1, 2, 3 ... (Dashboard)
	Last          bool // letztes Foto (Dashboard)
}

func (a *app) gallery(salonID int64) ([]galleryItem, error) {
	rows, err := a.db.Query(`SELECT id, hash, alt, width, height FROM salon_media
		WHERE salon_id = ? AND kind = 'gallery' ORDER BY sort, id`, salonID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []galleryItem
	for rows.Next() {
		var g galleryItem
		if err := rows.Scan(&g.ID, &g.Hash, &g.Alt, &g.Width, &g.Height); err != nil {
			return nil, err
		}
		g.Pos = len(out) + 1
		out = append(out, g)
	}
	if len(out) > 0 {
		out[len(out)-1].Last = true
	}
	return out, rows.Err()
}

// bookingExtras ergänzt die Daten der Buchungsseite um die Anpassung.
func (a *app) bookingExtras(s *salon, d map[string]any) error {
	d["Social"] = socialLinks(s)
	if s.ShowHours {
		hours, err := a.salonHours(s.ID)
		if err != nil {
			return err
		}
		d["Hours"] = openingHours(hours, s.Lang, time.Now().In(s.Loc).Weekday())
	}
	if a.proFeatures(s) {
		g, err := a.gallery(s.ID)
		if err != nil {
			return err
		}
		d["Gallery"], d["Cover"], d["Welcome"] = g, s.Cover, s.Welcome
	}
	return nil
}

// mailLogo: Logo für den Kopf der Kunden-Mails (absolute Adresse auf der
// Domain der Salon-Sprache, Höhe 40 px, höchstens 160 px breit) oder nil.
func (a *app) mailLogo(s *salon) map[string]any {
	if s.Logo == "" {
		return nil
	}
	var w, h int
	if a.db.QueryRow(`SELECT width, height FROM salon_media WHERE salon_id = ? AND hash = ?`, s.ID, s.Logo).Scan(&w, &h) != nil || w < 1 || h < 1 {
		return nil
	}
	mw, mh := 40*w/h, 40
	if mw > 160 {
		mw, mh = 160, max(1, 160*h/w)
	}
	return map[string]any{"URL": a.siteURLLang(s.Lang) + "/media/" + s.Logo, "W": max(1, mw), "H": mh}
}

// keepPro übernimmt für Salons ohne Pro die gespeicherten Pro-Werte, damit
// Speichern im Gratis-Plan nichts davon überschreibt.
func keepPro(dst, cur *salon) {
	dst.ThemeFont, dst.ThemeRadius, dst.ThemeMode, dst.Welcome = cur.ThemeFont, cur.ThemeRadius, cur.ThemeMode, cur.Welcome
}
