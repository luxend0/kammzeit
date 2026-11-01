package main

import (
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"math"
	"net/http"
	"net/mail"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// ponytail: festes 15-Minuten-Raster, pro Salon einstellbar erst bei Bedarf.
const slotStep = 15 * time.Minute

type service struct {
	ID, PriceCents    int64
	Name, Description string
	Duration          int
}

type staffMember struct {
	ID   int64
	Name string
}

// querier = *sql.DB oder *sql.Tx.
type querier interface {
	Query(string, ...any) (*sql.Rows, error)
	QueryRow(string, ...any) *sql.Row
}

// Spam-Schutz der öffentlichen Buchungsseite. Pro Anschluss (clientKey) zählt
// jeder Versuch, pro Salon und pro Empfänger-Mail nur Buchungen, die sonst
// gespeichert und gemailt würden: wechselnde Adressen können so weder einen
// Salon fluten noch eine fremde Mail-Adresse mit Bestätigungen zuschütten.
var (
	bookLimit      = newLimiter(10, time.Hour)    // Buchungsversuche pro Anschluss
	slotLimit      = newLimiter(120, time.Minute) // Abfragen freier Zeiten pro Anschluss
	bookSalonLimit = newLimiter(30, time.Hour)    // Buchungen pro Salon
	bookMailLimit  = newLimiter(3, time.Hour)     // Buchungen pro Empfänger-Mail
)

func (a *app) routes() http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(webFS, "web/static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))
	mux.HandleFunc("GET /{$}", a.home)
	mux.HandleFunc("GET /slots", a.slots)
	mux.HandleFunc("GET /mitarbeiter", a.bookStaff)
	mux.HandleFunc("POST /book", a.book)
	mux.HandleFunc("GET /storno/{token}", a.cancelPage)
	mux.HandleFunc("POST /storno/{token}", a.cancel)
	mux.HandleFunc("GET /termin/{file}", a.bookingICS) // {token}.ics, ServeMux erlaubt keine Endung am Platzhalter
	mux.HandleFunc("GET /impressum", a.static("impressum", "Impressum"))
	mux.HandleFunc("GET /datenschutz", a.static("datenschutz", "Datenschutz"))
	mux.HandleFunc("GET /_tls/allow", a.tlsAllow)
	mux.HandleFunc("GET /agb", a.static("agb", "AGB"))
	mux.HandleFunc("GET /avv", a.static("avv", "Auftragsverarbeitung"))
	// Jeder Bereich registriert seine Routen in seiner eigenen Datei.
	a.routesAuth(mux)      // auth.go: /login, /logout
	a.routesSignup(mux)    // signup.go: /registrieren
	a.routesCalendar(mux)  // dash_calendar.go: /dashboard, /dashboard/termine
	a.routesCustomers(mux) // dash_customers.go: /dashboard/kunden
	a.routesCatalog(mux)   // dash_catalog.go: /dashboard/leistungen, /dashboard/team
	a.routesSettings(mux)  // dash_settings.go: /dashboard/einstellungen
	a.routesBilling(mux)   // billing.go: /dashboard/abo, /stripe/webhook
	a.routesQueue(mux)     // queue.go: /dashboard/warteschlange, /warteschlange
	a.routesSite(mux)      // site.go: öffentliche Seiten, sitemap.xml, robots.txt
	return securityHeaders(a.langRedirect(mux))
}

// static rendert eine Seite ohne Salon-Bezug (Impressum, Datenschutz).
func (a *app) static(name, title string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a.render(w, http.StatusOK, name, a.page(nil, title))
	}
}

// tlsAllow beantwortet Caddys on_demand-"ask": 200 nur für die Basis-Domain,
// www und Subdomains existierender Salons, deren Inhaber sich schon einmal
// angemeldet hat. Sonst könnte jeder beliebige Subdomains aufrufen und
// Zertifikate (Let's-Encrypt-Limits) verbrennen.
func (a *app) tlsAllow(w http.ResponseWriter, r *http.Request) {
	if !a.tlsAllowed(r.URL.Query().Get("domain")) {
		http.Error(w, "nein", http.StatusForbidden)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// Ein Salon zählt erst, wenn sein Inhaber den Login-Link einmal eingelöst hat
// (last_login_at): sonst ließe sich über die Registrierung mit erfundenen
// Mail-Adressen trotzdem ein Zertifikat nach dem anderen anfordern.
func (a *app) tlsAllowed(domain string) bool {
	domain = strings.ToLower(domain)
	for _, d := range a.domains() {
		if domain == "www."+d.Host {
			return true
		}
	}
	slug, root, _ := a.tenant(domain)
	if root {
		return true
	}
	if slug == "" {
		return false
	}
	var one int
	return a.db.QueryRow(`SELECT 1 FROM salons s JOIN users u ON u.salon_id = s.id
		WHERE s.slug = ? AND u.last_login_at IS NOT NULL LIMIT 1`, slug).Scan(&one) == nil
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		// Kein form-action: das Abo-Formular leitet nach dem POST zu Stripe weiter.
		h.Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'none'")
		h.Set("X-Frame-Options", "DENY") // für Browser ohne frame-ancestors
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin") // Storno-Token steht in der URL
		if secureRequest(r) {
			h.Set("Strict-Transport-Security", "max-age=31536000")
		}
		next.ServeHTTP(w, r)
	})
}

