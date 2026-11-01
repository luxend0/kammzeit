package main

// Dashboard-Startseite mit Kalender und Terminen: /dashboard, /dashboard/termine/...

import (
	"cmp"
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/mail"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

func (a *app) routesCalendar(mux *http.ServeMux) {
	mux.HandleFunc("GET /dashboard", a.dash(a.kalIndex))
	mux.HandleFunc("GET /dashboard/termine/neu", a.dash(a.kalNew))
	mux.HandleFunc("POST /dashboard/termine/neu", a.dash(a.kalCreate))
	mux.HandleFunc("GET /dashboard/termine/kundensuche", a.dash(a.kalSearch))
	mux.HandleFunc("GET /dashboard/termine/zeiten", a.dash(a.kalTimesFrag))
	mux.HandleFunc("GET /dashboard/termine/{id}", a.dash(a.kalDetail))
	mux.HandleFunc("GET /dashboard/termine/{id}/verschieben", a.dash(a.kalMoveForm))
	mux.HandleFunc("POST /dashboard/termine/{id}/verschieben", a.dash(a.kalAction(a.kalMove)))
	mux.HandleFunc("POST /dashboard/termine/{id}/absagen", a.dash(a.kalAction(a.kalCancel)))
	mux.HandleFunc("POST /dashboard/termine/{id}/nicht-erschienen", a.dash(a.kalAction(a.kalNoShow)))
	mux.HandleFunc("POST /dashboard/termine/{id}/notiz", a.dash(a.kalAction(a.kalNote)))
}

// Höhe einer Stunde im Tagesraster, muss zu background-size in .kal-lines passen.
const kalHourPx = 72

var kalMonths = [...]string{"Januar", "Februar", "März", "April", "Mai", "Juni", "Juli", "August", "September", "Oktober", "November", "Dezember"}

func kalDate(t time.Time) string {
	return fmt.Sprintf("%s, %d.%d.%d", weekdaysDE[t.Weekday()], t.Day(), t.Month(), t.Year())
}

// kalShortDay: "Mo 2.11."
func kalShortDay(t time.Time) string {
	return fmt.Sprintf("%s %d.%d.", weekdaysDE[t.Weekday()][:2], t.Day(), t.Month())
}

// kalParseDay liest YYYY-MM-DD als Mitternacht in der Salon-Zeitzone, sonst heute.
func kalParseDay(v string, loc *time.Location, now time.Time) time.Time {
	if d, err := time.ParseInLocation(time.DateOnly, v, loc); err == nil {
		return d
	}
	y, m, d := now.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, loc)
}

// kalMin = Minuten ab Mitternacht (Wanduhr) am Tag day, begrenzt auf [0, 1440].
func kalMin(t, day time.Time) int {
	t = t.In(day.Location())
	if t.Before(day) {
		return 0
	}
	if !t.Before(day.AddDate(0, 0, 1)) {
		return 1440
	}
	return t.Hour()*60 + t.Minute()
}

// --- Tages- und Wochenansicht ---

type kalAppt struct {
	ID, StaffID              int64
	Start, End               time.Time
	Time, Status             string
	Customer, Service, Staff string
	Color                    int
}

// kalColorSQL: feste Farbe je Mitarbeiter (0-4, CSS-Klasse kal-c0 bis kal-c4),
// reihum nach Anlage-Reihenfolge im Salon. Bleibt beim Umbenennen und Umsortieren gleich.
const kalColorSQL = `(SELECT COUNT(*) FROM staff s2 WHERE s2.salon_id = st.salon_id AND s2.id < st.id) % 5`

// kalAppts liefert alle Termine des Salons, die [from, to) berühren, in Salon-Zeit.
func (a *app) kalAppts(s *salon, from, to time.Time) ([]kalAppt, error) {
	rows, err := a.db.Query(`SELECT a.id, a.staff_id, a.starts_at, a.ends_at, a.status, c.name, sv.name, st.name, `+kalColorSQL+`
		FROM appointments a
		JOIN customers c ON c.id = a.customer_id
		JOIN services sv ON sv.id = a.service_id
		JOIN staff st ON st.id = a.staff_id
		WHERE a.salon_id = ? AND a.starts_at < ? AND a.ends_at > ?
		ORDER BY a.starts_at, a.status <> 'booked', a.id`, s.ID, to.Unix(), from.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []kalAppt
	for rows.Next() {
		var ap kalAppt
		var st, en int64
		if err := rows.Scan(&ap.ID, &ap.StaffID, &st, &en, &ap.Status, &ap.Customer, &ap.Service, &ap.Staff, &ap.Color); err != nil {
			return nil, err
		}
		ap.Start, ap.End = time.Unix(st, 0).In(s.Loc), time.Unix(en, 0).In(s.Loc)
		ap.Time = ap.Start.Format("15:04") + "-" + ap.End.Format("15:04")
		out = append(out, ap)
	}
	return out, rows.Err()
}

func (a *app) kalIndex(w http.ResponseWriter, r *http.Request, c *dctx) {
	s, now, q := c.Salon, time.Now(), r.URL.Query()
	day := kalParseDay(q.Get("tag"), s.Loc, now)
	week := q.Get("ansicht") == "woche"
	d := a.dashPage(c, r, "kalender", "Kalender")
	d["Heading"] = "" // eigene Überschrift unter den Kennzahlen
	if id, _ := strconv.ParseInt(q.Get("termin"), 10, 64); id > 0 && d["Saved"] == true {
		if t, err := a.kalLoad(s, id); err == nil { // fremde ID: keine Meldung mit Daten
			d["SavedMsg"] = kalSavedMsg(q.Get("gespeichert"), t)
		}
	}
	d["Day"], d["Week"] = day.Format(time.DateOnly), week
	if err := a.kalHead(d, s, now); err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	nav := func(t time.Time) string {
		if week {
			return "/dashboard?ansicht=woche&tag=" + t.Format(time.DateOnly)
		}
		return "/dashboard?tag=" + t.Format(time.DateOnly)
	}
	step := 1
	if week {
		step = 7
	}
	d["PrevHref"], d["TodayHref"], d["NextHref"] = nav(day.AddDate(0, 0, -step)), nav(kalParseDay("", s.Loc, now)), nav(day.AddDate(0, 0, step))
	d["DayHref"], d["WeekHref"] = "/dashboard?tag="+day.Format(time.DateOnly), "/dashboard?ansicht=woche&tag="+day.Format(time.DateOnly)

	tpl, err := "kal-tag", error(nil)
	if week {
		tpl, err = "kal-woche", a.kalWeek(d, s, day, now)
	} else {
		err = a.kalDay(d, s, day, now)
	}
	if err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	a.render(w, http.StatusOK, tpl, d)
}

type kalNext struct {
	ID             int64
	When, Customer string
}

// kalHead füllt die Kennzahlen (immer bezogen auf heute) und ob Team/Leistungen existieren.
func (a *app) kalHead(d map[string]any, s *salon, now time.Time) error {
	today := kalParseDay("", s.Loc, now)
	var count, staff, services int
	err := a.db.QueryRow(`SELECT
		(SELECT COUNT(*) FROM appointments WHERE salon_id = ?1 AND status <> 'cancelled' AND starts_at >= ?2 AND starts_at < ?3),
		(SELECT COUNT(*) FROM staff WHERE salon_id = ?1 AND active = 1),
		(SELECT COUNT(*) FROM services WHERE salon_id = ?1 AND active = 1)`,
		s.ID, today.Unix(), today.AddDate(0, 0, 1).Unix()).Scan(&count, &staff, &services)
	if err != nil {
		return err
	}
	d["TodayCount"], d["HasStaff"], d["HasServices"] = count, staff > 0, services > 0
	steps, err := a.kalSteps(s)
	if err != nil {
		return err
	}
	if steps != nil {
		done := 0
		for _, st := range steps {
			if st.Done {
				done++
			}
		}
		d["Steps"], d["StepsDone"] = steps, done
	}
	var n kalNext
	var start int64
	err = a.db.QueryRow(`SELECT a.id, a.starts_at, c.name FROM appointments a JOIN customers c ON c.id = a.customer_id
		WHERE a.salon_id = ? AND a.status = 'booked' AND a.starts_at >= ? ORDER BY a.starts_at LIMIT 1`,
		s.ID, now.Unix()).Scan(&n.ID, &start, &n.Customer)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	} else if err != nil {
		return err
	}
	t := time.Unix(start, 0).In(s.Loc)
	n.When = t.Format("15:04")
	if t.Format(time.DateOnly) != today.Format(time.DateOnly) {
		n.When = kalShortDay(t) + ", " + n.When
	}
	d["Next"] = &n
	return nil
}

