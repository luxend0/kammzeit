package main

// Mehrere Domains und Sprachen: kammzeit.de (Deutsch) und kammzeit.com (Englisch,
// EU-Markt). Gleiche Datenbank, gleiche Konten; Sessions gelten je Domain.
//
// Übersetzung: der deutsche Text ist der Schlüssel. Templates schreiben
// {{t .Lang "Speichern"}}, Go-Code a.tr(lang, "Speichern"). Englische Texte trägt
// jeder Bereich in seiner eigenen Datei i18n_<bereich>.go per addEN(...) ein.
// Fehlt eine Übersetzung, bleibt der deutsche Text stehen (und im Test fällt es auf).

import (
	"fmt"
	"html"
	"log"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"sync"
	"time"
)

type domainCfg struct {
	Host string // z. B. kammzeit.com
	Lang string // de | en
}

// parseDomains liest BASE_DOMAINS="kammzeit.de=de,kammzeit.com=en". Ohne die
// Variable gibt es nur BASE_DOMAIN auf Deutsch (Selbsthosten, Tests).
func parseDomains(spec, base string) []domainCfg {
	var out []domainCfg
	for _, part := range strings.Split(spec, ",") {
		host, lang, _ := strings.Cut(strings.TrimSpace(part), "=")
		host, lang = strings.ToLower(strings.TrimSpace(host)), strings.TrimSpace(lang)
		if host == "" {
			continue
		}
		if lang != "en" {
			lang = "de"
		}
		out = append(out, domainCfg{host, lang})
	}
	if len(out) == 0 {
		out = []domainCfg{{base, "de"}}
	}
	return out
}

// domains = konfigurierte Hauptdomains; ohne Konfiguration (z. B. in Tests, die
// config von Hand bauen) nur BaseDomain auf Deutsch.
func (a *app) domains() []domainCfg {
	if len(a.cfg.Domains) > 0 {
		return a.cfg.Domains
	}
	return []domainCfg{{a.cfg.BaseDomain, "de"}}
}

// tenant ordnet einen Host einer Domain zu: Salon-Subdomain (slug), Hauptdomain
// (root) oder gar nichts (dom.Host == "").
func (a *app) tenant(host string) (slug string, root bool, dom domainCfg) {
	for _, d := range a.domains() {
		s, r := tenantSlug(host, d.Host)
		if r || s != "" {
			return s, r, d
		}
	}
	return "", false, domainCfg{}
}

// langFor = Sprache der Domain, über die die Anfrage kommt (Hauptseiten,
// Login, Dashboard). Salon-Buchungsseiten nutzen stattdessen salon.Lang.
func (a *app) langFor(r *http.Request) string {
	if _, _, d := a.tenant(r.Host); d.Lang != "" {
		return d.Lang
	}
	return "de"
}

// siteURLFor = Hauptseite der Domain dieser Anfrage (für Links in Login-Mails,
// Weiterleitungen, Stripe-Rückkehr). Die erste Domain nutzt SITE_URL (lokal anders).
func (a *app) siteURLFor(r *http.Request) string {
	if _, _, d := a.tenant(r.Host); d.Host != "" && d.Host != a.domains()[0].Host {
		return "https://" + d.Host
	}
	return a.cfg.SiteURL
}

// siteURLLang = Hauptseite für eine Sprache (z. B. für Mails ohne Anfrage).
func (a *app) siteURLLang(lang string) string {
	for i, d := range a.domains() {
		if d.Lang == lang && i > 0 {
			return "https://" + d.Host
		}
	}
	return a.cfg.SiteURL
}

// bookingURL = Buchungsseite eines Salons auf der Domain seiner Sprache.
func (a *app) bookingURL(s *salon, path string) string {
	for i, d := range a.domains() {
		if d.Lang == s.Lang && i > 0 && !a.cfg.SingleTenant {
			return "https://" + s.Slug + "." + d.Host + path
		}
	}
	return a.link(s.Slug, path)
}

// otherSite = Hauptseite der jeweils anderen Sprache (Sprachumschalter).
func (a *app) otherSite(lang string) (url, otherLang string) {
	for _, d := range a.domains() {
		if d.Lang != lang {
			if d.Host == a.domains()[0].Host {
				return a.cfg.SiteURL, d.Lang
			}
			return "https://" + d.Host, d.Lang
		}
	}
	return "", ""
}

// --- Übersetzungen ---

var (
	enMu sync.RWMutex
	en   = map[string]string{}
	// enConflicts sammelt Schlüssel, die zwei Bereiche verschieden übersetzen (Test prüft: leer).
	enConflicts []string
)

// addEN trägt englische Texte ein (Schlüssel = deutscher Text). Doppelte
// Schlüssel mit anderem Text sind ein Fehler im Katalog und werden geloggt.
func addEN(m map[string]string) {
	enMu.Lock()
	defer enMu.Unlock()
	for k, v := range m {
		if old, ok := en[k]; ok && old != v {
			log.Printf("i18n: Schlüssel doppelt mit anderem Text: %q", k)
			enConflicts = append(enConflicts, k)
		}
		en[k] = v
	}
}

// tr übersetzt einen deutschen Text in die Sprache lang.
func tr(lang, de string) string {
	if lang != "en" {
		return de
	}
	enMu.RLock()
	defer enMu.RUnlock()
	if v, ok := en[de]; ok {
		return v
	}
	return de
}

