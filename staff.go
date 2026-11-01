package main

// Mitarbeiter-Logins (nur Pro): Inhaber laden auf der Team-Seite ein
// (/dashboard/team/{id}, Karte "Login fürs Dashboard"), Mitarbeiter melden sich
// wie alle per Magic-Link an. Sie sehen Kalender und Kunden (ohne Löschen und
// Export), bedienen die Warteschlange und pflegen ihre eigenen Arbeitszeiten
// (/dashboard/meine-zeiten). Rechte: a.dash = alle Logins, a.dashOwner = nur
// Inhaber (session.go). Ohne Pro sind Mitarbeiter-Logins pausiert, gelöscht wird nichts.
// Bezeichner beginnen mit staff.

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"
)

// staffInviteLimit: Einladungsmails pro Salon (Variable, damit Tests sie zurücksetzen können).
var staffInviteLimit = newLimiter(20, time.Hour)

func (a *app) routesStaff(mux *http.ServeMux) {
	mux.HandleFunc("POST /dashboard/team/{id}/zugang", a.dashOwner(a.staffInvite))
	mux.HandleFunc("POST /dashboard/team/{id}/zugang/entfernen", a.dashOwner(a.staffRevoke))
	mux.HandleFunc("GET /dashboard/meine-zeiten", a.dash(a.staffHours))
	mux.HandleFunc("POST /dashboard/meine-zeiten", a.dash(a.staffHoursSave))
}

// staffLogins: Mitarbeiter-Logins wirken nur im Pro-Plan; im Einzelbetrieb
// (Selbsthosten) immer, wie die Mitarbeitergrenze.
func (a *app) staffLogins(s *salon) bool { return s.Pro() || a.cfg.SingleTenant }

// staffPaused: ein Mitarbeiter-Login, der sich gerade nicht anmelden darf
// (Salon ohne Pro, Mitarbeiter inaktiv oder gelöscht). Inhaber nie.
func (a *app) staffPaused(userID int64) (bool, error) {
	var role, plan string
	var active bool
	err := a.db.QueryRow(`SELECT u.role, s.plan,
		EXISTS (SELECT 1 FROM staff st WHERE st.id = u.staff_id AND st.salon_id = u.salon_id AND st.active = 1)
		FROM users u JOIN salons s ON s.id = u.salon_id WHERE u.id = ?`, userID).Scan(&role, &plan, &active)
	if err != nil {
		return false, err
	}
	return role == "staff" && (!active || !a.staffLogins(&salon{Plan: plan})), nil
}

// staffPausedPage erklärt im Login-Layout, warum es nicht weitergeht.
func (a *app) staffPausedPage(w http.ResponseWriter, r *http.Request, status int) {
	a.authMessage(w, r, status, "Zugang pausiert",
		"Dein Mitarbeiter-Zugang ist gerade pausiert: Der Salon hat kein Pro-Abo mehr, oder du bist im Team nicht aktiv. Gelöscht ist nichts. Sprich mit der Inhaberin oder dem Inhaber deines Salons.",
		"Zurück zum Login", "/login", "kammi-schlaeft.webp")
}

// --- Einladen und entfernen (Inhaber, Team-Seite) ---

type staffAccess struct {
	Email     string
	LastLogin string // fertig formatiert, leer = noch nie angemeldet
}

// Meldungen zu ?zugang=<code> auf der Team-Seite. Nur feste Texte, nie etwas aus der URL.
var (
	staffAccessMsgs = map[string]string{
		"eingeladen": "Einladung verschickt. Der Login-Link darin gilt 15 Minuten, danach meldet sich dein Mitarbeiter einfach über die Login-Seite an.",
		"erneut":     "Einladung noch einmal verschickt.",
		"entfernt":   "Zugang entfernt. Alle Anmeldungen dieses Logins sind beendet.",
	}
	staffAccessErrs = map[string]string{
		"mail":     "Bitte eine gültige E-Mail-Adresse angeben.",
		"vergeben": "Diese E-Mail-Adresse hat schon ein Kammzeit-Konto. Jede Adresse kann nur zu einem Login gehören, bitte nimm eine andere.",
		"inaktiv":  "Inaktive Mitarbeiter können sich nicht anmelden. Stell den Mitarbeiter zuerst auf aktiv und speichere.",
		"zu-viele": "Zu viele Einladungen in kurzer Zeit. Bitte versuch es in einer Stunde noch einmal.",
		"versand":  "Die Einladung konnte nicht verschickt werden. Der Zugang ist angelegt, versuch es gleich noch einmal mit \"Einladung erneut schicken\".",
	}
)

