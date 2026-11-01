package main

import (
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/mail"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// ponytail: festes 15-Minuten-Raster, pro Salon einstellbar erst bei Bedarf.
const slotStep = 15 * time.Minute

type salon struct {
	ID                              int64
	Slug, Name, Email, Accent, Plan string
	Loc                             *time.Location
}

type service struct {
	ID, PriceCents int64
	Name           string
	Duration       int
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

func (a *app) routes() http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(webFS, "web/static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))
	mux.HandleFunc("GET /{$}", a.home)
	mux.HandleFunc("GET /slots", a.slots)
	mux.HandleFunc("POST /book", a.book)
	mux.HandleFunc("GET /storno/{token}", a.cancelPage)
	mux.HandleFunc("POST /storno/{token}", a.cancel)
	mux.HandleFunc("GET /impressum", a.static("impressum", "Impressum"))
	mux.HandleFunc("GET /datenschutz", a.static("datenschutz", "Datenschutz"))
	mux.HandleFunc("GET /_tls/allow", a.tlsAllow)
	// ponytail: Admin (Kalender, Kundenkartei, Leistungen pflegen) kommt mit dem
	// Login. Bis dahin bewusst gesperrt statt ungeschützt.
	admin := func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Der Admin-Bereich folgt (Login fehlt noch).", http.StatusNotImplemented)
	}
	mux.HandleFunc("/admin", admin)
	mux.HandleFunc("/admin/", admin)
	return securityHeaders(mux)
}

// static rendert eine Seite ohne Salon-Bezug (Impressum, Datenschutz).
func (a *app) static(name, title string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a.render(w, http.StatusOK, name, a.page(nil, title))
	}
}

