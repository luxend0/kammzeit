package main

// Salon-Einstellungen, Datenexport, Konto löschen: /dashboard/einstellungen/...

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/mail"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

func (a *app) routesSettings(mux *http.ServeMux) {
	mux.HandleFunc("GET /dashboard/einstellungen", a.dashOwner(a.settingsPage))
	mux.HandleFunc("POST /dashboard/einstellungen", a.dashOwner(a.settingsSave))
	mux.HandleFunc("GET /dashboard/einstellungen/export", a.dashOwner(a.settingsExport))
	mux.HandleFunc("GET /dashboard/einstellungen/loeschen", a.dashOwner(a.deletePage))
	mux.HandleFunc("POST /dashboard/einstellungen/loeschen", a.dashOwner(a.deleteSalon))
	mux.HandleFunc("POST /dashboard/einstellungen/news", a.dashOwner(a.settingsNews))
}

// choice ist ein Eintrag einer Auswahlliste. Nur diese Werte werden gespeichert.
type choice[T comparable] struct {
	Value T
	Label string
}

func allowed[T comparable](list []choice[T], v T) bool {
	return slices.ContainsFunc(list, func(c choice[T]) bool { return c.Value == v })
}

var (
	timezoneChoices = []choice[string]{
		{"Europe/Berlin", "Deutschland (Berlin)"},
		{"Europe/Vienna", "Österreich (Wien)"},
		{"Europe/Zurich", "Schweiz (Zürich)"},
		{"Europe/Amsterdam", "Niederlande (Amsterdam)"},
		{"Europe/Brussels", "Belgien (Brüssel)"},
		{"Europe/Luxembourg", "Luxemburg"},
		{"Europe/Paris", "Frankreich (Paris)"},
		{"Europe/Madrid", "Spanien (Madrid)"},
		{"Europe/Lisbon", "Portugal (Lissabon)"},
		{"Europe/Dublin", "Irland (Dublin)"},
		{"Europe/Rome", "Italien (Rom)"},
		{"Europe/Malta", "Malta"},
		{"Europe/Copenhagen", "Dänemark (Kopenhagen)"},
		{"Europe/Stockholm", "Schweden (Stockholm)"},
		{"Europe/Helsinki", "Finnland (Helsinki)"},
		{"Europe/Tallinn", "Estland (Tallinn)"},
		{"Europe/Riga", "Lettland (Riga)"},
		{"Europe/Vilnius", "Litauen (Vilnius)"},
		{"Europe/Warsaw", "Polen (Warschau)"},
		{"Europe/Prague", "Tschechien (Prag)"},
		{"Europe/Bratislava", "Slowakei (Bratislava)"},
		{"Europe/Budapest", "Ungarn (Budapest)"},
		{"Europe/Ljubljana", "Slowenien (Ljubljana)"},
		{"Europe/Zagreb", "Kroatien (Zagreb)"},
		{"Europe/Bucharest", "Rumänien (Bukarest)"},
		{"Europe/Sofia", "Bulgarien (Sofia)"},
		{"Europe/Athens", "Griechenland (Athen)"},
		{"Asia/Nicosia", "Zypern (Nikosia)"},
	}
	minNoticeChoices = []choice[int]{
		{0, "sofort"}, {30, "30 Minuten vorher"}, {60, "1 Stunde vorher"}, {120, "2 Stunden vorher"},
		{240, "4 Stunden vorher"}, {1440, "1 Tag vorher"},
	}
	cancelNoticeChoices = []choice[int]{
		{0, "bis Terminbeginn"}, {60, "bis 1 Stunde vorher"}, {120, "bis 2 Stunden vorher"},
		{1440, "bis 1 Tag vorher"}, {2880, "bis 2 Tage vorher"},
	}
	horizonChoices = []choice[int]{
		{14, "14 Tage"}, {30, "30 Tage"}, {60, "60 Tage"}, {90, "90 Tage"}, {180, "180 Tage"},
	}
	// Sprache der Buchungsseite; Sprachnamen bleiben in ihrer eigenen Sprache.
	langChoices = []choice[string]{{"de", "Deutsch"}, {"en", "English"}}
)

// webURL prüft eine Adresse: leer ist erlaubt, sonst absolute URL mit einem der
// Schemata, ohne Benutzerteil (https://bank.de@betrug.de).
func webURL(s string, schemes ...string) bool {
	if s == "" {
		return true
	}
	u, err := url.Parse(s)
	return err == nil && u.Host != "" && u.User == nil && slices.Contains(schemes, u.Scheme) && len(s) <= 200
}

