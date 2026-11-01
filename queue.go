package main

// Warteschlange für Laufkundschaft (Pro-Plan und Einzelbetrieb, je Salon
// schaltbar): Kunden scannen einen QR-Code im Laden, tragen ihren Vornamen ein
// und sehen live, wann sie dran sind. Der Salon ruft im Dashboard auf, ein
// Bildschirm im Laden zeigt die Nummern. Tabelle queue_entries (Migration 3).
//
// Dashboard (Hauptdomain): /dashboard/warteschlange[/liste|/qr|/anzeige[/stand]],
// POST /dashboard/warteschlange/einstellungen, /neu, /{id}/{aktion}.
// Öffentlich (Salon-Subdomain): /warteschlange, /warteschlange/{token}[/stand],
// POST /warteschlange, /warteschlange/{token}/gehen.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	queueMaxPerDay   = 50 // öffentliche Einträge pro Salon und Tag (Spam-Deckel)
	queueFallbackMin = 20 // Dauer pro Person ohne gewählte Leistung
	queueCookie      = "kz_queue"
)

var (
	// Pro Anschluss und Salon. Großzügig, weil im Laden-WLAN viele Kunden
	// über eine IP kommen; der Tagesdeckel queueMaxPerDay begrenzt den Rest.
	queueJoinLimit = newLimiter(20, time.Hour)
	queueReadLimit = newLimiter(240, time.Minute) // Statusseiten (Polling alle 15 s)
)

var errQueueFull = errors.New("warteschlange für heute voll")

func (a *app) routesQueue(mux *http.ServeMux) {
	mux.HandleFunc("GET /dashboard/warteschlange", a.dash(a.queueDash))
	mux.HandleFunc("GET /dashboard/warteschlange/liste", a.dash(a.queueDash))
	mux.HandleFunc("GET /dashboard/warteschlange/qr", a.dash(a.queueQR))
	mux.HandleFunc("GET /dashboard/warteschlange/anzeige", a.dash(a.queueDisplay))
	mux.HandleFunc("GET /dashboard/warteschlange/anzeige/stand", a.dash(a.queueDisplay))
	mux.HandleFunc("POST /dashboard/warteschlange/einstellungen", a.dashOwner(a.queueSettings)) // an/aus + Hinweis: nur Inhaber
	mux.HandleFunc("POST /dashboard/warteschlange/neu", a.dash(a.queueDashAdd))
	mux.HandleFunc("POST /dashboard/warteschlange/{id}/{aktion}", a.dash(a.queueAction))
	mux.HandleFunc("GET /warteschlange", a.queueJoinPage)
	mux.HandleFunc("POST /warteschlange", a.queueJoin)
	mux.HandleFunc("GET /warteschlange/{token}", a.queueStatus)
	mux.HandleFunc("GET /warteschlange/{token}/stand", a.queueStatus)
	mux.HandleFunc("POST /warteschlange/{token}/gehen", a.queueLeave)
}

// queueAllowed: die Warteschlange gibt es im Pro-Plan und im Einzelbetrieb (wie staffLimit).
func (a *app) queueAllowed(s *salon) bool { return a.cfg.SingleTenant || s.Pro() }

// queueBookingLink: Hinweis "Online anstellen" auf der Buchungsseite zeigen?
// Nur mit Recht (Pro/Einzelbetrieb), eingeschaltet und laut Arbeitszeiten geöffnet.
func (a *app) queueBookingLink(s *salon) bool {
	if !a.queueAllowed(s) {
		return false
	}
	st, err := a.queueLoad(s, time.Now())
	return err == nil && st.Enabled && st.Open
}

type queueEntry struct {
	ID                         int64
	Number, Duration           int // Duration: Minuten der gewählten Leistung, 0 = keine
	Name, Email, Status, Token string
	Service, Staff             string
	Created                    time.Time
	Ahead, WaitMin, SinceMin   int // berechnet von queueEstimate
}

// queueState = Stand der Warteschlange eines Salons für heute.
type queueState struct {
	Enabled bool
	Note    string
	Open    bool // laut Arbeitszeiten gerade geöffnet
	Staff   int  // Mitarbeiter mit Arbeitszeit heute (Teiler der Schätzung)
	Day     string
	Entries []queueEntry // alle von heute, nach Nummer
	NewWait int          // geschätzte Wartezeit für jemanden, der sich jetzt anstellt
}

// queueWait schätzt die Wartezeit: Summe der Leistungsdauern der Wartenden
// davor (ohne Leistung queueFallbackMin) geteilt durch die Mitarbeiter heute,
// aufgerundet auf 5 Minuten.
func queueWait(durations []int, staff int) int {
	sum := 0
	for _, d := range durations {
		if d <= 0 {
			d = queueFallbackMin
		}
		sum += d
	}
	staff = max(staff, 1)
	m := (sum + staff - 1) / staff
	return (m + 4) / 5 * 5
}

// queueEstimate füllt SinceMin für alle und Ahead/WaitMin für Wartende und
// Aufgerufene (beide zählen als "vor dir"). Liefert die Wartezeit für Neue.
// ponytail: wer gerade bedient wird, zählt nicht mit; Mitarbeiterwunsch auch nicht.
func queueEstimate(list []queueEntry, staff int, now time.Time) int {
	var ahead []int
	for i := range list {
		e := &list[i]
		e.SinceMin = max(int(now.Sub(e.Created).Minutes()), 0)
		if e.Status != "waiting" && e.Status != "called" {
			continue
		}
		e.Ahead = len(ahead)
		if e.Status == "waiting" {
			e.WaitMin = queueWait(ahead, staff)
		}
		ahead = append(ahead, e.Duration)
	}
	return queueWait(ahead, staff)
}

