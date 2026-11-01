package main

// Leistungen und Team (Mitarbeiter, Arbeitszeiten, Zuordnung): /dashboard/leistungen/..., /dashboard/team/...
// Alle Namen hier tragen das Präfix leist/team/catalog, damit sie im gemeinsamen Paket nicht kollidieren.

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

func (a *app) routesCatalog(mux *http.ServeMux) {
	mux.HandleFunc("GET /dashboard/leistungen", a.dash(a.leistList))
	mux.HandleFunc("GET /dashboard/leistungen/neu", a.dash(a.leistEdit))
	mux.HandleFunc("POST /dashboard/leistungen/neu", a.dash(a.leistSave))
	mux.HandleFunc("GET /dashboard/leistungen/{id}", a.dash(a.leistEdit))
	mux.HandleFunc("POST /dashboard/leistungen/{id}", a.dash(a.leistSave))
	mux.HandleFunc("POST /dashboard/leistungen/{id}/verschieben", a.dash(a.leistMove))
	mux.HandleFunc("POST /dashboard/leistungen/{id}/loeschen", a.dash(a.catalogDelete("services", "/dashboard/leistungen")))
	mux.HandleFunc("GET /dashboard/team", a.dash(a.teamList))
	mux.HandleFunc("GET /dashboard/team/neu", a.dash(a.teamEdit))
	mux.HandleFunc("POST /dashboard/team/neu", a.dash(a.teamSave))
	mux.HandleFunc("GET /dashboard/team/{id}", a.dash(a.teamEdit))
	mux.HandleFunc("POST /dashboard/team/{id}", a.dash(a.teamSave))
	mux.HandleFunc("POST /dashboard/team/{id}/loeschen", a.dash(a.catalogDelete("staff", "/dashboard/team")))
}

// catalogID ist die ID aus dem Pfad, 0 wenn ungültig (findet dann nichts).
func catalogID(r *http.Request) int64 {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id
}

func catalogNotFound(a *app, w http.ResponseWriter, r *http.Request, c *dctx) {
	a.dashError(w, r, c, http.StatusNotFound, "Diesen Eintrag gibt es nicht.")
}