// einstSchema ergänzt https://, wenn eine Adresse ohne Schema kommt ("salon-demo.de").
func einstSchema(s string) string {
	if s != "" && !strings.Contains(s, "://") {
		return "https://" + s
	}
	return s
}

// einstGespeichert: Erfolgsmeldung je ?gespeichert=<code>. Nur feste Texte, nie Text aus der URL.
var einstGespeichert = map[string]string{
	"einstellungen":  "Einstellungen gespeichert. Deine Buchungsseite zeigt die Änderungen sofort.",
	"news-unterwegs": "Bestätigungsmail ist unterwegs. Bitte klick auf den Link darin, dann bist du angemeldet.",
	"news-aus":       "Abbestellt. Du bekommst keine Kammzeit-News mehr.",
}

// parseSettings liest das Einstellungsformular. Fehler je Feldname; die
// Eingaben bleiben im Ergebnis, damit das Formular sie wieder anzeigen kann.
func parseSettings(f url.Values) (salon, map[string]string) {
	text := func(k string) string { return strings.ReplaceAll(strings.TrimSpace(f.Get(k)), "\r\n", "\n") }
	num := func(k string) int {
		if n, err := strconv.Atoi(f.Get(k)); err == nil {
			return n
		}
		return -1 // nie in einer Auswahlliste
	}
	runes := utf8.RuneCountInString
	s := salon{
		Name: text("name"), Email: text("email"), Phone: text("phone"), Address: text("address"),
		Website: einstSchema(text("website")), ImprintURL: einstSchema(text("imprint_url")), About: text("about"),
		Accent: strings.ToLower(text("accent")), TZ: f.Get("timezone"), Lang: f.Get("lang"),
		MinNoticeMin: num("min_notice"), CancelNoticeMin: num("cancel_notice"), HorizonDays: num("horizon"),
	}
	errs := map[string]string{}
	if n := runes(s.Name); n < 2 || n > 80 || strings.ContainsFunc(s.Name, unicode.IsControl) {
		errs["name"] = "Bitte einen Namen mit 2 bis 80 Zeichen angeben."
	}
	if s.Email != "" {
		if addr, err := mail.ParseAddress(s.Email); err != nil || len(addr.Address) > 200 {
			errs["email"] = "Bitte eine gültige E-Mail-Adresse angeben oder das Feld leer lassen."
		} else {
			s.Email = addr.Address
		}
	}
	if runes(s.Phone) > 40 || strings.ContainsAny(s.Phone, "\r\n") {
		errs["phone"] = "Die Telefonnummer darf höchstens 40 Zeichen haben."
	}
	if runes(s.Address) > 300 {
		errs["address"] = "Die Adresse darf höchstens 300 Zeichen haben."
	}
	if !webURL(s.Website, "http", "https") {
		errs["website"] = "Bitte eine gültige Web-Adresse angeben, z. B. www.dein-salon.de (höchstens 200 Zeichen)."
	}
	if !webURL(s.ImprintURL, "https") {
		errs["imprint_url"] = "Bitte eine gültige Adresse angeben, z. B. www.dein-salon.de/impressum. Nur https://, nicht http:// (höchstens 200 Zeichen)."
	}
	if runes(s.About) > 500 {
		errs["about"] = "Der Text darf höchstens 500 Zeichen haben."
	}
	if !hexColor.MatchString(s.Accent) {
		errs["accent"] = "Bitte eine Farbe wählen."
	}
	if !allowed(langChoices, s.Lang) {
		errs["lang"] = "Bitte eine Sprache aus der Liste wählen."
	}
	if !allowed(timezoneChoices, s.TZ) {
		errs["timezone"] = "Bitte eine Zeitzone aus der Liste wählen."
	}
	if !allowed(minNoticeChoices, s.MinNoticeMin) {
		errs["min_notice"] = "Bitte einen Wert aus der Liste wählen."
	}
	if !allowed(cancelNoticeChoices, s.CancelNoticeMin) {
		errs["cancel_notice"] = "Bitte einen Wert aus der Liste wählen."
	}
	if !allowed(horizonChoices, s.HorizonDays) {
		errs["horizon"] = "Bitte einen Wert aus der Liste wählen."
	}
	return s, errs
}

