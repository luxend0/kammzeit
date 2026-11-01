package main

// Mails: Daten für die Templates in web/mail.html (je Mail "-subject", "-text",
// "-html", Versand über a.mailT). Kunden-Mails in der Sprache der Buchungsseite
// (salon.Lang), Mails an den Salon, Login und Marketing in der Sprache des Kontos
// (users.lang). Bewusst ohne Tracking-Pixel und ohne Klick-Tracking: alle Links
// zeigen direkt aufs Ziel, Bilder sind nur Logo und Kammi.

import (
	"fmt"
	"html/template"
	"log"
	"math"
	"net/mail"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const mailNight = "#13294B" // Nachtblau aus der Palette

// mailColors = Farben eines Buttons. Class "kz-bn" bekommt im Dark Mode ein
// helleres Blau, sonst verschwände ein nachtblauer Button auf der dunklen Karte.
type mailColors struct{ BG, FG, Line, Class string }

var (
	mailSun     = mailColors{"#FFC93C", mailNight, "#FFC93C", ""}      // Haupt-Button Kammzeit
	mailOutline = mailColors{"#ffffff", mailNight, mailNight, ""}      // zweiter Button
	mailNightBt = mailColors{mailNight, "#ffffff", mailNight, "kz-bn"} // Salon-Farbe zu blass
)

func init() {
	// mt wählt den Text nach Sprache: {{mt .Lang "Wann" "When"}}.
	templateFuncs["mt"] = func(lang any, de, en string) string {
		if fmt.Sprint(lang) == "en" {
			return en
		}
		return de
	}
	// mailArgs baut die Daten für Teil-Templates: (mailArgs "URL" .ICS "Label" "...").
	templateFuncs["mailArgs"] = func(kv ...any) map[string]any {
		m := map[string]any{}
		for i := 0; i+1 < len(kv); i += 2 {
			m[fmt.Sprint(kv[i])] = kv[i+1]
		}
		return m
	}
	// mso: Rahmen-Tabelle nur für Outlook (Word-Engine kennt kein max-width).
	// html/template entfernt Kommentare im Template selbst, daher als Wert.
	templateFuncs["mso"] = func(open bool) template.HTML {
		if open {
			return `<!--[if mso]><table role="presentation" width="600" align="center" border="0" cellspacing="0" cellpadding="0"><tr><td><![endif]-->`
		}
		return `<!--[if mso]></td></tr></table><![endif]-->`
	}
}

// mailDefaults ergänzt, was jede Mail braucht (Bilder, Kammzeit-Links, Farben).
// mailT ruft das auf, Aufrufer geben nur ihre eigenen Werte mit.
func (a *app) mailDefaults(lang string, d map[string]any) {
	site := a.siteURLLang(lang)
	imprint, privacy := "/impressum", "/datenschutz"
	if lang == "en" {
		imprint, privacy = "/imprint", "/privacy"
	}
	for k, v := range map[string]any{
		"Site": site, "Img": site + "/static/img/mail/",
		"Imprint": site + imprint, "Privacy": site + privacy,
		"Legal": !a.cfg.SingleTenant, // Selbsthoster haben keine Kammzeit-Rechtstexte
		"Pri":   mailSun, "Sec": mailOutline,
	} {
		if _, ok := d[k]; !ok {
			d[k] = v
		}
	}
	// Mails, die nur den Salon-Namen mitgeben (z. B. Warteschlange: SalonName),
	// tragen trotzdem den Salon im Kopf, in Kammzeit-Blau.
	if name, ok := d["SalonName"].(string); ok && name != "" && d["HeadName"] == nil {
		d["HeadName"], d["Initial"] = name, initial(name)
		acc, _ := d["Accent"].(string) // optional: Akzentfarbe des Salons
		d["Accent"], d["OnAccent"] = acc, onAccent(acc)
		if !hexColor.MatchString(acc) {
			d["Accent"], d["OnAccent"] = "#1E6BFF", "#ffffff" // Palette: auf Blau nur Weiß
		}
	}
}

// customerMailData: Termin und Salon für Mails an Kunden. Links gehen auf die
// Buchungsseite in der Domain der Salon-Sprache.
func (a *app) customerMailData(s *salon, b *booking, now time.Time) map[string]any {
	deadline := cancelDeadline(s, b.Start)
	d := map[string]any{
		"S": s, "B": b, "Min": int(b.End.Sub(b.Start).Minutes()),
		"Address":   addressLines(s.Address),
		"Tel":       telURL(s.Phone),
		"Book":      a.bookingURL(s, "/"),
		"ICS":       a.bookingURL(s, "/termin/"+b.Token+".ics"),
		"Cancel":    a.bookingURL(s, "/storno/"+b.Token),
		"CanCancel": now.Before(deadline),
		"HeadName":  s.Name, "HeadURL": a.bookingURL(s, "/"), // Kopf und Fuß: Salon statt Kammzeit
		"Initial": initial(s.Name),
		"Accent":  s.Accent, "OnAccent": onAccent(s.Accent),
		"Pri":          salonButton(s.Accent),
		"Branding":     !a.cfg.SingleTenant && s.Plan == "free", // wie auf der Buchungsseite
		"SalonImprint": extURL(s.ImprintURL),
	}
	if s.CancelNoticeMin > 0 {
		d["Deadline"] = deadline
	}
	return d
}

// salonMailData: Termin für Mails an den Salon, in der Sprache des Kontos.
func (a *app) salonMailData(s *salon, b *booking) (lang string, d map[string]any) {
	lang = a.ownerLang(s)
	site := a.siteURLLang(lang)
	return lang, map[string]any{
		"Salon": s, "B": b, "Min": int(b.End.Sub(b.Start).Minutes()), "ForSalon": true,
		"Dash": site + "/dashboard/termine/" + strconv.FormatInt(b.ID, 10),
		"Day":  site + "/dashboard?tag=" + b.Start.Format(time.DateOnly),
	}
}

// ownerLang = Sprache des (ersten) Inhaber-Kontos des Salons, sonst die der Buchungsseite.
// ponytail: ein Inhaber pro Salon; bei mehreren zählt das älteste. Mitarbeiter-Logins zählen nicht.
func (a *app) ownerLang(s *salon) string {
	var lang string
	a.db.QueryRow(`SELECT lang FROM users WHERE salon_id = ? AND role = 'owner' ORDER BY id LIMIT 1`, s.ID).Scan(&lang)
	if lang != "de" && lang != "en" {
		return s.Lang
	}
	return lang
}

// mailTo prüft die Adresse (kein Header-Einschleusen) und schickt die Mail.
// Fehler landen zusätzlich im Log.
func (a *app) mailTo(to, lang, name string, d map[string]any) error {
	addr, err := mail.ParseAddress(to)
	if err == nil {
		err = a.mailT(addr.Address, lang, name, d)
	}
	if err != nil {
		log.Printf("Mail %s an %q: %v", name, to, err)
	}
	return err
}

// addressLines: Salon-Adresse zeilenweise, ohne Leerzeilen.
func addressLines(addr string) []string {
	var out []string
	for _, l := range strings.Split(strings.ReplaceAll(addr, "\r", ""), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// initial: erster Buchstabe des Salon-Namens für das Farbfeld im Kopf.
func initial(name string) string {
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return string(unicode.ToUpper(r))
		}
	}
	r, _ := utf8.DecodeRuneInString(name)
	if r == utf8.RuneError {
		return "K"
	}
	return string(r)
}

// salonButton: Akzentfarbe des Salons, wenn sie sich vom weißen Grund abhebt
// (3:1) und die Schrift darauf lesbar ist (4,5:1), sonst Nachtblau.
func salonButton(accent string) mailColors {
	fg := onAccent(accent)
	if hexColor.MatchString(accent) && contrast(accent, "#ffffff") >= 3 && contrast(accent, fg) >= 4.5 {
		return mailColors{accent, fg, accent, ""}
	}
	return mailNightBt
}

// contrast = WCAG-Kontrastverhältnis zweier Farben (#rgb oder #rrggbb).
func contrast(c1, c2 string) float64 {
	l1, l2 := luminance(c1), luminance(c2)
	if l1 < l2 {
		l1, l2 = l2, l1
	}
	return (l1 + 0.05) / (l2 + 0.05)
}

func luminance(hex string) float64 {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) == 3 {
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	}
	if len(hex) != 6 {
		return 0
	}
	ch := func(i int) float64 {
		v, _ := strconv.ParseUint(hex[i:i+2], 16, 8)
		c := float64(v) / 255
		if c <= 0.04045 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*ch(0) + 0.7152*ch(2) + 0.0722*ch(4)
}
