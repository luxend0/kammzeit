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
	mux.HandleFunc("GET /dashboard/leistungen", a.dashOwner(a.leistList))
	mux.HandleFunc("GET /dashboard/leistungen/neu", a.dashOwner(a.leistEdit))
	mux.HandleFunc("POST /dashboard/leistungen/neu", a.dashOwner(a.leistSave))
	mux.HandleFunc("GET /dashboard/leistungen/{id}", a.dashOwner(a.leistEdit))
	mux.HandleFunc("POST /dashboard/leistungen/{id}", a.dashOwner(a.leistSave))
	mux.HandleFunc("POST /dashboard/leistungen/{id}/verschieben", a.dashOwner(a.leistMove))
	mux.HandleFunc("POST /dashboard/leistungen/{id}/loeschen", a.dashOwner(a.catalogDelete("services", "/dashboard/leistungen?gespeichert=leistung")))
	mux.HandleFunc("GET /dashboard/team", a.dashOwner(a.teamList))
	mux.HandleFunc("GET /dashboard/team/neu", a.dashOwner(a.teamEdit))
	mux.HandleFunc("POST /dashboard/team/neu", a.dashOwner(a.teamSave))
	mux.HandleFunc("GET /dashboard/team/{id}", a.dashOwner(a.teamEdit))
	mux.HandleFunc("POST /dashboard/team/{id}", a.dashOwner(a.teamSave))
	mux.HandleFunc("POST /dashboard/team/{id}/loeschen", a.dashOwner(a.catalogDelete("staff", "/dashboard/team?gespeichert=mitarbeiter")))
}

// catalogID ist die ID aus dem Pfad, 0 wenn ungültig (findet dann nichts).
func catalogID(r *http.Request) int64 {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id
}

func catalogNotFound(a *app, w http.ResponseWriter, r *http.Request, c *dctx) {
	a.dashError(w, r, c, http.StatusNotFound, tr(a.langFor(r), "Diesen Eintrag gibt es nicht."))
}

// catalogInvalid steht über dem Formular, die Einzelheiten stehen an den Feldern.
const catalogInvalid = "Bitte die markierten Felder prüfen. Es wurde noch nichts gespeichert."

// catalogSaved: Erfolgsmeldung zu ?gespeichert=<code>. Nur feste Texte, nie etwas aus der URL.
// Übersetzt wird beim Anzeigen (catalogSavedMsg).
var catalogSaved = map[string]string{
	"leistung":                "Leistung gespeichert.",
	"leistung-neu":            "Leistung angelegt. Alle aktiven Mitarbeiter bieten sie an, ändern kannst du das unter Team.",
	"leistung-geloescht":      "Leistung gelöscht.",
	"leistung-deaktiviert":    "Die Leistung hat Termine und wurde deshalb nicht gelöscht, sondern deaktiviert.",
	"reihenfolge":             "Reihenfolge geändert. So stehen die Leistungen auch auf der Buchungsseite.",
	"mitarbeiter":             "Mitarbeiter gespeichert.",
	"mitarbeiter-neu":         "Mitarbeiter angelegt.",
	"mitarbeiter-geloescht":   "Mitarbeiter gelöscht.",
	"mitarbeiter-deaktiviert": "Der Mitarbeiter hat Termine und wurde deshalb nicht gelöscht, sondern deaktiviert.",
}

// catalogSavedMsg: übersetzte Erfolgsmeldung; ohne bekannten Code nil, dann zeigt das Layout "Gespeichert.".
func catalogSavedMsg(r *http.Request, lang string) any {
	if msg := catalogSaved[r.URL.Query().Get("gespeichert")]; msg != "" {
		return tr(lang, msg)
	}
	return nil
}

// catalogAppts zählt alle Termine (auch vergangene und abgesagte) an einer Leistung
// (col = "service_id") oder einem Mitarbeiter ("staff_id"). col ist nie Nutzereingabe.
func catalogAppts(q querier, salonID int64, col string, id int64) (int, error) {
	var n int
	err := q.QueryRow(`SELECT COUNT(*) FROM appointments WHERE salon_id = ? AND `+col+` = ?`, salonID, id).Scan(&n)
	return n, err
}