// catalogDelete löscht eine Leistung oder einen Mitarbeiter. Hängen Termine daran,
// wird nur deaktiviert, damit die Termine nicht per ON DELETE CASCADE mit verschwinden.
// table ist eine Konstante aus routesCatalog, nie Nutzereingabe.
func (a *app) catalogDelete(table, back string) func(http.ResponseWriter, *http.Request, *dctx) {
	col := map[string]string{"services": "service_id", "staff": "staff_id"}[table]
	return func(w http.ResponseWriter, r *http.Request, c *dctx) {
		id := catalogID(r)
		tx, err := a.db.BeginTx(r.Context(), nil)
		if err != nil {
			a.dashServerError(w, r, c, err)
			return
		}
		defer tx.Rollback()
		var n int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM appointments WHERE salon_id = ? AND `+col+` = ?`, c.Salon.ID, id).Scan(&n); err != nil {
			a.dashServerError(w, r, c, err)
			return
		}
		q, done := `DELETE FROM `+table+` WHERE id = ? AND salon_id = ?`, "geloescht"
		if n > 0 {
			q, done = `UPDATE `+table+` SET active = 0 WHERE id = ? AND salon_id = ?`, "deaktiviert"
		}
		res, err := tx.Exec(q, id, c.Salon.ID)
		if err != nil {
			a.dashServerError(w, r, c, err)
			return
		}
		if k, _ := res.RowsAffected(); k == 0 {
			catalogNotFound(a, w, r, c)
			return
		}
		if err := tx.Commit(); err != nil {
			a.dashServerError(w, r, c, err)
			return
		}
		http.Redirect(w, r, back+"?"+done, http.StatusSeeOther)
	}
}

// --- Leistungen ---

type leistRow struct {
	ID, PriceCents      int64
	Name                string
	Duration            int
	Active, First, Last bool
}

// leistForm hält die Eingaben als Text, damit sie bei Fehlern unverändert zurückkommen.
type leistForm struct {
	ID                                 int64
	Name, Description, Duration, Price string
	Active                             bool
}

func (a *app) leistList(w http.ResponseWriter, r *http.Request, c *dctx) {
	rows, err := a.db.Query(`SELECT id, name, duration_min, price_cents, active FROM services
		WHERE salon_id = ? ORDER BY sort, name, id`, c.Salon.ID)
	if err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	defer rows.Close()
	var list []leistRow
	for rows.Next() {
		var s leistRow
		if err := rows.Scan(&s.ID, &s.Name, &s.Duration, &s.PriceCents, &s.Active); err != nil {
			a.dashServerError(w, r, c, err)
			return
		}
		list = append(list, s)
	}
	if err := rows.Err(); err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	if n := len(list); n > 0 {
		list[0].First, list[n-1].Last = true, true
	}
	d := a.dashPage(c, r, "leistungen", "Leistungen")
	d["Services"] = list
	d["Deactivated"] = r.URL.Query().Has("deaktiviert")
	d["Deleted"] = r.URL.Query().Has("geloescht")
	a.render(w, http.StatusOK, "leist-liste", d)
}

func (a *app) leistEdit(w http.ResponseWriter, r *http.Request, c *dctx) {
	f := leistForm{Duration: "30", Active: true}
	if r.PathValue("id") != "" {
		f.ID = catalogID(r)
		var dur int
		var cents int64
		err := a.db.QueryRow(`SELECT name, description, duration_min, price_cents, active FROM services WHERE id = ? AND salon_id = ?`,
			f.ID, c.Salon.ID).Scan(&f.Name, &f.Description, &dur, &cents, &f.Active)
		if errors.Is(err, sql.ErrNoRows) {
			catalogNotFound(a, w, r, c)
			return
		} else if err != nil {
			a.dashServerError(w, r, c, err)
			return
		}
		f.Duration, f.Price = strconv.Itoa(dur), fmt.Sprintf("%d,%02d", cents/100, cents%100)
	}
	a.leistRender(w, r, c, http.StatusOK, f, "")
}

func (a *app) leistRender(w http.ResponseWriter, r *http.Request, c *dctx, status int, f leistForm, msg string) {
	title := "Neue Leistung"
	if f.ID != 0 {
		title = "Leistung bearbeiten"
	}
	d := a.dashPage(c, r, "leistungen", title)
	d["F"], d["Error"] = f, msg
	a.render(w, status, "leist-form", d)
}

func (a *app) leistSave(w http.ResponseWriter, r *http.Request, c *dctx) {
	isNew := r.PathValue("id") == ""
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := r.ParseForm(); err != nil {
		a.dashError(w, r, c, http.StatusBadRequest, "Das Formular war unvollständig.")
		return
	}
	f := leistForm{
		Name:        strings.TrimSpace(r.PostFormValue("name")),
		Description: strings.ReplaceAll(strings.TrimSpace(r.PostFormValue("beschreibung")), "\r\n", "\n"),
		Duration:    strings.TrimSpace(r.PostFormValue("dauer")),
		Price:       strings.TrimSpace(r.PostFormValue("preis")),
		Active:      r.PostFormValue("aktiv") == "1",
	}
	if !isNew {
		f.ID = catalogID(r)
	}
	dur, durErr := strconv.Atoi(f.Duration)
	cents, priceErr := leistParsePrice(f.Price)
	msg := ""
	switch {
	case f.Name == "" || utf8.RuneCountInString(f.Name) > 80:
		msg = "Bitte einen Namen mit höchstens 80 Zeichen angeben."
	case utf8.RuneCountInString(f.Description) > 300:
		msg = "Die Beschreibung darf höchstens 300 Zeichen lang sein."
	case durErr != nil || dur < 5 || dur > 480 || dur%5 != 0:
		msg = "Die Dauer muss zwischen 5 und 480 Minuten liegen, in 5-Minuten-Schritten."
	case priceErr != nil:
		msg = "Bitte einen gültigen Preis angeben, z. B. 25 oder 25,50."
	}
	if msg != "" {
		a.leistRender(w, r, c, http.StatusUnprocessableEntity, f, msg)
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	defer tx.Rollback()
	if isNew {
		var id int64
		err = tx.QueryRow(`INSERT INTO services (salon_id, name, description, duration_min, price_cents, active, sort)
			VALUES (?, ?, ?, ?, ?, ?, (SELECT COALESCE(MAX(sort), 0) + 1 FROM services WHERE salon_id = ?)) RETURNING id`,
			c.Salon.ID, f.Name, f.Description, dur, cents, f.Active, c.Salon.ID).Scan(&id)
		if err == nil { // neue Leistung -> alle aktiven Mitarbeiter (siehe schema.sql)
			_, err = tx.Exec(`INSERT INTO staff_services (staff_id, service_id) SELECT id, ? FROM staff WHERE salon_id = ? AND active = 1`, id, c.Salon.ID)
		}
	} else {
		var res sql.Result
		res, err = tx.Exec(`UPDATE services SET name = ?, description = ?, duration_min = ?, price_cents = ?, active = ?
			WHERE id = ? AND salon_id = ?`, f.Name, f.Description, dur, cents, f.Active, f.ID, c.Salon.ID)
		if err == nil {
			if n, _ := res.RowsAffected(); n == 0 {
				catalogNotFound(a, w, r, c)
				return
			}
		}
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	http.Redirect(w, r, "/dashboard/leistungen?gespeichert", http.StatusSeeOther)
}

// leistMove schiebt eine Leistung eine Position hoch oder runter und nummeriert neu.
// ponytail: schreibt alle sort-Werte des Salons neu, bei ein paar Dutzend Leistungen egal.
func (a *app) leistMove(w http.ResponseWriter, r *http.Request, c *dctx) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<10)
	up := r.PostFormValue("richtung") == "hoch"
	id := catalogID(r)
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT id FROM services WHERE salon_id = ? ORDER BY sort, name, id`, c.Salon.ID)
	if err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	var ids []int64
	for rows.Next() {
		var x int64
		if err := rows.Scan(&x); err != nil {
			rows.Close()
			a.dashServerError(w, r, c, err)
			return
		}
		ids = append(ids, x)
	}
	rows.Close()
	i := slices.Index(ids, id)
	if i < 0 {
		catalogNotFound(a, w, r, c)
		return
	}
	j := i + 1
	if up {
		j = i - 1
	}
	if j >= 0 && j < len(ids) {
		ids[i], ids[j] = ids[j], ids[i]
		for k, x := range ids {
			if _, err := tx.Exec(`UPDATE services SET sort = ? WHERE id = ? AND salon_id = ?`, k, x, c.Salon.ID); err != nil {
				a.dashServerError(w, r, c, err)
				return
			}
		}
		if err := tx.Commit(); err != nil {
			a.dashServerError(w, r, c, err)
			return
		}
	}
	http.Redirect(w, r, "/dashboard/leistungen", http.StatusSeeOther)
}