// tlsAllow beantwortet Caddys on_demand-"ask": 200 nur für die Basis-Domain,
// www und Subdomains existierender Salons. Sonst könnte jeder beliebige
// Subdomains aufrufen und Zertifikate (Let's-Encrypt-Limits) verbrennen.
func (a *app) tlsAllow(w http.ResponseWriter, r *http.Request) {
	if !a.tlsAllowed(r.URL.Query().Get("domain")) {
		http.Error(w, "nein", http.StatusForbidden)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (a *app) tlsAllowed(domain string) bool {
	domain = strings.ToLower(domain)
	if domain == "www."+a.cfg.BaseDomain {
		return true
	}
	slug, root := tenantSlug(domain, a.cfg.BaseDomain)
	if root {
		return true
	}
	if slug == "" {
		return false
	}
	var one int
	return a.db.QueryRow(`SELECT 1 FROM salons WHERE slug = ?`, slug).Scan(&one) == nil
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin") // Storno-Token steht in der URL
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
	d := map[string]any{"Title": title}
	if s != nil {
		d["Salon"] = s
		d["Accent"] = s.Accent
		d["Branding"] = !a.cfg.SingleTenant && s.Plan == "free"
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
	const cols = `SELECT id, slug, name, email, accent, plan, timezone FROM salons `
	if a.cfg.SingleTenant {
		row = a.db.QueryRow(cols + `ORDER BY id LIMIT 1`)
	} else {
		slug, _ := tenantSlug(r.Host, a.cfg.BaseDomain)
		if slug == "" {
			a.fail(w, nil, http.StatusNotFound, "Diesen Salon gibt es nicht.")
			return nil
		}
		row = a.db.QueryRow(cols+`WHERE slug = ?`, slug)
	}
	var s salon
	var tz string
	err := row.Scan(&s.ID, &s.Slug, &s.Name, &s.Email, &s.Accent, &s.Plan, &tz)
	if errors.Is(err, sql.ErrNoRows) {
		a.fail(w, nil, http.StatusNotFound, "Diesen Salon gibt es nicht.")
		return nil
	}
	if err != nil {
		a.serverError(w, nil, err)
		return nil
	}
	s.Loc = location(tz)
	if !hexColor.MatchString(s.Accent) {
		s.Accent = "#3b82f6"
	}
	return &s
}

func (a *app) home(w http.ResponseWriter, r *http.Request) {
	if !a.cfg.SingleTenant {
		if _, root := tenantSlug(r.Host, a.cfg.BaseDomain); root {
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
	staff, err := a.staff(s.ID)
	if err != nil {
		a.serverError(w, s, err)
		return
	}
	d := a.page(s, s.Name+" - Termin buchen")
	d["Services"] = services
	d["Staff"] = staff
	d["Today"] = time.Now().In(s.Loc).Format(time.DateOnly)
	a.render(w, http.StatusOK, "book", d)
}

func (a *app) services(salonID int64) ([]service, error) {
	rows, err := a.db.Query(`SELECT id, name, duration_min, price_cents FROM services WHERE salon_id = ? AND active = 1 ORDER BY name`, salonID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []service
	for rows.Next() {
		var s service
		if err := rows.Scan(&s.ID, &s.Name, &s.Duration, &s.PriceCents); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ponytail: jeder Mitarbeiter kann jede Leistung; Zuordnung (staff_services) erst, wenn Salons danach fragen.
func (a *app) staff(salonID int64) ([]staffMember, error) {
	rows, err := a.db.Query(`SELECT id, name FROM staff WHERE salon_id = ? AND active = 1 ORDER BY name`, salonID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
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

// lookup lädt Leistung und Mitarbeiter und prüft, dass beide zum Salon gehören.
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
// und gibt die freien Startzeiten nach `now` zurück.
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
		if t.After(now) {
			out = append(out, t)
		}
	}
	return out, nil
}

func (a *app) slots(w http.ResponseWriter, r *http.Request) {
	s := a.salonFor(w, r)
	if s == nil {
		return
	}
	q := r.URL.Query()
	d := map[string]any{}
	sv, st, err := lookup(a.db, s.ID, q.Get("service"), q.Get("staff"))
	day, dayErr := time.ParseInLocation(time.DateOnly, q.Get("date"), s.Loc)
	switch {
	case errors.Is(err, sql.ErrNoRows) || dayErr != nil:
		d["Empty"] = "Bitte Leistung, Mitarbeiter und Tag wählen."
	case err != nil:
		a.serverError(w, s, err)
		return
	default:
		free, err := freeSlotsFor(a.db, st.ID, time.Duration(sv.Duration)*time.Minute, day, time.Now())
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
		d["Empty"] = "An diesem Tag ist nichts mehr frei. Bitte einen anderen Tag wählen."
	}
	a.render(w, http.StatusOK, "slots", d)
}

func (a *app) book(w http.ResponseWriter, r *http.Request) {
	s := a.salonFor(w, r)
	if s == nil {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := r.ParseForm(); err != nil {
		a.fail(w, s, http.StatusBadRequest, "Das Formular war unvollständig.")
		return
	}
	name := strings.TrimSpace(r.PostFormValue("name"))
	phone := strings.TrimSpace(r.PostFormValue("phone"))
	addr, mailErr := mail.ParseAddress(strings.TrimSpace(r.PostFormValue("email")))
	start, startErr := strconv.ParseInt(r.PostFormValue("start"), 10, 64)
	switch {
	case name == "" || utf8.RuneCountInString(name) > 100:
		a.fail(w, s, http.StatusBadRequest, "Bitte einen Namen angeben.")
		return
	case mailErr != nil || len(addr.Address) > 200:
		a.fail(w, s, http.StatusBadRequest, "Bitte eine gültige E-Mail-Adresse angeben.")
		return
	case len(phone) > 40:
		a.fail(w, s, http.StatusBadRequest, "Die Telefonnummer ist zu lang.")
		return
	case startErr != nil:
		a.fail(w, s, http.StatusBadRequest, "Bitte eine Uhrzeit wählen.")
		return
	}
	email := strings.ToLower(addr.Address)

	// ponytail: kein Spam-/Rate-Limit; kommt, sobald jemand das Formular zumüllt.
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		a.serverError(w, s, err)
		return
	}
	defer tx.Rollback()
	sv, st, err := lookup(tx, s.ID, r.PostFormValue("service"), r.PostFormValue("staff"))
	if errors.Is(err, sql.ErrNoRows) {
		a.fail(w, s, http.StatusBadRequest, "Leistung oder Mitarbeiter gibt es nicht mehr.")
		return
	} else if err != nil {
		a.serverError(w, s, err)
		return
	}
	startT := time.Unix(start, 0).In(s.Loc)
	dur := time.Duration(sv.Duration) * time.Minute
	// Erneut in derselben Transaktion prüfen: der Slot muss noch frei sein.
	free, err := freeSlotsFor(tx, st.ID, dur, startT, time.Now())
	if err != nil {
		a.serverError(w, s, err)
		return
	}
	ok := false
	for _, t := range free {
		ok = ok || t.Equal(startT)
	}
	if !ok {
		a.fail(w, s, http.StatusConflict, "Dieser Termin ist leider nicht mehr frei. Bitte eine andere Uhrzeit wählen.")
		return
	}
	// Bestehende Karteikarte behält ihren Namen: wer nur die Mail-Adresse kennt, soll ihn nicht überschreiben.
	var customerID int64
	err = tx.QueryRow(`INSERT INTO customers (salon_id, name, email, phone) VALUES (?, ?, ?, ?)
		ON CONFLICT (salon_id, email) DO UPDATE SET email = excluded.email RETURNING id`,
		s.ID, name, email, phone).Scan(&customerID)
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

	cancelURL := a.link(s.Slug, "/storno/"+token)
	when := deDate(startT)
	go func() {
		body := fmt.Sprintf("Hallo %s,\n\ndein Termin bei %s ist gebucht:\n\n%s\n%s bei %s\n\nAbsagen: %s\n", name, s.Name, when, sv.Name, st.Name, cancelURL)
		if err := a.mail.send(email, "Termin bestätigt: "+s.Name, body); err != nil {
			log.Printf("Bestätigung an %s: %v", email, err)
		}
		if s.Email != "" {
			body := fmt.Sprintf("Neue Buchung:\n\n%s\n%s bei %s\nKunde: %s, %s %s\n", when, sv.Name, st.Name, name, email, phone)
			if err := a.mail.send(s.Email, "Neue Buchung: "+when, body); err != nil {
				log.Printf("Salon-Info an %s: %v", s.Email, err)
			}
		}
	}()

	d := a.page(s, "Termin gebucht")
	d["When"], d["Service"], d["Staff"], d["Email"], d["CancelURL"] = when, sv.Name, st.Name, email, cancelURL
	a.render(w, http.StatusOK, "booked", d)
}

type booking struct {
	Start                  time.Time
	Status, Service, Staff string
}

func (a *app) findBooking(s *salon, token string) (*booking, error) {
	var b booking
	var start int64
	err := a.db.QueryRow(`SELECT a.starts_at, a.status, sv.name, st.name FROM appointments a
		JOIN services sv ON sv.id = a.service_id JOIN staff st ON st.id = a.staff_id
		WHERE a.cancel_token = ? AND a.salon_id = ?`, token, s.ID).Scan(&start, &b.Status, &b.Service, &b.Staff)
	if err != nil {
		return nil, err
	}
	b.Start = time.Unix(start, 0).In(s.Loc)
	return &b, nil
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
	d := a.page(s, "Termin absagen")
	d["When"], d["Service"], d["Staff"] = deDate(b.Start), b.Service, b.Staff
	d["Cancelled"] = b.Status == "cancelled"
	d["Past"] = !b.Start.After(time.Now())
	a.render(w, http.StatusOK, "cancel", d)
}

// ponytail: Absagen geht bis zum Terminbeginn; eine Frist (z. B. 24 h) wird Salon-Einstellung, wenn gewünscht.
func (a *app) cancel(w http.ResponseWriter, r *http.Request) {
	s := a.salonFor(w, r)
	if s == nil {
		return
	}
	token := r.PathValue("token")
	res, err := a.db.Exec(`UPDATE appointments SET status = 'cancelled'
		WHERE cancel_token = ? AND salon_id = ? AND status = 'booked' AND starts_at > ?`, token, s.ID, time.Now().Unix())
	if err != nil {
		a.serverError(w, s, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		a.fail(w, s, http.StatusConflict, "Dieser Termin kann nicht mehr abgesagt werden.")
		return
	}
	if b, err := a.findBooking(s, token); err == nil && s.Email != "" {
		go func() {
			if err := a.mail.send(s.Email, "Termin abgesagt: "+deDate(b.Start), fmt.Sprintf("Abgesagt:\n\n%s\n%s bei %s\n", deDate(b.Start), b.Service, b.Staff)); err != nil {
				log.Printf("Absage-Info an %s: %v", s.Email, err)
			}
		}()
	}
	a.render(w, http.StatusOK, "cancelled", a.page(s, "Termin abgesagt"))
}

// --- Formatierung ---

var weekdaysDE = [...]string{"Sonntag", "Montag", "Dienstag", "Mittwoch", "Donnerstag", "Freitag", "Samstag"}

func deDate(t time.Time) string {
	return fmt.Sprintf("%s, %d.%d.%d, %s Uhr", weekdaysDE[t.Weekday()], t.Day(), t.Month(), t.Year(), t.Format("15:04"))
}

var templateFuncs = map[string]any{
	"euro": func(cents int64) string { return fmt.Sprintf("%d,%02d €", cents/100, cents%100) },
}