func (a *app) queueLoad(s *salon, now time.Time) (*queueState, error) {
	t := now.In(s.Loc)
	st := &queueState{Day: t.Format(time.DateOnly)}
	if err := a.db.QueryRow(`SELECT queue_enabled, queue_note FROM salons WHERE id = ?`, s.ID).Scan(&st.Enabled, &st.Note); err != nil {
		return nil, err
	}
	var from, to int
	if err := a.db.QueryRow(`SELECT COUNT(DISTINCT st.id), COALESCE(MIN(wh.start_min), 0), COALESCE(MAX(wh.end_min), 0)
		FROM staff st JOIN working_hours wh ON wh.staff_id = st.id
		WHERE st.salon_id = ? AND st.active = 1 AND wh.weekday = ?`, s.ID, int(t.Weekday())).Scan(&st.Staff, &from, &to); err != nil {
		return nil, err
	}
	m := t.Hour()*60 + t.Minute()
	st.Open = st.Staff > 0 && m >= from && m < to
	rows, err := a.db.Query(`SELECT q.id, q.number, q.name, COALESCE(q.email, ''), q.status, q.token, q.created_at,
			COALESCE(sv.name, ''), COALESCE(sv.duration_min, 0), COALESCE(sf.name, '')
		FROM queue_entries q
		LEFT JOIN services sv ON sv.id = q.service_id AND sv.salon_id = q.salon_id
		LEFT JOIN staff sf ON sf.id = q.staff_id AND sf.salon_id = q.salon_id
		WHERE q.salon_id = ? AND q.day = ? ORDER BY q.number`, s.ID, st.Day)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var e queueEntry
		var created int64
		if err := rows.Scan(&e.ID, &e.Number, &e.Name, &e.Email, &e.Status, &e.Token, &created, &e.Service, &e.Duration, &e.Staff); err != nil {
			return nil, err
		}
		e.Created = time.Unix(created, 0)
		st.Entries = append(st.Entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	st.NewWait = queueEstimate(st.Entries, st.Staff, now)
	return st, nil
}

// waiting zählt, wer noch wartet oder aufgerufen ist.
func (st *queueState) waiting() int {
	n := 0
	for _, e := range st.Entries {
		if e.Status == "waiting" || e.Status == "called" {
			n++
		}
	}
	return n
}

// queuePurge löscht Einträge früherer Tage: Namen und Mail-Adressen braucht
// danach niemand mehr (Datenschutz).
func (a *app) queuePurge(s *salon, day string) {
	if _, err := a.db.Exec(`DELETE FROM queue_entries WHERE salon_id = ? AND day < ?`, s.ID, day); err != nil {
		log.Printf("Warteschlange %d aufräumen: %v", s.ID, err)
	}
}

// queueAdd reiht ein und vergibt die nächste Nummer des Tages (Salon-Zeitzone).
// limit > 0 begrenzt die Einträge pro Tag (öffentliches Formular).
// Die Nummer entsteht im selben INSERT (MAX + 1), alles in einer Transaktion
// über die einzige DB-Verbindung; UNIQUE (salon_id, day, number) sichert zusätzlich.
func (a *app) queueAdd(ctx context.Context, s *salon, now time.Time, name, email string, serviceID, staffID int64, limit int) (int, string, error) {
	day := now.In(s.Loc).Format(time.DateOnly)
	a.queuePurge(s, day)
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, "", err
	}
	defer tx.Rollback()
	if limit > 0 {
		var n int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM queue_entries WHERE salon_id = ? AND day = ?`, s.ID, day).Scan(&n); err != nil {
			return 0, "", err
		}
		if n >= limit {
			return 0, "", errQueueFull
		}
	}
	null := func(v any, empty bool) any {
		if empty {
			return nil
		}
		return v
	}
	token := randomToken()
	var number int
	err = tx.QueryRow(`INSERT INTO queue_entries (salon_id, number, day, name, email, service_id, staff_id, token, created_at)
		SELECT ?1, COALESCE(MAX(number), 0) + 1, ?2, ?3, ?4, ?5, ?6, ?7, ?8 FROM queue_entries WHERE salon_id = ?1 AND day = ?2
		RETURNING number`, s.ID, day, name, null(email, email == ""), null(serviceID, serviceID == 0), null(staffID, staffID == 0),
		token, now.Unix()).Scan(&number)
	if err != nil {
		return 0, "", err
	}
	return number, token, tx.Commit()
}

// queueName prüft den Namen: 1 bis 60 Zeichen, keine Steuerzeichen (landet in Mails).
func queueName(s string) (string, bool) {
	s = strings.TrimSpace(s)
	return s, s != "" && utf8.RuneCountInString(s) <= 60 && !hasControl(s)
}

// queueRef prüft eine optionale Leistung oder einen optionalen Mitarbeiter aus
// dem Formular: leer = 0, sonst muss die ID aktiv zum Salon gehören.
// table ist eine Konstante des Aufrufers (services | staff).
func (a *app) queueRef(table string, salonID int64, v string) (int64, bool) {
	if v == "" {
		return 0, true
	}
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, false
	}
	var one int
	return id, a.db.QueryRow(`SELECT 1 FROM `+table+` WHERE id = ? AND salon_id = ? AND active = 1`, id, salonID).Scan(&one) == nil
}

func isHX(r *http.Request) bool { return r.Header.Get("HX-Request") != "" }

// --- Dashboard ---

// queueGate: im Gratis-Plan gibt es statt der Warteschlange den Pro-Hinweis.
func (a *app) queueGate(w http.ResponseWriter, r *http.Request, c *dctx) bool {
	if a.queueAllowed(c.Salon) {
		return true
	}
	lang := a.langFor(r)
	if r.Method != http.MethodGet || isHX(r) {
		http.Error(w, tr(lang, "Die Warteschlange gibt es im Pro-Plan."), http.StatusForbidden)
		return false
	}
	d := a.dashPage(c, r, "warteschlange", tr(lang, "Warteschlange"))
	d["PageTitle"], d["Heading"] = d["Heading"], ""
	a.render(w, http.StatusOK, "queue-pro", d)
	return false
}

// queueDash: Seite und (unter /liste, für das Polling) nur die Liste.
func (a *app) queueDash(w http.ResponseWriter, r *http.Request, c *dctx) {
	if !a.queueGate(w, r, c) {
		return
	}
	now := time.Now()
	lang := a.langFor(r)
	list := strings.HasSuffix(r.URL.Path, "/liste")
	if !list {
		a.queuePurge(c.Salon, now.In(c.Salon.Loc).Format(time.DateOnly))
	}
	st, err := a.queueLoad(c.Salon, now)
	if err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	d := a.dashPage(c, r, "warteschlange", tr(lang, "Warteschlange"))
	d["PageTitle"], d["Heading"] = d["Heading"], ""
	d["Q"] = st
	var active []queueEntry
	count := map[string]int{}
	for _, e := range st.Entries {
		count[e.Status]++
		if e.Status == "waiting" || e.Status == "called" || e.Status == "serving" {
			active = append(active, e)
		}
	}
	d["Active"] = active
	d["NWaiting"] = count["waiting"] + count["called"]
	d["NDone"], d["NNoshow"], d["NLeft"] = count["done"], count["noshow"], count["left"]
	if list {
		w.Header().Set("Cache-Control", "no-store")
		a.render(w, http.StatusOK, "queue-liste", d)
		return
	}
	services, err := a.services(c.Salon.ID)
	if err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	staff, err := a.staff(c.Salon.ID)
	if err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	d["Services"], d["Staff"] = services, staff
	d["JoinURL"] = a.bookingURL(c.Salon, "/warteschlange")
	q := r.URL.Query()
	switch q.Get("gespeichert") {
	case "an":
		d["SavedMsg"] = tr(lang, "Die Warteschlange ist an. Kunden können sich jetzt anstellen.")
	case "aus":
		d["SavedMsg"] = tr(lang, "Die Warteschlange ist aus.")
	case "hinweis":
		d["SavedMsg"] = tr(lang, "Hinweis gespeichert.")
	case "neu":
		if nr, err := strconv.Atoi(q.Get("nr")); err == nil {
			d["SavedMsg"] = trf(lang, "Eingereiht mit Nummer %d.", nr)
		}
	}
	a.render(w, http.StatusOK, "queue-dash", d)
}

// queueSettings schaltet an/aus (Feld an=1|0) und speichert den Hinweis (Feld hinweis).
func (a *app) queueSettings(w http.ResponseWriter, r *http.Request, c *dctx) {
	if !a.queueGate(w, r, c) {
		return
	}
	lang := a.langFor(r)
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	if err := r.ParseForm(); err != nil {
		a.dashError(w, r, c, http.StatusBadRequest, tr(lang, "Das Formular war unvollständig."))
		return
	}
	saved := "1"
	if r.PostForm.Has("an") {
		on := r.PostForm.Get("an") == "1"
		if _, err := a.db.Exec(`UPDATE salons SET queue_enabled = ? WHERE id = ?`, on, c.Salon.ID); err != nil {
			a.dashServerError(w, r, c, err)
			return
		}
		saved = map[bool]string{true: "an", false: "aus"}[on]
	}
	if r.PostForm.Has("hinweis") {
		note := strings.TrimSpace(strings.ReplaceAll(r.PostForm.Get("hinweis"), "\r", ""))
		if utf8.RuneCountInString(note) > 300 {
			a.dashError(w, r, c, http.StatusBadRequest, tr(lang, "Der Hinweis ist zu lang (höchstens 300 Zeichen)."))
			return
		}
		if _, err := a.db.Exec(`UPDATE salons SET queue_note = ? WHERE id = ?`, note, c.Salon.ID); err != nil {
			a.dashServerError(w, r, c, err)
			return
		}
		saved = "hinweis"
	}
	http.Redirect(w, r, "/dashboard/warteschlange?gespeichert="+saved, http.StatusSeeOther)
}

// queueDashAdd reiht jemanden am Tresen ein (ohne Tagesdeckel, ohne Mail).
func (a *app) queueDashAdd(w http.ResponseWriter, r *http.Request, c *dctx) {
	if !a.queueGate(w, r, c) {
		return
	}
	lang := a.langFor(r)
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	if err := r.ParseForm(); err != nil {
		a.dashError(w, r, c, http.StatusBadRequest, tr(lang, "Das Formular war unvollständig."))
		return
	}
	name, ok := queueName(r.PostFormValue("name"))
	if !ok {
		a.dashError(w, r, c, http.StatusBadRequest, tr(lang, "Bitte gib einen Namen an (höchstens 60 Zeichen, eine Zeile)."))
		return
	}
	serviceID, ok1 := a.queueRef("services", c.Salon.ID, r.PostFormValue("service"))
	staffID, ok2 := a.queueRef("staff", c.Salon.ID, r.PostFormValue("staff"))
	if !ok1 || !ok2 {
		a.dashError(w, r, c, http.StatusBadRequest, tr(lang, "Diese Leistung oder diesen Mitarbeiter gibt es so nicht mehr. Bitte wähl neu."))
		return
	}
	nr, _, err := a.queueAdd(r.Context(), c.Salon, time.Now(), name, "", serviceID, staffID, 0)
	if err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	http.Redirect(w, r, "/dashboard/warteschlange?gespeichert=neu&nr="+strconv.Itoa(nr), http.StatusSeeOther)
}

// queueActions: Ziel-Status und erlaubte Ausgangs-Status je Aktion. Die
// from-Listen sind Konstanten und landen so im SQL.
var queueActions = map[string]struct{ to, from string }{
	"aufrufen":  {"called", "'waiting'"},
	"bedienen":  {"serving", "'waiting', 'called'"},
	"fertig":    {"done", "'waiting', 'called', 'serving'"},
	"nichtda":   {"noshow", "'waiting', 'called'"},
	"entfernen": {"left", "'waiting', 'called', 'serving'"},
}

func (a *app) queueAction(w http.ResponseWriter, r *http.Request, c *dctx) {
	if !a.queueGate(w, r, c) {
		return
	}
	act, ok := queueActions[r.PathValue("aktion")]
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if !ok || err != nil {
		a.dashError(w, r, c, http.StatusNotFound, tr(a.langFor(r), "Diese Aktion gibt es nicht."))
		return
	}
	now := time.Now().Unix()
	var e queueEntry
	var notified sql.NullInt64
	err = a.db.QueryRow(`UPDATE queue_entries SET status = ?1,
			called_at = CASE WHEN ?1 = 'called' THEN ?2 ELSE called_at END,
			done_at = CASE WHEN ?1 IN ('done', 'noshow', 'left') THEN ?2 ELSE done_at END
		WHERE id = ?3 AND salon_id = ?4 AND status IN (`+act.from+`)
		RETURNING number, name, COALESCE(email, ''), token, notified_at`, act.to, now, id, c.Salon.ID).
		Scan(&e.Number, &e.Name, &e.Email, &e.Token, &notified)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		// fremder Eintrag oder Stand schon überholt (anderer Tresen-PC): nur neu anzeigen
	case err != nil:
		a.dashServerError(w, r, c, err)
		return
	case act.to == "called" && e.Email != "" && !notified.Valid:
		if _, err := a.db.Exec(`UPDATE queue_entries SET notified_at = ? WHERE id = ? AND salon_id = ?`, now, id, c.Salon.ID); err != nil {
			a.dashServerError(w, r, c, err)
			return
		}
		go a.queueMail(c.Salon, e)
	}
	if isHX(r) {
		r.URL.Path = "/dashboard/warteschlange/liste"
		r.Method = http.MethodGet
		a.queueDash(w, r, c)
		return
	}
	http.Redirect(w, r, "/dashboard/warteschlange", http.StatusSeeOther)
}

// queueMail: "Gleich bist du dran" an den Kunden, in der Sprache des Salons.
// HTML-Mail, wenn das Template mail-queue-called-* da ist, sonst Text.
func (a *app) queueMail(s *salon, e queueEntry) {
	status := a.bookingURL(s, "/warteschlange/"+e.Token)
	var err error
	if a.tpl != nil && a.tpl.Lookup("mail-queue-called-subject") != nil {
		err = a.mailT(e.Email, s.Lang, "queue-called", map[string]any{
			"Name": e.Name, "Number": e.Number, "SalonName": s.Name, "StatusURL": status})
	} else {
		body := trf(s.Lang, "Hallo %s,\n\ngleich bist du bei %s dran (Nummer %d). Bitte komm jetzt zum Empfang.\n\nDein Platz in der Warteschlange:\n%s\n\nBis gleich!\n%s\n",
			e.Name, s.Name, e.Number, status, s.Name)
		err = a.mail.send(e.Email, trf(s.Lang, "Gleich bist du dran: %s", s.Name), body)
	}
	if err != nil {
		log.Printf("Warteschlange: Mail an %s: %v", e.Email, err)
	}
}

// queueQR: Druckseite mit großem QR-Code der Einreih-Adresse.
func (a *app) queueQR(w http.ResponseWriter, r *http.Request, c *dctx) {
	if !a.queueGate(w, r, c) {
		return
	}
	lang := a.langFor(r)
	var enabled bool
	if err := a.db.QueryRow(`SELECT queue_enabled FROM salons WHERE id = ?`, c.Salon.ID).Scan(&enabled); err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	join := a.bookingURL(c.Salon, "/warteschlange")
	d := a.dashPage(c, r, "warteschlange", tr(lang, "QR-Code zum Anstellen"))
	d["Enabled"], d["URL"] = enabled, join
	d["ShortURL"] = strings.TrimPrefix(strings.TrimPrefix(join, "https://"), "http://")
	d["QR"] = qrSVG(join, tr(lang, "QR-Code zum Anstellen"))
	a.render(w, http.StatusOK, "queue-qr", d)
}

// queueDisplay: Vollbild-Anzeige für einen Bildschirm im Laden, unter /stand nur der Inhalt (Polling).
func (a *app) queueDisplay(w http.ResponseWriter, r *http.Request, c *dctx) {
	if !a.queueGate(w, r, c) {
		return
	}
	st, err := a.queueLoad(c.Salon, time.Now())
	if err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	lang := a.langFor(r)
	d := a.dashPage(c, r, "warteschlange", tr(lang, "Anzeige der Warteschlange"))
	var called, serving, waiting []int
	for _, e := range st.Entries {
		switch e.Status {
		case "called":
			called = append(called, e.Number)
		case "serving":
			serving = append(serving, e.Number)
		case "waiting":
			waiting = append(waiting, e.Number)
		}
	}
	const shown = 12
	if len(waiting) > shown {
		d["More"] = len(waiting) - shown
		waiting = waiting[:shown]
	}
	d["Called"], d["Serving"], d["Waiting"] = called, serving, waiting
	d["CanJoin"] = st.Enabled && st.Open
	if strings.HasSuffix(r.URL.Path, "/stand") {
		w.Header().Set("Cache-Control", "no-store")
		a.render(w, http.StatusOK, "queue-anzeige-stand", d)
		return
	}
	join := a.bookingURL(c.Salon, "/warteschlange")
	d["ShortURL"] = strings.TrimPrefix(strings.TrimPrefix(join, "https://"), "http://")
	d["QR"] = qrSVG(join, tr(lang, "QR-Code zum Anstellen"))
	a.render(w, http.StatusOK, "queue-anzeige", d)
}

// --- Öffentlich (Salon-Subdomain) ---

// queuePublicSalon: Salon zur Subdomain; ohne Warteschlangen-Recht (Gratis) 404.
func (a *app) queuePublicSalon(w http.ResponseWriter, r *http.Request) *salon {
	s := a.salonFor(w, r)
	if s == nil {
		return nil
	}
	if !a.queueAllowed(s) {
		a.fail(w, s, http.StatusNotFound, tr(s.Lang, "Diese Seite gibt es nicht."))
		return nil
	}
	return s
}

func (a *app) queueJoinPage(w http.ResponseWriter, r *http.Request) {
	s := a.queuePublicSalon(w, r)
	if s == nil {
		return
	}
	// Schon eingereiht (Cookie vom selben Handy)? Dann gleich zur Statusseite.
	if ck, err := r.Cookie(queueCookie); err == nil {
		var status, day string
		today := time.Now().In(s.Loc).Format(time.DateOnly)
		if a.db.QueryRow(`SELECT status, day FROM queue_entries WHERE token = ? AND salon_id = ?`, ck.Value, s.ID).Scan(&status, &day) == nil &&
			day == today && (status == "waiting" || status == "called" || status == "serving") {
			http.Redirect(w, r, "/warteschlange/"+url.PathEscape(ck.Value), http.StatusSeeOther)
			return
		}
	}
	a.queueJoinRender(w, s, http.StatusOK, nil, "")
}

func (a *app) queueJoinRender(w http.ResponseWriter, s *salon, status int, form url.Values, msg string) {
	st, err := a.queueLoad(s, time.Now())
	if err != nil {
		a.serverError(w, s, err)
		return
	}
	services, err := a.services(s.ID)
	if err != nil {
		a.serverError(w, s, err)
		return
	}
	d := a.page(s, tr(s.Lang, "Warteschlange")+" - "+s.Name)
	d["Q"], d["Services"], d["Form"], d["Error"] = st, services, form, msg
	d["CanJoin"] = st.Enabled && st.Open
	d["Waiting"] = st.waiting()
	a.render(w, status, "queue-join", d)
}

func (a *app) queueJoin(w http.ResponseWriter, r *http.Request) {
	s := a.queuePublicSalon(w, r)
	if s == nil {
		return
	}
	if !sameOrigin(r) {
		a.fail(w, s, http.StatusForbidden, tr(s.Lang, "Ungültige Herkunft der Anfrage. Bitte lade die Seite neu."))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	if err := r.ParseForm(); err != nil {
		a.fail(w, s, http.StatusBadRequest, tr(s.Lang, "Das Formular war unvollständig."))
		return
	}
	if !queueJoinLimit.allow(clientKey(r) + "|" + strconv.FormatInt(s.ID, 10)) {
		a.queueJoinRender(w, s, http.StatusTooManyRequests, r.PostForm,
			tr(s.Lang, "Von deinem Anschluss kamen gerade sehr viele Anmeldungen. Bitte melde dich am Empfang."))
		return
	}
	if r.PostFormValue("firma") != "" {
		// Honeypot: Bots landen wieder auf dem Formular, gespeichert wird nichts.
		http.Redirect(w, r, "/warteschlange", http.StatusSeeOther)
		return
	}
	name, ok := queueName(r.PostFormValue("name"))
	email := strings.TrimSpace(r.PostFormValue("email"))
	serviceID, okService := a.queueRef("services", s.ID, r.PostFormValue("service"))
	msg := ""
	switch {
	case !ok:
		msg = "Bitte gib deinen Vornamen an (höchstens 60 Zeichen)."
	case email != "":
		addr, err := mail.ParseAddress(email)
		if err != nil || len(addr.Address) > 200 {
			msg = "Bitte gib eine gültige E-Mail-Adresse an oder lass das Feld leer."
		} else {
			email = strings.ToLower(addr.Address)
		}
	}
	if msg == "" && !okService {
		msg = "Diese Leistung gibt es nicht mehr. Bitte wähl neu."
	}
	if msg != "" {
		a.queueJoinRender(w, s, http.StatusBadRequest, r.PostForm, tr(s.Lang, msg))
		return
	}
	now := time.Now()
	st, err := a.queueLoad(s, now)
	if err != nil {
		a.serverError(w, s, err)
		return
	}
	if !st.Enabled || !st.Open {
		a.queueJoinRender(w, s, http.StatusConflict, r.PostForm, "") // Seite erklärt, dass geschlossen ist
		return
	}
	_, token, err := a.queueAdd(r.Context(), s, now, name, email, serviceID, 0, queueMaxPerDay)
	if errors.Is(err, errQueueFull) {
		a.queueJoinRender(w, s, http.StatusConflict, r.PostForm, tr(s.Lang, "Für heute ist die Online-Warteschlange voll. Bitte melde dich am Empfang."))
		return
	} else if err != nil {
		a.serverError(w, s, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: queueCookie, Value: token, Path: "/warteschlange", MaxAge: 12 * 3600,
		HttpOnly: true, Secure: secureRequest(r), SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, "/warteschlange/"+token, http.StatusSeeOther)
}

// queueStatusData sucht den Eintrag (nur heute, nur dieser Salon); nil = gibt es nicht.
func (a *app) queueStatusData(s *salon, token string) (map[string]any, error) {
	st, err := a.queueLoad(s, time.Now())
	if err != nil {
		return nil, err
	}
	for _, e := range st.Entries {
		if e.Token != token {
			continue
		}
		d := a.page(s, trf(s.Lang, "Nummer %d", e.Number)+" - "+s.Name)
		d["Q"], d["E"] = st, e
		d["Active"] = e.Status == "waiting" || e.Status == "called"
		d["Poll"] = e.Status == "waiting" || e.Status == "called" || e.Status == "serving"
		return d, nil
	}
	return nil, nil
}

func (a *app) queueStatus(w http.ResponseWriter, r *http.Request) {
	s := a.queuePublicSalon(w, r)
	if s == nil {
		return
	}
	if !queueReadLimit.allow(clientKey(r)) {
		http.Error(w, tr(s.Lang, "Zu viele Anfragen. Bitte warte kurz."), http.StatusTooManyRequests)
		return
	}
	d, err := a.queueStatusData(s, r.PathValue("token"))
	if err != nil {
		a.serverError(w, s, err)
		return
	}
	if d == nil {
		a.fail(w, s, http.StatusNotFound, tr(s.Lang, "Diesen Platz in der Warteschlange gibt es nicht (mehr)."))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Robots-Tag", "noindex")
	name := "queue-status"
	if strings.HasSuffix(r.URL.Path, "/stand") {
		name = "queue-stand"
	}
	a.render(w, http.StatusOK, name, d)
}

// queueLeave: "Ich gehe doch nicht".
func (a *app) queueLeave(w http.ResponseWriter, r *http.Request) {
	s := a.queuePublicSalon(w, r)
	if s == nil {
		return
	}
	if !sameOrigin(r) {
		a.fail(w, s, http.StatusForbidden, tr(s.Lang, "Ungültige Herkunft der Anfrage. Bitte lade die Seite neu."))
		return
	}
	token := r.PathValue("token")
	if _, err := a.db.Exec(`UPDATE queue_entries SET status = 'left', done_at = ? WHERE token = ? AND salon_id = ? AND status IN ('waiting', 'called')`,
		time.Now().Unix(), token, s.ID); err != nil {
		a.serverError(w, s, err)
		return
	}
	if isHX(r) {
		r.URL.Path = "/warteschlange/" + token + "/stand"
		a.queueStatus(w, r)
		return
	}
	http.Redirect(w, r, "/warteschlange/"+url.PathEscape(token), http.StatusSeeOther)
}

// --- QR-Code ---
//
// Kleiner QR-Encoder nach ISO/IEC 18004: Byte-Modus, Fehlerkorrektur M,
// Version 1 bis 10 (bis 213 Byte). ponytail: reicht für die Einreih-Adresse,
// spart eine Abhängigkeit (rsc.io/qr wäre BSD und klein, aber ein weiteres Modul).

// qrM[v] = EC-Bytes je Block, Blöcke Gruppe 1, Daten-Bytes je Block Gruppe 1,
// Blöcke Gruppe 2 (je ein Daten-Byte mehr). Stufe M, Tabelle 9 der Norm.
var qrM = [11][4]int{{}, {10, 1, 16, 0}, {16, 1, 28, 0}, {26, 1, 44, 0}, {18, 2, 32, 0}, {24, 2, 43, 0},
	{16, 4, 27, 0}, {18, 4, 31, 0}, {22, 2, 38, 2}, {22, 3, 36, 2}, {26, 4, 43, 1}}

var qrAlign = [11][]int{nil, nil, {6, 18}, {6, 22}, {6, 26}, {6, 30}, {6, 34}, {6, 22, 38}, {6, 24, 42}, {6, 26, 46}, {6, 28, 50}}

type qrCode struct {
	size     int
	dark, fn [][]bool // [y][x]; fn = Funktionsmodul (nicht maskieren)
}

func (q *qrCode) set(x, y int, d bool) { q.dark[y][x], q.fn[y][x] = d, true }

func qrEncode(data []byte) (*qrCode, error) {
	ver, countBits := 0, 8
	for v := 1; v <= 10; v++ {
		b := qrM[v]
		cb := 8
		if v >= 10 {
			cb = 16
		}
		if 4+cb+8*len(data) <= (b[1]*b[2]+b[3]*(b[2]+1))*8 {
			ver, countBits = v, cb
			break
		}
	}
	if ver == 0 {
		return nil, errors.New("qr: Text zu lang")
	}
	ec, n1, d1, n2 := qrM[ver][0], qrM[ver][1], qrM[ver][2], qrM[ver][3]
	dataLen := n1*d1 + n2*(d1+1)

	// Datenbits: Modus 0100, Länge, Bytes, Abschluss, Füllbytes.
	var bits []bool
	put := func(v, n int) {
		for i := n - 1; i >= 0; i-- {
			bits = append(bits, v>>i&1 == 1)
		}
	}
	put(4, 4)
	put(len(data), countBits)
	for _, c := range data {
		put(int(c), 8)
	}
	put(0, min(4, dataLen*8-len(bits)))
	put(0, (8-len(bits)%8)%8)
	for pad := 0xEC; len(bits) < dataLen*8; pad ^= 0xEC ^ 0x11 {
		put(pad, 8)
	}
	cw := make([]byte, dataLen)
	for i, b := range bits {
		if b {
			cw[i/8] |= 0x80 >> (i % 8)
		}
	}

	// Blöcke mit Reed-Solomon, dann verschränkt.
	div := rsDivisor(ec)
	var blocks, ecs [][]byte
	for i, off := 0, 0; i < n1+n2; i++ {
		n := d1
		if i >= n1 {
			n++
		}
		blocks = append(blocks, cw[off:off+n])
		ecs = append(ecs, rsRemainder(cw[off:off+n], div))
		off += n
	}
	var out []byte
	for i := 0; i <= d1; i++ {
		for _, blk := range blocks {
			if i < len(blk) {
				out = append(out, blk[i])
			}
		}
	}
	for i := 0; i < ec; i++ {
		for _, e := range ecs {
			out = append(out, e[i])
		}
	}

	size := 17 + 4*ver
	q := &qrCode{size: size, dark: make([][]bool, size), fn: make([][]bool, size)}
	for i := range q.dark {
		q.dark[i], q.fn[i] = make([]bool, size), make([]bool, size)
	}
	for i := 0; i < size; i++ { // Taktlinien
		q.set(6, i, i%2 == 0)
		q.set(i, 6, i%2 == 0)
	}
	for _, c := range [][2]int{{3, 3}, {size - 4, 3}, {3, size - 4}} { // Suchmuster mit Rand
		for dy := -4; dy <= 4; dy++ {
			for dx := -4; dx <= 4; dx++ {
				x, y, d := c[0]+dx, c[1]+dy, max(qrAbs(dx), qrAbs(dy))
				if x >= 0 && x < size && y >= 0 && y < size {
					q.set(x, y, d != 2 && d != 4)
				}
			}
		}
	}
	al := qrAlign[ver]
	for i, ax := range al { // Ausrichtungsmuster, nicht auf den Suchmustern
		for j, ay := range al {
			if i == 0 && j == 0 || i == 0 && j == len(al)-1 || i == len(al)-1 && j == 0 {
				continue
			}
			for dy := -2; dy <= 2; dy++ {
				for dx := -2; dx <= 2; dx++ {
					q.set(ax+dx, ay+dy, max(qrAbs(dx), qrAbs(dy)) != 1)
				}
			}
		}
	}
	q.format(0) // reserviert die Formatbereiche
	if ver >= 7 {
		rem := ver
		for i := 0; i < 12; i++ {
			rem = rem<<1 ^ (rem>>11)*0x1F25
		}
		vb := ver<<12 | rem
		for i := 0; i < 18; i++ {
			a, b := size-11+i%3, i/3
			q.set(a, b, vb>>i&1 == 1)
			q.set(b, a, vb>>i&1 == 1)
		}
	}
	// Daten im Zickzack von rechts unten, Spaltenpaare, Spalte 6 auslassen.
	i := 0
	for right := size - 1; right >= 1; right -= 2 {
		if right == 6 {
			right = 5
		}
		for vert := 0; vert < size; vert++ {
			for j := 0; j < 2; j++ {
				x, y := right-j, vert
				if (right+1)&2 == 0 {
					y = size - 1 - vert
				}
				if !q.fn[y][x] && i < len(out)*8 {
					q.dark[y][x] = out[i/8]>>(7-i%8)&1 == 1
					i++
				}
			}
		}
	}
	best, bestScore := 0, -1
	for m := 0; m < 8; m++ {
		q.mask(m)
		q.format(m)
		if s := q.penalty(); bestScore < 0 || s < bestScore {
			best, bestScore = m, s
		}
		q.mask(m) // XOR: zweimal = rückgängig
	}
	q.mask(best)
	q.format(best)
	return q, nil
}

func qrAbs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// format setzt die 15 Formatbits (Stufe M = 00, Maske) doppelt und das dunkle Modul.
func (q *qrCode) format(mask int) {
	bits := qrFormatBits(mask)
	bit := func(i int) bool { return bits>>i&1 == 1 }
	for i := 0; i <= 5; i++ {
		q.set(8, i, bit(i))
	}
	q.set(8, 7, bit(6))
	q.set(8, 8, bit(7))
	q.set(7, 8, bit(8))
	for i := 9; i < 15; i++ {
		q.set(14-i, 8, bit(i))
	}
	for i := 0; i < 8; i++ {
		q.set(q.size-1-i, 8, bit(i))
	}
	for i := 8; i < 15; i++ {
		q.set(8, q.size-15+i, bit(i))
	}
	q.set(8, q.size-8, true)
}

// qrFormatBits: BCH(15,5) über Stufe M (Bits 00) und Maske, XOR 0x5412.
func qrFormatBits(mask int) int {
	rem := mask
	for i := 0; i < 10; i++ {
		rem = rem<<1 ^ (rem>>9)*0x537
	}
	return (mask<<10 | rem) ^ 0x5412
}

func (q *qrCode) mask(m int) {
	for y := 0; y < q.size; y++ {
		for x := 0; x < q.size; x++ {
			if q.fn[y][x] {
				continue
			}
			var inv bool
			switch m {
			case 0:
				inv = (x+y)%2 == 0
			case 1:
				inv = y%2 == 0
			case 2:
				inv = x%3 == 0
			case 3:
				inv = (x+y)%3 == 0
			case 4:
				inv = (x/3+y/2)%2 == 0
			case 5:
				inv = x*y%2+x*y%3 == 0
			case 6:
				inv = (x*y%2+x*y%3)%2 == 0
			case 7:
				inv = ((x+y)%2+x*y%3)%2 == 0
			}
			if inv {
				q.dark[y][x] = !q.dark[y][x]
			}
		}
	}
}

// penalty: die vier Strafregeln der Norm (Läufe, 2x2-Blöcke, suchmusterähnliche
// Folgen, Dunkelanteil), damit die gewählte Maske gut lesbar ist.
func (q *qrCode) penalty() int {
	n, p := q.size, 0
	at := func(i, j int, col bool) bool {
		if col {
			return q.dark[i][j]
		}
		return q.dark[j][i]
	}
	finder := [2][11]bool{
		{true, false, true, true, true, false, true, false, false, false, false},
		{false, false, false, false, true, false, true, true, true, false, true},
	}
	for _, col := range []bool{false, true} {
		for j := 0; j < n; j++ {
			run := 0
			for i := 0; i < n; i++ {
				if i > 0 && at(i, j, col) == at(i-1, j, col) {
					run++
				} else {
					run = 1
				}
				if run == 5 {
					p += 3
				} else if run > 5 {
					p++
				}
			}
			for i := 0; i+11 <= n; i++ {
				for _, f := range finder {
					k := 0
					for k < 11 && at(i+k, j, col) == f[k] {
						k++
					}
					if k == 11 {
						p += 40
					}
				}
			}
		}
	}
	dark := 0
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			c := q.dark[y][x]
			if c {
				dark++
			}
			if x+1 < n && y+1 < n && c == q.dark[y][x+1] && c == q.dark[y+1][x] && c == q.dark[y+1][x+1] {
				p += 3
			}
		}
	}
	return p + qrAbs(dark*20-n*n*10)/(n*n)*10
}

// rsMul multipliziert in GF(256) mit dem Polynom 0x11D.
func rsMul(x, y byte) byte {
	z := 0
	for i := 7; i >= 0; i-- {
		z = z<<1 ^ (z>>7)*0x11D
		z ^= int(y>>i&1) * int(x)
	}
	return byte(z)
}

func rsDivisor(degree int) []byte {
	res := make([]byte, degree)
	res[degree-1] = 1
	root := byte(1)
	for i := 0; i < degree; i++ {
		for j := range res {
			res[j] = rsMul(res[j], root)
			if j+1 < len(res) {
				res[j] ^= res[j+1]
			}
		}
		root = rsMul(root, 2)
	}
	return res
}

func rsRemainder(data, div []byte) []byte {
	res := make([]byte, len(div))
	for _, b := range data {
		f := b ^ res[0]
		copy(res, res[1:])
		res[len(res)-1] = 0
		for i := range res {
			res[i] ^= rsMul(div[i], f)
		}
	}
	return res
}

// qrSVG liefert den QR-Code als SVG (Nachtblau auf Weiß, 4 Module Rand) oder
// "" bei zu langem Text. Sicher als template.HTML: nur Zahlen und der
// maskierte label.
func qrSVG(text, label string) template.HTML {
	q, err := qrEncode([]byte(text))
	if err != nil {
		log.Printf("QR-Code für %q: %v", text, err)
		return ""
	}
	var b strings.Builder
	n := q.size + 8
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" shape-rendering="crispEdges" role="img" aria-label="%s"><rect width="%d" height="%d" fill="#fff"/><path fill="#13294b" d="`,
		n, n, template.HTMLEscapeString(label), n, n)
	for y := 0; y < q.size; y++ {
		for x := 0; x < q.size; x++ {
			if !q.dark[y][x] {
				continue
			}
			run := 1
			for x+run < q.size && q.dark[y][x+run] {
				run++
			}
			fmt.Fprintf(&b, "M%d %dh%dv1h-%dz", x+4, y+4, run, run)
			x += run - 1
		}
	}
	b.WriteString(`"/></svg>`)
	return template.HTML(b.String())
}