func (a *app) render(w http.ResponseWriter, status int, name string, data map[string]any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := a.tpl.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("Template %s: %v", name, err)
	}
}

// page füllt die Felder, die jedes Layout braucht.
func (a *app) page(s *salon, title string) map[string]any {
	d := map[string]any{"Title": title, "Lang": "de"}
	if s != nil {
		d["Lang"] = s.Lang
		d["Salon"] = s
		d["Accent"] = s.Accent
		d["OnAccent"] = onAccent(s.Accent)
		d["Branding"] = !a.cfg.SingleTenant && s.Plan == "free"
		d["Imprint"] = extURL(s.ImprintURL)
		d["Tel"] = telURL(s.Phone)
	}
	return d
}

func (a *app) fail(w http.ResponseWriter, s *salon, status int, msg string) {
	d := a.page(s, "Hinweis")
	d["Message"] = msg
	a.render(w, status, "error", d)
}

func (a *app) serverError(w http.ResponseWriter, s *salon, err error) {
	log.Print(err)
	a.fail(w, s, http.StatusInternalServerError, "Da ist etwas schiefgelaufen. Bitte später noch einmal versuchen.")
}

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// salonFor sucht den Salon zum Host. Antwortet selbst mit 404, wenn es keinen gibt.
func (a *app) salonFor(w http.ResponseWriter, r *http.Request) *salon {
	var row *sql.Row
	if a.cfg.SingleTenant {
		row = a.db.QueryRow(salonCols + `ORDER BY id LIMIT 1`)
	} else {
		slug, _, _ := a.tenant(r.Host)
		if slug == "" {
			a.fail(w, nil, http.StatusNotFound, "Diesen Salon gibt es nicht.")
			return nil
		}
		row = a.db.QueryRow(salonCols+`WHERE slug = ?`, slug)
	}
	s, err := scanSalon(row)
	if errors.Is(err, sql.ErrNoRows) {
		a.fail(w, nil, http.StatusNotFound, "Diesen Salon gibt es nicht.")
		return nil
	}
	if err != nil {
		a.serverError(w, nil, err)
		return nil
	}
	return s
}

func (a *app) home(w http.ResponseWriter, r *http.Request) {
	if !a.cfg.SingleTenant {
		if _, root, d := a.tenant(r.Host); root {
			if d.Lang == "en" && a.tpl.Lookup("landing-en") != nil {
				p := a.page(nil, "Kammzeit - Online booking for salons")
				p["Lang"] = "en"
				a.render(w, http.StatusOK, "landing-en", p)
				return
			}
			a.render(w, http.StatusOK, "landing", a.page(nil, "Kammzeit - Online-Terminbuchung für Salons"))
			return
		}
	}
	s := a.salonFor(w, r)
	if s == nil {
		return
	}
	services, err := a.services(s.ID)
	if err != nil {
		a.serverError(w, s, err)
		return
	}
	d := a.page(s, s.Name+" - Termin buchen")
	if len(services) > 0 {
		staff, err := bookableStaff(a.db, s.ID, services[0].ID, a.staffLimit(s))
		if err != nil {
			a.serverError(w, s, err)
			return
		}
		d["Staff"] = staff
	}
	first, last := bookingDays(s, time.Now())
	d["Home"] = true
	d["Services"] = services
	d["Sel"] = int64(0)
	d["Today"], d["MaxDate"] = first.Format(time.DateOnly), last.Format(time.DateOnly)
	if web := extURL(s.Website); web != "" {
		d["Website"] = web
		d["WebsiteLabel"] = strings.TrimSuffix(web[strings.Index(web, "://")+3:], "/")
	}
	if s.CancelNoticeMin > 0 {
		d["CancelNotice"] = noticeText(s.CancelNoticeMin)
	}
	d["QueueLink"] = a.queueBookingLink(s)
	a.render(w, http.StatusOK, "book", d)
}