func (a *app) tr(lang, de string) string { return tr(lang, de) }

// trf übersetzt ein Format (deutsches Format = Schlüssel) und setzt Werte ein.
func trf(lang, deFormat string, args ...any) string { return fmt.Sprintf(tr(lang, deFormat), args...) }

var weekdaysEN = [...]string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
var monthsEN = [...]string{"", "Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}

// dateFor formatiert Datum + Uhrzeit je Sprache ("Dienstag, 3.11.2026, 10:30 Uhr" /
// "Tuesday, 3 Nov 2026, 10:30"). 24-Stunden-Uhr auch auf Englisch (EU-Markt).
func dateFor(lang string, t time.Time) string {
	if lang == "en" {
		return fmt.Sprintf("%s, %d %s %d, %s", weekdaysEN[t.Weekday()], t.Day(), monthsEN[t.Month()], t.Year(), t.Format("15:04"))
	}
	return deDate(t)
}

func init() {
	templateFuncs["t"] = func(lang any, s string) string { return tr(fmt.Sprint(lang), s) }
	templateFuncs["tf"] = func(lang any, f string, args ...any) string { return trf(fmt.Sprint(lang), f, args...) }
	templateFuncs["date"] = func(lang any, t time.Time) string { return dateFor(fmt.Sprint(lang), t) }
}

// --- Weiterleitung nach Browsersprache ---

// prefersGerman: erste Sprache im Accept-Language-Header ist Deutsch.
func prefersGerman(acceptLang string) bool {
	first := strings.TrimSpace(strings.Split(acceptLang, ",")[0])
	first = strings.ToLower(strings.Split(first, ";")[0])
	return first == "de" || strings.HasPrefix(first, "de-")
}

// langRedirect leitet Besucher, deren Browser Deutsch bevorzugt, beim Einstieg
// auf die englische Startseite zur deutschen Seite um. Kein Cookie, keine IP:
// nur Startseite, nicht bei ?lang=en (Sprachumschalter) und nicht, wenn man von
// einer eigenen Seite kommt.
func (a *app) langRedirect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/" && !a.cfg.SingleTenant && len(a.domains()) > 1 &&
			r.URL.Query().Get("lang") != "en" && prefersGerman(r.Header.Get("Accept-Language")) {
			if _, root, d := a.tenant(r.Host); root && d.Lang == "en" && !a.ownReferer(r) {
				w.Header().Set("Vary", "Accept-Language")
				http.Redirect(w, r, a.cfg.SiteURL+"/", http.StatusFound)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (a *app) ownReferer(r *http.Request) bool {
	u, err := url.Parse(r.Referer())
	if err != nil || u.Host == "" {
		return false
	}
	_, root, d := a.tenant(u.Host)
	return root || d.Host != ""
}

// --- HTML-Mails ---

// mailT rendert die Templates "mail-<name>-subject", "mail-<name>-text" und
// "mail-<name>-html" (Daten bekommen zusätzlich .Lang) und schickt eine Mail mit
// Text- und HTML-Teil. Fehlt das HTML-Template, geht nur Text raus.
func (a *app) mailT(to, lang, name string, data map[string]any) error {
	subj, text, page, err := a.renderMail(lang, name, data)
	if err != nil {
		return err
	}
	return a.mail.sendHTML(to, subj, text, page)
}

// renderMail rendert Betreff, Text- und HTML-Teil (ohne Versand, auch für Tests).
// Gemeinsame Werte (Bilder, Links, Farben) ergänzt mailDefaults (mail.go).
func (a *app) renderMail(lang, name string, data map[string]any) (subject, text, page string, err error) {
	if data == nil {
		data = map[string]any{}
	}
	data["Lang"] = lang
	a.mailDefaults(lang, data)
	var subj, txt, htm strings.Builder
	if err := a.tpl.ExecuteTemplate(&subj, "mail-"+name+"-subject", data); err != nil {
		return "", "", "", err
	}
	// html/template maskiert auch Betreff und Textteil ("&amp;", "&#39;"): zurück
	// in Klartext. Die Mail-Templates selbst enthalten dort keine Entities.
	subject = html.UnescapeString(strings.TrimSpace(subj.String()))
	data["Subject"] = subject
	if err := a.tpl.ExecuteTemplate(&txt, "mail-"+name+"-text", data); err != nil {
		return "", "", "", err
	}
	if a.tpl.Lookup("mail-"+name+"-html") != nil {
		if err := a.tpl.ExecuteTemplate(&htm, "mail-"+name+"-html", data); err != nil {
			return "", "", "", err
		}
	}
	return subject, html.UnescapeString(txt.String()), htm.String(), nil
}

// multipartAlternative baut den Rumpf einer Text+HTML-Mail und gibt den
// Content-Type mit Boundary zurück.
func multipartAlternative(text, html string) (contentType, body string) {
	var b strings.Builder
	mw := multipart.NewWriter(&b)
	for _, p := range []struct{ typ, content string }{{"text/plain", text}, {"text/html", html}} {
		h := textproto.MIMEHeader{}
		h.Set("Content-Type", p.typ+"; charset=utf-8")
		h.Set("Content-Transfer-Encoding", "8bit")
		part, _ := mw.CreatePart(h)
		part.Write([]byte(p.content))
	}
	mw.Close()
	return "multipart/alternative; boundary=" + mw.Boundary(), b.String()
}