type kalStep struct {
	Text, Href string
	Done       bool
}

// kalSteps liefert die Liste "Erste Schritte", bis die erste Online-Buchung da
// ist (danach nil). Erledigt wird ohne eigene Spalte aus den Daten abgeleitet:
// Leistungen = mindestens eine aktiv (die Registrierung legt Beispiele inaktiv an),
// Team = kein aktiver Mitarbeiter heißt mehr "Team" und einer hat Arbeitszeiten,
// Adresse = Adresse und Impressum eingetragen. Ob der Link geteilt wurde, sieht
// man erst an der ersten Buchung.
func (a *app) kalSteps(s *salon) ([]kalStep, error) {
	var online, services, team, hours bool
	err := a.db.QueryRow(`SELECT
		EXISTS(SELECT 1 FROM appointments WHERE salon_id = ?1 AND source = 'online'),
		EXISTS(SELECT 1 FROM services WHERE salon_id = ?1 AND active = 1),
		EXISTS(SELECT 1 FROM staff WHERE salon_id = ?1 AND active = 1 AND trim(name) = 'Team' COLLATE NOCASE),
		EXISTS(SELECT 1 FROM staff st JOIN working_hours wh ON wh.staff_id = st.id WHERE st.salon_id = ?1 AND st.active = 1)`,
		s.ID).Scan(&online, &services, &team, &hours)
	if err != nil || online {
		return nil, err
	}
	return []kalStep{
		{"Leistungen und Preise prüfen und auf aktiv stellen", "/dashboard/leistungen", services},
		{"Mitarbeiter benennen und Arbeitszeiten eintragen", "/dashboard/team", !team && hours},
		{"Adresse und Link zum Impressum eintragen", "/dashboard/einstellungen",
			strings.TrimSpace(s.Address) != "" && strings.TrimSpace(s.ImprintURL) != ""},
		{"Buchungsseite ansehen und den Link teilen", "/dashboard/einstellungen#teilen", false},
	}, nil
}

// kalBox ist ein Rechteck im Tagesraster, Angaben in Prozent der Spalte.
type kalBox struct {
	Top, Height, Left, Width float64
	ID                       int64
	Status, Time, Title      string
	Customer, Service        string
	Short, Half              bool // < 30 Min. einzeilig; 30-44 Min. am Handy zweizeilig
	s, e, lane               int  // Minuten ab Mitternacht, Spur bei Überlappung
}

type kalCol struct {
	ID          int64
	Name        string
	Inactive    bool
	Color       int
	Work, Appts []kalBox
}

type kalTick struct {
	Top, Height float64
	Time        string
}