func (a *app) services(salonID int64) ([]service, error) {
	rows, err := a.db.Query(`SELECT id, name, description, duration_min, price_cents FROM services
		WHERE salon_id = ? AND active = 1 ORDER BY sort, name`, salonID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []service
	for rows.Next() {
		var s service
		if err := rows.Scan(&s.ID, &s.Name, &s.Description, &s.Duration, &s.PriceCents); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// staff: alle aktiven Mitarbeiter des Salons. Die Buchungsseite nimmt bookableStaff.
func (a *app) staff(salonID int64) ([]staffMember, error) {
	rows, err := a.db.Query(`SELECT id, name FROM staff WHERE salon_id = ? AND active = 1 ORDER BY sort, name`, salonID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanStaff(rows)
}

func scanStaff(rows *sql.Rows) ([]staffMember, error) {
	var out []staffMember
	for rows.Next() {
		var s staffMember
		if err := rows.Scan(&s.ID, &s.Name); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// staffLimit: so viele aktive Mitarbeiter (die ersten nach sort, id) sind
// online buchbar, -1 = alle. Gratis-Plan freeStaffLimit, etwa wenn ein Salon von
// Pro zurück auf Gratis wechselt; gelöscht oder deaktiviert wird dabei nichts.
// Der Einzelbetrieb hat keine Grenze.
func (a *app) staffLimit(s *salon) int {
	if a.cfg.SingleTenant || s.Pro() {
		return -1
	}
	return freeStaffLimit
}

// bookableStaff: aktive Mitarbeiter, die die Leistung laut staff_services
// anbieten, in Buchungsreihenfolge (sort, Name). limit siehe staffLimit.
func bookableStaff(q querier, salonID, serviceID int64, limit int) ([]staffMember, error) {
	rows, err := q.Query(`SELECT st.id, st.name FROM staff st
		JOIN staff_services ss ON ss.staff_id = st.id
		JOIN services sv ON sv.id = ss.service_id
		WHERE ss.service_id = ? AND st.salon_id = ? AND sv.salon_id = ? AND st.active = 1
			AND st.id IN (SELECT id FROM staff WHERE salon_id = ? AND active = 1 ORDER BY sort, id LIMIT ?)
		ORDER BY st.sort, st.name`, serviceID, salonID, salonID, salonID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanStaff(rows)
}

// pickable wendet die Mitarbeiterwahl an: "egal" (oder leer) = alle, sonst
// genau der gewählte, falls er dabei ist.
func pickable(staff []staffMember, choice string) []staffMember {
	if choice == "" || choice == "egal" {
		return staff
	}
	for _, st := range staff {
		if strconv.FormatInt(st.ID, 10) == choice {
			return []staffMember{st}
		}
	}
	return nil
}

// loadService lädt eine aktive Leistung des Salons.
func loadService(q querier, salonID int64, id string) (*service, error) {
	sid, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return nil, sql.ErrNoRows
	}
	var sv service
	err = q.QueryRow(`SELECT id, name, description, duration_min, price_cents FROM services WHERE id = ? AND salon_id = ? AND active = 1`,
		sid, salonID).Scan(&sv.ID, &sv.Name, &sv.Description, &sv.Duration, &sv.PriceCents)
	if err != nil {
		return nil, err
	}
	return &sv, nil
}

// lookup lädt Leistung und Mitarbeiter und prüft, dass beide zum Salon gehören.
// Prüft staff_services nicht (z. B. für manuelle Termine im Dashboard).
func lookup(q querier, salonID int64, serviceID, staffID string) (*service, *staffMember, error) {
	var sv service
	err := q.QueryRow(`SELECT id, name, duration_min, price_cents FROM services WHERE id = ? AND salon_id = ? AND active = 1`,
		serviceID, salonID).Scan(&sv.ID, &sv.Name, &sv.Duration, &sv.PriceCents)
	if err != nil {
		return nil, nil, err
	}
	var st staffMember
	err = q.QueryRow(`SELECT id, name FROM staff WHERE id = ? AND salon_id = ? AND active = 1`,
		staffID, salonID).Scan(&st.ID, &st.Name)
	if err != nil {
		return nil, nil, err
	}
	return &sv, &st, nil
}

// freeSlotsFor holt Arbeitszeiten und Termine eines Mitarbeiters für einen Tag
// und gibt die freien Startzeiten ab `now` (einschließlich) zurück.
func freeSlotsFor(q querier, staffID int64, dur time.Duration, day, now time.Time) ([]time.Time, error) {
	rows, err := q.Query(`SELECT weekday, start_min, end_min FROM working_hours WHERE staff_id = ?`, staffID)
	if err != nil {
		return nil, err
	}
	var hours []WorkingHours
	for rows.Next() {
		var h WorkingHours
		if err := rows.Scan(&h.Weekday, &h.StartMin, &h.EndMin); err != nil {
			rows.Close()
			return nil, err
		}
		hours = append(hours, h)
	}
	rows.Close()

	y, m, d := day.Date()
	dayStart := time.Date(y, m, d, 0, 0, 0, 0, day.Location())
	dayEnd := dayStart.AddDate(0, 0, 1)
	rows, err = q.Query(`SELECT starts_at, ends_at FROM appointments WHERE staff_id = ? AND status = 'booked' AND starts_at < ? AND ends_at > ?`,
		staffID, dayEnd.Unix(), dayStart.Unix())
	if err != nil {
		return nil, err
	}
	var busy []Busy
	for rows.Next() {
		var s, e int64
		if err := rows.Scan(&s, &e); err != nil {
			rows.Close()
			return nil, err
		}
		busy = append(busy, Busy{time.Unix(s, 0), time.Unix(e, 0)})
	}
	rows.Close()

	var out []time.Time
	for _, t := range freeSlots(hours, busy, dur, dayStart, slotStep) {
		if !t.Before(now) {
			out = append(out, t)
		}
	}
	return out, nil
}

// bookStaff liefert die Mitarbeiterauswahl zur gewählten Leistung (htmx).
func (a *app) bookStaff(w http.ResponseWriter, r *http.Request) {
	s := a.salonFor(w, r)
	if s == nil {
		return
	}
	q := r.URL.Query()
	d := map[string]any{"Sel": int64(0)}
	sv, err := loadService(a.db, s.ID, q.Get("service"))
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		a.serverError(w, s, err)
		return
	}
	if sv != nil {
		staff, err := bookableStaff(a.db, s.ID, sv.ID, a.staffLimit(s))
		if err != nil {
			a.serverError(w, s, err)
			return
		}
		d["Staff"] = staff
		// Auswahl behalten, wenn der Mitarbeiter auch die neue Leistung anbietet.
		if q.Get("staff") != "egal" {
			if sel := pickable(staff, q.Get("staff")); len(sel) == 1 {
				d["Sel"] = sel[0].ID
			}
		}
	}
	w.Header().Set("HX-Trigger-After-Swap", "staff-loaded") // lädt danach die Uhrzeiten neu
	a.render(w, http.StatusOK, "book-staff", d)
}

func (a *app) slots(w http.ResponseWriter, r *http.Request) {
	s := a.salonFor(w, r)
	if s == nil {
		return
	}
	d := map[string]any{}
	if !slotLimit.allow(clientKey(r)) {
		d["Empty"] = "Zu viele Anfragen. Bitte warte kurz und versuch es dann noch einmal."
		status := http.StatusTooManyRequests
		if r.Header.Get("HX-Request") != "" {
			status = http.StatusOK // htmx zeigt nur 2xx-Antworten an
		}
		a.render(w, status, "slots", d)
		return
	}
	q := r.URL.Query()
	now := time.Now()
	first, last := bookingDays(s, now)
	sv, err := loadService(a.db, s.ID, q.Get("service"))
	day, dayErr := time.ParseInLocation(time.DateOnly, q.Get("date"), s.Loc)
	switch {
	case errors.Is(err, sql.ErrNoRows) || dayErr != nil:
		d["Empty"] = "Bitte Leistung und Tag wählen."
	case err != nil:
		a.serverError(w, s, err)
		return
	case day.Before(first):
		d["Empty"] = "Bitte einen Tag ab heute wählen."
	case day.After(last):
		d["Empty"] = "Online buchbar bis " + last.Format("2.1.2006") + ". Für spätere Termine ruf bitte im Salon an."
	default:
		staff, err := bookableStaff(a.db, s.ID, sv.ID, a.staffLimit(s))
		if err != nil {
			a.serverError(w, s, err)
			return
		}
		free, err := slotsFor(a.db, s, pickable(staff, q.Get("staff")), time.Duration(sv.Duration)*time.Minute, day, now)
		if err != nil {
			a.serverError(w, s, err)
			return
		}
		type slot struct {
			Unix  int64
			Label string
		}
		var list []slot
		for _, t := range free {
			list = append(list, slot{t.Unix(), t.Format("15:04")})
		}
		d["Slots"] = list
		d["Empty"] = "An diesem Tag ist nichts mehr frei. Bitte wähl einen anderen Tag."
	}
	a.render(w, http.StatusOK, "slots", d)
}

func (a *app) book(w http.ResponseWriter, r *http.Request) {
	s := a.salonFor(w, r)
	if s == nil {
		return
	}
	if !bookLimit.allow(clientKey(r)) {
		a.fail(w, s, http.StatusTooManyRequests, "Zu viele Buchungen von deinem Anschluss. Bitte versuch es später noch einmal oder ruf im Salon an.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := r.ParseForm(); err != nil {
		a.fail(w, s, http.StatusBadRequest, "Das Formular war unvollständig.")
		return
	}
	if r.PostFormValue("firma") != "" {
		// Honeypot: Bots sehen eine Bestätigung, gespeichert und gemailt wird nichts.
		d := a.page(s, "Termin gebucht")
		d["Name"], d["Email"] = r.PostFormValue("name"), r.PostFormValue("email")
		a.render(w, http.StatusOK, "booked", d)
		return
	}
	name := strings.TrimSpace(r.PostFormValue("name"))
	phone := strings.TrimSpace(r.PostFormValue("phone"))
	addr, mailErr := mail.ParseAddress(strings.TrimSpace(r.PostFormValue("email")))
	start, startErr := strconv.ParseInt(r.PostFormValue("start"), 10, 64)
	switch {
	case name == "" || utf8.RuneCountInString(name) > 100:
		a.fail(w, s, http.StatusBadRequest, "Bitte gib deinen Namen an.")
		return
	case hasControl(name) || hasControl(phone):
		// Name und Telefon landen in Mails an Kunde und Salon: keine Zeilenumbrüche einschleusen.
		a.fail(w, s, http.StatusBadRequest, "Bitte gib Name und Telefonnummer ohne Zeilenumbrüche oder Tabulatoren an.")
		return
	case mailErr != nil || len(addr.Address) > 200:
		a.fail(w, s, http.StatusBadRequest, "Bitte gib eine gültige E-Mail-Adresse an.")
		return
	case len(phone) > 40:
		a.fail(w, s, http.StatusBadRequest, "Die Telefonnummer ist zu lang.")
		return
	case startErr != nil:
		a.fail(w, s, http.StatusBadRequest, "Bitte wähl eine Uhrzeit.")
		return
	}
	email := strings.ToLower(addr.Address)

	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		a.serverError(w, s, err)
		return
	}
	defer tx.Rollback()
	sv, err := loadService(tx, s.ID, r.PostFormValue("service"))
	var staff []staffMember
	if err == nil {
		staff, err = bookableStaff(tx, s.ID, sv.ID, a.staffLimit(s))
		staff = pickable(staff, r.PostFormValue("staff"))
	}
	if errors.Is(err, sql.ErrNoRows) || err == nil && len(staff) == 0 {
		a.fail(w, s, http.StatusBadRequest, "Diese Leistung oder diesen Mitarbeiter gibt es so nicht mehr. Bitte wähl neu.")
		return
	} else if err != nil {
		a.serverError(w, s, err)
		return
	}
	startT := time.Unix(start, 0).In(s.Loc)
	dur := time.Duration(sv.Duration) * time.Minute
	// Erneut in derselben Transaktion prüfen (Vorlauf, Horizont, frei) und bei
	// "egal" den ersten freien Mitarbeiter nehmen.
	st, err := pickStaff(tx, s, staff, dur, startT, time.Now())
	if err != nil {
		a.serverError(w, s, err)
		return
	}
	if st == nil {
		a.fail(w, s, http.StatusConflict, "Dieser Termin ist leider nicht mehr frei. Bitte wähl eine andere Uhrzeit.")
		return
	}
	// Erst hier zählen: abgelehnte Formulare und vergebene Uhrzeiten kosten nichts.
	if !bookMailLimit.allow(email) {
		a.fail(w, s, http.StatusTooManyRequests, "Mit dieser E-Mail-Adresse wurde gerade schon mehrmals gebucht. Bitte versuch es später noch einmal oder ruf im Salon an.")
		return
	}
	if !bookSalonLimit.allow(strconv.FormatInt(s.ID, 10)) {
		a.fail(w, s, http.StatusTooManyRequests, "Gerade gehen sehr viele Buchungen ein. Bitte versuch es später noch einmal oder ruf im Salon an.")
		return
	}
	// Bestehende Karteikarte behält ihren Namen: wer nur die Mail-Adresse kennt, soll ihn nicht überschreiben.
	var customerID int64
	err = tx.QueryRow(`INSERT INTO customers (salon_id, name, email, phone) VALUES (?, ?, ?, ?)
		ON CONFLICT (salon_id, email) DO UPDATE SET phone = CASE WHEN customers.phone = '' THEN excluded.phone ELSE customers.phone END
		RETURNING id`, s.ID, name, email, phone).Scan(&customerID)
	if err != nil {
		a.serverError(w, s, err)
		return
	}
	token := randomToken()
	// Kurzfristige Buchung (< 24 h): die Bestätigung ersetzt die Erinnerung.
	var reminded any
	if time.Until(startT) < 24*time.Hour {
		reminded = time.Now().Unix()
	}
	_, err = tx.Exec(`INSERT INTO appointments (salon_id, staff_id, service_id, customer_id, starts_at, ends_at, cancel_token, reminder_sent_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, s.ID, st.ID, sv.ID, customerID, startT.Unix(), startT.Add(dur).Unix(), token, reminded)
	if err != nil {
		a.serverError(w, s, err)
		return
	}
	if err := tx.Commit(); err != nil {
		a.serverError(w, s, err)
		return
	}

	b, err := a.findBooking(s, token)
	if err != nil {
		a.serverError(w, s, err)
		return
	}
	go func() {
		if err := a.mail.send(email, "Termin bestätigt: "+s.Name, a.customerMail(s, b, "danke für deine Buchung! Dein Termin bei "+s.Name+" steht:")); err != nil {
			log.Printf("Bestätigung an %s: %v", email, err)
		}
		if s.Email != "" {
			body := fmt.Sprintf("Neue Online-Buchung:\n\n%s\nKunde:    %s\nE-Mail:   %s\nTelefon:  %s\n\nAlle Termine: %s/dashboard\n",
				bookingLines(b), b.Customer, b.Email, orDash(b.Phone), a.siteURLLang(s.Lang))
			if err := a.mail.send(s.Email, "Neue Buchung: "+deDate(b.Start), body); err != nil {
				log.Printf("Salon-Info an %s: %v", s.Email, err)
			}
		}
	}()

	d := a.page(s, "Termin gebucht")
	d["When"], d["Service"], d["Staff"], d["Name"], d["Email"] = deDate(b.Start), b.Service, b.Staff, name, email
	d["CancelURL"], d["ICSURL"] = "/storno/"+token, "/termin/"+token+".ics"
	if s.CancelNoticeMin > 0 {
		d["Deadline"] = deDate(cancelDeadline(s, b.Start))
	}
	a.render(w, http.StatusOK, "booked", d)
}

type booking struct {
	ID                     int64
	Token                  string
	Start, End             time.Time
	Status, Service, Staff string
	Customer, Email, Phone string // Email leer = Kunde ohne Mail-Adresse
}

func (a *app) findBooking(s *salon, token string) (*booking, error) {
	b := booking{Token: token}
	var start, end int64
	err := a.db.QueryRow(`SELECT a.id, a.starts_at, a.ends_at, a.status, sv.name, st.name, c.name, COALESCE(c.email, ''), c.phone
		FROM appointments a
		JOIN services sv ON sv.id = a.service_id JOIN staff st ON st.id = a.staff_id JOIN customers c ON c.id = a.customer_id
		WHERE a.cancel_token = ? AND a.salon_id = ?`, token, s.ID).
		Scan(&b.ID, &start, &end, &b.Status, &b.Service, &b.Staff, &b.Customer, &b.Email, &b.Phone)
	if err != nil {
		return nil, err
	}
	b.Start, b.End = time.Unix(start, 0).In(s.Loc), time.Unix(end, 0).In(s.Loc)
	return &b, nil
}

// cancelDeadline: bis hierhin ist die Online-Absage möglich.
func cancelDeadline(s *salon, start time.Time) time.Time {
	return start.Add(-time.Duration(s.CancelNoticeMin) * time.Minute)
}

func (a *app) cancelData(s *salon, b *booking, now time.Time) map[string]any {
	d := a.page(s, "Termin absagen")
	d["When"], d["Service"], d["Staff"] = deDate(b.Start), b.Service, b.Staff
	deadline := cancelDeadline(s, b.Start)
	d["Deadline"] = deDate(deadline)
	d["Cancelled"] = b.Status == "cancelled"
	d["Past"] = b.Status != "booked" || !b.Start.After(now)
	d["TooLate"] = !now.Before(deadline)
	return d
}

// cancelPage zeigt nur an. Storniert wird per POST, damit Link-Vorschauen in
// Mailprogrammen keinen Termin absagen.
func (a *app) cancelPage(w http.ResponseWriter, r *http.Request) {
	s := a.salonFor(w, r)
	if s == nil {
		return
	}
	b, err := a.findBooking(s, r.PathValue("token"))
	if errors.Is(err, sql.ErrNoRows) {
		a.fail(w, s, http.StatusNotFound, "Diesen Termin gibt es nicht.")
		return
	} else if err != nil {
		a.serverError(w, s, err)
		return
	}
	a.render(w, http.StatusOK, "cancel", a.cancelData(s, b, time.Now()))
}

func (a *app) cancel(w http.ResponseWriter, r *http.Request) {
	s := a.salonFor(w, r)
	if s == nil {
		return
	}
	b, err := a.findBooking(s, r.PathValue("token"))
	if errors.Is(err, sql.ErrNoRows) {
		a.fail(w, s, http.StatusNotFound, "Diesen Termin gibt es nicht.")
		return
	} else if err != nil {
		a.serverError(w, s, err)
		return
	}
	now := time.Now()
	res, err := a.db.Exec(`UPDATE appointments SET status = 'cancelled'
		WHERE id = ? AND salon_id = ? AND status = 'booked' AND starts_at > ?`,
		b.ID, s.ID, now.Add(time.Duration(s.CancelNoticeMin)*time.Minute).Unix())
	if err != nil {
		a.serverError(w, s, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		a.render(w, http.StatusConflict, "cancel", a.cancelData(s, b, now)) // erklärt, warum nicht (mehr)
		return
	}
	go func() {
		if s.Email != "" {
			body := fmt.Sprintf("%s hat online abgesagt:\n\n%s\nDer Termin ist im Kalender wieder frei.\n", b.Customer, bookingLines(b))
			if err := a.mail.send(s.Email, "Termin abgesagt: "+deDate(b.Start), body); err != nil {
				log.Printf("Absage-Info an %s: %v", s.Email, err)
			}
		}
		if b.Email != "" {
			body := fmt.Sprintf("Hallo %s,\n\ndeine Absage ist angekommen. Dieser Termin ist storniert:\n\n%s\nNeuen Termin buchen: %s\n\nViele Grüße\n%s\n",
				b.Customer, bookingLines(b), a.link(s.Slug, "/"), salonContact(s))
			if err := a.mail.send(b.Email, "Termin abgesagt: "+s.Name, body); err != nil {
				log.Printf("Absage-Bestätigung an %s: %v", b.Email, err)
			}
		}
	}()
	a.render(w, http.StatusOK, "cancelled", a.page(s, "Termin abgesagt"))
}

// bookingICS liefert den Kalendereintrag zu einem gebuchten Termin des Salons.
func (a *app) bookingICS(w http.ResponseWriter, r *http.Request) {
	s := a.salonFor(w, r)
	if s == nil {
		return
	}
	token, ok := strings.CutSuffix(r.PathValue("file"), ".ics")
	var b *booking
	err := sql.ErrNoRows
	if ok {
		b, err = a.findBooking(s, token)
	}
	if errors.Is(err, sql.ErrNoRows) || err == nil && b.Status != "booked" {
		a.fail(w, s, http.StatusNotFound, "Diesen Termin gibt es nicht (mehr).")
		return
	} else if err != nil {
		a.serverError(w, s, err)
		return
	}
	where := s.Name
	if addr := oneLine(s.Address); addr != "" {
		where += ", " + addr
	}
	desc := "Bei " + b.Staff + "\n"
	if s.Phone != "" {
		desc += "Telefon: " + s.Phone + "\n"
	}
	desc += "Absagen: " + a.link(s.Slug, "/storno/"+token)
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="termin.ics"`)
	io.WriteString(w, buildICS(hashToken(token)[:32]+"@kammzeit", b.Service+" bei "+s.Name, where, desc, b.Start, b.End, time.Now()))
}

// buildICS baut ein iCalendar-Dokument (RFC 5545) mit einem Termin, Zeiten in UTC.
func buildICS(uid, summary, location, description string, start, end, stamp time.Time) string {
	const utc = "20060102T150405Z"
	lines := []string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//Kammzeit//Terminbuchung//DE",
		"CALSCALE:GREGORIAN",
		"METHOD:PUBLISH",
		"BEGIN:VEVENT",
		"UID:" + uid,
		"DTSTAMP:" + stamp.UTC().Format(utc),
		"DTSTART:" + start.UTC().Format(utc),
		"DTEND:" + end.UTC().Format(utc),
		"SUMMARY:" + icsText(summary),
		"LOCATION:" + icsText(location),
		"DESCRIPTION:" + icsText(description),
		"END:VEVENT",
		"END:VCALENDAR",
	}
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(icsFold(l))
		b.WriteString("\r\n")
	}
	return b.String()
}

var icsEscaper = strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\r\n", `\n`, "\n", `\n`, "\r", `\n`)

// icsText maskiert einen TEXT-Wert (RFC 5545 3.3.11).
func icsText(s string) string { return icsEscaper.Replace(s) }

// icsFold bricht nach höchstens 75 Bytes um (RFC 5545 3.1), ohne UTF-8-Zeichen zu zerteilen.
func icsFold(line string) string {
	var b strings.Builder
	n := 0
	for _, r := range line {
		size := utf8.RuneLen(r)
		if size < 0 {
			size = 3 // ungültiges Byte, WriteRune schreibt U+FFFD
		}
		if n+size > 75 {
			b.WriteString("\r\n ")
			n = 1
		}
		b.WriteRune(r)
		n += size
	}
	return b.String()
}

// --- Mails ---

// bookingLines: Termindaten für Mails.
func bookingLines(b *booking) string {
	return fmt.Sprintf("Wann:     %s\nLeistung: %s (%d Min.)\nBei:      %s\n", deDate(b.Start), b.Service, int(b.End.Sub(b.Start).Minutes()), b.Staff)
}

// salonContact: Name, Adresse und Telefon des Salons für Mails an Kunden.
func salonContact(s *salon) string {
	out := s.Name + "\n"
	if addr := strings.TrimSpace(strings.ReplaceAll(s.Address, "\r", "")); addr != "" {
		out += addr + "\n"
	}
	if s.Phone != "" {
		out += "Telefon: " + s.Phone + "\n"
	}
	return out
}

// customerMail: Bestätigung und Erinnerung an den Kunden.
func (a *app) customerMail(s *salon, b *booking, intro string) string {
	var m strings.Builder
	fmt.Fprintf(&m, "Hallo %s,\n\n%s\n\n%s\n", b.Customer, intro, bookingLines(b))
	fmt.Fprintf(&m, "So findest du uns:\n%s\n", salonContact(s))
	fmt.Fprintf(&m, "In deinen Kalender eintragen:\n%s\n\n", a.link(s.Slug, "/termin/"+b.Token+".ics"))
	fmt.Fprintf(&m, "Du kannst doch nicht? Bitte sag ab, dann wird der Termin für andere frei:\n%s\n", a.link(s.Slug, "/storno/"+b.Token))
	if s.CancelNoticeMin > 0 {
		fmt.Fprintf(&m, "Online-Absage bis %s möglich, danach bitte im Salon anrufen.\n", deDate(cancelDeadline(s, b.Start)))
	}
	fmt.Fprintf(&m, "\nBis bald!\n%s\n", s.Name)
	return m.String()
}

// --- Formatierung ---

var weekdaysDE = [...]string{"Sonntag", "Montag", "Dienstag", "Mittwoch", "Donnerstag", "Freitag", "Samstag"}

func deDate(t time.Time) string {
	return fmt.Sprintf("%s, %d.%d.%d, %s Uhr", weekdaysDE[t.Weekday()], t.Day(), t.Month(), t.Year(), t.Format("15:04"))
}

// noticeText: Frist in Minuten als "24 Stunden", "2 Tage", "90 Minuten".
func noticeText(m int) string {
	unit := func(n int, one, many string) string {
		if n == 1 {
			return "1 " + one
		}
		return strconv.Itoa(n) + " " + many
	}
	switch {
	case m%1440 == 0:
		return unit(m/1440, "Tag", "Tage")
	case m%60 == 0:
		return unit(m/60, "Stunde", "Stunden")
	}
	return unit(m, "Minute", "Minuten")
}

// onAccent wählt Schwarz oder Weiß als Textfarbe auf der Akzentfarbe, je
// nachdem, was nach WCAG den höheren Kontrast hat.
func onAccent(hex string) string {
	if !hexColor.MatchString(hex) {
		return "#000"
	}
	lin := func(i int) float64 {
		v, _ := strconv.ParseUint(hex[i:i+2], 16, 8)
		c := float64(v) / 255
		if c <= 0.04045 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	l := 0.2126*lin(1) + 0.7152*lin(3) + 0.0722*lin(5)
	// Kontrast zu Weiß 1.05/(l+0.05), zu Schwarz (l+0.05)/0.05
	if (l+0.05)*(l+0.05) > 1.05*0.05 {
		return "#000"
	}
	return "#fff"
}

// extURL macht aus einer Salon-Eingabe einen http(s)-Link, sonst "".
func extURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	return u.String()
}

// telURL baut einen tel:-Link (nur Ziffern und führendes +). html/template
// würde tel: sonst als unsicher verwerfen.
func telURL(phone string) template.URL {
	var b strings.Builder
	for i, r := range strings.TrimSpace(phone) {
		if r >= '0' && r <= '9' || r == '+' && i == 0 {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return ""
	}
	return template.URL("tel:" + b.String())
}

// oneLine fasst eine mehrzeilige Adresse zu "Straße, Ort" zusammen.
func oneLine(s string) string {
	var parts []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			parts = append(parts, l)
		}
	}
	return strings.Join(parts, ", ")
}

// hasControl: enthält Steuerzeichen (auch CR, LF, Tab).
func hasControl(s string) bool { return strings.IndexFunc(s, unicode.IsControl) >= 0 }

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

var templateFuncs = map[string]any{
	"euro": func(cents int64) string { return fmt.Sprintf("%d,%02d €", cents/100, cents%100) },
}