// leistParsePrice liest Euro-Beträge wie "25", "25,5", "25,50" oder "25.50" als Cent.
// Tausenderpunkte, Vorzeichen und mehr als zwei Nachkommastellen sind Fehler.
func leistParsePrice(s string) (int64, error) {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "€"))
	eur, ct, sep := strings.Cut(strings.Replace(s, ",", ".", 1), ".")
	if !sep {
		ct = "00"
	} else if len(ct) == 1 {
		ct += "0"
	}
	if eur == "" || len(eur) > 5 || len(ct) != 2 || strings.Trim(eur+ct, "0123456789") != "" {
		return 0, errors.New("ungültiger Preis")
	}
	e, _ := strconv.ParseInt(eur, 10, 64)
	c, _ := strconv.ParseInt(ct, 10, 64)
	return e*100 + c, nil
}

// --- Team ---

type teamRow struct {
	ID       int64
	Name     string
	Active   bool
	Services int
	Hours    []string
}

type teamService struct {
	ID              int64
	Name            string
	Active, Checked bool
}

// teamDay: Arbeitszeiten eines Wochentags im Formular, bis zu zwei Blöcke als "HH:MM".
type teamDay struct {
	WD       int // wie time.Weekday, 0 = Sonntag
	Label    string
	From, To [2]string
}

type teamForm struct {
	ID       int64
	Name     string
	Active   bool
	Services []teamService
	Days     []teamDay
}

// Anzeige Mo..So.
var teamOrder = [...]time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday, time.Sunday}