// catalogDelete löscht eine Leistung oder einen Mitarbeiter. Hängen Termine daran,
// wird nur deaktiviert, damit die Termine nicht per ON DELETE CASCADE mit verschwinden.
// table ist eine Konstante aus routesCatalog, nie Nutzereingabe; an back wird
// "-geloescht" oder "-deaktiviert" gehängt (Codes in catalogSaved).
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
		n, err := catalogAppts(tx, c.Salon.ID, col, id)
		if err != nil {
			a.dashServerError(w, r, c, err)
			return
		}
		q, done := `DELETE FROM `+table+` WHERE id = ? AND salon_id = ?`, "-geloescht"
		if n > 0 {
			q, done = `UPDATE `+table+` SET active = 0 WHERE id = ? AND salon_id = ?`, "-deaktiviert"
		} else if table == "staff" {
			// Ein gelöschter Mitarbeiter verliert seinen Login (mit Sessions); ein
			// deaktivierter behält ihn, kann sich aber nicht anmelden (a.dash).
			if _, err := tx.Exec(`DELETE FROM users WHERE role = 'staff' AND staff_id = ? AND salon_id = ?`, id, c.Salon.ID); err != nil {
				a.dashServerError(w, r, c, err)
				return
			}
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
		http.Redirect(w, r, back+done, http.StatusSeeOther)
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
	Errs                               map[string]string // Feldname -> Fehler
	Appts                              int               // Termine an der Leistung, für den Löschen-Hinweis
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
	lang := a.langFor(r)
	d := a.dashPage(c, r, "leistungen", tr(lang, "Leistungen"))
	d["PageTitle"], d["Heading"] = d["Heading"], "" // Überschrift steht im .page-head des Templates
	d["Services"] = list
	d["SavedMsg"] = catalogSavedMsg(r, lang)
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
		f.Duration, f.Price = strconv.Itoa(dur), trf(a.langFor(r), "%d,%02d", cents/100, cents%100)
	}
	a.leistRender(w, r, c, http.StatusOK, f, "")
}

func (a *app) leistRender(w http.ResponseWriter, r *http.Request, c *dctx, status int, f leistForm, msg string) {
	title := "Neue Leistung"
	if f.ID != 0 {
		title = "Leistung bearbeiten"
		var err error
		if f.Appts, err = catalogAppts(a.db, c.Salon.ID, "service_id", f.ID); err != nil {
			a.dashServerError(w, r, c, err)
			return
		}
	}
	lang := a.langFor(r)
	for k, v := range f.Errs {
		f.Errs[k] = tr(lang, v)
	}
	d := a.dashPage(c, r, "leistungen", tr(lang, title))
	d["PageTitle"], d["Heading"] = d["Heading"], "" // Überschrift steht im .page-head des Templates
	d["F"], d["Error"] = f, tr(lang, msg)
	a.render(w, status, "leist-form", d)
}