// kalDay baut das Tagesraster: eine Spalte je Mitarbeiter, Zeitachse von der
// frühesten Arbeitszeit bis zum spätesten Ende (ohne Arbeitszeiten 8-20 Uhr),
// erweitert um Termine außerhalb.
func (a *app) kalDay(d map[string]any, s *salon, day, now time.Time) error {
	next := day.AddDate(0, 0, 1)
	// Aktive Mitarbeiter plus inaktive, die an diesem Tag Termine haben (sonst wären die unsichtbar).
	rows, err := a.db.Query(`SELECT st.id, st.name, st.active, `+kalColorSQL+` FROM staff st WHERE st.salon_id = ?1 AND (st.active = 1 OR st.id IN
		(SELECT staff_id FROM appointments WHERE salon_id = ?1 AND starts_at < ?2 AND ends_at > ?3)) ORDER BY st.sort, st.name, st.id`,
		s.ID, next.Unix(), day.Unix())
	if err != nil {
		return err
	}
	var cols []kalCol
	idx := map[int64]int{}
	for rows.Next() {
		var col kalCol
		var active bool
		if err := rows.Scan(&col.ID, &col.Name, &active, &col.Color); err != nil {
			rows.Close()
			return err
		}
		col.Inactive = !active
		idx[col.ID] = len(cols)
		cols = append(cols, col)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	rows, err = a.db.Query(`SELECT wh.staff_id, wh.start_min, wh.end_min FROM working_hours wh JOIN staff st ON st.id = wh.staff_id
		WHERE st.salon_id = ? AND wh.weekday = ?`, s.ID, int(day.Weekday()))
	if err != nil {
		return err
	}
	var work []kalBox // lane = Spaltenindex
	lo, hi := 1440, 0
	for rows.Next() {
		var id int64
		var b kalBox
		if err := rows.Scan(&id, &b.s, &b.e); err != nil {
			rows.Close()
			return err
		}
		i, ok := idx[id]
		if !ok {
			continue
		}
		b.lane = i
		work = append(work, b)
		lo, hi = min(lo, b.s), max(hi, b.e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if lo >= hi {
		lo, hi = 8*60, 20*60
	}

	appts, err := a.kalAppts(s, day, next)
	if err != nil {
		return err
	}
	var boxes []kalBox
	count := 0
	for _, ap := range appts {
		i, ok := idx[ap.StaffID]
		if !ok {
			continue
		}
		b := kalBox{ID: ap.ID, Status: ap.Status, Time: ap.Time, Customer: ap.Customer, Service: ap.Service,
			s: kalMin(ap.Start, day), e: kalMin(ap.End, day), lane: i}
		if b.e-b.s < 10 { // sehr kurze Termine bleiben anklickbar
			b.e = min(b.s+10, 1440)
			b.s = b.e - 10
		}
		b.Short, b.Half = b.e-b.s < 30, b.e-b.s >= 30 && b.e-b.s < 45
		b.Title = ap.Time + " " + ap.Customer + ", " + ap.Service
		switch ap.Status {
		case "cancelled":
			b.Title += " (abgesagt)"
		case "noshow":
			b.Title += " (nicht erschienen)"
		}
		if ap.Status != "cancelled" {
			count++
		}
		boxes = append(boxes, b)
		lo, hi = min(lo, b.s), max(hi, b.e)
	}
	lo, hi = lo/60*60, min(1440, (hi+59)/60*60)
	pct := func(m int) float64 { return float64(m-lo) * 100 / float64(hi-lo) }

	for _, b := range work {
		b.Top, b.Height = pct(b.s), pct(b.e)-pct(b.s)
		cols[b.lane].Work = append(cols[b.lane].Work, b)
	}
	for _, b := range boxes {
		b.Top, b.Height = pct(b.s), pct(b.e)-pct(b.s)
		cols[b.lane].Appts = append(cols[b.lane].Appts, b)
	}
	for i := range cols {
		kalLanes(cols[i].Appts)
	}
	var hours, slots []kalTick
	for m := lo; m < hi; m += 30 {
		label := fmt.Sprintf("%02d:%02d", m/60, m%60)
		if m%60 == 0 {
			hours = append(hours, kalTick{Top: pct(m), Time: label})
		}
		slots = append(slots, kalTick{Top: pct(m), Height: pct(m+30) - pct(m), Time: label})
	}
	if day.Equal(kalParseDay("", s.Loc, now)) {
		if m := kalMin(now, day); m >= lo && m < hi {
			d["NowTop"] = pct(m)
		}
	}
	d["Title"] = kalDate(day) + " - Kammzeit"
	d["DayTitle"] = fmt.Sprintf("%s, %d. %s %d", weekdaysDE[day.Weekday()], day.Day(), kalMonths[day.Month()-1], day.Year())
	d["DayCount"], d["Cols"], d["Hours"], d["Slots"] = count, cols, hours, slots
	d["Off"] = len(work) == 0 && len(boxes) == 0 // niemand arbeitet, kein Termin: Text statt leerem Raster
	d["HeightPx"] = (hi - lo) * kalHourPx / 60
	return nil
}

// kalLanes legt sich überlappende Termine (z. B. abgesagt und neu gebucht)
// nebeneinander. bs muss nach Beginn sortiert sein.
func kalLanes(bs []kalBox) {
	slices.SortStableFunc(bs, func(x, y kalBox) int { return x.s - y.s })
	var laneEnd []int
	first, clusterEnd := 0, -1
	done := func(to int) {
		for i := first; i < to; i++ {
			bs[i].Width = 100 / float64(len(laneEnd))
			bs[i].Left = float64(bs[i].lane) * bs[i].Width
		}
	}
	for i := range bs {
		if bs[i].s >= clusterEnd { // neue Gruppe ohne Überlappung mit der vorigen
			done(i)
			first, laneEnd = i, nil
		}
		lane := slices.IndexFunc(laneEnd, func(e int) bool { return e <= bs[i].s })
		if lane < 0 {
			lane = len(laneEnd)
			laneEnd = append(laneEnd, 0)
		}
		laneEnd[lane] = bs[i].e
		bs[i].lane = lane
		clusterEnd = max(clusterEnd, bs[i].e)
	}
	done(len(bs))
}

type kalWeekDay struct {
	Label, Href string
	Today       bool
	Appts       []kalAppt
}

// kalWeek listet die Termine Mo-So der Woche, in der day liegt.
func (a *app) kalWeek(d map[string]any, s *salon, day, now time.Time) error {
	mon := day.AddDate(0, 0, -((int(day.Weekday()) + 6) % 7))
	appts, err := a.kalAppts(s, mon, mon.AddDate(0, 0, 7))
	if err != nil {
		return err
	}
	today := kalParseDay("", s.Loc, now)
	days := make([]kalWeekDay, 7)
	byDate := map[string]int{}
	for i := range days {
		t := mon.AddDate(0, 0, i)
		days[i] = kalWeekDay{Label: kalDate(t), Href: "/dashboard?tag=" + t.Format(time.DateOnly), Today: t.Equal(today)}
		byDate[t.Format(time.DateOnly)] = i
	}
	for _, ap := range appts {
		if i, ok := byDate[ap.Start.Format(time.DateOnly)]; ok {
			days[i].Appts = append(days[i].Appts, ap)
		}
	}
	sun := mon.AddDate(0, 0, 6)
	_, kw := mon.ISOWeek()
	d["Title"] = fmt.Sprintf("KW %d - Kammzeit", kw)
	d["DayTitle"] = fmt.Sprintf("KW %d: %d.%d. bis %d.%d.%d", kw, mon.Day(), mon.Month(), sun.Day(), sun.Month(), sun.Year())
	d["Days"] = days
	return nil
}

// --- Termin-Details und Aktionen ---

type kalTermin struct {
	ID, CustomerID, StaffID, Price         int64
	Start, End                             time.Time
	When, Created, Status, Source, Note    string
	Customer, Email, Phone, Service, Staff string
	CustomerNotes                          string
	Tel                                    template.URL
	Minutes                                int
}

func (a *app) kalLoad(s *salon, id int64) (*kalTermin, error) {
	var t kalTermin
	var st, en, cr int64
	var email sql.NullString
	err := a.db.QueryRow(`SELECT a.id, a.starts_at, a.ends_at, a.created_at, a.status, a.source, a.note, a.customer_id, a.staff_id,
		c.name, c.email, c.phone, c.notes, sv.name, sv.price_cents, st.name
		FROM appointments a
		JOIN customers c ON c.id = a.customer_id
		JOIN services sv ON sv.id = a.service_id
		JOIN staff st ON st.id = a.staff_id
		WHERE a.id = ? AND a.salon_id = ?`, id, s.ID).Scan(&t.ID, &st, &en, &cr, &t.Status, &t.Source, &t.Note, &t.CustomerID, &t.StaffID,
		&t.Customer, &email, &t.Phone, &t.CustomerNotes, &t.Service, &t.Price, &t.Staff)
	if err != nil {
		return nil, err
	}
	t.Email = email.String
	t.Start, t.End = time.Unix(st, 0).In(s.Loc), time.Unix(en, 0).In(s.Loc)
	t.Minutes = int(t.End.Sub(t.Start).Minutes())
	t.When = kalDate(t.Start) + ", " + t.Start.Format("15:04") + "-" + t.End.Format("15:04") + " Uhr"
	t.Created = deDate(time.Unix(cr, 0).In(s.Loc))
	t.Tel = telURL(t.Phone)
	return &t, nil
}

// kalFind lädt den Termin aus {id}. Fremde oder unbekannte IDs ergeben 404.
func (a *app) kalFind(w http.ResponseWriter, r *http.Request, c *dctx) *kalTermin {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64) // ungültig -> 0 -> nicht gefunden
	t, err := a.kalLoad(c.Salon, id)
	if errors.Is(err, sql.ErrNoRows) {
		a.dashError(w, r, c, http.StatusNotFound, "Diesen Termin gibt es nicht.")
		return nil
	} else if err != nil {
		a.dashServerError(w, r, c, err)
		return nil
	}
	return t
}

// kalSavedMsg: Erfolgsmeldung zu einem festen Code aus ?gespeichert=. Namen und
// Zeiten kommen aus der Datenbank, nie Text aus der URL. Unbekannt -> "" (Layout: "Gespeichert.").
func kalSavedMsg(code string, t *kalTermin) string {
	at := kalShortDay(t.Start) + " um " + t.Start.Format("15:04")
	switch code {
	case "abgesagt":
		return "Termin abgesagt."
	case "abgesagt-mail":
		return "Termin abgesagt. " + t.Customer + " bekommt eine Mail."
	case "nicht-erschienen":
		return "Termin als nicht erschienen markiert."
	case "notiz":
		return "Notiz gespeichert."
	case "verschoben":
		return "Termin verschoben auf " + at + " bei " + t.Staff + "."
	case "verschoben-mail":
		return "Termin verschoben auf " + at + " bei " + t.Staff + ". " + t.Customer + " bekommt eine Mail."
	case "eingetragen":
		return "Termin für " + t.Customer + " am " + at + " eingetragen."
	case "eingetragen-mail":
		return "Termin für " + t.Customer + " am " + at + " eingetragen. Die Bestätigung geht per Mail raus."
	}
	return ""
}

func (a *app) kalDetail(w http.ResponseWriter, r *http.Request, c *dctx) {
	t := a.kalFind(w, r, c)
	if t == nil {
		return
	}
	d := a.dashPage(c, r, "kalender", t.Customer+", "+kalShortDay(t.Start)+", "+t.Start.Format("15:04"))
	d["T"], d["Future"] = t, t.Start.After(time.Now())
	d["BackHref"] = "/dashboard?tag=" + t.Start.Format(time.DateOnly)
	d["SavedMsg"] = kalSavedMsg(r.URL.Query().Get("gespeichert"), t)
	a.render(w, http.StatusOK, "kal-termin", d)
}

// kalAction: gemeinsamer Rahmen der Termin-Aktionen (Termin laden, Formular lesen).
func (a *app) kalAction(fn func(http.ResponseWriter, *http.Request, *dctx, *kalTermin)) func(http.ResponseWriter, *http.Request, *dctx) {
	return func(w http.ResponseWriter, r *http.Request, c *dctx) {
		t := a.kalFind(w, r, c)
		if t == nil {
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
		if err := r.ParseForm(); err != nil {
			a.dashError(w, r, c, http.StatusBadRequest, "Das Formular war unvollständig.")
			return
		}
		fn(w, r, c, t)
	}
}

func (a *app) kalSetStatus(w http.ResponseWriter, r *http.Request, c *dctx, t *kalTermin, status string) bool {
	if _, err := a.db.Exec(`UPDATE appointments SET status = ? WHERE id = ? AND salon_id = ?`, status, t.ID, c.Salon.ID); err != nil {
		a.dashServerError(w, r, c, err)
		return false
	}
	return true
}

func (a *app) kalCancel(w http.ResponseWriter, r *http.Request, c *dctx, t *kalTermin) {
	if t.Status != "booked" {
		a.dashError(w, r, c, http.StatusConflict, "Dieser Termin ist nicht mehr gebucht.")
		return
	}
	if !a.kalSetStatus(w, r, c, t, "cancelled") {
		return
	}
	code := "abgesagt"
	if r.PostFormValue("informieren") == "1" && t.Email != "" && t.Start.After(time.Now()) {
		s := c.Salon
		body := fmt.Sprintf("Hallo %s,\n\nleider müssen wir deinen Termin bei %s absagen:\n\n%s\n%s bei %s\n\nNeuen Termin buchen: %s\n",
			t.Customer, s.Name, deDate(t.Start), t.Service, t.Staff, a.link(s.Slug, "/"))
		if s.Phone != "" {
			body += "Fragen? Ruf uns an: " + s.Phone + "\n"
		}
		a.kalMail(t.Email, "Termin abgesagt: "+s.Name, body)
		code = "abgesagt-mail"
	}
	kalSaved(w, r, t.ID, code)
}

func (a *app) kalNoShow(w http.ResponseWriter, r *http.Request, c *dctx, t *kalTermin) {
	if t.Status != "booked" || t.Start.After(time.Now()) {
		a.dashError(w, r, c, http.StatusConflict, "Nicht erschienen geht nur bei gebuchten Terminen ab Terminbeginn.")
		return
	}
	if a.kalSetStatus(w, r, c, t, "noshow") {
		kalSaved(w, r, t.ID, "nicht-erschienen")
	}
}

func (a *app) kalNote(w http.ResponseWriter, r *http.Request, c *dctx, t *kalTermin) {
	note := strings.TrimSpace(r.PostFormValue("notiz"))
	if utf8.RuneCountInString(note) > 1000 {
		a.dashError(w, r, c, http.StatusBadRequest, "Die Notiz ist zu lang (höchstens 1000 Zeichen).")
		return
	}
	if _, err := a.db.Exec(`UPDATE appointments SET note = ? WHERE id = ? AND salon_id = ?`, note, t.ID, c.Salon.ID); err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	kalSaved(w, r, t.ID, "notiz")
}

func kalSaved(w http.ResponseWriter, r *http.Request, id int64, code string) {
	http.Redirect(w, r, fmt.Sprintf("/dashboard/termine/%d?gespeichert=%s", id, code), http.StatusSeeOther)
}

// kalMail verschickt im Hintergrund wie die Buchungsseite. Adresse wird vorher geprüft.
func (a *app) kalMail(to, subject, body string) {
	addr, err := mail.ParseAddress(to)
	if err != nil {
		log.Printf("Mail an %q: %v", to, err)
		return
	}
	go func() {
		if err := a.mail.send(addr.Address, subject, body); err != nil {
			log.Printf("Mail an %s: %v", addr.Address, err)
		}
	}()
}

// --- Freie Zeiten als Vorschläge unter dem Feld Uhrzeit ---

type kalSlot struct {
	Time, ID string
	Selected bool
}

// kalZeit = Feld Uhrzeit mit Vorschlägen (Fragment kal-zeit).
type kalZeit struct {
	Zeit, Staff, Hint string
	Slots             []kalSlot
	Invalid           bool // Fehler steht am Feld (aria-invalid)
}

// kalTimes berechnet die Vorschläge: freie Startzeiten des Mitarbeiters am Tag
// für die Dauer dur, laut Arbeitszeiten und gebuchten Terminen (freeSlotsFor).
// Fehlende oder fremde Angaben ergeben nur einen Hinweis.
// ponytail: beim Verschieben zählt die bisherige Zeit des Termins als belegt.
func (a *app) kalTimes(s *salon, staffID string, dur time.Duration, day, zeit string, now time.Time) (kalZeit, error) {
	z := kalZeit{Zeit: zeit}
	d, err := time.ParseInLocation(time.DateOnly, day, s.Loc)
	if dur <= 0 || err != nil {
		z.Hint = "Wähl Leistung und Datum, dann stehen hier die freien Zeiten."
		return z, nil
	}
	var id int64
	err = a.db.QueryRow(`SELECT id, name FROM staff WHERE id = ? AND salon_id = ?`, staffID, s.ID).Scan(&id, &z.Staff)
	if errors.Is(err, sql.ErrNoRows) {
		z.Hint = "Wähl einen Mitarbeiter, dann stehen hier die freien Zeiten."
		return z, nil
	} else if err != nil {
		return z, err
	}
	free, err := freeSlotsFor(a.db, id, dur, d, now)
	if err != nil {
		return z, err
	}
	for _, t := range free {
		hm := t.In(s.Loc).Format("15:04")
		z.Slots = append(z.Slots, kalSlot{Time: hm, ID: hm[:2] + hm[3:], Selected: hm == zeit})
	}
	if len(z.Slots) == 0 {
		z.Hint = z.Staff + " hat an diesem Tag laut Arbeitszeiten keine freie Zeit."
	}
	return z, nil
}

// kalTimesFrag liefert das Feld Uhrzeit neu (htmx), wenn Leistung, Mitarbeiter
// oder Datum wechseln oder ein Vorschlag angeklickt wird. Die Dauer kommt aus
// der Leistung (neuer Termin) oder dem Termin selbst (?termin=, verschieben).
func (a *app) kalTimesFrag(w http.ResponseWriter, r *http.Request, c *dctx) {
	q := r.URL.Query()
	var dur time.Duration
	var err error
	if id, _ := strconv.ParseInt(q.Get("termin"), 10, 64); id > 0 {
		var t *kalTermin
		if t, err = a.kalLoad(c.Salon, id); err == nil {
			dur = t.End.Sub(t.Start)
		}
	} else if q.Get("leistung") != "" {
		var sv *service
		if sv, err = loadService(a.db, c.Salon.ID, q.Get("leistung")); err == nil {
			dur = time.Duration(sv.Duration) * time.Minute
		}
	}
	zeit := q.Get("zeit")
	if _, perr := time.Parse("15:04", zeit); perr != nil {
		zeit = ""
	}
	var z kalZeit
	if err == nil || errors.Is(err, sql.ErrNoRows) { // fremde oder unbekannte ID: nur der Hinweis
		z, err = a.kalTimes(c.Salon, q.Get("mitarbeiter"), dur, q.Get("tag"), zeit, time.Now())
	}
	if err != nil {
		log.Print(err)
		http.Error(w, "Fehler beim Laden der freien Zeiten.", http.StatusInternalServerError)
		return
	}
	a.render(w, http.StatusOK, "kal-zeit", map[string]any{"Z": z})
}

// --- Termin verschieben ---

// kalMoveStaff: aktive Mitarbeiter, dazu der bisherige, falls er inaktiv ist.
func (a *app) kalMoveStaff(s *salon, t *kalTermin) ([]staffMember, error) {
	staff, err := a.staff(s.ID)
	if err != nil {
		return nil, err
	}
	if !slices.ContainsFunc(staff, func(m staffMember) bool { return m.ID == t.StaffID }) {
		staff = append(staff, staffMember{ID: t.StaffID, Name: t.Staff + " (inaktiv)"})
	}
	return staff, nil
}

func (a *app) kalMoveForm(w http.ResponseWriter, r *http.Request, c *dctx) {
	t := a.kalFind(w, r, c)
	if t == nil {
		return
	}
	f := kalForm{Staff: strconv.FormatInt(t.StaffID, 10), Date: t.Start.Format(time.DateOnly), Time: t.Start.Format("15:04")}
	a.kalMovePage(w, r, c, t, f, true, http.StatusOK, "", "")
}

// kalMovePage zeigt das Formular; field = Name des fehlerhaften Felds zu msg.
func (a *app) kalMovePage(w http.ResponseWriter, r *http.Request, c *dctx, t *kalTermin, f kalForm, inform bool, status int, field, msg string) {
	if t.Status != "booked" {
		a.dashError(w, r, c, http.StatusConflict, "Nur gebuchte Termine lassen sich verschieben.")
		return
	}
	staff, err := a.kalMoveStaff(c.Salon, t)
	if err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	z, err := a.kalTimes(c.Salon, f.Staff, t.End.Sub(t.Start), f.Date, f.Time, time.Now())
	if err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	z.Invalid = field == "zeit"
	d := a.dashPage(c, r, "kalender", "Termin verschieben")
	d["T"], d["F"], d["Staff"], d["Z"], d["Inform"], d["Error"] = t, f, staff, z, inform, msg
	d["Invalid"] = map[string]bool{field: true}
	a.render(w, status, "kal-verschieben", d)
}

func (a *app) kalMove(w http.ResponseWriter, r *http.Request, c *dctx, t *kalTermin) {
	s, now := c.Salon, time.Now()
	f := kalFormFrom(r.PostForm)
	inform := r.PostFormValue("informieren") == "1"
	bad := func(status int, field, msg string) { a.kalMovePage(w, r, c, t, f, inform, status, field, msg) }
	day, dayErr := time.ParseInLocation(time.DateOnly, f.Date, s.Loc)
	hm, hmErr := time.Parse("15:04", f.Time)
	switch {
	case t.Status != "booked":
		bad(http.StatusConflict, "", "") // kalMovePage zeigt "Nur gebuchte Termine ..."
		return
	case f.Staff == "":
		bad(http.StatusBadRequest, "mitarbeiter", "Bitte einen Mitarbeiter wählen.")
		return
	case dayErr != nil:
		bad(http.StatusBadRequest, "tag", "Bitte ein Datum wählen.")
		return
	case hmErr != nil || hm.Minute()%5 != 0:
		bad(http.StatusBadRequest, "zeit", "Bitte eine Uhrzeit im 5-Minuten-Raster wählen (z. B. 09:05).")
		return
	}
	y, m, dd := day.Date()
	start := time.Date(y, m, dd, hm.Hour(), hm.Minute(), 0, 0, s.Loc)
	mailed := inform && t.Email != "" && start.After(now)
	token, err := a.kalUpdate(s, t, f.Staff, start, now, mailed)
	var ke *kalErr
	switch {
	case errors.As(err, &ke) && ke.status == http.StatusNotFound:
		a.dashError(w, r, c, ke.status, ke.msg)
		return
	case errors.As(err, &ke):
		bad(ke.status, "zeit", ke.msg) // Überschneidung
		return
	case err != nil:
		a.dashServerError(w, r, c, err)
		return
	}
	code := "verschoben"
	if mailed {
		if b, err := a.findBooking(s, token); err != nil {
			log.Printf("Verschieben %d: %v", t.ID, err)
		} else {
			intro := fmt.Sprintf("dein Termin bei %s wurde verschoben. Bisher: %s. Neu:", s.Name, deDate(t.Start))
			a.kalMail(b.Email, "Termin verschoben: "+s.Name, a.customerMail(s, b, intro))
			code = "verschoben-mail"
		}
	}
	kalSaved(w, r, t.ID, code)
}

// kalUpdate verschiebt einen gebuchten Termin (gleiche Dauer) auf start bei
// Mitarbeiter staffID, mit derselben Überschneidungsprüfung wie kalInsert (ohne
// den Termin selbst). Liegt der neue Termin mehr als 24 h entfernt, geht die
// Erinnerung später erneut raus; ersetzt die Mail sie (< 24 h), gilt sie als
// verschickt. Liefert den cancel_token für die Mail.
func (a *app) kalUpdate(s *salon, t *kalTermin, staffID string, start, now time.Time, mailed bool) (string, error) {
	tx, err := a.db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	// Der neue Mitarbeiter muss aktiv sein; der bisherige darf es auch inaktiv bleiben.
	var st staffMember
	err = tx.QueryRow(`SELECT id, name FROM staff WHERE id = ? AND salon_id = ? AND (active = 1 OR id = ?)`, staffID, s.ID, t.StaffID).
		Scan(&st.ID, &st.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", &kalErr{http.StatusNotFound, "Diesen Mitarbeiter gibt es nicht."}
	} else if err != nil {
		return "", err
	}
	end := start.Add(t.End.Sub(t.Start))
	if err := kalOverlap(tx, s, &st, start, end, t.ID); err != nil {
		return "", err
	}
	q, args := `UPDATE appointments SET staff_id = ?, starts_at = ?, ends_at = ?`, []any{st.ID, start.Unix(), end.Unix()}
	switch {
	case start.Sub(now) > 24*time.Hour:
		q += `, reminder_sent_at = NULL`
	case mailed:
		q, args = q+`, reminder_sent_at = ?`, append(args, now.Unix())
	}
	var token string
	err = tx.QueryRow(q+` WHERE id = ? AND salon_id = ? AND status = 'booked' RETURNING cancel_token`, append(args, t.ID, s.ID)...).Scan(&token)
	if errors.Is(err, sql.ErrNoRows) {
		return "", &kalErr{http.StatusConflict, "Dieser Termin ist nicht mehr gebucht."}
	} else if err != nil {
		return "", err
	}
	return token, tx.Commit()
}

// kalOverlap meldet einen gebuchten Termin des Mitarbeiters, der [start, end)
// überschneidet, als 409. except = Termin, der gerade verschoben wird (sonst 0).
func kalOverlap(tx *sql.Tx, s *salon, st *staffMember, start, end time.Time, except int64) error {
	var bs, be int64
	err := tx.QueryRow(`SELECT starts_at, ends_at FROM appointments WHERE salon_id = ? AND staff_id = ? AND status = 'booked'
		AND starts_at < ? AND ends_at > ? AND id <> ? ORDER BY starts_at LIMIT 1`, s.ID, st.ID, end.Unix(), start.Unix(), except).Scan(&bs, &be)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	} else if err != nil {
		return err
	}
	hhmm := func(u int64) string { return time.Unix(u, 0).In(s.Loc).Format("15:04") }
	return &kalErr{http.StatusConflict, fmt.Sprintf("%s hat von %s bis %s Uhr schon einen Termin. Bitte eine andere Uhrzeit wählen.",
		st.Name, hhmm(bs), hhmm(be))}
}

// --- Neuer Termin ---

// kalForm = Felder von /dashboard/termine/neu, Namen wie die Query-Vorbelegung.
type kalForm struct {
	Service, Staff, Date, Time, Customer string
	Name, Email, Phone, Note             string
	Confirm                              bool
}

func kalFormFrom(v url.Values) kalForm {
	g := func(k string) string { return strings.TrimSpace(v.Get(k)) }
	return kalForm{Service: g("leistung"), Staff: g("mitarbeiter"), Date: g("tag"), Time: g("zeit"), Customer: g("kunde"),
		Name: g("name"), Email: g("email"), Phone: g("telefon"), Note: g("notiz"), Confirm: v.Get("bestaetigung") == "1"}
}

type kalHit struct {
	ID            int64
	Name, Contact string
	Checked       bool
}

// kalCustomers sucht Kunden des Salons nach Name, E-Mail oder Telefon
// (id > 0: genau dieser Kunde, q wird dann ignoriert).
func (a *app) kalCustomers(salonID, id int64, q string) ([]kalHit, error) {
	like := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q) + "%"
	rows, err := a.db.Query(`SELECT id, name, COALESCE(email, ''), phone FROM customers
		WHERE salon_id = ?1 AND (id = ?2 OR (?2 = 0 AND (name LIKE ?3 ESCAPE '\' OR email LIKE ?3 ESCAPE '\' OR phone LIKE ?3 ESCAPE '\')))
		ORDER BY name, id LIMIT 10`, salonID, id, like)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []kalHit
	for rows.Next() {
		var h kalHit
		var email, phone string
		if err := rows.Scan(&h.ID, &h.Name, &email, &phone); err != nil {
			return nil, err
		}
		h.Contact = strings.Trim(email+", "+phone, ", ")
		out = append(out, h)
	}
	return out, rows.Err()
}

// kalSearch: Live-Suche aus dem Suchfeld (q), dem Namens- oder dem Telefonfeld.
func (a *app) kalSearch(w http.ResponseWriter, r *http.Request, c *dctx) {
	v := r.URL.Query()
	q := cmp.Or(strings.TrimSpace(v.Get("q")), strings.TrimSpace(v.Get("name")), strings.TrimSpace(v.Get("telefon")))
	if utf8.RuneCountInString(q) > 100 {
		q = string([]rune(q)[:100])
	}
	var hits []kalHit
	if q != "" {
		var err error
		if hits, err = a.kalCustomers(c.Salon.ID, 0, q); err != nil {
			log.Print(err)
			http.Error(w, "Fehler bei der Suche.", http.StatusInternalServerError)
			return
		}
	}
	a.render(w, http.StatusOK, "kal-treffer", map[string]any{"Hits": hits, "Q": q})
}

func (a *app) kalNew(w http.ResponseWriter, r *http.Request, c *dctx) {
	f := kalFormFrom(r.URL.Query())
	if _, err := time.ParseInLocation(time.DateOnly, f.Date, c.Salon.Loc); err != nil {
		f.Date = time.Now().In(c.Salon.Loc).Format(time.DateOnly)
	}
	if _, err := time.Parse("15:04", f.Time); err != nil {
		f.Time = ""
	}
	// ponytail: Suche ohne JavaScript (Enter im Suchfeld) lädt die Seite mit ?q= neu.
	a.kalFormPage(w, r, c, f, strings.TrimSpace(r.URL.Query().Get("q")), http.StatusOK, "", "")
}

// kalFormPage zeigt das Formular; field = Name des fehlerhaften Felds zu msg.
func (a *app) kalFormPage(w http.ResponseWriter, r *http.Request, c *dctx, f kalForm, q string, status int, field, msg string) {
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
	var hits []kalHit
	if f.Customer != "" {
		// Vorauswahl (?kunde= aus der Kundenkartei): fremde oder unbekannte ID wird
		// ignoriert, das Formular steht dann auf "Neuer Kunde". Beim Speichern -> 404.
		if id, _ := strconv.ParseInt(f.Customer, 10, 64); id > 0 {
			hits, err = a.kalCustomers(c.Salon.ID, id, "")
		}
		if len(hits) == 0 {
			f.Customer = ""
		}
		for i := range hits {
			hits[i].Checked = true
		}
	} else if q != "" {
		hits, err = a.kalCustomers(c.Salon.ID, 0, q)
	}
	if err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	// Vorschläge für die Uhrzeit: Leistung aus dem Formular, sonst der erste Mitarbeiter der Auswahl.
	var dur time.Duration
	for _, sv := range services {
		if strconv.FormatInt(sv.ID, 10) == f.Service {
			dur = time.Duration(sv.Duration) * time.Minute
		}
	}
	staffID := f.Staff
	if staffID == "" && len(staff) > 0 {
		staffID = strconv.FormatInt(staff[0].ID, 10)
	}
	z, err := a.kalTimes(c.Salon, staffID, dur, f.Date, f.Time, time.Now())
	if err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	z.Invalid = field == "zeit"
	d := a.dashPage(c, r, "kalender", "Neuer Termin")
	d["F"], d["Q"], d["Hits"], d["Error"], d["Z"] = f, q, hits, msg, z
	d["Invalid"] = map[string]bool{field: true}
	d["Services"], d["Staff"] = services, staff
	d["HasStaff"], d["HasServices"] = len(staff) > 0, len(services) > 0
	a.render(w, status, "kal-neu", d)
}

// kalErr ist ein Eingabefehler mit HTTP-Status (404 fremde ID, 409 Überschneidung).
type kalErr struct {
	status int
	msg    string
}

func (e *kalErr) Error() string { return e.msg }

func (a *app) kalCreate(w http.ResponseWriter, r *http.Request, c *dctx) {
	s := c.Salon
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := r.ParseForm(); err != nil {
		a.dashError(w, r, c, http.StatusBadRequest, "Das Formular war unvollständig.")
		return
	}
	f := kalFormFrom(r.PostForm)
	bad := func(field, msg string) { a.kalFormPage(w, r, c, f, "", http.StatusBadRequest, field, msg) }
	day, dayErr := time.ParseInLocation(time.DateOnly, f.Date, s.Loc)
	hm, hmErr := time.Parse("15:04", f.Time)
	var email string
	if f.Customer == "" && f.Email != "" {
		addr, err := mail.ParseAddress(f.Email)
		if err != nil || len(addr.Address) > 200 {
			bad("email", "Bitte eine gültige E-Mail-Adresse angeben oder das Feld leer lassen.")
			return
		}
		email = strings.ToLower(addr.Address)
	}
	switch {
	case f.Service == "":
		bad("leistung", "Bitte eine Leistung wählen.")
		return
	case f.Staff == "":
		bad("mitarbeiter", "Bitte einen Mitarbeiter wählen.")
		return
	case dayErr != nil:
		bad("tag", "Bitte ein Datum wählen.")
		return
	case hmErr != nil || hm.Minute()%5 != 0:
		bad("zeit", "Bitte eine Uhrzeit im 5-Minuten-Raster wählen (z. B. 09:05).")
		return
	case f.Customer == "" && (f.Name == "" || utf8.RuneCountInString(f.Name) > 100):
		bad("name", "Bitte einen bestehenden Kunden wählen oder einen Namen eintragen.")
		return
	case len(f.Phone) > 40:
		bad("telefon", "Die Telefonnummer ist zu lang.")
		return
	case utf8.RuneCountInString(f.Note) > 1000:
		bad("notiz", "Die Notiz ist zu lang (höchstens 1000 Zeichen).")
		return
	}
	y, m, dd := day.Date()
	start := time.Date(y, m, dd, hm.Hour(), hm.Minute(), 0, 0, s.Loc)

	n, err := a.kalInsert(s, f, start, email, time.Now())
	var ke *kalErr
	switch {
	case errors.As(err, &ke) && ke.status == http.StatusNotFound:
		a.dashError(w, r, c, ke.status, ke.msg)
		return
	case errors.As(err, &ke):
		a.kalFormPage(w, r, c, f, "", ke.status, "zeit", ke.msg) // Überschneidung
		return
	case err != nil:
		a.dashServerError(w, r, c, err)
		return
	}
	code := "eingetragen"
	if f.Confirm && n.email != "" {
		body := fmt.Sprintf("Hallo %s,\n\ndein Termin bei %s ist eingetragen:\n\n%s\n%s bei %s\n\nAbsagen: %s\n",
			n.customer, s.Name, deDate(start), n.service, n.staff, a.link(s.Slug, "/storno/"+n.token))
		a.kalMail(n.email, "Termin bestätigt: "+s.Name, body)
		code = "eingetragen-mail"
	}
	http.Redirect(w, r, fmt.Sprintf("/dashboard?tag=%s&gespeichert=%s&termin=%d", start.Format(time.DateOnly), code, n.id), http.StatusSeeOther)
}

type kalCreated struct {
	id                                     int64
	token, customer, email, service, staff string
}

// kalPhone: nur Ziffern (und führendes +), zum Vergleich von Telefonnummern.
func kalPhone(p string) string {
	return strings.TrimPrefix(string(telURL(p)), "tel:")
}

// kalInsert prüft und speichert einen manuellen Termin in einer Transaktion.
// Achtung: innerhalb der Transaktion nur tx benutzen (eine DB-Verbindung).
func (a *app) kalInsert(s *salon, f kalForm, start time.Time, email string, now time.Time) (*kalCreated, error) {
	tx, err := a.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	// lookup prüft salon_id und active; Arbeitszeiten werden bewusst nicht geprüft.
	sv, st, err := lookup(tx, s.ID, f.Service, f.Staff)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &kalErr{http.StatusNotFound, "Leistung oder Mitarbeiter gibt es nicht."}
	} else if err != nil {
		return nil, err
	}
	end := start.Add(time.Duration(sv.Duration) * time.Minute)
	if err := kalOverlap(tx, s, st, start, end, 0); err != nil {
		return nil, err
	}

	n := &kalCreated{token: randomToken(), service: sv.Name, staff: st.Name}
	var custID int64
	var custEmail sql.NullString
	if phone := kalPhone(f.Phone); f.Customer == "" && email == "" && len(phone) >= 6 {
		// Ohne E-Mail, aber mit Telefon: bestehende Karteikarte mit derselben Nummer
		// wiederverwenden (Leerzeichen, Striche usw. egal), bei mehreren die mit gleichem Namen.
		// ponytail: "+49 171" und "0171" gelten als verschiedene Nummern.
		err = tx.QueryRow(`SELECT id FROM customers WHERE salon_id = ? AND
			replace(replace(replace(replace(replace(replace(phone, ' ', ''), '-', ''), '/', ''), '(', ''), ')', ''), '.', '') = ?
			ORDER BY lower(name) = lower(?) DESC, id LIMIT 1`, s.ID, phone, f.Name).Scan(&custID)
		if err == nil {
			f.Customer = strconv.FormatInt(custID, 10)
		} else if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	if f.Customer != "" {
		err = tx.QueryRow(`SELECT id, name, email FROM customers WHERE id = ? AND salon_id = ?`, f.Customer, s.ID).
			Scan(&custID, &n.customer, &custEmail)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, &kalErr{http.StatusNotFound, "Diesen Kunden gibt es nicht."}
		}
	} else {
		// Ohne E-Mail (und ohne bekannte Nummer): NULL, also ein neuer Kunde. Mit E-Mail:
		// bestehende Karteikarte (salon_id, email) wiederverwenden, Name bleibt, Telefon nur ergänzen.
		var em any
		if email != "" {
			em = email
		}
		err = tx.QueryRow(`INSERT INTO customers (salon_id, name, email, phone) VALUES (?, ?, ?, ?)
			ON CONFLICT (salon_id, email) DO UPDATE SET phone = CASE WHEN customers.phone = '' THEN excluded.phone ELSE customers.phone END
			RETURNING id, name, email`, s.ID, f.Name, em, f.Phone).Scan(&custID, &n.customer, &custEmail)
	}
	if err != nil {
		return nil, err
	}
	n.email = custEmail.String
	// Ohne Mail-Adresse keine Erinnerung; kurzfristig (< 24 h) ersetzt die Bestätigung sie.
	var reminded any
	if n.email == "" || start.Sub(now) < 24*time.Hour {
		reminded = now.Unix()
	}
	res, err := tx.Exec(`INSERT INTO appointments (salon_id, staff_id, service_id, customer_id, starts_at, ends_at, source, note, cancel_token, reminder_sent_at)
		VALUES (?, ?, ?, ?, ?, ?, 'manual', ?, ?, ?)`, s.ID, st.ID, sv.ID, custID, start.Unix(), end.Unix(), f.Note, n.token, reminded)
	if err != nil {
		return nil, err
	}
	n.id, _ = res.LastInsertId()
	return n, tx.Commit()
}