func (a *app) renderSettings(w http.ResponseWriter, r *http.Request, c *dctx, status int, form salon, errs map[string]string) {
	d := a.dashPage(c, r, "einstellungen", "Einstellungen")
	d["PageTitle"], d["Heading"] = d["Heading"], "" // Überschrift steht im .page-head des Templates
	d["Form"], d["Errors"], d["PreviewOn"] = form, errs, onAccent(form.Accent)
	d["SavedMsg"] = einstGespeichert[r.URL.Query().Get("gespeichert")]
	d["Timezones"], d["MinNotice"], d["CancelNotice"], d["Horizon"] = timezoneChoices, minNoticeChoices, cancelNoticeChoices, horizonChoices
	d["Langs"], d["News"] = langChoices, a.marketingState(c.User.ID)
	a.render(w, status, "einst-seite", d)
}

func (a *app) settingsPage(w http.ResponseWriter, r *http.Request, c *dctx) {
	a.renderSettings(w, r, c, http.StatusOK, *c.Salon, nil)
}

func (a *app) settingsSave(w http.ResponseWriter, r *http.Request, c *dctx) {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := r.ParseForm(); err != nil {
		a.dashError(w, r, c, http.StatusBadRequest, "Das Formular war unvollständig.")
		return
	}
	s, errs := parseSettings(r.PostForm)
	if len(errs) > 0 {
		a.renderSettings(w, r, c, http.StatusUnprocessableEntity, s, errs)
		return
	}
	_, err := a.db.Exec(`UPDATE salons SET name = ?, email = ?, phone = ?, address = ?, website = ?, imprint_url = ?, about = ?,
		accent = ?, timezone = ?, min_notice_min = ?, cancel_notice_min = ?, booking_horizon_days = ?, lang = ? WHERE id = ?`,
		s.Name, s.Email, s.Phone, s.Address, s.Website, s.ImprintURL, s.About,
		s.Accent, s.TZ, s.MinNoticeMin, s.CancelNoticeMin, s.HorizonDays, s.Lang, c.Salon.ID)
	if err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	http.Redirect(w, r, "/dashboard/einstellungen?gespeichert=einstellungen", http.StatusSeeOther)
}

// einstNewsLimit begrenzt Bestätigungsmails für die Kammzeit-News je Konto.
var einstNewsLimit = newLimiter(5, time.Hour)

// settingsNews: Kammzeit-News per E-Mail an (Double-Opt-in, Mail mit Bestätigungslink)
// oder aus (Widerruf). Nur das eigene Konto (c.User).
func (a *app) settingsNews(w http.ResponseWriter, r *http.Request, c *dctx) {
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	if err := r.ParseForm(); err != nil {
		a.dashError(w, r, c, http.StatusBadRequest, "Das Formular war unvollständig.")
		return
	}
	var err error
	code := "news-unterwegs"
	switch r.PostFormValue("news") {
	case "an":
		if a.marketingState(c.User.ID) == "an" {
			http.Redirect(w, r, "/dashboard/einstellungen", http.StatusSeeOther)
			return
		}
		if !einstNewsLimit.allow(strconv.FormatInt(c.User.ID, 10)) {
			a.dashError(w, r, c, http.StatusTooManyRequests, "Zu viele Bestätigungsmails. Bitte versuch es in einer Stunde noch einmal.")
			return
		}
		err = a.startMarketingDOI(c.User.ID, "einstellungen")
	case "aus":
		code = "news-aus"
		err = a.withdrawMarketing(c.User.ID, "einstellungen")
	default:
		a.dashError(w, r, c, http.StatusBadRequest, "Das Formular war unvollständig.")
		return
	}
	if err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	http.Redirect(w, r, "/dashboard/einstellungen?gespeichert="+code, http.StatusSeeOther)
}

// exportQueries: alles, was zu einem Salon gehört (?1 = Salon-ID). Ohne
// Stripe-IDs und ohne Storno-Token (damit könnte man Termine absagen).
var exportQueries = []struct{ key, query string }{
	{"salon", `SELECT id, slug, name, email, phone, address, website, imprint_url, about, accent, timezone, lang, plan,
		min_notice_min, cancel_notice_min, booking_horizon_days, created_at FROM salons WHERE id = ?1`},
	{"staff", `SELECT id, name, sort, active FROM staff WHERE salon_id = ?1 ORDER BY id`},
	{"services", `SELECT id, name, description, duration_min, price_cents, sort, active FROM services WHERE salon_id = ?1 ORDER BY id`},
	{"staff_services", `SELECT ss.staff_id, ss.service_id FROM staff_services ss JOIN staff st ON st.id = ss.staff_id
		JOIN services sv ON sv.id = ss.service_id WHERE st.salon_id = ?1 AND sv.salon_id = ?1 ORDER BY 1, 2`},
	{"working_hours", `SELECT wh.id, wh.staff_id, wh.weekday, wh.start_min, wh.end_min FROM working_hours wh
		JOIN staff st ON st.id = wh.staff_id WHERE st.salon_id = ?1 ORDER BY wh.staff_id, wh.weekday, wh.start_min`},
	{"customers", `SELECT id, name, email, phone, notes, created_at FROM customers WHERE salon_id = ?1 ORDER BY id`},
	{"appointments", `SELECT id, staff_id, service_id, customer_id, starts_at, ends_at, status, source, note,
		reminder_sent_at, created_at FROM appointments WHERE salon_id = ?1 ORDER BY starts_at, id`},
}