func (a *app) leistSave(w http.ResponseWriter, r *http.Request, c *dctx) {
	isNew := r.PathValue("id") == ""
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := r.ParseForm(); err != nil {
		a.dashError(w, r, c, http.StatusBadRequest, tr(a.langFor(r), "Das Formular war unvollständig."))
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
	f.Errs = map[string]string{}
	if f.Name == "" || utf8.RuneCountInString(f.Name) > 80 {
		f.Errs["name"] = "Bitte einen Namen mit höchstens 80 Zeichen angeben."
	}
	if utf8.RuneCountInString(f.Description) > 300 {
		f.Errs["beschreibung"] = "Die Beschreibung darf höchstens 300 Zeichen lang sein."
	}
	if durErr != nil || dur < 5 || dur > 480 || dur%5 != 0 {
		f.Errs["dauer"] = "Die Dauer muss zwischen 5 und 480 Minuten liegen, in 5-Minuten-Schritten."
	}
	if priceErr != nil {
		f.Errs["preis"] = "Bitte einen gültigen Preis angeben, z. B. 25 oder 25,50."
	}
	if len(f.Errs) > 0 {
		a.leistRender(w, r, c, http.StatusUnprocessableEntity, f, catalogInvalid)
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
	code := "leistung"
	if isNew {
		code = "leistung-neu"
	}
	http.Redirect(w, r, "/dashboard/leistungen?gespeichert="+code, http.StatusSeeOther)
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
	// Anker: die Seite springt zurück zur verschobenen Zeile.
	http.Redirect(w, r, fmt.Sprintf("/dashboard/leistungen?gespeichert=reihenfolge#leist-%d", id), http.StatusSeeOther)
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
	Login    int // Mitarbeiter-Login: 0 keiner, 1 eingeladen, 2 schon angemeldet (staff.go)
}

type teamService struct {
	ID              int64
	Name            string
	Active, Checked bool
}

// teamDay: Arbeitszeiten eines Wochentags im Formular, bis zu zwei Blöcke als "HH:MM".
// Err und Bad setzt teamParseHours (Fehlertext des Tages, welcher Block falsch ist).
type teamDay struct {
	WD       int // wie time.Weekday, 0 = Sonntag
	Label    string
	From, To [2]string
	Err      string
	Bad      [2]bool
}

type teamForm struct {
	ID        int64
	Name      string
	Active    bool
	Services  []teamService
	Days      []teamDay
	Errs      map[string]string // Feldname -> Fehler
	Notice    string            // nach "Zeiten übertragen"
	CopyFrom  int               // Vorlage für den Übertragen-Knopf, siehe teamCopySource
	CopyLabel string
	Appts     int // Termine des Mitarbeiters, für den Löschen-Hinweis
}

// Anzeige Mo..So.
var teamOrder = [...]time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday, time.Sunday}

func (a *app) teamList(w http.ResponseWriter, r *http.Request, c *dctx) {
	rows, err := a.db.Query(`SELECT id, name, active, (SELECT COUNT(*) FROM staff_services WHERE staff_id = staff.id),
		COALESCE((SELECT IIF(last_login_at IS NULL, 1, 2) FROM users WHERE role = 'staff' AND staff_id = staff.id AND salon_id = staff.salon_id LIMIT 1), 0)
		FROM staff WHERE salon_id = ? ORDER BY sort, name, id`, c.Salon.ID)
	if err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	var list []teamRow
	for rows.Next() {
		var s teamRow
		if err := rows.Scan(&s.ID, &s.Name, &s.Active, &s.Services, &s.Login); err != nil {
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
	lang := a.langFor(r)
	for i := range list {
		list[i].Hours = teamHoursSummary(lang, hours[list[i].ID])
	}
	d := a.dashPage(c, r, "team", tr(lang, "Team"))
	d["PageTitle"], d["Heading"] = d["Heading"], "" // Überschrift steht im .page-head des Templates
	d["Staff"] = list
	d["SavedMsg"] = catalogSavedMsg(r, lang)
	d["ShowLimit"] = !c.Salon.Pro() && !a.cfg.SingleTenant
	d["StaffLogins"] = a.staffLogins(c.Salon)
	d["ProHint"] = trf(lang, "Im Gratis-Plan ist höchstens %d Mitarbeiter aktiv. Mit Pro unbegrenzt viele.", freeStaffLimit)
	a.render(w, http.StatusOK, "team-liste", d)
}

// teamWeekday: Name des Wochentags in der Sprache lang.
func teamWeekday(lang string, wd time.Weekday) string {
	if lang == "en" {
		return weekdaysEN[wd]
	}
	return weekdaysDE[wd]
}

// teamHoursSummary fasst Wochentage mit gleichen Zeiten zusammen, z. B.
// ["Di-Fr 9:00-18:00", "Sa 9:00-14:00"] bzw. ["Tue-Fri 9:00-18:00", ...].
// hours muss nach start_min sortiert sein.
func teamHoursSummary(lang string, hours []WorkingHours) []string {
	clock := func(m int) string { return fmt.Sprintf("%d:%02d", m/60, m%60) }
	day := map[time.Weekday]string{}
	for _, h := range hours {
		if day[h.Weekday] != "" {
			day[h.Weekday] += ", "
		}
		day[h.Weekday] += clock(h.StartMin) + "-" + clock(h.EndMin)
	}
	n := 2 // "Di", englisch "Tue"
	if lang == "en" {
		n = 3
	}
	var out []string
	for i := 0; i < len(teamOrder); {
		j := i
		for j+1 < len(teamOrder) && day[teamOrder[j+1]] == day[teamOrder[i]] {
			j++
		}
		if txt := day[teamOrder[i]]; txt != "" {
			label := teamWeekday(lang, teamOrder[i])[:n]
			if j > i {
				label += "-" + teamWeekday(lang, teamOrder[j])[:n]
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
		if hours, err = a.teamLoadHours(f.ID); err != nil {
			a.dashServerError(w, r, c, err)
			return
		}
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
	f.Days = teamDaysOf(a.langFor(r), hours)
	a.teamRender(w, r, c, http.StatusOK, &f, "", false)
}

// teamLoadHours lädt die Arbeitszeiten eines Mitarbeiters, sortiert nach Beginn.
// staffID muss vorher gegen den Salon geprüft sein.
func (a *app) teamLoadHours(staffID int64) ([]WorkingHours, error) {
	rows, err := a.db.Query(`SELECT weekday, start_min, end_min FROM working_hours WHERE staff_id = ? ORDER BY start_min`, staffID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hours []WorkingHours
	for rows.Next() {
		var h WorkingHours
		if err := rows.Scan(&h.Weekday, &h.StartMin, &h.EndMin); err != nil {
			return nil, err
		}
		hours = append(hours, h)
	}
	return hours, rows.Err()
}

// teamDaysOf baut die Formularzeilen Mo..So aus Arbeitszeiten (nach Beginn sortiert).
func teamDaysOf(lang string, hours []WorkingHours) []teamDay {
	var days []teamDay
	for _, wd := range teamOrder {
		d := teamDay{WD: int(wd), Label: teamWeekday(lang, wd)}
		i := 0
		for _, h := range hours {
			if h.Weekday == wd && i < 2 { // ponytail: mehr als zwei Blöcke am Tag kann das Formular nicht zeigen
				d.From[i], d.To[i] = fmt.Sprintf("%02d:%02d", h.StartMin/60, h.StartMin%60), fmt.Sprintf("%02d:%02d", h.EndMin/60, h.EndMin%60)
				i++
			}
		}
		days = append(days, d)
	}
	return days
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
	lang := a.langFor(r)
	title := "Neuer Mitarbeiter"
	if f != nil {
		f.CopyFrom, f.CopyLabel = teamCopySource(lang, f.Days)
		for k, v := range f.Errs {
			f.Errs[k] = tr(lang, v)
		}
	}
	if f != nil && f.ID != 0 {
		title = "Mitarbeiter bearbeiten"
		var err error
		if f.Appts, err = catalogAppts(a.db, c.Salon.ID, "staff_id", f.ID); err != nil {
			a.dashServerError(w, r, c, err)
			return
		}
	}
	d := a.dashPage(c, r, "team", tr(lang, title))
	d["PageTitle"], d["Heading"] = d["Heading"], "" // Überschrift steht im .page-head des Templates
	d["F"], d["Error"], d["LimitHit"], d["Limit"] = f, tr(lang, msg), limit, freeStaffLimit
	if f != nil && f.ID != 0 {
		if err := a.staffAccessData(d, r, c, f.ID); err != nil { // Karte "Login fürs Dashboard" (staff.go)
			a.dashServerError(w, r, c, err)
			return
		}
	}
	d["ProHint"] = tr(lang, "Mit Pro sind beliebig viele Mitarbeiter aktiv, für 14,90 € im Monat oder 149 € im Jahr.")
	a.render(w, status, "team-form", d)
}

func (a *app) teamSave(w http.ResponseWriter, r *http.Request, c *dctx) {
	isNew := r.PathValue("id") == ""
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		a.dashError(w, r, c, http.StatusBadRequest, tr(a.langFor(r), "Das Formular war unvollständig."))
		return
	}
	lang := a.langFor(r)
	f := teamForm{
		Name:   strings.TrimSpace(r.PostFormValue("name")),
		Active: r.PostFormValue("aktiv") == "1",
		Days:   teamDaysFromForm(lang, r.PostForm),
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
	// "Zeiten übertragen" speichert nicht, sondern zeigt das Formular mit den kopierten Zeiten.
	if notice := teamCopyDays(lang, f.Days, r.PostFormValue("uebertragen")); notice != "" {
		f.Notice = notice
		a.teamRender(w, r, c, http.StatusOK, &f, "", false)
		return
	}
	hours, ok := teamParseHours(lang, f.Days)
	f.Errs = map[string]string{}
	if f.Name == "" || utf8.RuneCountInString(f.Name) > 60 {
		f.Errs["name"] = "Bitte einen Namen mit höchstens 60 Zeichen angeben."
	}
	if !ok || len(f.Errs) > 0 {
		a.teamRender(w, r, c, http.StatusUnprocessableEntity, &f, catalogInvalid, false)
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
			tx.Rollback() // teamRender fragt die DB ab, es gibt nur eine Verbindung
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
	code := "mitarbeiter"
	if isNew {
		code = "mitarbeiter-neu"
	}
	http.Redirect(w, r, "/dashboard/team?gespeichert="+code, http.StatusSeeOther)
}

// teamCopySource wählt die Vorlage für den Knopf "... auf ... bis Freitag übertragen":
// den ersten Tag Mo-Do mit Zeiten (viele Salons haben montags zu). 0 = kein Knopf.
// days ist wie teamOrder sortiert (Mo zuerst).
func teamCopySource(lang string, days []teamDay) (int, string) {
	for _, d := range days[:4] {
		if d.From != [2]string{} || d.To != [2]string{} {
			return d.WD, trf(lang, "%s übertragen", teamCopyRange(lang, d.WD))
		}
	}
	return 0, ""
}

// teamCopyDays erledigt den Knopf "... übertragen" (Formularwert src = Wochentag 1-4):
// kopiert die Zeiten des Tages auf die Folgetage bis Freitag und liefert den
// Hinweis dazu. "" = kein gültiger Knopf, dann normal speichern.
func teamCopyDays(lang string, days []teamDay, srcVal string) string {
	src, err := strconv.Atoi(srcVal)
	if err != nil || src < 1 || src > 4 {
		return ""
	}
	var from teamDay
	for _, d := range days {
		if d.WD == src {
			from = d
		}
	}
	for i := range days {
		if days[i].WD > src && days[i].WD <= 5 { // bis Freitag
			days[i].From, days[i].To = from.From, from.To
		}
	}
	return trf(lang, "Zeiten übertragen: %s. Noch nicht gespeichert.", teamCopyRange(lang, src))
}

// teamCopyRange: "Dienstag auf Mittwoch bis Freitag", für Donnerstag nur "Donnerstag auf Freitag".
func teamCopyRange(lang string, src int) string {
	if src == int(time.Thursday) {
		return tr(lang, "Donnerstag auf Freitag")
	}
	return trf(lang, "%s auf %s bis Freitag", teamWeekday(lang, time.Weekday(src)), teamWeekday(lang, time.Weekday(src+1)))
}

// teamDaysFromForm liest die Felder von-<wd>-<i> / bis-<wd>-<i> (i = 0, 1) roh ein.
func teamDaysFromForm(lang string, f url.Values) []teamDay {
	var days []teamDay
	for _, wd := range teamOrder {
		d := teamDay{WD: int(wd), Label: teamWeekday(lang, wd)}
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
// Fehler schreibt es an den Tag (Err, Bad, in der Sprache lang) und liefert dann ok = false.
func teamParseHours(lang string, days []teamDay) (out []WorkingHours, ok bool) {
	ok = true
	for k := range days {
		d := &days[k]
		fail := func(i int, msg string) {
			d.Bad[i], ok = true, false
			d.Err = strings.TrimSpace(d.Err + " " + trf(lang, "%s Block %d: %s", d.Label, i+1, tr(lang, msg)))
		}
		var blocks []WorkingHours
		for i := range 2 {
			if d.From[i] == "" && d.To[i] == "" {
				continue
			}
			s, okS := teamParseClock(d.From[i])
			e, okE := teamParseClock(d.To[i])
			switch {
			case !okS || !okE:
				fail(i, "Bitte Beginn und Ende angeben, im 5-Minuten-Raster (z. B. 09:00).")
			case e <= s:
				fail(i, "Das Ende muss nach dem Beginn liegen.")
			default:
				blocks = append(blocks, WorkingHours{time.Weekday(d.WD), s, e})
			}
		}
		if len(blocks) == 2 && blocks[0].StartMin < blocks[1].EndMin && blocks[1].StartMin < blocks[0].EndMin {
			d.Bad, d.Err, ok = [2]bool{true, true}, trf(lang, "%s: Die beiden Blöcke überschneiden sich.", d.Label), false
		}
		out = append(out, blocks...)
	}
	if !ok {
		return nil, false
	}
	return out, true
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