// staffAccessData füllt die Karte "Login fürs Dashboard" im Mitarbeiter-Formular
// (teamRender, nur für gespeicherte Mitarbeiter).
func (a *app) staffAccessData(d map[string]any, r *http.Request, c *dctx, staffID int64) error {
	lang := d["Lang"].(string)
	var acc staffAccess
	var last sql.NullInt64
	err := a.db.QueryRow(`SELECT email, last_login_at FROM users WHERE role = 'staff' AND staff_id = ? AND salon_id = ?`,
		staffID, c.Salon.ID).Scan(&acc.Email, &last)
	switch {
	case err == nil:
		if last.Valid {
			acc.LastLogin = dateFor(lang, time.Unix(last.Int64, 0).In(c.Salon.Loc))
		}
		d["Access"] = &acc
	case !errors.Is(err, sql.ErrNoRows):
		return err
	}
	d["StaffLogins"] = a.staffLogins(c.Salon)
	code := r.URL.Query().Get("zugang")
	if msg := staffAccessMsgs[code]; msg != "" {
		d["AccessMsg"] = tr(lang, msg)
	}
	if msg := staffAccessErrs[code]; msg != "" {
		d["AccessErr"] = tr(lang, msg)
	}
	return nil
}

// staffInvite legt den Login an (falls noch keiner) und schickt die Einladung.
// Gibt es schon einen Login, geht die Einladung noch einmal raus (der Link gilt
// nur 15 Minuten). Ergebnis als ?zugang=<code> zurück auf die Team-Seite.
func (a *app) staffInvite(w http.ResponseWriter, r *http.Request, c *dctx) {
	if !a.staffLogins(c.Salon) {
		a.dashError(w, r, c, http.StatusForbidden, "Mitarbeiter-Logins gibt es im Pro-Plan.")
		return
	}
	id := catalogID(r)
	var name string
	var active bool
	err := a.db.QueryRow(`SELECT name, active FROM staff WHERE id = ? AND salon_id = ?`, id, c.Salon.ID).Scan(&name, &active)
	if errors.Is(err, sql.ErrNoRows) {
		catalogNotFound(a, w, r, c)
		return
	} else if err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	back := func(code string) {
		http.Redirect(w, r, fmt.Sprintf("/dashboard/team/%d?zugang=%s#zugang", id, code), http.StatusSeeOther)
	}
	if !active {
		back("inaktiv")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	if err := r.ParseForm(); err != nil {
		a.dashError(w, r, c, http.StatusBadRequest, "Das Formular war unvollständig.")
		return
	}
	var userID int64
	var email string
	code := "erneut"
	err = a.db.QueryRow(`SELECT id, email FROM users WHERE role = 'staff' AND staff_id = ? AND salon_id = ?`, id, c.Salon.ID).Scan(&userID, &email)
	if errors.Is(err, sql.ErrNoRows) {
		addr, perr := mail.ParseAddress(strings.TrimSpace(r.PostFormValue("email")))
		if perr != nil || len(addr.Address) > 200 {
			back("mail")
			return
		}
		email, code = strings.ToLower(addr.Address), "eingeladen"
		// Adressen sind global eindeutig (ein Konto = ein Salon, eine Rolle).
		var taken bool
		if err := a.db.QueryRow(`SELECT EXISTS (SELECT 1 FROM users WHERE email = ?)`, email).Scan(&taken); err != nil {
			a.dashServerError(w, r, c, err)
			return
		}
		if taken {
			back("vergeben")
			return
		}
	} else if err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	if !staffInviteLimit.allow(strconv.FormatInt(c.Salon.ID, 10)) {
		back("zu-viele")
		return
	}
	if userID == 0 {
		// Sprache der Buchungsseite: das Team spricht meist die Sprache der Kunden.
		// Meldet sich der Mitarbeiter über die andere Domain an, übernimmt loginMail deren Sprache.
		err = a.db.QueryRow(`INSERT INTO users (salon_id, email, role, staff_id, lang) VALUES (?, ?, 'staff', ?, ?) RETURNING id`,
			c.Salon.ID, email, id, c.Salon.Lang).Scan(&userID)
		if err != nil {
			a.dashServerError(w, r, c, err)
			return
		}
	}
	if err := a.staffInviteMail(c.Salon, userID, email, name); err != nil {
		log.Printf("Einladung an %s: %v", email, err)
		back("versand")
		return
	}
	back(code)
}

// staffInviteMail schickt die Einladung mit frischem Login-Link in der Sprache des Logins.
func (a *app) staffInviteMail(s *salon, userID int64, email, staffName string) error {
	var lang string
	a.db.QueryRow(`SELECT lang FROM users WHERE id = ?`, userID).Scan(&lang)
	if lang != "en" {
		lang = "de"
	}
	token, err := a.newLoginToken(userID)
	if err != nil {
		return err
	}
	site := a.siteURLLang(lang)
	return a.mailT(email, lang, "staff-invite", map[string]any{ // web/mail.html
		"Link": site + "/login/" + token, "LoginURL": site + "/login", "Salon": s.Name, "Name": staffName,
	})
}

// staffRevoke löscht den Login des Mitarbeiters; Sessions und Login-Links
// verschwinden per ON DELETE CASCADE mit.
func (a *app) staffRevoke(w http.ResponseWriter, r *http.Request, c *dctx) {
	id := catalogID(r)
	res, err := a.db.Exec(`DELETE FROM users WHERE role = 'staff' AND staff_id = ? AND salon_id = ?`, id, c.Salon.ID)
	if err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		catalogNotFound(a, w, r, c)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/dashboard/team/%d?zugang=entfernt#zugang", id), http.StatusSeeOther)
}

// --- Meine Zeiten (Mitarbeiter) ---

// staffHours zeigt die eigenen Arbeitszeiten. c.User.StaffID hat a.dash schon
// gegen den Salon geprüft (aktiver Mitarbeiter dieses Salons). Inhaber pflegen
// Zeiten unter Team.
func (a *app) staffHours(w http.ResponseWriter, r *http.Request, c *dctx) {
	if c.Owner() {
		http.Redirect(w, r, "/dashboard/team", http.StatusFound)
		return
	}
	hours, err := a.teamLoadHours(c.User.StaffID)
	if err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	a.staffHoursRender(w, r, c, http.StatusOK, &teamForm{Days: teamDaysOf(a.langFor(r), hours)}, "")
}

func (a *app) staffHoursRender(w http.ResponseWriter, r *http.Request, c *dctx, status int, f *teamForm, msg string) {
	lang := a.langFor(r)
	f.CopyFrom, f.CopyLabel = teamCopySource(lang, f.Days)
	d := a.dashPage(c, r, "meine-zeiten", "Meine Zeiten")
	d["Lead"] = "Wann du arbeitest. Online buchbar bist du nur in diesen Zeiten."
	d["SavedMsg"] = "Arbeitszeiten gespeichert."
	d["F"], d["Error"] = f, tr(lang, msg)
	a.render(w, status, "staff-zeiten", d)
}

// staffHoursSave ersetzt die Arbeitszeiten des eigenen Mitarbeiters, nie die eines anderen:
// die ID kommt nur aus dem Login, und jede Query prüft den Salon.
func (a *app) staffHoursSave(w http.ResponseWriter, r *http.Request, c *dctx) {
	if c.Owner() {
		http.Redirect(w, r, "/dashboard/team", http.StatusSeeOther)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		a.dashError(w, r, c, http.StatusBadRequest, "Das Formular war unvollständig.")
		return
	}
	lang := a.langFor(r)
	f := &teamForm{Days: teamDaysFromForm(lang, r.PostForm)}
	if notice := teamCopyDays(lang, f.Days, r.PostFormValue("uebertragen")); notice != "" {
		f.Notice = notice
		a.staffHoursRender(w, r, c, http.StatusOK, f, "")
		return
	}
	hours, ok := teamParseHours(lang, f.Days)
	if !ok {
		a.staffHoursRender(w, r, c, http.StatusUnprocessableEntity, f, catalogInvalid)
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	defer tx.Rollback()
	own := `(SELECT id FROM staff WHERE id = ? AND salon_id = ?)`
	_, err = tx.Exec(`DELETE FROM working_hours WHERE staff_id = `+own, c.User.StaffID, c.Salon.ID)
	for _, h := range hours {
		if err == nil {
			_, err = tx.Exec(`INSERT INTO working_hours (staff_id, weekday, start_min, end_min) SELECT id, ?, ?, ? FROM staff WHERE id = ? AND salon_id = ?`,
				int(h.Weekday), h.StartMin, h.EndMin, c.User.StaffID, c.Salon.ID)
		}
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	http.Redirect(w, r, "/dashboard/meine-zeiten?gespeichert", http.StatusSeeOther)
}