func (a *app) teamList(w http.ResponseWriter, r *http.Request, c *dctx) {
	rows, err := a.db.Query(`SELECT id, name, active, (SELECT COUNT(*) FROM staff_services WHERE staff_id = staff.id)
		FROM staff WHERE salon_id = ? ORDER BY sort, name, id`, c.Salon.ID)
	if err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	var list []teamRow
	for rows.Next() {
		var s teamRow
		if err := rows.Scan(&s.ID, &s.Name, &s.Active, &s.Services); err != nil {
			rows.Close()
			a.dashServerError(w, r, c, err)
			return
		}
		list = append(list, s)
	}
	rows.Close()
	rows, err = a.db.Query(`SELECT wh.staff_id, wh.weekday, wh.start_min, wh.end_min FROM working_hours wh
		JOIN staff st ON st.id = wh.staff_id WHERE st.salon_id = ? ORDER BY wh.start_min`, c.Salon.ID)
	if err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	hours := map[int64][]WorkingHours{}
	for rows.Next() {
		var id int64
		var h WorkingHours
		if err := rows.Scan(&id, &h.Weekday, &h.StartMin, &h.EndMin); err != nil {
			rows.Close()
			a.dashServerError(w, r, c, err)
			return
		}
		hours[id] = append(hours[id], h)
	}
	rows.Close()
	for i := range list {
		list[i].Hours = teamHoursSummary(hours[list[i].ID])
	}
	d := a.dashPage(c, r, "team", "Team")
	d["Staff"] = list
	d["Deactivated"] = r.URL.Query().Has("deaktiviert")
	d["Deleted"] = r.URL.Query().Has("geloescht")
	d["ShowLimit"] = !c.Salon.Pro() && !a.cfg.SingleTenant
	d["Limit"] = freeStaffLimit
	a.render(w, http.StatusOK, "team-liste", d)
}

// teamHoursSummary fasst Wochentage mit gleichen Zeiten zusammen, z. B.
// ["Di-Fr 9:00-18:00", "Sa 9:00-14:00"]. hours muss nach start_min sortiert sein.
func teamHoursSummary(hours []WorkingHours) []string {
	clock := func(m int) string { return fmt.Sprintf("%d:%02d", m/60, m%60) }
	day := map[time.Weekday]string{}
	for _, h := range hours {
		if day[h.Weekday] != "" {
			day[h.Weekday] += ", "
		}
		day[h.Weekday] += clock(h.StartMin) + "-" + clock(h.EndMin)
	}
	var out []string
	for i := 0; i < len(teamOrder); {
		j := i
		for j+1 < len(teamOrder) && day[teamOrder[j+1]] == day[teamOrder[i]] {
			j++
		}
		if txt := day[teamOrder[i]]; txt != "" {
			label := weekdaysDE[teamOrder[i]][:2]
			if j > i {
				label += "-" + weekdaysDE[teamOrder[j]][:2]
			}
			out = append(out, label+" "+txt)
		}
		i = j + 1
	}
	return out
}

func (a *app) teamEdit(w http.ResponseWriter, r *http.Request, c *dctx) {
	f := teamForm{Active: true}
	var hours []WorkingHours
	if r.PathValue("id") == "" {
		full, err := a.teamLimitReached(a.db, c.Salon, 0)
		if err != nil {
			a.dashServerError(w, r, c, err)
			return
		}
		if full {
			a.teamRender(w, r, c, http.StatusOK, nil, "", true)
			return
		}
		// Vorschlag für neue Mitarbeiter: Di-Fr 9-18, Sa 9-14.
		for wd := time.Tuesday; wd <= time.Friday; wd++ {
			hours = append(hours, WorkingHours{wd, 9 * 60, 18 * 60})
		}
		hours = append(hours, WorkingHours{time.Saturday, 9 * 60, 14 * 60})
	} else {
		f.ID = catalogID(r)
		err := a.db.QueryRow(`SELECT name, active FROM staff WHERE id = ? AND salon_id = ?`, f.ID, c.Salon.ID).Scan(&f.Name, &f.Active)
		if errors.Is(err, sql.ErrNoRows) {
			catalogNotFound(a, w, r, c)
			return
		} else if err != nil {
			a.dashServerError(w, r, c, err)
			return
		}
		rows, err := a.db.Query(`SELECT weekday, start_min, end_min FROM working_hours WHERE staff_id = ? ORDER BY start_min`, f.ID)
		if err != nil {
			a.dashServerError(w, r, c, err)
			return
		}
		for rows.Next() {
			var h WorkingHours
			if err := rows.Scan(&h.Weekday, &h.StartMin, &h.EndMin); err != nil {
				rows.Close()
				a.dashServerError(w, r, c, err)
				return
			}
			hours = append(hours, h)
		}
		rows.Close()
	}
	svcs, err := a.teamServices(c.Salon.ID, f.ID)
	if err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	if f.ID == 0 { // neuer Mitarbeiter -> alle aktiven Leistungen
		for i := range svcs {
			svcs[i].Checked = svcs[i].Active
		}
	}
	f.Services = svcs
	for _, wd := range teamOrder {
		d := teamDay{WD: int(wd), Label: weekdaysDE[wd]}
		i := 0
		for _, h := range hours {
			if h.Weekday == wd && i < 2 { // ponytail: mehr als zwei Blöcke am Tag kann das Formular nicht zeigen
				d.From[i], d.To[i] = fmt.Sprintf("%02d:%02d", h.StartMin/60, h.StartMin%60), fmt.Sprintf("%02d:%02d", h.EndMin/60, h.EndMin%60)
				i++
			}
		}
		f.Days = append(f.Days, d)
	}
	a.teamRender(w, r, c, http.StatusOK, &f, "", false)
}