// exportRows liest eine Abfrage als Liste von Objekten (Spaltenname -> Wert).
// Spalten auf _at sind Unix-Sekunden und werden RFC3339 in der Salon-Zeitzone.
func exportRows(db *sql.DB, loc *time.Location, query string, salonID int64) ([]map[string]any, error) {
	rows, err := db.Query(query, salonID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		m := make(map[string]any, len(cols))
		for i, col := range cols {
			if n, ok := vals[i].(int64); ok && strings.HasSuffix(col, "_at") {
				vals[i] = time.Unix(n, 0).In(loc).Format(time.RFC3339)
			}
			m[col] = vals[i]
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (a *app) settingsExport(w http.ResponseWriter, r *http.Request, c *dctx) {
	data := map[string]any{"exported_at": time.Now().In(c.Salon.Loc).Format(time.RFC3339)}
	for _, q := range exportQueries {
		list, err := exportRows(a.db, c.Salon.Loc, q.query, c.Salon.ID)
		if err == nil && q.key == "salon" && len(list) != 1 {
			err = fmt.Errorf("export: salon %d nicht gefunden", c.Salon.ID)
		}
		if err != nil {
			a.dashServerError(w, r, c, err)
			return
		}
		data[q.key] = list
		if q.key == "salon" {
			data[q.key] = list[0]
		}
	}
	body, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "application/json; charset=utf-8")
	h.Set("Content-Disposition", `attachment; filename="kammzeit-`+c.Salon.Slug+`.json"`) // Slug: nur a-z, 0-9, -
	h.Set("Cache-Control", "no-store")
	w.Write(body)
}

const aboFirst = "Bitte zuerst das Pro-Abo unter /dashboard/abo kündigen. Danach kannst du das Konto löschen."

func (a *app) renderDelete(w http.ResponseWriter, r *http.Request, c *dctx, status int, msg string) {
	d := a.dashPage(c, r, "einstellungen", "Konto löschen")
	d["PageTitle"], d["Heading"] = d["Heading"], "" // Überschrift steht im .page-head des Templates
	d["Error"] = msg
	d["HasAbo"] = c.Salon.StripeSubscriptionID != ""
	a.render(w, status, "einst-loeschen", d)
}

func (a *app) deletePage(w http.ResponseWriter, r *http.Request, c *dctx) {
	a.renderDelete(w, r, c, http.StatusOK, "")
}

// deleteSalon löscht den Salon. ON DELETE CASCADE räumt Team, Leistungen,
// Kunden, Termine, Konten, Sessions und Login-Links mit ab.
func (a *app) deleteSalon(w http.ResponseWriter, r *http.Request, c *dctx) {
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	if err := r.ParseForm(); err != nil {
		a.dashError(w, r, c, http.StatusBadRequest, "Das Formular war unvollständig.")
		return
	}
	if c.Salon.StripeSubscriptionID != "" {
		a.renderDelete(w, r, c, http.StatusConflict, aboFirst)
		return
	}
	if strings.TrimSpace(strings.ToLower(r.PostFormValue("slug"))) != c.Salon.Slug {
		a.renderDelete(w, r, c, http.StatusUnprocessableEntity, "Die eingetippte Adresse stimmt nicht. Es wurde nichts gelöscht.")
		return
	}
	// Abo-Bedingung auch im DELETE: ein gleichzeitig abgeschlossenes Abo verhindert das Löschen.
	res, err := a.db.Exec(`DELETE FROM salons WHERE id = ? AND stripe_subscription_id = ''`, c.Salon.ID)
	if err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		a.renderDelete(w, r, c, http.StatusConflict, aboFirst)
		return
	}
	a.endSession(w, r)
	http.Redirect(w, r, "/?geloescht", http.StatusSeeOther)
}