// teamServices lädt alle Leistungen des Salons; Checked = dem Mitarbeiter staffID zugeordnet.
func (a *app) teamServices(salonID, staffID int64) ([]teamService, error) {
	rows, err := a.db.Query(`SELECT sv.id, sv.name, sv.active,
		EXISTS (SELECT 1 FROM staff_services ss WHERE ss.staff_id = ? AND ss.service_id = sv.id)
		FROM services sv WHERE sv.salon_id = ? ORDER BY sv.sort, sv.name, sv.id`, staffID, salonID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []teamService
	for rows.Next() {
		var s teamService
		if err := rows.Scan(&s.ID, &s.Name, &s.Active, &s.Checked); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// teamLimitReached: Gratis-Plan, und die anderen aktiven Mitarbeiter (ohne exceptID)
// schöpfen freeStaffLimit schon aus. Im Einzelbetrieb (Selbsthosten) gibt es keine Grenze.
func (a *app) teamLimitReached(q querier, s *salon, exceptID int64) (bool, error) {
	if s.Pro() || a.cfg.SingleTenant {
		return false, nil
	}
	var n int
	err := q.QueryRow(`SELECT COUNT(*) FROM staff WHERE salon_id = ? AND active = 1 AND id != ?`, s.ID, exceptID).Scan(&n)
	return n >= freeStaffLimit, err
}

// teamRender zeigt das Mitarbeiter-Formular. f == nil zusammen mit limit: nur der Hinweis auf Pro.
func (a *app) teamRender(w http.ResponseWriter, r *http.Request, c *dctx, status int, f *teamForm, msg string, limit bool) {
	title := "Neuer Mitarbeiter"
	if f != nil && f.ID != 0 {
		title = "Mitarbeiter bearbeiten"
	}
	d := a.dashPage(c, r, "team", title)
	d["F"], d["Error"], d["LimitHit"], d["Limit"] = f, msg, limit, freeStaffLimit
	a.render(w, status, "team-form", d)
}

func (a *app) teamSave(w http.ResponseWriter, r *http.Request, c *dctx) {
	isNew := r.PathValue("id") == ""
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		a.dashError(w, r, c, http.StatusBadRequest, "Das Formular war unvollständig.")
		return
	}
	f := teamForm{
		Name:   strings.TrimSpace(r.PostFormValue("name")),
		Active: r.PostFormValue("aktiv") == "1",
		Days:   teamDaysFromForm(r.PostForm),
	}
	wasActive := false
	if !isNew {
		f.ID = catalogID(r)
		err := a.db.QueryRow(`SELECT active FROM staff WHERE id = ? AND salon_id = ?`, f.ID, c.Salon.ID).Scan(&wasActive)
		if errors.Is(err, sql.ErrNoRows) {
			catalogNotFound(a, w, r, c)
			return
		} else if err != nil {
			a.dashServerError(w, r, c, err)
			return
		}
	}
	// Nur Leistungen DIESES Salons können angehakt sein; fremde IDs aus dem Formular fallen hier raus.
	svcs, err := a.teamServices(c.Salon.ID, 0)
	if err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	for i := range svcs {
		svcs[i].Checked = slices.Contains(r.PostForm["leistung"], strconv.FormatInt(svcs[i].ID, 10))
	}
	f.Services = svcs
	hours, msg := teamParseHours(f.Days)
	if f.Name == "" || utf8.RuneCountInString(f.Name) > 60 {
		msg = "Bitte einen Namen mit höchstens 60 Zeichen angeben."
	}
	if msg != "" {
		a.teamRender(w, r, c, http.StatusUnprocessableEntity, &f, msg, false)
		return
	}

	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	defer tx.Rollback()
	// Gratis-Plan: Anlegen und Reaktivieren zählen gegen die Grenze, Bearbeiten eines
	// schon aktiven Mitarbeiters nicht (sonst sperrt ein Downgrade die Bearbeitung).
	if f.Active && (isNew || !wasActive) {
		full, err := a.teamLimitReached(tx, c.Salon, f.ID)
		if err != nil {
			a.dashServerError(w, r, c, err)
			return
		}
		if full {
			fp := &f
			if isNew {
				fp = nil
			}
			a.teamRender(w, r, c, http.StatusForbidden, fp, "", true)
			return
		}
	}
	id := f.ID
	if isNew {
		err = tx.QueryRow(`INSERT INTO staff (salon_id, name, active, sort)
			VALUES (?, ?, ?, (SELECT COALESCE(MAX(sort), 0) + 1 FROM staff WHERE salon_id = ?)) RETURNING id`,
			c.Salon.ID, f.Name, f.Active, c.Salon.ID).Scan(&id)
	} else {
		_, err = tx.Exec(`UPDATE staff SET name = ?, active = ? WHERE id = ? AND salon_id = ?`, f.Name, f.Active, id, c.Salon.ID)
	}
	// Zuordnung und Arbeitszeiten komplett ersetzen.
	exec := func(q string, args ...any) {
		if err == nil {
			_, err = tx.Exec(q, args...)
		}
	}
	exec(`DELETE FROM staff_services WHERE staff_id = ?`, id)
	for _, sv := range svcs {
		if sv.Checked {
			exec(`INSERT INTO staff_services (staff_id, service_id) SELECT ?, id FROM services WHERE id = ? AND salon_id = ?`, id, sv.ID, c.Salon.ID)
		}
	}
	exec(`DELETE FROM working_hours WHERE staff_id = ?`, id)
	for _, h := range hours {
		exec(`INSERT INTO working_hours (staff_id, weekday, start_min, end_min) VALUES (?, ?, ?, ?)`, id, int(h.Weekday), h.StartMin, h.EndMin)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	http.Redirect(w, r, "/dashboard/team?gespeichert", http.StatusSeeOther)
}

// teamDaysFromForm liest die Felder von-<wd>-<i> / bis-<wd>-<i> (i = 0, 1) roh ein.
func teamDaysFromForm(f url.Values) []teamDay {
	var days []teamDay
	for _, wd := range teamOrder {
		d := teamDay{WD: int(wd), Label: weekdaysDE[wd]}
		for i := range 2 {
			d.From[i] = strings.TrimSpace(f.Get(fmt.Sprintf("von-%d-%d", wd, i)))
			d.To[i] = strings.TrimSpace(f.Get(fmt.Sprintf("bis-%d-%d", wd, i)))
		}
		days = append(days, d)
	}
	return days
}

// teamParseHours prüft die Arbeitszeiten: beide Felder leer = frei, sonst
// Beginn und Ende im 5-Minuten-Raster, Ende nach Beginn, Blöcke ohne Überschneidung.
// Liefert die Blöcke oder eine Fehlermeldung für den Nutzer.
func teamParseHours(days []teamDay) ([]WorkingHours, string) {
	var out []WorkingHours
	for _, d := range days {
		var blocks []WorkingHours
		for i := range 2 {
			if d.From[i] == "" && d.To[i] == "" {
				continue
			}
			s, okS := teamParseClock(d.From[i])
			e, okE := teamParseClock(d.To[i])
			if !okS || !okE {
				return nil, d.Label + ": Bitte Beginn und Ende angeben, im 5-Minuten-Raster (z. B. 09:00)."
			}
			if e <= s {
				return nil, d.Label + ": Das Ende muss nach dem Beginn liegen."
			}
			blocks = append(blocks, WorkingHours{time.Weekday(d.WD), s, e})
		}
		if len(blocks) == 2 && blocks[0].StartMin < blocks[1].EndMin && blocks[1].StartMin < blocks[0].EndMin {
			return nil, d.Label + ": Die beiden Blöcke überschneiden sich."
		}
		out = append(out, blocks...)
	}
	return out, ""
}

// teamParseClock liest "HH:MM" als Minuten ab Mitternacht, nur im 5-Minuten-Raster.
func teamParseClock(s string) (int, bool) {
	hs, ms, ok := strings.Cut(s, ":")
	h, err1 := strconv.Atoi(hs)
	m, err2 := strconv.Atoi(ms)
	if !ok || err1 != nil || err2 != nil || len(hs) > 2 || len(ms) != 2 || h < 0 || h > 23 || m < 0 || m > 59 || m%5 != 0 {
		return 0, false
	}
	return h*60 + m, true
}
